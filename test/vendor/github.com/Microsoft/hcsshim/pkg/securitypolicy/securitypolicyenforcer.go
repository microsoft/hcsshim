package securitypolicy

import (
	"errors"
	"fmt"
	"sync"

	"github.com/google/go-cmp/cmp"
)

type SecurityPolicyEnforcer interface {
	EnforcePmemMountPolicy(target string, deviceHash string) (err error)
	EnforceOverlayMountPolicy(containerID string, layerPaths []string) (err error)
	EnforceCommandPolicy(containerID string, argList []string) (err error)
}

func NewSecurityPolicyEnforcer(policy *SecurityPolicy) (SecurityPolicyEnforcer, error) {
	if policy == nil {
		return nil, errors.New("security policy can't be nil")
	}

	if policy.AllowAll {
		return &OpenDoorSecurityPolicyEnforcer{}, nil
	} else {
		return NewStandardSecurityPolicyEnforcer(policy)
	}
}

type StandardSecurityPolicyEnforcer struct {
	// The user supplied security policy.
	SecurityPolicy SecurityPolicy
	// Devices and ContainerIndexToContainerIds are used to build up an
	// understanding of the containers running with a UVM as they come up and
	// map them back to a container definition from the user supplied
	// SecurityPolicy
	//
	// Devices is a listing of dm-verity root hashes seen when mounting a device
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
	// record the root hash in Devices[0][0].
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
	// the time we are doing overlay, the ContainerIndexToContainerIds in an
	// array of possible containers for a given container id.
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
	// EnforceCommandPolicy where most of the functionality is handling the case
	// were policy containers share an overlay and have to try to distinguish them
	// based on the command line arguments.
	//
	// implementation details are availanle in:
	// - EnforcePmemMountPolicy
	// - EnforceOverlayMountPolicy
	// - EnforceCommandPolicy
	// - NewStandardSecurityPolicyEnforcer
	Devices                      [][]string
	ContainerIndexToContainerIds map[int][]string
	// Set of container IDs that we've allowed to start. Because Go doesn't have
	// sets as a built-in data structure, we are using a map
	startedContainers map[string]struct{}
	// Mutex to prevent concurrent access to fields
	mutex *sync.Mutex
}

var _ SecurityPolicyEnforcer = (*StandardSecurityPolicyEnforcer)(nil)

func NewStandardSecurityPolicyEnforcer(policy *SecurityPolicy) (*StandardSecurityPolicyEnforcer, error) {
	if policy == nil {
		return nil, errors.New("security policy can't be nil")
	}

	// create new StandardSecurityPolicyEnforcer and add the new SecurityPolicy
	// to it
	// fill out corresponding devices structure by creating a "same shapped"
	// devices listing that corresponds to our container root hash lists
	// the devices list will get filled out as layers are mounted
	devices := make([][]string, len(policy.Containers))

	for i, container := range policy.Containers {
		devices[i] = make([]string, len(container.Layers))
	}

	return &StandardSecurityPolicyEnforcer{
		SecurityPolicy:               *policy,
		Devices:                      devices,
		ContainerIndexToContainerIds: map[int][]string{},
		startedContainers:            map[string]struct{}{},
		mutex:                        &sync.Mutex{},
	}, nil
}

func (policyState *StandardSecurityPolicyEnforcer) EnforcePmemMountPolicy(target string, deviceHash string) (err error) {
	policyState.mutex.Lock()
	defer policyState.mutex.Unlock()

	if len(policyState.SecurityPolicy.Containers) < 1 {
		return errors.New("policy doesn't allow mounting containers")
	}

	if deviceHash == "" {
		return errors.New("device is missing verity root hash")
	}

	found := false

	for i, container := range policyState.SecurityPolicy.Containers {
		for ii, layer := range container.Layers {
			if deviceHash == layer {
				policyState.Devices[i][ii] = target
				found = true
			}
		}
	}

	if !found {
		return fmt.Errorf("roothash %s for mount %s doesn't match policy", deviceHash, target)
	}

	return nil
}

