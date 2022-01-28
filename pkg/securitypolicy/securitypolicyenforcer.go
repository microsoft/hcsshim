package securitypolicy

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"sync"

	"github.com/google/go-cmp/cmp"
)

type SecurityPolicyEnforcer interface {
	EnforceDeviceMountPolicy(target string, deviceHash string) (err error)
	EnforceDeviceUnmountPolicy(unmountTarget string) (err error)
	EnforceOverlayMountPolicy(containerID string, layerPaths []string) (err error)
	EnforceCreateContainerPolicy(containerID string, argList []string, envList []string) (err error)
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
	// a specfic container ID as we bring up each container. See
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
}

var _ SecurityPolicyEnforcer = (*StandardSecurityPolicyEnforcer)(nil)

func NewStandardSecurityPolicyEnforcer(containers []securityPolicyContainer, encoded string) *StandardSecurityPolicyEnforcer {
	// create new StandardSecurityPolicyEnforcer and add the expected containers
	// to it
	// fill out corresponding devices structure by creating a "same shapped"
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
		return nil, fmt.Errorf("container numbers don't match in policy. expected: %d, actual: %d", c.Length, containerMapLength)
	}

	internal := make([]securityPolicyContainer, containerMapLength)

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

	return securityPolicyContainer{
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

func (e EnvRules) toInternal() ([]EnvRule, error) {
	envRulesMapLength := len(e.Elements)
	if e.Length != envRulesMapLength {
		return nil, fmt.Errorf("env rule numbers don't match in policy. expected: %d, actual: %d", e.Length, envRulesMapLength)
	}

	envRules := make([]EnvRule, envRulesMapLength)
	for i := 0; i < envRulesMapLength; i++ {
		eIndex := strconv.Itoa(i)
		rule := EnvRule{
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

func (pe *StandardSecurityPolicyEnforcer) EnforceCreateContainerPolicy(containerID string, argList []string, envList []string) (err error) {
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

func (pe *StandardSecurityPolicyEnforcer) enforceCommandPolicy(containerID string, argList []string) (err error) {
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

func (pe *StandardSecurityPolicyEnforcer) enforceEnvironmentVariablePolicy(containerID string, envList []string) (err error) {
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

func envIsMatchedByRule(envVariable string, rules []EnvRule) bool {
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

func (pe *StandardSecurityPolicyEnforcer) expandMatchesForContainerIndex(index int, idToAdd string) {
	_, keyExists := pe.ContainerIndexToContainerIds[index]
	if !keyExists {
		pe.ContainerIndexToContainerIds[index] = map[string]struct{}{}
	}

	pe.ContainerIndexToContainerIds[index][idToAdd] = struct{}{}
}

func (pe *StandardSecurityPolicyEnforcer) narrowMatchesForContainerIndex(index int, idToRemove string) {
	delete(pe.ContainerIndexToContainerIds[index], idToRemove)
}

func equalForOverlay(a1 []string, a2 []string) bool {
	// We've stored the layers from bottom to topl they are in layerPaths as
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
	possibles := []int{}
	for index, ids := range mapping {
		for id := range ids {
			if containerID == id {
				possibles = append(possibles, index)
			}
		}
	}

	return possibles
}

type OpenDoorSecurityPolicyEnforcer struct{}

var _ SecurityPolicyEnforcer = (*OpenDoorSecurityPolicyEnforcer)(nil)

func (p *OpenDoorSecurityPolicyEnforcer) EnforceDeviceMountPolicy(target string, deviceHash string) (err error) {
	return nil
}

func (p *OpenDoorSecurityPolicyEnforcer) EnforceDeviceUnmountPolicy(target string) (err error) {
	return nil
}

func (p *OpenDoorSecurityPolicyEnforcer) EnforceOverlayMountPolicy(containerID string, layerPaths []string) (err error) {
	return nil
}

func (p *OpenDoorSecurityPolicyEnforcer) EnforceCreateContainerPolicy(containerID string, argList []string, envList []string) (err error) {
	return nil
}

type ClosedDoorSecurityPolicyEnforcer struct{}

var _ SecurityPolicyEnforcer = (*ClosedDoorSecurityPolicyEnforcer)(nil)

func (p *ClosedDoorSecurityPolicyEnforcer) EnforceDeviceMountPolicy(target string, deviceHash string) (err error) {
	return errors.New("mounting is denied by policy")
}

func (p *ClosedDoorSecurityPolicyEnforcer) EnforceDeviceUnmountPolicy(target string) (err error) {
	return errors.New("unmounting is denied by policy")
}

func (p *ClosedDoorSecurityPolicyEnforcer) EnforceOverlayMountPolicy(containerID string, layerPaths []string) (err error) {
	return errors.New("creating an overlay fs is denied by policy")
}

func (p *ClosedDoorSecurityPolicyEnforcer) EnforceCreateContainerPolicy(containerID string, argList []string, envList []string) (err error) {
	return errors.New("running commands is denied by policy")
}
