//go:build linux && rego
// +build linux,rego

package securitypolicy

import (
	"context"

	"strconv"

	"testing"
	"testing/quick"

	"github.com/google/go-cmp/cmp"
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

	if err := quick.Check(f, &quick.Config{MaxCount: 1000, Rand: testRand}); err != nil {
		t.Errorf("Test_StandardSecurityPolicyEnforcer_From_Security_Policy_Conversion failed: %v", err)
	}
}

// Verify that StandardSecurityPolicyEnforcer.EnforceDeviceMountPolicy will
// return an error when there's no matching root hash in the policy
func Test_EnforceDeviceMountPolicy_No_Matches(t *testing.T) {
	f := func(p *generatedConstraints) bool {

		policy := NewStandardSecurityPolicyEnforcer(p.containers, ignoredEncodedPolicyString)

		target := generateMountTarget(testRand)
		rootHash := generateInvalidRootHash(testRand)

		err := policy.EnforceDeviceMountPolicy(p.ctx, target, rootHash)

		// we expect an error, not getting one means something is broken
		return err != nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000, Rand: testRand}); err != nil {
		t.Errorf("Test_EnforceDeviceMountPolicy_No_Matches failed: %v", err)
	}
}

// Verify that StandardSecurityPolicyEnforcer.EnforceDeviceMountPolicy doesn't
// return an error when there's a matching root hash in the policy
func Test_EnforceDeviceMountPolicy_Matches(t *testing.T) {
	f := func(p *generatedConstraints) bool {

		policy := NewStandardSecurityPolicyEnforcer(p.containers, ignoredEncodedPolicyString)

		target := generateMountTarget(testRand)
		rootHash := selectRootHashFromConstraints(p, testRand)

		err := policy.EnforceDeviceMountPolicy(p.ctx, target, rootHash)

		// getting an error means something is broken
		return err == nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000, Rand: testRand}); err != nil {
		t.Errorf("Test_EnforceDeviceMountPolicy_No_Matches failed: %v", err)
	}
}

func Test_EnforceDeviceUmountPolicy_Removes_Device_Entries(t *testing.T) {
	f := func(p *generatedConstraints) bool {

		policy := NewStandardSecurityPolicyEnforcer(p.containers, ignoredEncodedPolicyString)
		target := generateMountTarget(testRand)
		rootHash := selectRootHashFromConstraints(p, testRand)

		err := policy.EnforceDeviceMountPolicy(p.ctx, target, rootHash)
		if err != nil {
			t.Error(err)
			return false
		}

		if v, ok := policy.Devices[target]; !ok || v != rootHash {
			t.Errorf("root hash is missing or doesn't match: actual=%q expected=%q", v, rootHash)
			return false
		}

		err = policy.EnforceDeviceUnmountPolicy(p.ctx, target)
		if err != nil {
			t.Error(err)
			return false
		}

		return cmp.Equal(policy.Devices, map[string]string{})
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000, Rand: testRand}); err != nil {
		t.Errorf("Test_EnforceDeviceUmountPolicy_Removes_Device_Entries failed: %v", err)
	}
}

// Verify that StandardSecurityPolicyEnforcer.EnforceOverlayMountPolicy will
// return an error when there's no matching overlay targets.
func Test_EnforceOverlayMountPolicy_No_Matches(t *testing.T) {
	f := func(p *generatedConstraints) bool {

		tc, err := setupContainerWithOverlay(p, false)
		if err != nil {
			t.Error(err)
			return false
		}

		err = tc.policy.EnforceOverlayMountPolicy(p.ctx, tc.containerID, tc.layers, generateMountTarget(testRand))

		// not getting an error means something is broken
		return err != nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000, Rand: testRand}); err != nil {
		t.Errorf("Test_EnforceOverlayMountPolicy_No_Matches failed: %v", err)
	}
}

// Verify that StandardSecurityPolicyEnforcer.EnforceOverlayMountPolicy doesn't
// return an error when there's a valid overlay target.
func Test_EnforceOverlayMountPolicy_Matches(t *testing.T) {
	f := func(p *generatedConstraints) bool {

		tc, err := setupContainerWithOverlay(p, true)
		if err != nil {
			t.Error(err)
			return false
		}

		err = tc.policy.EnforceOverlayMountPolicy(p.ctx, tc.containerID, tc.layers, generateMountTarget(testRand))

		// getting an error means something is broken
		return err == nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000, Rand: testRand}); err != nil {
		t.Errorf("Test_EnforceOverlayMountPolicy_Matches: %v", err)
	}
}

