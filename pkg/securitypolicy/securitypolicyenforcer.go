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

	oci "github.com/opencontainers/runtime-spec/specs-go"

	specInternal "github.com/Microsoft/hcsshim/internal/guest/spec"
	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/Microsoft/hcsshim/internal/hooks"
	"github.com/Microsoft/hcsshim/pkg/annotations"
)

type SecurityPolicyEnforcer interface {
	EnforceDeviceMountPolicy(target string, deviceHash string) (err error)
	EnforceDeviceUnmountPolicy(unmountTarget string) (err error)
	EnforceOverlayMountPolicy(containerID string, layerPaths []string) (err error)
	EnforceCreateContainerPolicy(containerID string, argList []string, envList []string, workingDir string) (err error)
	EnforceWaitMountPointsPolicy(containerID string, spec *oci.Spec) error
	EnforceMountPolicy(sandboxID, containerID string, spec *oci.Spec) error
	ExtendDefaultMounts([]oci.Mount) error
	EncodedSecurityPolicy() string
}

func NewSecurityPolicyEnforcer(state SecurityPolicyState, eOpts ...standardEnforcerOpt) (SecurityPolicyEnforcer, error) {
	if state.SecurityPolicy.AllowAll {
		if state.SecurityPolicy.Containers.Length > 0 || len(state.SecurityPolicy.Containers.Elements) > 0 {
			return nil, ErrInvalidAllowAllPolicy
		}
		return &OpenDoorSecurityPolicyEnforcer{
			encodedSecurityPolicy: state.EncodedSecurityPolicy.SecurityPolicy,
		}, nil
	} else {
		containers, err := state.SecurityPolicy.Containers.toInternal()
		if err != nil {
			return nil, err
		}
		enforcer := NewStandardSecurityPolicyEnforcer(containers, state.EncodedSecurityPolicy.SecurityPolicy)
		for _, o := range eOpts {
			if err := o(enforcer); err != nil {
				return nil, err
			}
		}
		return enforcer, nil
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

type standardEnforcerOpt func(e *StandardSecurityPolicyEnforcer) error

// WithPrivilegedMounts converts the input mounts to internal mount constraints
// and extends existing internal mount constraints if the container is allowed
// to be executed in elevated mode.
func WithPrivilegedMounts(mounts []oci.Mount) standardEnforcerOpt {
	return func(e *StandardSecurityPolicyEnforcer) error {
		for _, c := range e.Containers {
			if c.AllowElevated {
				for _, m := range mounts {
					mi := mountInternal{
						Source:      m.Source,
						Destination: m.Destination,
						Type:        m.Type,
						Options:     m.Options,
					}
					c.Mounts = append(c.Mounts, mi)
				}
			}
		}
		return nil
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
	WaitMountPoints []string
	// A list of constraints for determining if a given mount is allowed.
	Mounts        []mountInternal
	AllowElevated bool
}

// StandardSecurityPolicyEnforcer implements SecurityPolicyEnforcer interface
// and is responsible for enforcing various SecurityPolicy constraints.
//
// Most of the work that this security policy enforcer does it around managing
// state needed to map from a container definition in the SecurityPolicy to
// a specific container ID as we bring up each container. For example, see
// EnforceCreateContainerPolicy where most of the functionality is handling the
// case were policy containers share an overlay and have to try to distinguish
// them based on the command line arguments, environment variables or working
// directory.
//
// Containers that share the same base image, and perhaps further information,
// will have an entry per container instance in the SecurityPolicy. For example,
// a policy that has two containers that use Ubuntu 18.04 will have an entry for
// each even if they share the same command line.
type StandardSecurityPolicyEnforcer struct {
	// encodedSecurityPolicy state is needed for key release
	encodedSecurityPolicy string
	// Containers is the internal representation of users' container policies.
	Containers []*securityPolicyContainer
	// Devices is a mapping between target and a corresponding root hash. Target
	// is a path to a particular block device or its mount point inside UVM and
	// root hash is the dm-verity root hash of that device. Mainly the stored
	// devices represent read-only container layers, but this may change.
	// As the UVM goes through its process of bringing up containers, we have to
	// piece together information about what is going on.
	Devices map[string]string
	// ContainerIndexToContainerIds is a mapping between containers in the
	// SecurityPolicy and possible container IDs that have been created by runc,
	// but have not yet been run.
	//
	// As containers can have exactly the same base image and be "the same" at
	// the time we are doing overlay, the ContainerIndexToContainerIds is a set
	// of possible containers for a given container id. Go doesn't have a set
	// type, so we are doing the idiomatic go thing of using a map[string]struct{}
	// to represent the set.
	ContainerIndexToContainerIds map[int]map[string]struct{}
	// startedContainers is a set of container IDs that were allowed to start.
	// Because Go doesn't have sets as a built-in data structure, we are using a map.
	startedContainers map[string]struct{}
	// mutex guards against concurrent access to fields.
	mutex *sync.Mutex
	// DefaultMounts are mount constraints for container mounts added by CRI and
	// GCS. Since default mounts will be allowed for all containers in the UVM
	// they are not added to each individual policy container and kept as global
	// policy rules.
	DefaultMounts []mountInternal
	// DefaultEnvs are environment variable constraints for variables added
	// by CRI and GCS. Since default envs will be allowed for all containers
	// in the UVM they are not added to each individual policy container and
	// kept as global policy rules.
	DefaultEnvs []EnvRuleConfig
}

var _ SecurityPolicyEnforcer = (*StandardSecurityPolicyEnforcer)(nil)

func NewStandardSecurityPolicyEnforcer(
	containers []*securityPolicyContainer,
	encoded string,
) *StandardSecurityPolicyEnforcer {
	return &StandardSecurityPolicyEnforcer{
		encodedSecurityPolicy:        encoded,
		Containers:                   containers,
		Devices:                      map[string]string{},
		ContainerIndexToContainerIds: map[int]map[string]struct{}{},
		startedContainers:            map[string]struct{}{},
		mutex:                        &sync.Mutex{},
	}
}

func (c Containers) toInternal() ([]*securityPolicyContainer, error) {
	containerMapLength := len(c.Elements)
	if c.Length != containerMapLength {
		err := fmt.Errorf(
			"container numbers don't match in policy. expected: %d, actual: %d",
			c.Length,
			containerMapLength,
		)
		return nil, err
	}

	internal := make([]*securityPolicyContainer, containerMapLength)

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
		internal[i] = &cInternal
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

	waitMounts, err := c.WaitMountPoints.toInternal()
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
		WorkingDir:      c.WorkingDir,
		WaitMountPoints: waitMounts,
		Mounts:          mounts,
		AllowElevated:   c.AllowElevated,
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

func (wm WaitMountPoints) toInternal() ([]string, error) {
	if wm.Length != len(wm.Elements) {
		return nil, fmt.Errorf("expectedMounts numbers don't match in policy. expected: %d, actual: %d", wm.Length, len(wm.Elements))
	}

	return stringMapToStringArray(wm.Elements)
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

// EnforceDeviceMountPolicy for StandardSecurityPolicyEnforcer validates that
// the target block device's root hash matches any container in SecurityPolicy.
// Block device targets with invalid root hashes are rejected.
//
// At the time that devices are being mounted, we do not know a container
// that they will be used for; only that there is a device with a given root
// hash that being mounted. We check to make sure that the root hash for the
// devices is a root hash that exists for 1 or more layers in any container
// in the supplied SecurityPolicy. Each "seen" layer is recorded in devices
// as it is mounted.
func (pe *StandardSecurityPolicyEnforcer) EnforceDeviceMountPolicy(target string, deviceHash string) (err error) {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	if len(pe.Containers) < 1 {
		return errors.New("policy doesn't allow mounting containers")
	}

	if deviceHash == "" {
		return errors.New("device is missing verity root hash")
	}

	for _, container := range pe.Containers {
		for _, layer := range container.Layers {
			if deviceHash == layer {
				if existingHash := pe.Devices[target]; existingHash != "" {
					return fmt.Errorf(
						"conflicting device hashes for target %s: old=%s, new=%s",
						target,
						existingHash,
						deviceHash,
					)
				}
				pe.Devices[target] = deviceHash
				return nil
			}
		}
	}

	return fmt.Errorf("roothash %s for mount %s doesn't match policy", deviceHash, target)
}

// EnforceDeviceUnmountPolicy for StandardSecurityPolicyEnforcer first validate
// that the target mount was one of the allowed devices and then removes it from
// the mapping.
//
// When proper protocol enforcement is in place, this will also make sure that
// the device isn't currently used by any running container in an overlay.
func (pe *StandardSecurityPolicyEnforcer) EnforceDeviceUnmountPolicy(unmountTarget string) (err error) {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	if _, ok := pe.Devices[unmountTarget]; !ok {
		return fmt.Errorf("device doesn't exist: %s", unmountTarget)
	}
	delete(pe.Devices, unmountTarget)

	return nil
}

// EnforceOverlayMountPolicy for StandardSecurityPolicyEnforcer validates that
// layerPaths represent a valid overlay file system and is allowed by the
// SecurityPolicy.
//
// When overlay filesystems created, look up the root hash chain for an incoming
// overlay and verify it against containers in the policy.
// Overlay filesystem creation is the first time we have a "container ID"
// available to us. The container id identifies the container in question going
// forward. We record the mapping of container index in the policy to a set of
// possible container IDs so that when we have future operations like
// "run command" which come with a container ID, we can find the corresponding
// container index and use that to look up the command in the appropriate
// security policy container instance.
func (pe *StandardSecurityPolicyEnforcer) EnforceOverlayMountPolicy(containerID string, layerPaths []string) (err error) {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	if len(pe.Containers) < 1 {
		return errors.New("policy doesn't allow mounting containers")
	}

	if _, e := pe.startedContainers[containerID]; e {
		return errors.New("container has already been started")
	}

	var incomingOverlay []string
	for _, layer := range layerPaths {
		if hash, ok := pe.Devices[layer]; !ok {
			return fmt.Errorf("overlay layer isn't mounted: %s", layer)
		} else {
			incomingOverlay = append(incomingOverlay, hash)
		}
	}

	// check if any of the containers allow the incoming overlay.
	var matchedContainers []int
	for i, container := range pe.Containers {
		if equalForOverlay(incomingOverlay, container.Layers) {
			matchedContainers = append(matchedContainers, i)
		}
	}

	if len(matchedContainers) == 0 {
		errmsg := fmt.Sprintf("layerPaths '%v' doesn't match any valid overlay", layerPaths)
		return errors.New(errmsg)
	}

	for _, i := range matchedContainers {
		existing := pe.ContainerIndexToContainerIds[i]
		if len(existing) < len(matchedContainers) {
			pe.expandMatchesForContainerIndex(i, containerID)
		} else {
			errmsg := fmt.Sprintf("layerPaths '%v' already used in maximum number of container overlays", layerPaths)
			return errors.New(errmsg)
		}
	}

	return nil
}

// EnforceCreateContainerPolicy for StandardSecurityPolicyEnforcer validates
// the input container command, env and working directory against containers in
// the SecurityPolicy. The enforcement also narrows down the containers that
// have the same overlays by comparing their command, env and working directory
// rules.
//
// Devices and ContainerIndexToContainerIds are used to build up an
// understanding of the containers running with a UVM as they come up and map
// them back to a container definition from the user supplied SecurityPolicy
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

// ExtendDefaultMounts for StandardSecurityPolicyEnforcer adds default mounts
// added by CRI and GCS to the list of DefaultMounts, which are always allowed.
func (pe *StandardSecurityPolicyEnforcer) ExtendDefaultMounts(defaultMounts []oci.Mount) error {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

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
// default mounts injected into container spec by GCS or containerD. As part of
// the enforcement, the method also narrows down possible container IDs with
// the same overlay.
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

// EnforceWaitMountPointsPolicy for StandardSecurityPolicyEnforcer injects a
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
func (pe *StandardSecurityPolicyEnforcer) EnforceWaitMountPointsPolicy(containerID string, spec *oci.Spec) error {
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
			wMounts = pe.Containers[index].WaitMountPoints
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

func (pe *StandardSecurityPolicyEnforcer) EncodedSecurityPolicy() string {
	return pe.encodedSecurityPolicy
}

type OpenDoorSecurityPolicyEnforcer struct {
	encodedSecurityPolicy string
}

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

func (OpenDoorSecurityPolicyEnforcer) EnforceWaitMountPointsPolicy(_ string, _ *oci.Spec) error {
	return nil
}

func (OpenDoorSecurityPolicyEnforcer) ExtendDefaultMounts(_ []oci.Mount) error {
	return nil
}

func (oe *OpenDoorSecurityPolicyEnforcer) EncodedSecurityPolicy() string {
	return oe.encodedSecurityPolicy
}

type ClosedDoorSecurityPolicyEnforcer struct {
	encodedSecurityPolicy string
}

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

func (ClosedDoorSecurityPolicyEnforcer) EnforceWaitMountPointsPolicy(_ string, _ *oci.Spec) error {
	return errors.New("enforcing wait mount points is denied by policy")
}

func (ClosedDoorSecurityPolicyEnforcer) EnforceMountPolicy(_, _ string, _ *oci.Spec) error {
	return errors.New("container mounts are denied by policy")
}

func (ClosedDoorSecurityPolicyEnforcer) ExtendDefaultMounts(_ []oci.Mount) error {
	return nil
}

func (ClosedDoorSecurityPolicyEnforcer) EncodedSecurityPolicy() string {
	return ""
}
