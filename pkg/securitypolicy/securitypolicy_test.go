package securitypolicy

import (
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/google/go-cmp/cmp"
)

const (
	// variables that influence generated test fixtures
	maxContainersInGeneratedPolicy            = 32
	maxLayersInGeneratedContainer             = 32
	maxGeneratedContainerID                   = 1000000
	maxGeneratedCommandLength                 = 128
	maxGeneratedCommandArgs                   = 12
	maxGeneratedEnvironmentVariables          = 24
	maxGeneratedEnvironmentVariableRuleLength = 64
	maxGeneratedEnvironmentVariableRules      = 12
	maxGeneratedMountTargetLength             = 256
	rootHashLength                            = 64
	// additional consts
	// the standard enforcer tests don't do anything with the encoded policy
	// string. this const exists to make that explicit
	ignoredEncodedPolicyString = ""
)

// Validate that our conversion from the external SecurityPolicy representation
// to our internal format is done correctly.
func Test_StandardSecurityPolicyEnforcer_From_Security_Policy_Conversion(t *testing.T) {
	f := func(p *SecurityPolicy) bool {
		containers, err := p.Containers.toInternal()
		if err != nil {
			t.Logf("unexpected setup error. this might mean test fixture setup has a bug: %v", err)
			return false
		}

		if len(containers) != p.Containers.Length {
			t.Errorf("number of containers doesn't match. internal: %d, external: %d", len(containers), p.Containers.Length)
			return false
		}

		// do by index comparison of containers
		for i := 0; i < len(containers); i++ {
			internal := containers[i]
			external := p.Containers.Elements[strconv.Itoa(i)]

			// verify sanity with size
			if len(internal.Command) != external.Command.Length {
				t.Errorf("number of command args doesn't match for container %d. internal: %d, external: %d", i, len(internal.Command), external.Command.Length)
			}

			if len(internal.EnvRules) != external.EnvRules.Length {
				t.Errorf("number of env rules doesn't match for container %d. internal: %d, external: %d", i, len(internal.EnvRules), external.EnvRules.Length)
			}

			if len(internal.Layers) != external.Layers.Length {
				t.Errorf("number of layers doesn't match for container %d. internal: %d, external: %d", i, len(internal.Layers), external.Layers.Length)
			}

			// do by index comparison of sub-items
			for j := 0; j < len(internal.Command); j++ {
				if internal.Command[j] != external.Command.Elements[strconv.Itoa(j)] {
					t.Errorf("command entries at index %d for for container %d don't match. internal: %s, external: %s", j, i, internal.Command[j], external.Command.Elements[strconv.Itoa(j)])
				}
			}

			for j := 0; j < len(internal.EnvRules); j++ {
				irule := internal.EnvRules[j]
				erule := external.EnvRules.Elements[strconv.Itoa(j)]
				if (irule.Strategy != erule.Strategy) ||
					(irule.Rule != erule.Rule) {
					t.Errorf("env rule entries at index %d for for container %d don't match. internal: %v, external: %v", j, i, irule, erule)
				}
			}

			for j := 0; j < len(internal.Layers); j++ {
				if internal.Layers[j] != external.Layers.Elements[strconv.Itoa(j)] {
					t.Errorf("layer entries at index %d for for container %d don't match. internal: %s, external: %s", j, i, internal.Layers[j], external.Layers.Elements[strconv.Itoa(j)])
				}
			}
		}

		return !t.Failed()
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000}); err != nil {
		t.Errorf("Test_StandardSecurityPolicyEnforcer_From_Security_Policy_Conversion failed: %v", err)
	}
}

// Do we correctly set up the data structures that are part of creating a new
// StandardSecurityPolicyEnforcer
func Test_StandardSecurityPolicyEnforcer_Devices_Initialization(t *testing.T) {
	f := func(p *generatedContainers) bool {
		policy := NewStandardSecurityPolicyEnforcer(p.containers, ignoredEncodedPolicyString)

		// there should be a device entry for each container
		if len(p.containers) != len(policy.Devices) {
			return false
		}

		// in each device entry that corresponds to a container,
		// the array should have space for all the root hashes
		for i := 0; i < len(p.containers); i++ {
			if len(p.containers[i].Layers) != len(policy.Devices[i]) {
				return false
			}
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000}); err != nil {
		t.Errorf("Test_StandardSecurityPolicyEnforcer_Devices_Initialization failed: %v", err)
	}
}

