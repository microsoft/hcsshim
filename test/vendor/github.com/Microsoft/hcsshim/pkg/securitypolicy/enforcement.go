package securitypolicy

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"sync"

	"github.com/google/go-cmp/cmp"
)

// PolicyEnforcer is an interface that encapsulates the logic necessary for
// enforcing a security policy
type PolicyEnforcer interface {
	EnforceDeviceMountPolicy(target string, deviceHash string) (err error)
	EnforceDeviceUnmountPolicy(unmountTarget string) (err error)
	EnforceOverlayMountPolicy(containerID string, layerPaths []string) (err error)
	EnforceCreateContainerPolicy(containerID string, argList []string, envList []string) (err error)
}

// NewSecurityPolicyEnforcer is a factory method that returns a corresponding
// PolicyEnforcer based on the State passed in
func NewSecurityPolicyEnforcer(state State) (PolicyEnforcer, error) {
	if state.SecurityPolicy.AllowAll {
		return &OpenDoorEnforcer{}, nil
	} else {
		containers, err := state.SecurityPolicy.Containers.toInternal()
		if err != nil {
			return nil, err
		}
		return NewStandardEnforcer(containers, state.EncodedSecurityPolicy.SecurityPolicy), nil
	}
}

// StandardEnforcer enforces user provided security policy and tracks the
// internal state of all containers.
//
// Most of the work that this security policy enforcer does is around managing
// state needed to map from a container definition in the Policy to a specific
// container ID as we bring up each container.
//
// Implementation details are available in:
// - EnforceDeviceMountPolicy
// - EnforceDeviceUnmountPolicy
// - EnforceOverlayMountPolicy
// - EnforceCreateContainerPolicy
// - NewStandardEnforcer
type StandardEnforcer struct {
	// EncodedSecurityPolicy state is needed for key release
	EncodedSecurityPolicy string
	// Containers from the user supplied security policy.
	//
	// Containers that share the same base image, and perhaps further
	// information, will have an entry per container instance in the
	// SecurityPolicy. For example, a policy that has two containers that
	// use Ubuntu 18.04 will have an entry for each even if they share the same
	// command line.
	Containers []container
	// Devices is a listing of targets seen when mounting a device
	// stored in a "per-container basis". As the UVM goes through its process of
	// bringing up containers, we have to piece together information about what
	// is going on.
	Devices [][]string
	// ContainerIndexToContainerIds is a mapping between a container defined in
	// the policy to potential container IDs, that were created in the runtime
	//
	// Devices and ContainerIndexToContainerIds are used to build up an
	// understanding of the containers running with a UVM as they come up and
	// map them back to a container definition from the user supplied Policy
	//
	// As containers can have exactly the same base image and be "the same" at
	// the time we are doing overlay, the ContainerIndexToContainerIds in a
	// set of possible containers for a given container id. Go doesn't have a set
	// type so we are doing the idiomatic go thing of using a map[string]struct{}
	// to represent the set.
	ContainerIndexToContainerIds map[int]map[string]struct{}
	// Set of container IDs that we've allowed to start. Because Go doesn't have
	// sets as a built-in data structure, we are using a map
	startedContainers map[string]struct{}
	// Mutex to prevent concurrent access to fields
	mutex *sync.Mutex
}

var _ PolicyEnforcer = (*StandardEnforcer)(nil)

// NewStandardEnforcer creates a new StandardEnforcer instance and adds the expected
// containers to it.
func NewStandardEnforcer(containers []container, encoded string) *StandardEnforcer {
	// Fill out corresponding devices structure by creating a "same shaped"
	// devices listing that corresponds to our container root hash lists
	// the devices list will get filled out as layers are mounted
	devices := make([][]string, len(containers))

	for i, c := range containers {
		devices[i] = make([]string, len(c.Layers))
	}

	return &StandardEnforcer{
		EncodedSecurityPolicy:        encoded,
		Containers:                   containers,
		Devices:                      devices,
		ContainerIndexToContainerIds: map[int]map[string]struct{}{},
		startedContainers:            map[string]struct{}{},
		mutex:                        &sync.Mutex{},
	}
}

func (c Containers) toInternal() ([]container, error) {
	containerMapLength := len(c.Elements)
	if c.Length != containerMapLength {
		return nil, fmt.Errorf("container numbers don't match in policy. expected: %d, actual: %d", c.Length, containerMapLength)
	}

	internal := make([]container, containerMapLength)

	for i := 0; i < containerMapLength; i++ {
		iContainer, err := c.Elements[strconv.Itoa(i)].toInternal()
		if err != nil {
			return nil, err
		}

		// save off new container
		internal[i] = iContainer
	}

	return internal, nil
}

