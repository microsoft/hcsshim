//go:build linux
// +build linux

package securitypolicy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	specInternal "github.com/Microsoft/hcsshim/internal/guest/spec"
	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/Microsoft/hcsshim/internal/hooks"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	oci "github.com/opencontainers/runtime-spec/specs-go"
)

type SecurityPolicyEnforcer interface {
	EnforceDeviceMountPolicy(target string, deviceHash string) (err error)
	EnforceDeviceUnmountPolicy(unmountTarget string) (err error)
	EnforceOverlayMountPolicy(containerID string, layerPaths []string) (err error)
	EnforceCreateContainerPolicy(containerID string, argList []string, envList []string, workingDir string) (err error)
	EnforceExpectedMountsPolicy(containerID string, spec *oci.Spec) error
	EnforceMountPolicy(sandboxID, containerID string, spec *oci.Spec) error
	ExtendDefaultMounts([]oci.Mount) error
}

func NewSecurityPolicyEnforcer(state SecurityPolicyState) (SecurityPolicyEnforcer, error) {
	if state.SecurityPolicy.AllowAll {
		return &OpenDoorSecurityPolicyEnforcer{}, nil
	} else {
		containers, err := state.SecurityPolicy.Containers.toInternal()
		if err != nil {
			return nil, err
		}
		return NewStandardSecurityPolicyEnforcer(containers, state.EncodedSecurityPolicy.SecurityPolicy), nil
	}
}

type mountInternal struct {
	Source      string
	Destination string
	Type        string
	Options     []string
}

// newMountConstraint creates an internal mount constraint object from given
// source, destination, type and options
func newMountConstraint(src, dst string, mType string, mOpts []string) mountInternal {
	return mountInternal{
		Source:      src,
		Destination: dst,
		Type:        mType,
		Options:     mOpts,
	}
}

// Internal version of Container
type securityPolicyContainer struct {
	// The command that we will allow the container to execute
	Command []string
	// The rules for determining if a given environment variable is allowed
	EnvRules []EnvRuleConfig
	// An ordered list of dm-verity root hashes for each layer that makes up
	// "a container". Containers are constructed as an overlay file system. The
	// order that the layers are overlayed is important and needs to be enforced
	// as part of policy.
	Layers []string
	// WorkingDir is a path to container's working directory, which all the processes
	// will default to.
	WorkingDir string
	// Unordered list of mounts which are expected to be present when the container
	// starts
	ExpectedMounts []string `json:"expected_mounts"`
	// A list of constraints for determining if a given mount is allowed.
	Mounts []mountInternal
}

type StandardSecurityPolicyEnforcer struct {
	// EncodedSecurityPolicy state is needed for key release
	EncodedSecurityPolicy string
	// Containers from the user supplied security policy.
	Containers []securityPolicyContainer
	// Devices and ContainerIndexToContainerIds are used to build up an
	// understanding of the containers running with a UVM as they come up and
	// map them back to a container definition from the user supplied
	// SecurityPolicy
	//
	// Devices is a listing of targets seen when mounting a device
	// stored in a "per-container basis". As the UVM goes through its process of
	// bringing up containers, we have to piece together information about what
	// is going on.
	//
	// At the time that devices are being mounted, we do not know a container
	// that they will be used for; only that there is a device with a given root
	// hash that being mounted. We check to make sure that the root hash for the
	// devices is a root hash that exists for 1 or more layers in any container
	// in the supplied SecurityPolicy. Each "seen" layer is recorded in devices
	// as it is mounted. So for example, if a root hash mount is found for the
	// device being mounted and the first layer of the first container then we
	// record the device target in Devices[0][0].
	//
	// Later, when overlay filesystems  created, we verify that the ordered layers
	// for said overlay filesystem match one of the device orderings in Devices.
	// When a match is found, the index in Devices is the same index in
	// SecurityPolicy.Containers. Overlay filesystem creation is the first time we
	// have a "container id" available to us. The container id identifies the
	// container in question going forward. We record the mapping of Container
	// index to container id so that when we have future operations like "run
	// command" which come with a container id, we can find the corresponding
	// container index and use that to look up the command in the appropriate
	// SecurityPolicyContainer instance.
	//
	// As containers can have exactly the same base image and be "the same" at
	// the time we are doing overlay, the ContainerIndexToContainerIds in a
	// set of possible containers for a given container id. Go doesn't have a set
	// type so we are doing the idiomatic go thing of using a map[string]struct{}
	// to represent the set.
	//
	// Containers that share the same base image, and perhaps further
	// information, will have an entry per container instance in the
	// SecurityPolicy. For example, a policy that has two containers that
	// use Ubuntu 18.04 will have an entry for each even if they share the same
	// command line.
	//
	// Most of the work that this security policy enforcer does it around managing
	// state needed to map from a container definition in the SecurityPolicy to
	// a specific container ID as we bring up each container. See
	// enforceCommandPolicy where most of the functionality is handling the case
	// were policy containers share an overlay and have to try to distinguish them
	// based on the command line arguments. enforceEnvironmentVariablePolicy can
	// further narrow based on environment variables if required.
	//
	// implementation details are available in:
	// - EnforceDeviceMountPolicy
	// - EnforceOverlayMountPolicy
	// - enforceCommandPolicy
	// - enforceEnvironmentVariablePolicy
	// - NewStandardSecurityPolicyEnforcer
	Devices                      [][]string
	ContainerIndexToContainerIds map[int]map[string]struct{}
	// Set of container IDs that we've allowed to start. Because Go doesn't have
	// sets as a built-in data structure, we are using a map
	startedContainers map[string]struct{}
	// Mutex to prevent concurrent access to fields
	mutex *sync.Mutex
	// DefaultMounts are mount constraints for container mounts added by CRI and GCS
	DefaultMounts []mountInternal
	// DefaultEnvs are environment variable constraints for variables added
	// by CRI and GCS
	DefaultEnvs []EnvRuleConfig
}