// Verify that StandardSecurityPolicyEnforcer.EnforceDeviceMountPolicy will
// return an error when there's no matching root hash in the policy
func Test_EnforceDeviceMountPolicy_No_Matches(t *testing.T) {
	f := func(p *generatedContainers) bool {
		policy := NewStandardSecurityPolicyEnforcer(p.containers, ignoredEncodedPolicyString)

		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		target := generateMountTarget(r)
		rootHash := generateInvalidRootHash(r)

		err := policy.EnforceDeviceMountPolicy(target, rootHash)

		// we expect an error, not getting one means something is broken
		return err != nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000}); err != nil {
		t.Errorf("Test_EnforceDeviceMountPolicy_No_Matches failed: %v", err)
	}
}

// Verify that StandardSecurityPolicyEnforcer.EnforceDeviceMountPolicy doesn't
// return an error when there's a matching root hash in the policy
func Test_EnforceDeviceMountPolicy_Matches(t *testing.T) {
	f := func(p *generatedContainers) bool {

		policy := NewStandardSecurityPolicyEnforcer(p.containers, ignoredEncodedPolicyString)

		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		target := generateMountTarget(r)
		rootHash := selectRootHashFromContainers(p, r)

		err := policy.EnforceDeviceMountPolicy(target, rootHash)

		// getting an error means something is broken
		return err == nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000}); err != nil {
		t.Errorf("Test_EnforceDeviceMountPolicy_No_Matches failed: %v", err)
	}
}

func Test_EnforceDeviceUmountPolicy_Removes_Device_Entries(t *testing.T) {
	f := func(p *generatedContainers) bool {
		policy := NewStandardSecurityPolicyEnforcer(p.containers, ignoredEncodedPolicyString)

		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		target := generateMountTarget(r)
		rootHash := selectRootHashFromContainers(p, r)

		err := policy.EnforceDeviceMountPolicy(target, rootHash)
		if err != nil {
			return false
		}

		// we set up an expected new data structure shape were
		// the target has been removed, but everything else is
		// the same
		setupCorrectlyDone := false
		expectedDevices := make([][]string, len(policy.Devices))
		for i, container := range policy.Devices {
			expectedDevices[i] = make([]string, len(container))
			for j, storedTarget := range container {
				if target == storedTarget {
					setupCorrectlyDone = true
				} else {
					expectedDevices[i][j] = storedTarget
				}
			}
		}
		if !setupCorrectlyDone {
			// somehow, setup failed. this should never happen without another test
			// also failing
			return false
		}

		err = policy.EnforceDeviceUnmountPolicy(target)
		if err != nil {
			return false
		}

		return cmp.Equal(policy.Devices, expectedDevices)
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000}); err != nil {
		t.Errorf("Test_EnforceDeviceUmountPolicy_Removes_Device_Entries failed: %v", err)
	}
}

// Verify that StandardSecurityPolicyEnforcer.EnforceOverlayMountPolicy will
// return an error when there's no matching overlay targets.
func Test_EnforceOverlayMountPolicy_No_Matches(t *testing.T) {
	f := func(p *generatedContainers) bool {

		policy := NewStandardSecurityPolicyEnforcer(p.containers, ignoredEncodedPolicyString)

		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		containerID := generateContainerID(r)
		container := selectContainerFromContainers(p, r)

		layerPaths, err := createInvalidOverlayForContainer(policy, container, r)
		if err != nil {
			return false
		}

		err = policy.EnforceOverlayMountPolicy(containerID, layerPaths)

		// not getting an error means something is broken
		return err != nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000}); err != nil {
		t.Errorf("Test_EnforceOverlayMountPolicy_No_Matches failed: %v", err)
	}
}