func (c Container) toInternal() (container, error) {
	command, err := c.Command.toInternal()
	if err != nil {
		return container{}, err
	}

	envRules, err := c.EnvRules.toInternal()
	if err != nil {
		return container{}, err
	}

	layers, err := c.Layers.toInternal()
	if err != nil {
		return container{}, err
	}

	return container{
		Command:  command,
		EnvRules: envRules,
		Layers:   layers,
	}, nil
}

func (c CommandArgs) toInternal() ([]string, error) {
	if c.Length != len(c.Elements) {
		return nil, fmt.Errorf("command argument numbers don't match in policy. expected: %d, actual: %d", c.Length, len(c.Elements))
	}

	return stringMapToStringArray(c.Elements), nil
}

func (e EnvRules) toInternal() ([]environmentVariableRule, error) {
	envRulesMapLength := len(e.Elements)
	if e.Length != envRulesMapLength {
		return nil, fmt.Errorf("env rule numbers don't match in policy. expected: %d, actual: %d", e.Length, envRulesMapLength)
	}

	envRules := make([]environmentVariableRule, envRulesMapLength)
	for i := 0; i < envRulesMapLength; i++ {
		eIndex := strconv.Itoa(i)
		rule := environmentVariableRule{
			Strategy: e.Elements[eIndex].Strategy,
			Rule:     e.Elements[eIndex].Rule,
		}
		envRules[i] = rule
	}

	return envRules, nil
}

func (l Layers) toInternal() ([]string, error) {
	if l.Length != len(l.Elements) {
		return nil, fmt.Errorf("layer numbers don't match in policy. expected: %d, actual: %d", l.Length, len(l.Elements))
	}

	return stringMapToStringArray(l.Elements), nil
}

func stringMapToStringArray(in map[string]string) []string {
	inLength := len(in)
	out := make([]string, inLength)

	for i := 0; i < inLength; i++ {
		out[i] = in[strconv.Itoa(i)]
	}

	return out
}