var _ SecurityPolicyEnforcer = (*StandardSecurityPolicyEnforcer)(nil)

func NewStandardSecurityPolicyEnforcer(containers []securityPolicyContainer, encoded string) *StandardSecurityPolicyEnforcer {
	// create new StandardSecurityPolicyEnforcer and add the expected containers
	// to it
	// fill out corresponding devices structure by creating a "same shaped"
	// devices listing that corresponds to our container root hash lists
	// the devices list will get filled out as layers are mounted
	devices := make([][]string, len(containers))

	for i, container := range containers {
		devices[i] = make([]string, len(container.Layers))
	}

	return &StandardSecurityPolicyEnforcer{
		EncodedSecurityPolicy:        encoded,
		Containers:                   containers,
		Devices:                      devices,
		ContainerIndexToContainerIds: map[int]map[string]struct{}{},
		startedContainers:            map[string]struct{}{},
		mutex:                        &sync.Mutex{},
	}
}

func (c Containers) toInternal() ([]securityPolicyContainer, error) {
	containerMapLength := len(c.Elements)
	if c.Length != containerMapLength {
		err := fmt.Errorf(
			"container numbers don't match in policy. expected: %d, actual: %d",
			c.Length,
			containerMapLength,
		)
		return nil, err
	}

	internal := make([]securityPolicyContainer, containerMapLength)

	for i := 0; i < containerMapLength; i++ {
		index := strconv.Itoa(i)
		cConf, ok := c.Elements[index]
		if !ok {
			return nil, fmt.Errorf("container constraint with index %q not found", index)
		}
		cInternal, err := cConf.toInternal()
		if err != nil {
			return nil, err
		}
		// save off new container
		internal[i] = cInternal
	}

	return internal, nil
}

func (c Container) toInternal() (securityPolicyContainer, error) {
	command, err := c.Command.toInternal()
	if err != nil {
		return securityPolicyContainer{}, err
	}

	envRules, err := c.EnvRules.toInternal()
	if err != nil {
		return securityPolicyContainer{}, err
	}

	layers, err := c.Layers.toInternal()
	if err != nil {
		return securityPolicyContainer{}, err
	}

	expectedMounts, err := c.ExpectedMounts.toInternal()
	if err != nil {
		return securityPolicyContainer{}, err
	}

	mounts, err := c.Mounts.toInternal()
	if err != nil {
		return securityPolicyContainer{}, err
	}
	return securityPolicyContainer{
		Command:  command,
		EnvRules: envRules,
		Layers:   layers,
		// No need to have toInternal(), because WorkingDir is a string both
		// internally and in the policy.
		WorkingDir:     c.WorkingDir,
		ExpectedMounts: expectedMounts,
		Mounts:         mounts,
	}, nil
}