// Verify that StandardSecurityPolicyEnforcer.EnforceOverlayMountPolicy doesn't
// return an error when there's a valid overlay target.
func Test_EnforceOverlayMountPolicy_Matches(t *testing.T) {
	f := func(p *generatedContainers) bool {

		policy := NewStandardSecurityPolicyEnforcer(p.containers, ignoredEncodedPolicyString)

		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		containerID := generateContainerID(r)
		container := selectContainerFromContainers(p, r)

		layerPaths, err := createValidOverlayForContainer(policy, container, r)
		if err != nil {
			return false
		}

		err = policy.EnforceOverlayMountPolicy(containerID, layerPaths)

		// getting an error means something is broken
		return err == nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000}); err != nil {
		t.Errorf("Test_EnforceOverlayMountPolicy_Matches: %v", err)
	}
}

// Tests the specific case of trying to mount the same overlay twice using the /// same container id. This should be disallowed.
func Test_EnforceOverlayMountPolicy_Overlay_Single_Container_Twice(t *testing.T) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	p := generateContainers(r, 1)

	policy := NewStandardSecurityPolicyEnforcer(p.containers, ignoredEncodedPolicyString)

	containerID := generateContainerID(r)
	container := selectContainerFromContainers(p, r)

	layerPaths, err := createValidOverlayForContainer(policy, container, r)
	if err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}

	err = policy.EnforceOverlayMountPolicy(containerID, layerPaths)
	if err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}

	err = policy.EnforceOverlayMountPolicy(containerID, layerPaths)
	if err == nil {
		t.Fatalf("able to create overlay for the same container twice")
	}
}

// Test that if more than 1 instance of the same image is started, that we can
// create all the overlays that are required. So for example, if there are
// 13 instances of image X that all share the same overlay of root hashes,
// all 13 should be allowed.
func Test_EnforceOverlayMountPolicy_Multiple_Instances_Same_Container(t *testing.T) {
	for containersToCreate := 2; containersToCreate <= maxContainersInGeneratedPolicy; containersToCreate++ {
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		var containers []securityPolicyContainer

		for i := 1; i <= int(containersToCreate); i++ {
			arg := "command " + strconv.Itoa(i)
			c := securityPolicyContainer{
				Command: []string{arg},
				Layers:  []string{"1", "2"},
			}

			containers = append(containers, c)
		}

		policy := NewStandardSecurityPolicyEnforcer(containers, "")

		idsUsed := map[string]bool{}
		for i := 0; i < len(containers); i++ {
			layerPaths, err := createValidOverlayForContainer(policy, containers[i], r)
			if err != nil {
				t.Fatal("unexpected error on test setup")
			}

			idUnique := false
			var id string
			for idUnique == false {
				id = generateContainerID(r)
				_, found := idsUsed[id]
				idUnique = !found
				idsUsed[id] = true
			}
			err = policy.EnforceOverlayMountPolicy(id, layerPaths)
			if err != nil {
				t.Fatalf("failed with %d containers", containersToCreate)
			}
		}

		t.Logf("ok for %d\n", containersToCreate)
	}
}

// Verify that can't create more containers using an overlay than exists in the
// policy. For example, if there is a single instance of image Foo in the
// policy, we should be able to create a single container for that overlay
// but no more than that one.
func Test_EnforceOverlayMountPolicy_Overlay_Single_Container_Twice_With_Different_IDs(t *testing.T) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	p := generateContainers(r, 1)

	policy := NewStandardSecurityPolicyEnforcer(p.containers, ignoredEncodedPolicyString)

	var containerIDOne, containerIDTwo string

	for containerIDOne == containerIDTwo {
		containerIDOne = generateContainerID(r)
		containerIDTwo = generateContainerID(r)
	}
	container := selectContainerFromContainers(p, r)

	layerPaths, err := createValidOverlayForContainer(policy, container, r)
	if err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}

	err = policy.EnforceOverlayMountPolicy(containerIDOne, layerPaths)
	if err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}

	err = policy.EnforceOverlayMountPolicy(containerIDTwo, layerPaths)
	if err == nil {
		t.Fatalf("able to reuse an overlay across containers")
	}
}

func Test_EnforceCommandPolicy_Matches(t *testing.T) {
	f := func(p *generatedContainers) bool {
		policy := NewStandardSecurityPolicyEnforcer(p.containers, ignoredEncodedPolicyString)

		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		containerID := generateContainerID(r)
		container := selectContainerFromContainers(p, r)

		layerPaths, err := createValidOverlayForContainer(policy, container, r)
		if err != nil {
			return false
		}

		err = policy.EnforceOverlayMountPolicy(containerID, layerPaths)
		if err != nil {
			return false
		}

		err = policy.enforceCommandPolicy(containerID, container.Command)

		// getting an error means something is broken
		return err == nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000}); err != nil {
		t.Errorf("Test_EnforceCommandPolicy_Matches: %v", err)
	}
}