// Tests the specific case of trying to mount the same overlay twice using the /// same container id. This should be disallowed.
func Test_EnforceOverlayMountPolicy_Overlay_Single_Container_Twice(t *testing.T) {

	gc := generateConstraints(testRand, 1)
	tc, err := setupContainerWithOverlay(gc, true)
	if err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}

	if err := tc.policy.EnforceOverlayMountPolicy(gc.ctx, tc.containerID, tc.layers, generateMountTarget(testRand)); err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}

	if err := tc.policy.EnforceOverlayMountPolicy(gc.ctx, tc.containerID, tc.layers, generateMountTarget(testRand)); err == nil {
		t.Fatal("able to create overlay for the same container twice")
	}
}

// Test that if more than 1 instance of the same image is started, that we can
// create all the overlays that are required. So for example, if there are
// 13 instances of image X that all share the same overlay of root hashes,
// all 13 should be allowed.
func Test_EnforceOverlayMountPolicy_Multiple_Instances_Same_Container(t *testing.T) {
	ctx := context.Background()
	for containersToCreate := 2; containersToCreate <= maxContainersInGeneratedConstraints; containersToCreate++ {
		var containers []*securityPolicyContainer

		for i := 1; i <= containersToCreate; i++ {
			arg := "command " + strconv.Itoa(i)
			c := &securityPolicyContainer{
				Command: []string{arg},
				Layers:  []string{"1", "2"},
			}

			containers = append(containers, c)
		}

		sp := NewStandardSecurityPolicyEnforcer(containers, "")

		for i := 0; i < len(containers); i++ {
			layerPaths, err := testDataGenerator.createValidOverlayForContainer(sp, containers[i])
			if err != nil {
				t.Fatal("unexpected error on test setup")
			}

			id := testDataGenerator.uniqueContainerID()
			err = sp.EnforceOverlayMountPolicy(ctx, id, layerPaths, generateMountTarget(testRand))
			if err != nil {
				t.Fatalf("failed with %d containers", containersToCreate)
			}
		}
	}
}

// Verify that can't create more containers using an overlay than exists in the
// policy. For example, if there is a single instance of image Foo in the
// policy, we should be able to create a single container for that overlay
// but no more than that one.
func Test_EnforceOverlayMountPolicy_Overlay_Single_Container_Twice_With_Different_IDs(t *testing.T) {

	p := generateConstraints(testRand, 1)
	sp := NewStandardSecurityPolicyEnforcer(p.containers, ignoredEncodedPolicyString)

	var containerIDOne, containerIDTwo string

	for containerIDOne == containerIDTwo {
		containerIDOne = generateContainerID(testRand)
		containerIDTwo = generateContainerID(testRand)
	}
	container := selectContainerFromContainerList(p.containers, testRand)

	layerPaths, err := testDataGenerator.createValidOverlayForContainer(sp, container)
	if err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}

	err = sp.EnforceOverlayMountPolicy(p.ctx, containerIDOne, layerPaths, generateMountTarget(testRand))
	if err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}

	err = sp.EnforceOverlayMountPolicy(p.ctx, containerIDTwo, layerPaths, generateMountTarget(testRand))
	if err == nil {
		t.Fatal("able to reuse an overlay across containers")
	}
}

func Test_EnforceCommandPolicy_Matches(t *testing.T) {
	f := func(p *generatedConstraints) bool {

		tc, err := setupContainerWithOverlay(p, true)
		if err != nil {
			t.Error(err)
			return false
		}

		if err := tc.policy.EnforceOverlayMountPolicy(p.ctx, tc.containerID, tc.layers, generateMountTarget(testRand)); err != nil {
			t.Errorf("failed to enforce overlay mount policy: %s", err)
			return false
		}

		err = tc.policy.enforceCommandPolicy(tc.containerID, tc.container.Command)

		// getting an error means something is broken
		return err == nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000, Rand: testRand}); err != nil {
		t.Errorf("Test_EnforceCommandPolicy_Matches: %v", err)
	}
}