func (c CommandArgs) toInternal() ([]string, error) {
	if c.Length != len(c.Elements) {
		return nil, fmt.Errorf("command argument numbers don't match in policy. expected: %d, actual: %d", c.Length, len(c.Elements))
	}

	return stringMapToStringArray(c.Elements)
}

func (e EnvRules) toInternal() ([]EnvRuleConfig, error) {
	envRulesMapLength := len(e.Elements)
	if e.Length != envRulesMapLength {
		return nil, fmt.Errorf("env rule numbers don't match in policy. expected: %d, actual: %d", e.Length, envRulesMapLength)
	}

	envRules := make([]EnvRuleConfig, envRulesMapLength)
	for i := 0; i < envRulesMapLength; i++ {
		eIndex := strconv.Itoa(i)
		elem, ok := e.Elements[eIndex]
		if !ok {
			return nil, fmt.Errorf("env rule with index %q doesn't exist", eIndex)
		}
		rule := EnvRuleConfig{
			Strategy: elem.Strategy,
			Rule:     elem.Rule,
		}
		envRules[i] = rule
	}

	return envRules, nil
}

func (l Layers) toInternal() ([]string, error) {
	if l.Length != len(l.Elements) {
		return nil, fmt.Errorf("layer numbers don't match in policy. expected: %d, actual: %d", l.Length, len(l.Elements))
	}

	return stringMapToStringArray(l.Elements)
}

func (em ExpectedMounts) toInternal() ([]string, error) {
	if em.Length != len(em.Elements) {
		return nil, fmt.Errorf("expectedMounts numbers don't match in policy. expected: %d, actual: %d", em.Length, len(em.Elements))
	}

	return stringMapToStringArray(em.Elements)
}

func (o Options) toInternal() ([]string, error) {
	optLength := len(o.Elements)
	if o.Length != optLength {
		return nil, fmt.Errorf("mount option numbers don't match in policy. expected: %d, actual: %d", o.Length, optLength)
	}
	return stringMapToStringArray(o.Elements)
}

func (m Mounts) toInternal() ([]mountInternal, error) {
	mountLength := len(m.Elements)
	if m.Length != mountLength {
		return nil, fmt.Errorf("mount constraint numbers don't match in policy. expected: %d, actual: %d", m.Length, mountLength)
	}

	mountConstraints := make([]mountInternal, mountLength)
	for i := 0; i < mountLength; i++ {
		mIndex := strconv.Itoa(i)
		mount, ok := m.Elements[mIndex]
		if !ok {
			return nil, fmt.Errorf("mount constraint with index %q not found", mIndex)
		}
		opts, err := mount.Options.toInternal()
		if err != nil {
			return nil, err
		}
		mountConstraints[i] = mountInternal{
			Source:      mount.Source,
			Destination: mount.Destination,
			Type:        mount.Type,
			Options:     opts,
		}
	}
	return mountConstraints, nil
}

func stringMapToStringArray(m map[string]string) ([]string, error) {
	mapSize := len(m)
	out := make([]string, mapSize)

	for i := 0; i < mapSize; i++ {
		index := strconv.Itoa(i)
		value, ok := m[index]
		if !ok {
			return nil, fmt.Errorf("element with index %q not found", index)
		}
		out[i] = value
	}

	return out, nil
}

func (pe *StandardSecurityPolicyEnforcer) EnforceDeviceMountPolicy(target string, deviceHash string) (err error) {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	if len(pe.Containers) < 1 {
		return errors.New("policy doesn't allow mounting containers")
	}

	if deviceHash == "" {
		return errors.New("device is missing verity root hash")
	}

	found := false

	for i, container := range pe.Containers {
		for ii, layer := range container.Layers {
			if deviceHash == layer {
				pe.Devices[i][ii] = target
				found = true
			}
		}
	}

	if !found {
		return fmt.Errorf("roothash %s for mount %s doesn't match policy", deviceHash, target)
	}

	return nil
}

func (pe *StandardSecurityPolicyEnforcer) EnforceDeviceUnmountPolicy(unmountTarget string) (err error) {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	for _, container := range pe.Devices {
		for j, storedTarget := range container {
			if unmountTarget == storedTarget {
				container[j] = ""
			}
		}
	}

	return nil
}