func (policyState *StandardSecurityPolicyEnforcer) EnforceOverlayMountPolicy(containerID string, layerPaths []string) (err error) {
	policyState.mutex.Lock()
	defer policyState.mutex.Unlock()

	if len(policyState.SecurityPolicy.Containers) < 1 {
		return errors.New("policy doesn't allow mounting containers")
	}

	if _, e := policyState.startedContainers[containerID]; e {
		return errors.New("container has already been started")
	}

	// find maximum number of containers that could share this overlay
	maxPossibleContainerIdsForOverlay := 0
	for _, deviceList := range policyState.Devices {
		if equalForOverlay(layerPaths, deviceList) {
			maxPossibleContainerIdsForOverlay++
		}
	}

	if maxPossibleContainerIdsForOverlay == 0 {
		errmsg := fmt.Sprintf("layerPaths '%v' doesn't match any valid layer path: '%v'", layerPaths, policyState.Devices)
		return errors.New(errmsg)
	}

	for i, deviceList := range policyState.Devices {
		if equalForOverlay(layerPaths, deviceList) {
			existing := policyState.ContainerIndexToContainerIds[i]
			if len(existing) < maxPossibleContainerIdsForOverlay {
				policyState.ContainerIndexToContainerIds[i] = append(existing, containerID)
			} else {
				errmsg := fmt.Sprintf("layerPaths '%v' already used in maximum number of container overlays", layerPaths)
				return errors.New(errmsg)
			}
		}
	}

	return nil
}

func (policyState *StandardSecurityPolicyEnforcer) EnforceCommandPolicy(containerID string, argList []string) (err error) {
	policyState.mutex.Lock()
	defer policyState.mutex.Unlock()

	if len(policyState.SecurityPolicy.Containers) < 1 {
		return errors.New("policy doesn't allow mounting containers")
	}

	if _, e := policyState.startedContainers[containerID]; e {
		return errors.New("container has already been started")
	}

	// Get a list of all the indexes into our security policy's list of
	// containers that are possible matches for this containerID based
	// on the image overlay layout
	possibleIndexes := possibleIndexesForID(containerID, policyState.ContainerIndexToContainerIds)

	// Loop through every possible match and do two things:
	// 1- see if any command matches. we need at least one match or
	//    we don't allow the container to start
	// 2- remove this containerID as a possible match for any container from the
	//    security policy whose command line isn't a match.
	matchingCommandFound := false
	for _, possibleIndex := range possibleIndexes {
		cmd := policyState.SecurityPolicy.Containers[possibleIndex].Command
		if cmp.Equal(cmd, argList) {
			matchingCommandFound = true
		} else {
			// a possible matching index turned out not to match, so we
			// need to update that list and remove it
			updatedContainerIds := []string{}
			existingContainerIds := policyState.ContainerIndexToContainerIds[possibleIndex]
			for _, id := range existingContainerIds {
				if id != containerID {
					updatedContainerIds = append(updatedContainerIds, id)
				}
			}
			policyState.ContainerIndexToContainerIds[possibleIndex] = updatedContainerIds
		}
	}

	if !matchingCommandFound {
		errmsg := fmt.Sprintf("command %v doesn't match policy", argList)
		return errors.New(errmsg)
	}

	// record that we've allowed this container to start
	policyState.startedContainers[containerID] = struct{}{}

	return nil
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

func possibleIndexesForID(containerID string, mapping map[int][]string) []int {
	possibles := []int{}
	for index, ids := range mapping {
		for _, id := range ids {
			if containerID == id {
				possibles = append(possibles, index)
			}
		}
	}

	return possibles
}

type OpenDoorSecurityPolicyEnforcer struct{}

var _ SecurityPolicyEnforcer = (*OpenDoorSecurityPolicyEnforcer)(nil)

func (p *OpenDoorSecurityPolicyEnforcer) EnforcePmemMountPolicy(target string, deviceHash string) (err error) {
	return nil
}

func (p *OpenDoorSecurityPolicyEnforcer) EnforceOverlayMountPolicy(containerID string, layerPaths []string) (err error) {
	return nil
}

func (p *OpenDoorSecurityPolicyEnforcer) EnforceCommandPolicy(containerID string, argList []string) (err error) {
	return nil
}

type ClosedDoorSecurityPolicyEnforcer struct{}

var _ SecurityPolicyEnforcer = (*ClosedDoorSecurityPolicyEnforcer)(nil)

func (p *ClosedDoorSecurityPolicyEnforcer) EnforcePmemMountPolicy(target string, deviceHash string) (err error) {
	return errors.New("mounting is denied by policy")
}

func (p *ClosedDoorSecurityPolicyEnforcer) EnforceOverlayMountPolicy(containerID string, layerPaths []string) (err error) {
	return errors.New("creating an overlay fs is denied by policy")
}

func (p *ClosedDoorSecurityPolicyEnforcer) EnforceCommandPolicy(containerID string, argList []string) (err error) {
	return errors.New("running commands is denied by policy")
}