func Test_EnforceCommandPolicy_NoMatches(t *testing.T) {
	f := func(p *generatedConstraints) bool {

		tc, err := setupContainerWithOverlay(p, true)
		if err != nil {
			t.Error(err)
			return false
		}

		if err := tc.policy.EnforceOverlayMountPolicy(p.ctx, tc.containerID, tc.layers, generateMountTarget(testRand)); err != nil {
			t.Errorf("failed to enforce overlay mount policy: %s", err)
			return false
		}

		err = tc.policy.enforceCommandPolicy(tc.containerID, generateCommand(testRand))

		// not getting an error means something is broken
		return err != nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000, Rand: testRand}); err != nil {
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
	f := func(p *generatedConstraints) bool {

		// create two additional containers that "share everything"
		// except that they have different commands
		testContainerOne := generateConstraintsContainer(testRand, 1, 5)
		testContainerTwo := *testContainerOne
		testContainerTwo.Command = generateCommand(testRand)
		// add new containers to policy before creating enforcer
		p.containers = append(p.containers, testContainerOne, &testContainerTwo)

		policy := NewStandardSecurityPolicyEnforcer(p.containers, ignoredEncodedPolicyString)

		testContainerOneID := ""
		testContainerTwoID := ""
		indexForContainerOne := -1
		indexForContainerTwo := -1

		// mount and overlay all our containers
		for index, container := range p.containers {
			containerID := generateContainerID(testRand)

			layerPaths, err := testDataGenerator.createValidOverlayForContainer(policy, container)
			if err != nil {
				return false
			}

			err = policy.EnforceOverlayMountPolicy(p.ctx, containerID, layerPaths, generateMountTarget(testRand))
			if err != nil {
				return false
			}

			if cmp.Equal(container, testContainerOne) {
				testContainerOneID = containerID
				indexForContainerOne = index
			}
			if cmp.Equal(container, &testContainerTwo) {
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
	f := func(p *generatedConstraints) bool {

		tc, err := setupContainerWithOverlay(p, true)

		if err != nil {
			t.Error(err)
			return false
		}
		if err = tc.policy.EnforceOverlayMountPolicy(p.ctx, tc.containerID, tc.layers, generateMountTarget(testRand)); err != nil {
			t.Errorf("failed to enforce overlay mount policy: %s", err)
			return false
		}

		envVars := buildEnvironmentVariablesFromEnvRules(tc.container.EnvRules, testRand)
		err = tc.policy.enforceEnvironmentVariablePolicy(tc.containerID, envVars)

		// getting an error means something is broken
		return err == nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000, Rand: testRand}); err != nil {
		t.Errorf("Test_EnforceEnvironmentVariablePolicy_Matches: %v", err)
	}
}

func Test_EnforceEnvironmentVariablePolicy_Re2Match(t *testing.T) {

	p := generateConstraints(testRand, 1)

	container := generateConstraintsContainer(testRand, 1, 1)
	// add a rule to re2 match
	re2MatchRule := EnvRuleConfig{
		Strategy: EnvVarRuleRegex,
		Rule:     "PREFIX_.+=.+",
	}
	container.EnvRules = append(container.EnvRules, re2MatchRule)
	p.containers = append(p.containers, container)

	policy := NewStandardSecurityPolicyEnforcer(p.containers, ignoredEncodedPolicyString)

	containerID := generateContainerID(testRand)

	layerPaths, err := testDataGenerator.createValidOverlayForContainer(policy, container)
	if err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}

	err = policy.EnforceOverlayMountPolicy(p.ctx, containerID, layerPaths, generateMountTarget(testRand))
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
	f := func(p *generatedConstraints) bool {

		tc, err := setupContainerWithOverlay(p, true)

		if err != nil {
			t.Error(err)
			return false
		}
		if err = tc.policy.EnforceOverlayMountPolicy(p.ctx, tc.containerID, tc.layers, generateMountTarget(testRand)); err != nil {
			t.Errorf("failed to enforce overlay mount policy: %s", err)
			return false
		}

		envVars := generateEnvironmentVariables(testRand)
		envVars = append(envVars, generateNeverMatchingEnvironmentVariable(testRand))
		err = tc.policy.enforceEnvironmentVariablePolicy(tc.containerID, envVars)

		// not getting an error means something is broken
		return err != nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000, Rand: testRand}); err != nil {
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
	f := func(p *generatedConstraints) bool {

		// create two additional containers that "share everything"
		// except that they have different environment variables
		testContainerOne := generateConstraintsContainer(testRand, 1, 5)
		testContainerTwo := *testContainerOne
		testContainerTwo.EnvRules = generateEnvironmentVariableRules(testRand)
		// add new containers to policy before creating enforcer
		p.containers = append(p.containers, testContainerOne, &testContainerTwo)

		policy := NewStandardSecurityPolicyEnforcer(p.containers, ignoredEncodedPolicyString)

		testContainerOneID := ""
		testContainerTwoID := ""
		indexForContainerOne := -1
		indexForContainerTwo := -1

		// mount and overlay all our containers
		for index, container := range p.containers {
			containerID := generateContainerID(testRand)

			layerPaths, err := testDataGenerator.createValidOverlayForContainer(policy, container)
			if err != nil {
				t.Error(err)
				return false
			}

			err = policy.EnforceOverlayMountPolicy(p.ctx, containerID, layerPaths, generateMountTarget(testRand))
			if err != nil {
				t.Error(err)
				return false
			}

			if cmp.Equal(container, testContainerOne) {
				testContainerOneID = containerID
				indexForContainerOne = index
			}
			if cmp.Equal(container, &testContainerTwo) {
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
		envVars := buildEnvironmentVariablesFromEnvRules(testContainerOne.EnvRules, testRand)
		err := policy.enforceEnvironmentVariablePolicy(testContainerOneID, envVars)
		if err != nil {
			t.Error(err)
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

func Test_WorkingDirectoryPolicy_Matches(t *testing.T) {
	testFunc := func(gc *generatedConstraints) bool {

		tc, err := setupContainerWithOverlay(gc, true)

		if err != nil {
			t.Error(err)
			return false
		}

		if err := tc.policy.EnforceOverlayMountPolicy(gc.ctx, tc.containerID, tc.layers, generateMountTarget(testRand)); err != nil {
			t.Errorf("failed to enforce overlay mount policy: %s", err)
			return false
		}

		return tc.policy.enforceWorkingDirPolicy(tc.containerID, tc.container.WorkingDir) == nil
	}

	if err := quick.Check(testFunc, &quick.Config{MaxCount: 1000, Rand: testRand}); err != nil {
		t.Errorf("Test_WorkingDirectoryPolicy_Matches: %v", err)
	}
}

func Test_WorkingDirectoryPolicy_NoMatches(t *testing.T) {
	testFunc := func(gc *generatedConstraints) bool {

		tc, err := setupContainerWithOverlay(gc, true)

		if err != nil {
			t.Error(err)
			return false
		}

		if err := tc.policy.EnforceOverlayMountPolicy(gc.ctx, tc.containerID, tc.layers, generateMountTarget(testRand)); err != nil {
			t.Errorf("failed to enforce overlay mount policy: %s", err)
			return false
		}

		return tc.policy.enforceWorkingDirPolicy(tc.containerID, randString(testRand, 20)) != nil
	}

	if err := quick.Check(testFunc, &quick.Config{MaxCount: 1000, Rand: testRand}); err != nil {
		t.Errorf("Test_WorkingDirectoryPolicy_NoMatches: %v", err)
	}
}

// Consequent layers.
func Test_Overlay_Duplicate_Layers(t *testing.T) {
	f := func(p *generatedConstraints) bool {

		c1 := generateConstraintsContainer(testRand, 5, 5)
		numLayers := len(c1.Layers)
		// make sure first container has two identical layers
		c1.Layers[numLayers-3] = c1.Layers[numLayers-2]

		policy := NewStandardSecurityPolicyEnforcer([]*securityPolicyContainer{c1}, ignoredEncodedPolicyString)

		// generate mount targets
		mountTargets := make([]string, numLayers)
		for i := 0; i < numLayers; i++ {
			mountTargets[i] = randString(testRand, maxGeneratedMountTargetLength)
		}

		// call into mount enforcement
		for i := 0; i < numLayers; i++ {
			if err := policy.EnforceDeviceMountPolicy(p.ctx, mountTargets[i], c1.Layers[i]); err != nil {
				t.Errorf("failed to enforce device mount policy: %s", err)
				return false
			}
		}

		if len(policy.Devices) != numLayers {
			t.Errorf("the number of mounted devices %v don't match the expectation: targets=%v layers=%v",
				policy.Devices, mountTargets, c1.Layers)
			return false
		}

		overlay := make([]string, numLayers)
		for i := 0; i < numLayers; i++ {
			overlay[i] = mountTargets[numLayers-i-1]
		}
		containerID := randString(testRand, 32)
		if err := policy.EnforceOverlayMountPolicy(p.ctx, containerID, overlay, generateMountTarget(testRand)); err != nil {
			t.Errorf("failed to enforce overlay mount policy: %s", err)
			return false
		}

		// validate the state of the ContainerIndexToContainerIds mapping
		if containerIDs, ok := policy.ContainerIndexToContainerIds[0]; !ok {
			t.Errorf("container index to containerIDs mapping was not set: %v", containerIDs)
			return false
		} else {
			if _, ok := containerIDs[containerID]; !ok {
				t.Errorf("containerID is missing from possible containerIDs set: %v", containerIDs)
				return false
			}
		}

		for _, mountTarget := range mountTargets {
			if err := policy.EnforceDeviceUnmountPolicy(p.ctx, mountTarget); err != nil {
				t.Errorf("failed to enforce unmount policy: %s", err)
				return false
			}
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1, Rand: testRand}); err != nil {
		t.Errorf("failed to run stuff: %s", err)
	}
}

func Test_EnforceDeviceMountPolicy_DifferentTargetsWithTheSameHash(t *testing.T) {
	ctx := context.Background()
	c := generateConstraintsContainer(testRand, 2, 2)
	policy := NewStandardSecurityPolicyEnforcer([]*securityPolicyContainer{c}, ignoredEncodedPolicyString)
	mountTarget := randString(testRand, 10)
	if err := policy.EnforceDeviceMountPolicy(ctx, mountTarget, c.Layers[0]); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	// Mounting the second layer at the same mount target should fail
	if err := policy.EnforceDeviceMountPolicy(ctx, mountTarget, c.Layers[1]); err == nil {
		t.Fatal("expected conflicting device hashes error")
	}
}

func Test_EnforcePrivileged_AllowElevatedAllowsPrivilegedContainer(t *testing.T) {

	c := generateConstraints(testRand, 1)
	c.containers[0].AllowElevated = true

	tc, err := setupContainerWithOverlay(c, true)
	if err != nil {
		t.Fatalf("unexpected error during test setup: %s", err)
	}

	if err := tc.policy.EnforceOverlayMountPolicy(c.ctx, tc.containerID, tc.layers, generateMountTarget(testRand)); err != nil {
		t.Fatalf("failed to enforce overlay mount policy: %s", err)
	}

	err = tc.policy.enforcePrivilegedPolicy(tc.containerID, true)
	if err != nil {
		t.Fatalf("expected privilege escalation to be allowed: %s", err)
	}
}

func Test_EnforcePrivileged_AllowElevatedAllowsUnprivilegedContainer(t *testing.T) {

	c := generateConstraints(testRand, 1)
	c.containers[0].AllowElevated = true

	tc, err := setupContainerWithOverlay(c, true)
	if err != nil {
		t.Fatalf("unexpected error during test setup: %s", err)
	}

	if err := tc.policy.EnforceOverlayMountPolicy(c.ctx, tc.containerID, tc.layers, generateMountTarget(testRand)); err != nil {
		t.Fatalf("failed to enforce overlay mount policy: %s", err)
	}

	err = tc.policy.enforcePrivilegedPolicy(tc.containerID, true)
	if err != nil {
		t.Fatalf("expected lack of escalation to be fine: %s", err)
	}
}

func Test_EnforcePrivileged_NoAllowElevatedDenysPrivilegedContainer(t *testing.T) {

	c := generateConstraints(testRand, 1)
	c.containers[0].AllowElevated = false

	tc, err := setupContainerWithOverlay(c, true)
	if err != nil {
		t.Fatalf("unexpected error during test setup: %s", err)
	}

	if err := tc.policy.EnforceOverlayMountPolicy(c.ctx, tc.containerID, tc.layers, generateMountTarget(testRand)); err != nil {
		t.Fatalf("failed to enforce overlay mount policy: %s", err)
	}

	err = tc.policy.enforcePrivilegedPolicy(tc.containerID, true)
	if err == nil {
		t.Fatal("expected escalation to be denied")
	}
}

func Test_EnforcePrivileged_NoAllowElevatedAllowsUnprivilegedContainer(t *testing.T) {

	c := generateConstraints(testRand, 1)
	c.containers[0].AllowElevated = false

	tc, err := setupContainerWithOverlay(c, true)
	if err != nil {
		t.Fatalf("unexpected error during test setup: %s", err)
	}

	if err := tc.policy.EnforceOverlayMountPolicy(c.ctx, tc.containerID, tc.layers, generateMountTarget(testRand)); err != nil {
		t.Fatalf("failed to enforce overlay mount policy: %s", err)
	}

	err = tc.policy.enforcePrivilegedPolicy(tc.containerID, false)
	if err != nil {
		t.Fatalf("expected lack of escalation to be fine: %s", err)
	}
}