func (pe *StandardSecurityPolicyEnforcer) EnforceOverlayMountPolicy(containerID string, layerPaths []string) (err error) {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	if len(pe.Containers) < 1 {
		return errors.New("policy doesn't allow mounting containers")
	}

	if _, e := pe.startedContainers[containerID]; e {
		return errors.New("container has already been started")
	}

	// find maximum number of containers that could share this overlay
	maxPossibleContainerIdsForOverlay := 0
	for _, deviceList := range pe.Devices {
		if equalForOverlay(layerPaths, deviceList) {
			maxPossibleContainerIdsForOverlay++
		}
	}

	if maxPossibleContainerIdsForOverlay == 0 {
		errmsg := fmt.Sprintf("layerPaths '%v' doesn't match any valid layer path: '%v'", layerPaths, pe.Devices)
		return errors.New(errmsg)
	}

	for i, deviceList := range pe.Devices {
		if equalForOverlay(layerPaths, deviceList) {
			existing := pe.ContainerIndexToContainerIds[i]
			if len(existing) < maxPossibleContainerIdsForOverlay {
				pe.expandMatchesForContainerIndex(i, containerID)
			} else {
				errmsg := fmt.Sprintf("layerPaths '%v' already used in maximum number of container overlays", layerPaths)
				return errors.New(errmsg)
			}
		}
	}

	return nil
}

func (pe *StandardSecurityPolicyEnforcer) EnforceCreateContainerPolicy(
	containerID string,
	argList []string,
	envList []string,
	workingDir string,
) (err error) {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	if len(pe.Containers) < 1 {
		return errors.New("policy doesn't allow mounting containers")
	}

	if _, e := pe.startedContainers[containerID]; e {
		return errors.New("container has already been started")
	}

	if err = pe.enforceCommandPolicy(containerID, argList); err != nil {
		return err
	}

	if err = pe.enforceEnvironmentVariablePolicy(containerID, envList); err != nil {
		return err
	}

	if err = pe.enforceWorkingDirPolicy(containerID, workingDir); err != nil {
		return err
	}

	// record that we've allowed this container to start
	pe.startedContainers[containerID] = struct{}{}

	return nil
}

func (pe *StandardSecurityPolicyEnforcer) enforceCommandPolicy(containerID string, argList []string) (err error) {
	// Get a list of all the indexes into our security policy's list of
	// containers that are possible matches for this containerID based
	// on the image overlay layout
	possibleIndices := pe.possibleIndicesForID(containerID)

	// Loop through every possible match and do two things:
	// 1- see if any command matches. we need at least one match or
	//    we don't allow the container to start
	// 2- remove this containerID as a possible match for any container from the
	//    security policy whose command line isn't a match.
	matchingCommandFound := false
	for _, possibleIndex := range possibleIndices {
		cmd := pe.Containers[possibleIndex].Command
		if stringSlicesEqual(cmd, argList) {
			matchingCommandFound = true
		} else {
			// a possible matching index turned out not to match, so we
			// need to update that list and remove it
			pe.narrowMatchesForContainerIndex(possibleIndex, containerID)
		}
	}

	if !matchingCommandFound {
		errmsg := fmt.Sprintf("command %v doesn't match policy", argList)
		return errors.New(errmsg)
	}

	return nil
}

func (pe *StandardSecurityPolicyEnforcer) enforceEnvironmentVariablePolicy(containerID string, envList []string) (err error) {
	// Get a list of all the indexes into our security policy's list of
	// containers that are possible matches for this containerID based
	// on the image overlay layout and command line
	possibleIndices := pe.possibleIndicesForID(containerID)

	for _, envVariable := range envList {
		matchingRuleFoundForSomeContainer := false
		for _, possibleIndex := range possibleIndices {
			envRules := pe.Containers[possibleIndex].EnvRules
			ok := envIsMatchedByRule(envVariable, envRules)
			if ok {
				matchingRuleFoundForSomeContainer = true
			} else {
				// a possible matching index turned out not to match, so we
				// need to update that list and remove it
				pe.narrowMatchesForContainerIndex(possibleIndex, containerID)
			}
		}

		if !matchingRuleFoundForSomeContainer {
			return fmt.Errorf("env variable %s unmatched by policy rule", envVariable)
		}
	}

	return nil
}