func Test_EnforceCommandPolicy_NoMatches(t *testing.T) {
	f := func(p *generatedContainers) bool {
		policy := NewStandardSecurityPolicyEnforcer(p.containers, ignoredEncodedPolicyString)

		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		containerID := generateContainerID(r)
		container := selectContainerFromContainers(p, r)

		layerPaths, err := createValidOverlayForContainer(policy, container, r)
		if err != nil {
			return false
		}

		err = policy.EnforceOverlayMountPolicy(containerID, layerPaths)
		if err != nil {
			return false
		}

		err = policy.enforceCommandPolicy(containerID, generateCommand(r))

		// not getting an error means something is broken
		return err != nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000}); err != nil {
		t.Errorf("Test_EnforceCommandPolicy_NoMatches: %v", err)
	}
}

// This is a tricky test.
// The key to understanding it is, that when we have multiple containers
// with the same base aka same mounts and overlay, then we don't know at the
// time of overlay which container from policy is a given container id refers
// to. Instead we have a list of possible container ids for the so far matching
// containers in policy. We can narrow down the list of possible containers
// at the time that we enforce commands.
//
// This test verifies the "narrowing possible container ids that could be
// the container in our policy" functionality works correctly.
func Test_EnforceCommandPolicy_NarrowingMatches(t *testing.T) {
	f := func(p *generatedContainers) bool {
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		// create two additional containers that "share everything"
		// except that they have different commands
		testContainerOne := generateContainersContainer(r, 5)
		testContainerTwo := testContainerOne
		testContainerTwo.Command = generateCommand(r)
		// add new containers to policy before creating enforcer
		p.containers = append(p.containers, testContainerOne, testContainerTwo)

		policy := NewStandardSecurityPolicyEnforcer(p.containers, ignoredEncodedPolicyString)

		testContainerOneID := ""
		testContainerTwoID := ""
		indexForContainerOne := -1
		indexForContainerTwo := -1

		// mount and overlay all our containers
		for index, container := range p.containers {
			containerID := generateContainerID(r)

			layerPaths, err := createValidOverlayForContainer(policy, container, r)
			if err != nil {
				return false
			}

			err = policy.EnforceOverlayMountPolicy(containerID, layerPaths)
			if err != nil {
				return false
			}

			if cmp.Equal(container, testContainerOne) {
				testContainerOneID = containerID
				indexForContainerOne = index
			}
			if cmp.Equal(container, testContainerTwo) {
				testContainerTwoID = containerID
				indexForContainerTwo = index
			}
		}

		// validate our expectations prior to enforcing command policy
		containerOneMapping := policy.ContainerIndexToContainerIds[indexForContainerOne]
		if len(containerOneMapping) != 2 {
			return false
		}
		for id := range containerOneMapping {
			if (id != testContainerOneID) && (id != testContainerTwoID) {
				return false
			}
		}

		containerTwoMapping := policy.ContainerIndexToContainerIds[indexForContainerTwo]
		if len(containerTwoMapping) != 2 {
			return false
		}
		for id := range containerTwoMapping {
			if (id != testContainerOneID) && (id != testContainerTwoID) {
				return false
			}
		}

		// enforce command policy for containerOne
		// this will narrow our list of possible ids down
		err := policy.enforceCommandPolicy(testContainerOneID, testContainerOne.Command)
		if err != nil {
			return false
		}

		// Ok, we have full setup and we can now verify that when we enforced
		// command policy above that it correctly narrowed down containerTwo
		updatedMapping := policy.ContainerIndexToContainerIds[indexForContainerTwo]
		if len(updatedMapping) != 1 {
			return false
		}
		for id := range updatedMapping {
			if id != testContainerTwoID {
				return false
			}
		}

		return true
	}

	// This is a more expensive test to run than others, so we run fewer times
	// for each run,
	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Test_EnforceCommandPolicy_NarrowingMatches: %v", err)
	}
}