// EnforceDeviceMountPolicy for StandardEnforcer validates the target and
// its deviceHash against the read-only layers described in the security
// policy and updates internal state when the corresponding layer hash is
// found.
//
// At the time that devices are being mounted, we do not know a container
// that they will be used for; only that there is a device with a given root
// hash that being mounted. We check to make sure that the root hash for the
// devices is a root hash that exists for 1 or more layers in any container
// in the supplied Policy. Each "seen" layer is recorded in devices as it is
// mounted. So for example, if a root hash mount is found for the device being
// mounted and the first layer of the first container then we record the device
// target in Devices[0][0].
func (pe *StandardEnforcer) EnforceDeviceMountPolicy(target string, deviceHash string) (err error) {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	if len(pe.Containers) < 1 {
		return errors.New("policy doesn't allow mounting containers")
	}

	if deviceHash == "" {
		return errors.New("device is missing verity root hash")
	}

	found := false

	for i, c := range pe.Containers {
		for ii, layer := range c.Layers {
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

// EnforceDeviceUnmountPolicy for StandardEnforcer finds the corresponding layers and resets
// the internal state.
func (pe *StandardEnforcer) EnforceDeviceUnmountPolicy(unmountTarget string) (err error) {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	for _, targets := range pe.Devices {
		for j, storedTarget := range targets {
			if unmountTarget == storedTarget {
				targets[j] = ""
			}
		}
	}

	return nil
}

// EnforceOverlayMountPolicy for StandardEnforcer validates provided layerPaths
// against internal state.
//
// When overlay filesystems  created, we verify that the ordered layers
// for said overlay filesystem match one of the device orderings in Devices.
// When a match is found, the index in Devices is the same index in
// Policy.Containers. Overlay filesystem creation is the first time we
// have a "container id" available to us. The container id identifies the
// container in question going forward. We record the mapping of Container
// index to container id so that when we have future operations like "run
// command" which come with a container id, we can find the corresponding
// container index and use that to look up the command in the appropriate
// Container instance.
func (pe *StandardEnforcer) EnforceOverlayMountPolicy(containerID string, layerPaths []string) (err error) {
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

// EnforceCreateContainerPolicy for StandardEnforcer validates the actual init
// process command line arguments and environment variables passed during
// container creation. The command line arguments must have an exact match
// where env vars must match a set of rules defined in the security policy.
//
// See enforceCommandPolicy where most of the functionality is handling the
// case were policy containers share an overlay and have to try to distinguish
// them based on the command line arguments. enforceEnvironmentVariablePolicy
// can further narrow based on environment variables if required.
func (pe *StandardEnforcer) EnforceCreateContainerPolicy(containerID string, argList []string, envList []string) (err error) {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	if len(pe.Containers) < 1 {
		return errors.New("policy doesn't allow mounting containers")
	}

	if _, e := pe.startedContainers[containerID]; e {
		return errors.New("container has already been started")
	}

	err = pe.enforceCommandPolicy(containerID, argList)
	if err != nil {
		return err
	}

	err = pe.enforceEnvironmentVariablePolicy(containerID, envList)
	if err != nil {
		return err
	}

	// record that we've allowed this container to start
	pe.startedContainers[containerID] = struct{}{}

	return nil
}

func (pe *StandardEnforcer) enforceCommandPolicy(containerID string, argList []string) (err error) {
	// Get a list of all the indexes into our security policy's list of
	// containers that are possible matches for this containerID based
	// on the image overlay layout
	possibleIndexes := possibleIndexesForID(containerID, pe.ContainerIndexToContainerIds)

	// Loop through every possible match and do two things:
	// 1- see if any command matches. we need at least one match or
	//    we don't allow the container to start
	// 2- remove this containerID as a possible match for any container from the
	//    security policy whose command line isn't a match.
	matchingCommandFound := false
	for _, possibleIndex := range possibleIndexes {
		cmd := pe.Containers[possibleIndex].Command
		if cmp.Equal(cmd, argList) {
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

func (pe *StandardEnforcer) enforceEnvironmentVariablePolicy(containerID string, envList []string) (err error) {
	// Get a list of all the indexes into our security policy's list of
	// containers that are possible matches for this containerID based
	// on the image overlay layout and command line
	possibleIndexes := possibleIndexesForID(containerID, pe.ContainerIndexToContainerIds)

	for _, envVariable := range envList {
		matchingRuleFoundForSomeContainer := false
		for _, possibleIndex := range possibleIndexes {
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

func envIsMatchedByRule(envVariable string, rules []environmentVariableRule) bool {
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

func (pe *StandardEnforcer) expandMatchesForContainerIndex(index int, idToAdd string) {
	_, keyExists := pe.ContainerIndexToContainerIds[index]
	if !keyExists {
		pe.ContainerIndexToContainerIds[index] = map[string]struct{}{}
	}

	pe.ContainerIndexToContainerIds[index][idToAdd] = struct{}{}
}

func (pe *StandardEnforcer) narrowMatchesForContainerIndex(index int, idToRemove string) {
	delete(pe.ContainerIndexToContainerIds[index], idToRemove)
}

func equalForOverlay(a1 []string, a2 []string) bool {
	// We've stored the layers from bottom to top, they are in layerPaths as
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

func possibleIndexesForID(containerID string, mapping map[int]map[string]struct{}) []int {
	var possibles []int
	for index, ids := range mapping {
		for id := range ids {
			if containerID == id {
				possibles = append(possibles, index)
			}
		}
	}

	return possibles
}

type OpenDoorEnforcer struct{}

var _ PolicyEnforcer = (*OpenDoorEnforcer)(nil)

// EnforceDeviceMountPolicy for OpenDoorEnforcer is a noop that allows everything
func (p *OpenDoorEnforcer) EnforceDeviceMountPolicy(target string, deviceHash string) (err error) {
	return nil
}

// EnforceDeviceUnmountPolicy for OpenDoorEnforcer is a noop that allows everything
func (p *OpenDoorEnforcer) EnforceDeviceUnmountPolicy(target string) (err error) {
	return nil
}

// EnforceOverlayMountPolicy for OpenDoorEnforcer is a noop that allows everything
func (p *OpenDoorEnforcer) EnforceOverlayMountPolicy(containerID string, layerPaths []string) (err error) {
	return nil
}

// EnforceCreateContainerPolicy for OpenDoorEnforcer is a noop that allows everything
func (p *OpenDoorEnforcer) EnforceCreateContainerPolicy(containerID string, argList []string, envList []string) (err error) {
	return nil
}

type ClosedDoorEnforcer struct{}

var _ PolicyEnforcer = (*ClosedDoorEnforcer)(nil)

// EnforceDeviceMountPolicy for ClosedDoorEnforcer is a noop that rejects everything
func (p *ClosedDoorEnforcer) EnforceDeviceMountPolicy(target string, deviceHash string) (err error) {
	return errors.New("mounting is denied by policy")
}

// EnforceDeviceUnmountPolicy for ClosedDoorEnforcer is a noop that rejects everything
func (p *ClosedDoorEnforcer) EnforceDeviceUnmountPolicy(target string) (err error) {
	return errors.New("unmounting is denied by policy")
}

// EnforceOverlayMountPolicy for ClosedDoorEnforcer is a noop that rejects everything
func (p *ClosedDoorEnforcer) EnforceOverlayMountPolicy(containerID string, layerPaths []string) (err error) {
	return errors.New("creating an overlay fs is denied by policy")
}

// EnforceCreateContainerPolicy for ClosedDoorEnforcer is a noop that rejects everything
func (p *ClosedDoorEnforcer) EnforceCreateContainerPolicy(containerID string, argList []string, envList []string) (err error) {
	return errors.New("running commands is denied by policy")
}