func (pe *StandardSecurityPolicyEnforcer) enforceWorkingDirPolicy(containerID string, workingDir string) error {
	possibleIndices := pe.possibleIndicesForID(containerID)

	matched := false
	for _, pIndex := range possibleIndices {
		pWorkingDir := pe.Containers[pIndex].WorkingDir
		if pWorkingDir == workingDir {
			matched = true
		} else {
			pe.narrowMatchesForContainerIndex(pIndex, containerID)
		}
	}
	if !matched {
		return fmt.Errorf("working_dir %q unmatched by policy rule", workingDir)
	}
	return nil
}

func envIsMatchedByRule(envVariable string, rules []EnvRuleConfig) bool {
	for _, rule := range rules {
		switch rule.Strategy {
		case "string":
			if rule.Rule == envVariable {
				return true
			}
		case "re2":
			// if the match errors out, we don't care. it's not a match
			matched, _ := regexp.MatchString(rule.Rule, envVariable)
			if matched {
				return true
			}
		}
	}

	return false
}

// StandardSecurityPolicyEnforcer.mutex lock must be held prior to calling this function.
func (pe *StandardSecurityPolicyEnforcer) expandMatchesForContainerIndex(index int, idToAdd string) {
	_, keyExists := pe.ContainerIndexToContainerIds[index]
	if !keyExists {
		pe.ContainerIndexToContainerIds[index] = map[string]struct{}{}
	}

	pe.ContainerIndexToContainerIds[index][idToAdd] = struct{}{}
}

// StandardSecurityPolicyEnforcer.mutex lock must be held prior to calling this function.
func (pe *StandardSecurityPolicyEnforcer) narrowMatchesForContainerIndex(index int, idToRemove string) {
	delete(pe.ContainerIndexToContainerIds[index], idToRemove)
}

func equalForOverlay(a1 []string, a2 []string) bool {
	// We've stored the layers from bottom to top they are in layerPaths as
	// top to bottom (the order a string gets concatenated for the unix mount
	// command). W do our check with that in mind.
	if len(a1) == len(a2) {
		topIndex := len(a2) - 1
		for i, v := range a1 {
			if v != a2[topIndex-i] {
				return false
			}
		}
	} else {
		return false
	}
	return true
}

// StandardSecurityPolicyEnforcer.mutex lock must be held prior to calling this function.
func (pe *StandardSecurityPolicyEnforcer) possibleIndicesForID(containerID string) []int {
	var possibleIndices []int
	for index, ids := range pe.ContainerIndexToContainerIds {
		for id := range ids {
			if containerID == id {
				possibleIndices = append(possibleIndices, index)
			}
		}
	}
	return possibleIndices
}

func (pe *StandardSecurityPolicyEnforcer) enforceDefaultMounts(specMount oci.Mount) error {
	for _, mountConstraint := range pe.DefaultMounts {
		if err := mountConstraint.validate(specMount); err == nil {
			return nil
		}
	}
	return fmt.Errorf("mount not allowed by default mount constraints: %+v", specMount)
}

func (pe *StandardSecurityPolicyEnforcer) ExtendDefaultMounts(defaultMounts []oci.Mount) error {
	for _, mnt := range defaultMounts {
		pe.DefaultMounts = append(pe.DefaultMounts, newMountConstraint(
			mnt.Source,
			mnt.Destination,
			mnt.Type,
			mnt.Options,
		))
	}
	return nil
}

// EnforceMountPolicy for StandardSecurityPolicyEnforcer validates various
// default mounts injected into container spec by GCS or containerD
func (pe *StandardSecurityPolicyEnforcer) EnforceMountPolicy(sandboxID, containerID string, spec *oci.Spec) (err error) {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	possibleIndices := pe.possibleIndicesForID(containerID)

	for _, specMnt := range spec.Mounts {
		// first check against default mounts
		if err := pe.enforceDefaultMounts(specMnt); err == nil {
			continue
		}

		mountOk := false
		// check against user provided mount constraints, which helps to figure
		// out which container this mount spec corresponds to.
		for _, pIndex := range possibleIndices {
			cont := pe.Containers[pIndex]
			if err = cont.matchMount(sandboxID, specMnt); err == nil {
				mountOk = true
			} else {
				pe.narrowMatchesForContainerIndex(pIndex, containerID)
			}
		}

		if !mountOk {
			retErr := fmt.Errorf("mount %+v is not allowed by mount constraints", specMnt)
			return retErr
		}
	}
	return nil
}