func Test_EnforceEnvironmentVariablePolicy_Matches(t *testing.T) {
	f := func(p *generatedContainers) bool {
		policy := NewStandardSecurityPolicyEnforcer(p.containers, ignoredEncodedPolicyString)

		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		containerID := generateContainerID(r)
		container := selectContainerFromContainers(p, r)

		layerPaths, err := createValidOverlayForContainer(policy, container, r)
		if err != nil {
			return false
		}

		err = policy.EnforceOverlayMountPolicy(containerID, layerPaths)
		if err != nil {
			return false
		}

		envVars := buildEnvironmentVariablesFromContainerRules(container, r)
		err = policy.enforceEnvironmentVariablePolicy(containerID, envVars)

		// getting an error means something is broken
		return err == nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000}); err != nil {
		t.Errorf("Test_EnforceEnvironmentVariablePolicy_Matches: %v", err)
	}
}

func Test_EnforceEnvironmentVariablePolicy_Re2Match(t *testing.T) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	p := generateContainers(r, 1)

	container := generateContainersContainer(r, 1)
	// add a rule to re2 match
	re2MatchRule := EnvRule{
		Strategy: EnvVarRuleRegex,
		Rule:     "PREFIX_.+=.+"}
	container.EnvRules = append(container.EnvRules, re2MatchRule)
	p.containers = append(p.containers, container)

	policy := NewStandardSecurityPolicyEnforcer(p.containers, ignoredEncodedPolicyString)

	containerID := generateContainerID(r)

	layerPaths, err := createValidOverlayForContainer(policy, container, r)
	if err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}

	err = policy.EnforceOverlayMountPolicy(containerID, layerPaths)
	if err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}

	envVars := []string{"PREFIX_FOO=BAR"}
	err = policy.enforceEnvironmentVariablePolicy(containerID, envVars)

	// getting an error means something is broken
	if err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}
}

func Test_EnforceEnvironmentVariablePolicy_NotAllMatches(t *testing.T) {
	f := func(p *generatedContainers) bool {
		policy := NewStandardSecurityPolicyEnforcer(p.containers, ignoredEncodedPolicyString)

		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		containerID := generateContainerID(r)
		container := selectContainerFromContainers(p, r)

		layerPaths, err := createValidOverlayForContainer(policy, container, r)
		if err != nil {
			return false
		}

		err = policy.EnforceOverlayMountPolicy(containerID, layerPaths)
		if err != nil {
			return false
		}

		envVars := generateEnvironmentVariables(r)
		envVars = append(envVars, generateNeverMatchingEnvironmentVariable(r))
		err = policy.enforceEnvironmentVariablePolicy(containerID, envVars)

		// not getting an error means something is broken
		return err != nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000}); err != nil {
		t.Errorf("Test_EnforceEnvironmentVariablePolicy_NotAllMatches: %v", err)
	}
}