// validate checks given OCI mount against mount policy. Destination is checked
// by direct string comparisons and Source is checked via a regular expression.
// This is done this way, because container path (Destination) is always fixed,
// however, the host/UVM path (Source) can include IDs generated at runtime and
// impossible to know in advance.
//
// NOTE: Different matching strategies can be added by introducing a separate
// path matching config, which isn't needed at the moment.
func (m *mountInternal) validate(mSpec oci.Mount) error {
	if m.Type != mSpec.Type {
		return fmt.Errorf("mount type not allowed by policy: expected=%q, actual=%q", m.Type, mSpec.Type)
	}
	if ok, _ := regexp.MatchString(m.Source, mSpec.Source); !ok {
		return fmt.Errorf("mount source not allowed by policy: expected=%q, actual=%q", m.Source, mSpec.Source)
	}
	if m.Destination != mSpec.Destination && m.Destination != "" {
		return fmt.Errorf("mount destination not allowed by policy: expected=%q, actual=%q", m.Destination, mSpec.Destination)
	}
	if !stringSlicesEqual(m.Options, mSpec.Options) {
		return fmt.Errorf("mount options not allowed by policy: expected=%q, actual=%q", m.Options, mSpec.Options)
	}
	return nil
}

// matchMount matches given OCI mount against mount constraints. If no match
// found, the mount is not allowed.
func (c *securityPolicyContainer) matchMount(sandboxID string, m oci.Mount) (err error) {
	for _, constraint := range c.Mounts {
		// now that we know the sandboxID we can get the actual path for
		// various destination path types by adding a UVM mount prefix
		constraint = substituteUVMPath(sandboxID, constraint)
		if err = constraint.validate(m); err == nil {
			return nil
		}
	}
	return fmt.Errorf("mount is not allowed by policy: %+v", m)
}

// substituteUVMPath substitutes mount prefix to an appropriate path inside
// UVM. At policy generation time, it's impossible to tell what the sandboxID
// will be, so the prefix substitution needs to happen during runtime.
func substituteUVMPath(sandboxID string, m mountInternal) mountInternal {
	if strings.HasPrefix(m.Source, guestpath.SandboxMountPrefix) {
		m.Source = specInternal.SandboxMountSource(sandboxID, m.Source)
	} else if strings.HasPrefix(m.Source, guestpath.HugePagesMountPrefix) {
		m.Source = specInternal.HugePagesMountSource(sandboxID, m.Source)
	}
	return m
}

func stringSlicesEqual(slice1, slice2 []string) bool {
	if len(slice1) != len(slice2) {
		return false
	}

	for i := 0; i < len(slice1); i++ {
		if slice1[i] != slice2[i] {
			return false
		}
	}
	return true
}

// EnforceExpectedMountsPolicy for StandardSecurityPolicyEnforcer injects a
// hooks.CreateRuntime hook into container spec and the hook ensures that
// the expected mounts appear prior container start. At the moment enforcement
// is expected to take place inside LCOW UVM.
//
// Expected mount is provided as a path under a sandbox mount path inside
// container, e.g., sandbox mount is at path "/path/in/container" and wait path
// is "/path/in/container/wait/path", which corresponds to
// "/run/gcs/c/<podID>/sandboxMounts/path/on/the/host/wait/path"
//
// Iterates through container mounts to identify the correct sandbox
// mount where the wait path is nested under. The mount spec will
// be something like:
// {
//    "source": "/run/gcs/c/<podID>/sandboxMounts/path/on/host",
//    "destination": "/path/in/container"
// }
// The wait path will be "/path/in/container/wait/path". To find the corresponding
// sandbox mount do a prefix match on wait path against all container mounts
// Destination and resolve the full path inside UVM. For example above it becomes
// "/run/gcs/c/<podID>/sandboxMounts/path/on/host/wait/path"
func (pe *StandardSecurityPolicyEnforcer) EnforceExpectedMountsPolicy(containerID string, spec *oci.Spec) error {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	if len(pe.Containers) < 1 {
		return errors.New("policy doesn't allow mounting containers")
	}

	sandboxID := spec.Annotations[annotations.KubernetesSandboxID]
	if sandboxID == "" {
		return errors.New("no sandbox ID present in spec annotations")
	}

	var wMounts []string
	pIndices := pe.possibleIndicesForID(containerID)
	if len(pIndices) == 0 {
		return errors.New("no valid container indices found")
	}

	// Unlike environment variable and command line enforcement, there isn't anything
	// to validate here, since we're essentially just injecting hooks when necessary
	// for all containers.
	matchFound := false
	for _, index := range pIndices {
		if !matchFound {
			matchFound = true
			wMounts = pe.Containers[index].ExpectedMounts
		} else {
			pe.narrowMatchesForContainerIndex(index, containerID)
		}
	}

	if len(wMounts) == 0 {
		return nil
	}

	var wPaths []string
	for _, mount := range wMounts {
		var wp string
		for _, m := range spec.Mounts {
			// prefix matching to find correct sandbox mount
			if strings.HasPrefix(mount, m.Destination) {
				wp = filepath.Join(m.Source, strings.TrimPrefix(mount, m.Destination))
				break
			}
		}
		if wp == "" {
			return fmt.Errorf("invalid mount path: %q", mount)
		}
		wPaths = append(wPaths, filepath.Clean(wp))
	}

	pathsArg := strings.Join(wPaths, ",")
	waitPathsBinary := "/bin/wait-paths"
	args := []string{
		waitPathsBinary,
		"--paths",
		pathsArg,
		"--timeout",
		"60",
	}
	hook := hooks.NewOCIHook(waitPathsBinary, args, os.Environ())
	return hooks.AddOCIHook(spec, hooks.CreateRuntime, hook)
}

type OpenDoorSecurityPolicyEnforcer struct{}

var _ SecurityPolicyEnforcer = (*OpenDoorSecurityPolicyEnforcer)(nil)

func (OpenDoorSecurityPolicyEnforcer) EnforceDeviceMountPolicy(_ string, _ string) error {
	return nil
}

func (OpenDoorSecurityPolicyEnforcer) EnforceDeviceUnmountPolicy(_ string) error {
	return nil
}

func (OpenDoorSecurityPolicyEnforcer) EnforceOverlayMountPolicy(_ string, _ []string) error {
	return nil
}

func (OpenDoorSecurityPolicyEnforcer) EnforceCreateContainerPolicy(_ string, _ []string, _ []string, _ string) error {
	return nil
}

func (OpenDoorSecurityPolicyEnforcer) EnforceMountPolicy(_, _ string, _ *oci.Spec) error {
	return nil
}

func (OpenDoorSecurityPolicyEnforcer) EnforceExpectedMountsPolicy(_ string, _ *oci.Spec) error {
	return nil
}

func (OpenDoorSecurityPolicyEnforcer) ExtendDefaultMounts(_ []oci.Mount) error {
	return nil
}

type ClosedDoorSecurityPolicyEnforcer struct{}

var _ SecurityPolicyEnforcer = (*ClosedDoorSecurityPolicyEnforcer)(nil)

func (ClosedDoorSecurityPolicyEnforcer) EnforceDeviceMountPolicy(_ string, _ string) error {
	return errors.New("mounting is denied by policy")
}

func (ClosedDoorSecurityPolicyEnforcer) EnforceDeviceUnmountPolicy(_ string) error {
	return errors.New("unmounting is denied by policy")
}

func (ClosedDoorSecurityPolicyEnforcer) EnforceOverlayMountPolicy(_ string, _ []string) error {
	return errors.New("creating an overlay fs is denied by policy")
}

func (ClosedDoorSecurityPolicyEnforcer) EnforceCreateContainerPolicy(_ string, _ []string, _ []string, _ string) error {
	return errors.New("running commands is denied by policy")
}

func (ClosedDoorSecurityPolicyEnforcer) EnforceExpectedMountsPolicy(_ string, _ *oci.Spec) error {
	return errors.New("enforcing expected mounts is denied by policy")
}

func (ClosedDoorSecurityPolicyEnforcer) EnforceMountPolicy(_, _ string, _ *oci.Spec) error {
	return errors.New("container mounts are denied by policy")
}

func (ClosedDoorSecurityPolicyEnforcer) ExtendDefaultMounts(_ []oci.Mount) error {
	return nil
}