// This is a tricky test.
// The key to understanding it is, that when we have multiple containers
// with the same base aka same mounts and overlay, then we don't know at the
// time of overlay which container from policy is a given container id refers
// to. Instead we have a list of possible container ids for the so far matching
// containers in policy. We can narrow down the list of possible containers
// at the time that we enforce environment variables, the same as we do with
// commands.
//
// This test verifies the "narrowing possible container ids that could be
// the container in our policy" functionality works correctly.
func Test_EnforceEnvironmentVariablePolicy_NarrowingMatches(t *testing.T) {
	f := func(p *generatedContainers) bool {
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		// create two additional containers that "share everything"
		// except that they have different environment variables
		testContainerOne := generateContainersContainer(r, 5)
		testContainerTwo := testContainerOne
		testContainerTwo.EnvRules = generateEnvironmentVariableRules(r)
		// add new containers to policy before creating enforcer
		p.containers = append(p.containers, testContainerOne, testContainerTwo)

		policy := NewStandardSecurityPolicyEnforcer(p.containers, ignoredEncodedPolicyString)

		testContainerOneID := ""
		testContainerTwoID := ""
		indexForContainerOne := -1
		indexForContainerTwo := -1

		// mount and overlay all our containers
		for index, container := range p.containers {
			containerID := generateContainerID(r)

			layerPaths, err := createValidOverlayForContainer(policy, container, r)
			if err != nil {
				return false
			}

			err = policy.EnforceOverlayMountPolicy(containerID, layerPaths)
			if err != nil {
				return false
			}

			if cmp.Equal(container, testContainerOne) {
				testContainerOneID = containerID
				indexForContainerOne = index
			}
			if cmp.Equal(container, testContainerTwo) {
				testContainerTwoID = containerID
				indexForContainerTwo = index
			}
		}

		// validate our expectations prior to enforcing command policy
		containerOneMapping := policy.ContainerIndexToContainerIds[indexForContainerOne]
		if len(containerOneMapping) != 2 {
			return false
		}
		for id := range containerOneMapping {
			if (id != testContainerOneID) && (id != testContainerTwoID) {
				return false
			}
		}

		containerTwoMapping := policy.ContainerIndexToContainerIds[indexForContainerTwo]
		if len(containerTwoMapping) != 2 {
			return false
		}
		for id := range containerTwoMapping {
			if (id != testContainerOneID) && (id != testContainerTwoID) {
				return false
			}
		}

		// enforce command policy for containerOne
		// this will narrow our list of possible ids down
		envVars := buildEnvironmentVariablesFromContainerRules(testContainerOne, r)
		err := policy.enforceEnvironmentVariablePolicy(testContainerOneID, envVars)
		if err != nil {
			return false
		}

		// Ok, we have full setup and we can now verify that when we enforced
		// command policy above that it correctly narrowed down containerTwo
		updatedMapping := policy.ContainerIndexToContainerIds[indexForContainerTwo]
		if len(updatedMapping) != 1 {
			return false
		}
		for id := range updatedMapping {
			if id != testContainerTwoID {
				return false
			}
		}

		return true
	}

	// This is a more expensive test to run than others, so we run fewer times
	// for each run,
	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Test_EnforceEnvironmentVariablePolicy_NarrowingMatches: %v", err)
	}
}

//
// Setup and "fixtures" follow...
//

func (*SecurityPolicy) Generate(r *rand.Rand, size int) reflect.Value {
	// This fixture setup is used from 1 test. Given the limited scope it is
	// used from, all functionality is in this single function. That saves having
	// confusing fixture name functions where we have generate* for both internal
	// and external versions
	p := &SecurityPolicy{
		Containers: Containers{
			Elements: map[string]Container{},
		},
	}
	p.AllowAll = false
	numContainers := int(atLeastOneAtMost(r, maxContainersInGeneratedPolicy))
	for i := 0; i < numContainers; i++ {
		c := Container{
			Command: CommandArgs{
				Elements: map[string]string{},
			},
			EnvRules: EnvRules{
				Elements: map[string]EnvRule{},
			},
			Layers: Layers{
				Elements: map[string]string{},
			},
		}

		// command
		numArgs := int(atLeastOneAtMost(r, maxGeneratedCommandArgs))
		for i := 0; i < numArgs; i++ {
			c.Command.Elements[strconv.Itoa(i)] = randVariableString(r, maxGeneratedCommandLength)
		}
		c.Command.Length = numArgs

		// layers
		numLayers := int(atLeastOneAtMost(r, maxLayersInGeneratedContainer))
		for i := 0; i < numLayers; i++ {
			c.Layers.Elements[strconv.Itoa(i)] = generateRootHash(r)
		}
		c.Layers.Length = numLayers

		// env variable rules
		numEnvRules := int(atMost(r, maxGeneratedEnvironmentVariableRules))
		for i := 0; i < numEnvRules; i++ {
			rule := EnvRule{
				Strategy: "string",
				Rule:     randVariableString(r, maxGeneratedEnvironmentVariableRuleLength),
			}
			c.EnvRules.Elements[strconv.Itoa(i)] = rule
		}
		c.EnvRules.Length = numEnvRules

		p.Containers.Elements[strconv.Itoa(i)] = c
	}

	p.Containers.Length = numContainers

	return reflect.ValueOf(p)
}

func (*generatedContainers) Generate(r *rand.Rand, size int) reflect.Value {
	c := generateContainers(r, maxContainersInGeneratedPolicy)
	return reflect.ValueOf(c)
}

func generateContainers(r *rand.Rand, upTo int32) *generatedContainers {
	containers := []securityPolicyContainer{}

	numContainers := (int)(atLeastOneAtMost(r, upTo))
	for i := 0; i < numContainers; i++ {
		containers = append(containers, generateContainersContainer(r, maxLayersInGeneratedContainer))
	}

	return &generatedContainers{
		containers: containers,
	}
}

func generateContainersContainer(r *rand.Rand, size int32) securityPolicyContainer {
	c := securityPolicyContainer{}
	c.Command = generateCommand(r)
	c.EnvRules = generateEnvironmentVariableRules(r)
	layers := int(atLeastOneAtMost(r, size))
	for i := 0; i < layers; i++ {
		c.Layers = append(c.Layers, generateRootHash(r))
	}

	return c
}

func generateRootHash(r *rand.Rand) string {
	return randString(r, rootHashLength)
}

func generateCommand(r *rand.Rand) []string {
	args := []string{}

	numArgs := atLeastOneAtMost(r, maxGeneratedCommandArgs)
	for i := 0; i < int(numArgs); i++ {
		args = append(args, randVariableString(r, maxGeneratedCommandLength))
	}

	return args
}

func generateEnvironmentVariableRules(r *rand.Rand) []EnvRule {
	var rules []EnvRule

	numArgs := atLeastOneAtMost(r, maxGeneratedEnvironmentVariableRules)
	for i := 0; i < int(numArgs); i++ {
		rule := EnvRule{
			Strategy: "string",
			Rule:     randVariableString(r, maxGeneratedEnvironmentVariableRuleLength),
		}
		rules = append(rules, rule)
	}

	return rules
}

func generateEnvironmentVariables(r *rand.Rand) []string {
	envVars := []string{}

	numVars := atLeastOneAtMost(r, maxGeneratedEnvironmentVariables)
	for i := 0; i < int(numVars); i++ {
		variable := randVariableString(r, maxGeneratedEnvironmentVariableRuleLength)
		envVars = append(envVars, variable)
	}

	return envVars
}

func generateNeverMatchingEnvironmentVariable(r *rand.Rand) string {
	return randString(r, maxGeneratedEnvironmentVariableRuleLength+1)
}

func buildEnvironmentVariablesFromContainerRules(c securityPolicyContainer, r *rand.Rand) []string {
	vars := make([]string, 0)

	// Select some number of the valid, matching rules to be environment
	// variable
	numberOfRules := int32(len(c.EnvRules))
	numberOfMatches := randMinMax(r, 1, numberOfRules)
	usedIndexes := map[int]struct{}{}
	for numberOfMatches > 0 {
		anIndex := -1
		if (numberOfMatches * 2) > numberOfRules {
			// if we have a lot of matches, randomly select
			exists := true

			for exists {
				anIndex = int(randMinMax(r, 0, numberOfRules-1))
				_, exists = usedIndexes[anIndex]
			}
		} else {
			// we have a "smaller set of rules. we'll just iterate and select from
			// available
			exists := true

			for exists {
				anIndex++
				_, exists = usedIndexes[anIndex]
			}
		}

		vars = append(vars, c.EnvRules[anIndex].Rule)
		usedIndexes[anIndex] = struct{}{}

		numberOfMatches--
	}

	return vars
}

func generateMountTarget(r *rand.Rand) string {
	return randVariableString(r, maxGeneratedMountTargetLength)
}

func generateInvalidRootHash(r *rand.Rand) string {
	// Guaranteed to be an incorrect size as it maxes out in size at one less
	// than the correct length. If this ever creates a hash that passes, we
	// have a seriously weird bug
	return randVariableString(r, rootHashLength-1)
}

func selectRootHashFromContainers(containers *generatedContainers, r *rand.Rand) string {

	numberOfContainersInPolicy := len(containers.containers)
	container := containers.containers[r.Intn(numberOfContainersInPolicy)]
	numberOfLayersInContainer := len(container.Layers)

	return container.Layers[r.Intn(numberOfLayersInContainer)]
}

func generateContainerID(r *rand.Rand) string {
	id := atLeastOneAtMost(r, maxGeneratedContainerID)
	return strconv.FormatInt(int64(id), 10)
}

func selectContainerFromContainers(containers *generatedContainers, r *rand.Rand) securityPolicyContainer {
	numberOfContainersInPolicy := len(containers.containers)
	return containers.containers[r.Intn(numberOfContainersInPolicy)]
}

func createValidOverlayForContainer(enforcer SecurityPolicyEnforcer, container securityPolicyContainer, r *rand.Rand) ([]string, error) {
	// storage for our mount paths
	overlay := make([]string, len(container.Layers))

	for i := 0; i < len(container.Layers); i++ {
		mount := generateMountTarget(r)
		err := enforcer.EnforceDeviceMountPolicy(mount, container.Layers[i])
		if err != nil {
			return overlay, err
		}

		overlay[len(overlay)-i-1] = mount
	}

	return overlay, nil
}

func createInvalidOverlayForContainer(enforcer SecurityPolicyEnforcer, container securityPolicyContainer, r *rand.Rand) ([]string, error) {
	method := r.Intn(3)
	if method == 0 {
		return invalidOverlaySameSizeWrongMounts(enforcer, container, r)
	} else if method == 1 {
		return invalidOverlayCorrectDevicesWrongOrderSomeMissing(enforcer, container, r)
	} else {
		return invalidOverlayRandomJunk(enforcer, container, r)
	}
}

func invalidOverlaySameSizeWrongMounts(enforcer SecurityPolicyEnforcer, container securityPolicyContainer, r *rand.Rand) ([]string, error) {
	// storage for our mount paths
	overlay := make([]string, len(container.Layers))

	for i := 0; i < len(container.Layers); i++ {
		mount := generateMountTarget(r)
		err := enforcer.EnforceDeviceMountPolicy(mount, container.Layers[i])
		if err != nil {
			return overlay, err
		}

		// generate a random new mount point to cause an error
		overlay[len(overlay)-i-1] = generateMountTarget(r)
	}

	return overlay, nil
}

func invalidOverlayCorrectDevicesWrongOrderSomeMissing(enforcer SecurityPolicyEnforcer, container securityPolicyContainer, r *rand.Rand) ([]string, error) {
	if len(container.Layers) == 1 {
		// won't work with only 1, we need to bail out to another method
		return invalidOverlayRandomJunk(enforcer, container, r)
	}
	// storage for our mount paths
	var overlay []string

	for i := 0; i < len(container.Layers); i++ {
		mount := generateMountTarget(r)
		err := enforcer.EnforceDeviceMountPolicy(mount, container.Layers[i])
		if err != nil {
			return overlay, err
		}

		if r.Intn(10) != 0 {
			overlay = append(overlay, mount)
		}
	}

	return overlay, nil
}

func invalidOverlayRandomJunk(enforcer SecurityPolicyEnforcer, container securityPolicyContainer, r *rand.Rand) ([]string, error) {
	// create "junk" for entry
	layersToCreate := r.Int31n(maxLayersInGeneratedContainer)
	overlay := make([]string, layersToCreate)

	for i := 0; i < int(layersToCreate); i++ {
		overlay[i] = generateMountTarget(r)
	}

	// setup entirely different and "correct" expected mounting
	for i := 0; i < len(container.Layers); i++ {
		mount := generateMountTarget(r)
		err := enforcer.EnforceDeviceMountPolicy(mount, container.Layers[i])
		if err != nil {
			return overlay, err
		}
	}

	return overlay, nil
}

func randVariableString(r *rand.Rand, maxLen int32) string {
	return randString(r, atLeastOneAtMost(r, maxLen))
}

func randString(r *rand.Rand, len int32) string {
	var s strings.Builder
	for i := 0; i < (int)(len); i++ {
		s.WriteRune((rune)(0x00ff & r.Int31n(256)))
	}

	return s.String()
}

func randMinMax(r *rand.Rand, min int32, max int32) int32 {
	return r.Int31n(max-min+1) + min
}

func atLeastOneAtMost(r *rand.Rand, most int32) int32 {
	return randMinMax(r, 1, most)
}

func atMost(r *rand.Rand, most int32) int32 {
	return randMinMax(r, 0, most)
}

// a type to hold a list of generated containers
type generatedContainers struct {
	containers []securityPolicyContainer
}
