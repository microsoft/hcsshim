package securitypolicy

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"testing/quick"
	"time"

	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/google/go-cmp/cmp"
)

const (
	// variables that influence generated test fixtures
	minStringLength                           = 10
	maxContainersInGeneratedConstraints       = 32
	maxLayersInGeneratedContainer             = 32
	maxGeneratedContainerID                   = 1000000
	maxGeneratedCommandLength                 = 128
	maxGeneratedCommandArgs                   = 12
	maxGeneratedEnvironmentVariables          = 16
	maxGeneratedEnvironmentVariableRuleLength = 64
	maxGeneratedEnvironmentVariableRules      = 8
	maxGeneratedFragmentNamespaceLength       = 32
	maxGeneratedMountTargetLength             = 256
	maxGeneratedVersion                       = 10
	rootHashLength                            = 64
	maxGeneratedMounts                        = 4
	maxGeneratedMountSourceLength             = 32
	maxGeneratedMountDestinationLength        = 32
	maxGeneratedMountOptions                  = 5
	maxGeneratedMountOptionLength             = 32
	maxGeneratedExecProcesses                 = 4
	maxGeneratedWorkingDirLength              = 128
	maxSignalNumber                           = 64
	maxGeneratedNameLength                    = 8
	maxGeneratedGroupNames                    = 4
	maxGeneratedCapabilities                  = 12
	maxGeneratedCapabilitesLength             = 24
	// additional consts
	// the standard enforcer tests don't do anything with the encoded policy
	// string. this const exists to make that explicit
	ignoredEncodedPolicyString = ""
)

var testRand *rand.Rand
var testDataGenerator *dataGenerator

func init() {
	seed := time.Now().Unix()
	if seedStr, ok := os.LookupEnv("SEED"); ok {
		if parsedSeed, err := strconv.ParseInt(seedStr, 10, 64); err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse seed: %d\n", seed)
		} else {
			seed = parsedSeed
		}
	}
	testRand = rand.New(rand.NewSource(seed))
	fmt.Fprintf(os.Stdout, "securitypolicy_test seed: %d\n", seed)
	testDataGenerator = newDataGenerator(testRand)
}

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

//
// Setup and "fixtures" follow...
//

func (*SecurityPolicy) Generate(r *rand.Rand, _ int) reflect.Value {
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
	numContainers := int(atLeastOneAtMost(r, maxContainersInGeneratedConstraints))
	for i := 0; i < numContainers; i++ {
		c := Container{
			Command: CommandArgs{
				Elements: map[string]string{},
			},
			EnvRules: EnvRules{
				Elements: map[string]EnvRuleConfig{},
			},
			Layers: Layers{
				Elements: map[string]string{},
			},
		}

		// command
		numArgs := int(atLeastOneAtMost(r, maxGeneratedCommandArgs))
		for j := 0; j < numArgs; j++ {
			c.Command.Elements[strconv.Itoa(j)] = randVariableString(r, maxGeneratedCommandLength)
		}
		c.Command.Length = numArgs

		// layers
		numLayers := int(atLeastOneAtMost(r, maxLayersInGeneratedContainer))
		for j := 0; j < numLayers; j++ {
			c.Layers.Elements[strconv.Itoa(j)] = generateRootHash(r)
		}
		c.Layers.Length = numLayers

		// env variable rules
		numEnvRules := int(atMost(r, maxGeneratedEnvironmentVariableRules))
		for j := 0; j < numEnvRules; j++ {
			rule := EnvRuleConfig{
				Strategy: "string",
				Rule:     randVariableString(r, maxGeneratedEnvironmentVariableRuleLength),
				Required: false,
			}
			c.EnvRules.Elements[strconv.Itoa(j)] = rule
		}
		c.EnvRules.Length = numEnvRules

		p.Containers.Elements[strconv.Itoa(i)] = c
	}

	p.Containers.Length = numContainers

	return reflect.ValueOf(p)
}

func (*generatedConstraints) Generate(r *rand.Rand, _ int) reflect.Value {
	c := generateConstraints(r, maxContainersInGeneratedConstraints)
	return reflect.ValueOf(c)
}

type testConfig struct {
	container   *securityPolicyContainer
	layers      []string
	containerID string
	policy      *StandardSecurityPolicyEnforcer
}

func setupContainerWithOverlay(gc *generatedConstraints, valid bool) (tc *testConfig, err error) {
	sp := NewStandardSecurityPolicyEnforcer(gc.containers, ignoredEncodedPolicyString)

	containerID := testDataGenerator.uniqueContainerID()
	c := selectContainerFromContainerList(gc.containers, testRand)

	var layerPaths []string
	if valid {
		layerPaths, err = testDataGenerator.createValidOverlayForContainer(sp, c)
		if err != nil {
			return nil, fmt.Errorf("error creating valid overlay: %w", err)
		}
	} else {
		layerPaths, err = testDataGenerator.createInvalidOverlayForContainer(sp, c)
		if err != nil {
			return nil, fmt.Errorf("error creating invalid overlay: %w", err)
		}
	}

	return &testConfig{
		container:   c,
		layers:      layerPaths,
		containerID: containerID,
		policy:      sp,
	}, nil
}

func generateConstraints(r *rand.Rand, maxContainers int32) *generatedConstraints {
	var containers []*securityPolicyContainer

	numContainers := (int)(atLeastOneAtMost(r, maxContainers))
	for i := 0; i < numContainers; i++ {
		containers = append(containers, generateConstraintsContainer(r, 1, maxLayersInGeneratedContainer))
	}

	return &generatedConstraints{
		containers:                       containers,
		externalProcesses:                make([]*externalProcess, 0),
		fragments:                        make([]*fragment, 0),
		allowGetProperties:               randBool(r),
		allowDumpStacks:                  randBool(r),
		allowRuntimeLogging:              false,
		allowEnvironmentVariableDropping: false,
		allowUnencryptedScratch:          randBool(r),
		namespace:                        generateFragmentNamespace(testRand),
		svn:                              generateSVN(testRand),
		allowCapabilityDropping:          false,
		ctx:                              context.Background(),
	}
}

func generateConstraintsContainer(r *rand.Rand, minNumberOfLayers, maxNumberOfLayers int32) *securityPolicyContainer {
	c := securityPolicyContainer{}
	p := generateContainerInitProcess(r)
	c.Command = p.Command
	c.EnvRules = p.EnvRules
	c.WorkingDir = p.WorkingDir
	c.Mounts = generateMounts(r)
	numLayers := int(atLeastNAtMostM(r, minNumberOfLayers, maxNumberOfLayers))
	for i := 0; i < numLayers; i++ {
		c.Layers = append(c.Layers, generateRootHash(r))
	}
	c.ExecProcesses = generateExecProcesses(r)
	c.Signals = generateListOfSignals(r, 0, maxSignalNumber)
	c.AllowStdioAccess = randBool(r)
	c.NoNewPrivileges = randBool(r)
	c.User = generateUser(r)
	c.Capabilities = generateInternalCapabilities(r)
	c.SeccompProfileSHA256 = generateSeccomp(r)

	return &c
}

func generateSeccomp(r *rand.Rand) string {
	if randBool(r) {
		// 50% chance of no seccomp profile
		return ""
	}

	return generateRootHash(r)
}

func generateInternalCapabilities(r *rand.Rand) *capabilitiesInternal {
	return &capabilitiesInternal{
		Bounding:    generateCapabilitiesSet(r, 0),
		Effective:   generateCapabilitiesSet(r, 0),
		Inheritable: generateCapabilitiesSet(r, 0),
		Permitted:   generateCapabilitiesSet(r, 0),
		Ambient:     generateCapabilitiesSet(r, 0),
	}
}

func generateCapabilitiesSet(r *rand.Rand, minSize int32) []string {
	capabilities := make([]string, 0)

	numArgs := atLeastNAtMostM(r, minSize, maxGeneratedCapabilities)
	for i := 0; i < int(numArgs); i++ {
		capabilities = append(capabilities, generateCapability(r))
	}

	return capabilities
}

func generateCapability(r *rand.Rand) string {
	return randVariableString(r, maxGeneratedCapabilitesLength)
}

func generateContainerInitProcess(r *rand.Rand) containerInitProcess {
	return containerInitProcess{
		Command:          generateCommand(r),
		EnvRules:         generateEnvironmentVariableRules(r),
		WorkingDir:       generateWorkingDir(r),
		AllowStdioAccess: randBool(r),
	}
}

func generateContainerExecProcess(r *rand.Rand) containerExecProcess {
	return containerExecProcess{
		Command: generateCommand(r),
		Signals: generateListOfSignals(r, 0, maxSignalNumber),
	}
}

func generateRootHash(r *rand.Rand) string {
	return randString(r, rootHashLength)
}

func generateWorkingDir(r *rand.Rand) string {
	return randVariableString(r, maxGeneratedWorkingDirLength)
}

func generateCommand(r *rand.Rand) []string {
	var args []string

	numArgs := atLeastOneAtMost(r, maxGeneratedCommandArgs)
	for i := 0; i < int(numArgs); i++ {
		args = append(args, randVariableString(r, maxGeneratedCommandLength))
	}

	return args
}

func generateEnvironmentVariableRules(r *rand.Rand) []EnvRuleConfig {
	var rules []EnvRuleConfig

	numArgs := atLeastOneAtMost(r, maxGeneratedEnvironmentVariableRules)
	for i := 0; i < int(numArgs); i++ {
		rule := EnvRuleConfig{
			Strategy: "string",
			Rule:     randVariableString(r, maxGeneratedEnvironmentVariableRuleLength),
		}
		rules = append(rules, rule)
	}

	return rules
}

func generateExecProcesses(r *rand.Rand) []containerExecProcess {
	var processes []containerExecProcess

	numProcesses := atLeastOneAtMost(r, maxGeneratedExecProcesses)
	for i := 0; i < int(numProcesses); i++ {
		processes = append(processes, generateContainerExecProcess(r))
	}

	return processes
}

func generateUmask(r *rand.Rand) string {
	// we are generating values from 000 to 777 as decimal values
	// to ensure we cover the full range of umask values
	// and so the resulting string will be a 4 digit octal representation
	// even though we are using decimal values
	return fmt.Sprintf("%04d", randMinMax(r, 0, 777))
}

func generateIDNameConfig(r *rand.Rand) IDNameConfig {
	strategies := []IDNameStrategy{IDNameStrategyName, IDNameStrategyID}
	strategy := strategies[randMinMax(r, 0, int32(len(strategies)-1))]
	switch strategy {
	case IDNameStrategyName:
		return IDNameConfig{
			Strategy: IDNameStrategyName,
			Rule:     randVariableString(r, maxGeneratedNameLength),
		}

	case IDNameStrategyID:
		return IDNameConfig{
			Strategy: IDNameStrategyID,
			Rule:     fmt.Sprintf("%d", r.Uint32()),
		}
	}
	panic("unreachable")
}

func generateUser(r *rand.Rand) UserConfig {
	numGroups := int(atLeastOneAtMost(r, maxGeneratedGroupNames))
	groupIDs := make([]IDNameConfig, numGroups)
	for i := 0; i < numGroups; i++ {
		groupIDs[i] = generateIDNameConfig(r)
	}

	return UserConfig{
		UserIDName:   generateIDNameConfig(r),
		GroupIDNames: groupIDs,
		Umask:        generateUmask(r),
	}
}

func generateEnvironmentVariables(r *rand.Rand) []string {
	var envVars []string

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

func buildEnvironmentVariablesFromEnvRules(rules []EnvRuleConfig, r *rand.Rand) []string {
	vars := make([]string, 0)

	// Select some number of the valid, matching rules to be environment
	// variable
	numberOfRules := int32(len(rules))
	numberOfMatches := randMinMax(r, 1, numberOfRules)

	// Build in all required rules, this isn't a setup method of "missing item"
	// tests
	for _, rule := range rules {
		if rule.Required {
			vars = append(vars, rule.Rule)
			numberOfMatches--
		}
	}

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

		vars = append(vars, rules[anIndex].Rule)
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

func generateFragmentNamespace(r *rand.Rand) string {
	return randChar(r) + randVariableString(r, maxGeneratedFragmentNamespaceLength)
}

func generateSVN(r *rand.Rand) string {
	return strconv.FormatInt(int64(randMinMax(r, 0, maxGeneratedVersion)), 10)
}

func selectRootHashFromConstraints(constraints *generatedConstraints, r *rand.Rand) string {
	numberOfContainersInConstraints := len(constraints.containers)
	container := constraints.containers[r.Intn(numberOfContainersInConstraints)]
	numberOfLayersInContainer := len(container.Layers)

	return container.Layers[r.Intn(numberOfLayersInContainer)]
}

func generateContainerID(r *rand.Rand) string {
	id := atLeastOneAtMost(r, maxGeneratedContainerID)
	return strconv.FormatInt(int64(id), 10)
}

func generateMounts(r *rand.Rand) []mountInternal {
	numberOfMounts := atLeastOneAtMost(r, maxGeneratedMounts)
	mounts := make([]mountInternal, numberOfMounts)

	for i := 0; i < int(numberOfMounts); i++ {
		numberOfOptions := atLeastOneAtMost(r, maxGeneratedMountOptions)
		options := make([]string, numberOfOptions)
		for j := 0; j < int(numberOfOptions); j++ {
			options[j] = randVariableString(r, maxGeneratedMountOptionLength)
		}

		sourcePrefix := ""
		// select a "source type". our default is "no special prefix" ie a
		// "standard source".
		prefixType := randMinMax(r, 1, 3)
		if prefixType == 2 {
			// sandbox mount, gets special handling
			sourcePrefix = guestpath.SandboxMountPrefix
		} else if prefixType == 3 {
			// huge page mount, gets special handling
			sourcePrefix = guestpath.HugePagesMountPrefix
		}

		source := filepath.Join(sourcePrefix, randVariableString(r, maxGeneratedMountSourceLength))
		destination := randVariableString(r, maxGeneratedMountDestinationLength)

		mounts[i] = mountInternal{
			Source:      source,
			Destination: destination,
			Options:     options,
			Type:        "bind",
		}
	}

	return mounts
}

func generateListOfSignals(r *rand.Rand, atLeast int32, atMost int32) []syscall.Signal {
	numSignals := int(atLeastNAtMostM(r, atLeast, atMost))
	signalSet := make(map[syscall.Signal]struct{})

	for i := 0; i < numSignals; i++ {
		signal := generateSignal(r)
		signalSet[signal] = struct{}{}
	}

	var signals []syscall.Signal
	for k := range signalSet {
		signals = append(signals, k)
	}

	return signals
}

func generateSignal(r *rand.Rand) syscall.Signal {
	return syscall.Signal(atLeastOneAtMost(r, maxSignalNumber))
}

func selectContainerFromContainerList(containers []*securityPolicyContainer, r *rand.Rand) *securityPolicyContainer {
	return containers[r.Intn(len(containers))]
}

type dataGenerator struct {
	rng                *rand.Rand
	mountTargets       stringSet
	containerIDs       stringSet
	sandboxIDs         stringSet
	enforcementPoints  stringSet
	fragmentIssuers    stringSet
	fragmentFeeds      stringSet
	fragmentNamespaces stringSet
}

func newDataGenerator(rng *rand.Rand) *dataGenerator {
	return &dataGenerator{
		rng:                rng,
		mountTargets:       make(stringSet),
		containerIDs:       make(stringSet),
		sandboxIDs:         make(stringSet),
		enforcementPoints:  make(stringSet),
		fragmentIssuers:    make(stringSet),
		fragmentFeeds:      make(stringSet),
		fragmentNamespaces: make(stringSet),
	}
}

func (s *stringSet) randUnique(r *rand.Rand, generator func(*rand.Rand) string) string {
	for {
		item := generator(r)
		if !s.contains(item) {
			s.add(item)
			return item
		}
	}
}

func (gen *dataGenerator) uniqueMountTarget() string {
	return gen.mountTargets.randUnique(gen.rng, generateMountTarget)
}

func (gen *dataGenerator) uniqueContainerID() string {
	return gen.containerIDs.randUnique(gen.rng, generateContainerID)
}

func (gen *dataGenerator) createValidOverlayForContainer(enforcer SecurityPolicyEnforcer, container *securityPolicyContainer) ([]string, error) {
	ctx := context.Background()
	// storage for our mount paths
	overlay := make([]string, len(container.Layers))

	for i := 0; i < len(container.Layers); i++ {
		mount := gen.uniqueMountTarget()
		err := enforcer.EnforceDeviceMountPolicy(ctx, mount, container.Layers[i])
		if err != nil {
			return overlay, err
		}

		overlay[len(overlay)-i-1] = mount
	}

	return overlay, nil
}

func (gen *dataGenerator) createInvalidOverlayForContainer(enforcer SecurityPolicyEnforcer, container *securityPolicyContainer) ([]string, error) {
	method := gen.rng.Intn(3)
	if method == 0 {
		return gen.invalidOverlaySameSizeWrongMounts(enforcer, container)
	} else if method == 1 {
		return gen.invalidOverlayCorrectDevicesWrongOrderSomeMissing(enforcer, container)
	} else {
		return gen.invalidOverlayRandomJunk(enforcer, container)
	}
}

func (gen *dataGenerator) invalidOverlaySameSizeWrongMounts(enforcer SecurityPolicyEnforcer, container *securityPolicyContainer) ([]string, error) {
	ctx := context.Background()
	// storage for our mount paths
	overlay := make([]string, len(container.Layers))

	for i := 0; i < len(container.Layers); i++ {
		mount := gen.uniqueMountTarget()
		err := enforcer.EnforceDeviceMountPolicy(ctx, mount, container.Layers[i])
		if err != nil {
			return overlay, err
		}

		// generate a random new mount point to cause an error
		overlay[len(overlay)-i-1] = gen.uniqueMountTarget()
	}

	return overlay, nil
}

func (gen *dataGenerator) invalidOverlayCorrectDevicesWrongOrderSomeMissing(enforcer SecurityPolicyEnforcer, container *securityPolicyContainer) ([]string, error) {
	ctx := context.Background()
	if len(container.Layers) == 1 {
		// won't work with only 1, we need to bail out to another method
		return gen.invalidOverlayRandomJunk(enforcer, container)
	}
	// storage for our mount paths
	var overlay []string

	for i := 0; i < len(container.Layers); i++ {
		mount := gen.uniqueMountTarget()
		err := enforcer.EnforceDeviceMountPolicy(ctx, mount, container.Layers[i])
		if err != nil {
			return overlay, err
		}

		if gen.rng.Intn(10) != 0 {
			overlay = append(overlay, mount)
		}
	}

	return overlay, nil
}

func (gen *dataGenerator) invalidOverlayRandomJunk(enforcer SecurityPolicyEnforcer, container *securityPolicyContainer) ([]string, error) {
	ctx := context.Background()
	// create "junk" for entry
	layersToCreate := gen.rng.Int31n(maxLayersInGeneratedContainer)
	overlay := make([]string, layersToCreate)

	for i := 0; i < int(layersToCreate); i++ {
		overlay[i] = gen.uniqueMountTarget()
	}

	// setup entirely different and "correct" expected mounting
	for i := 0; i < len(container.Layers); i++ {
		mount := gen.uniqueMountTarget()
		err := enforcer.EnforceDeviceMountPolicy(ctx, mount, container.Layers[i])
		if err != nil {
			return overlay, err
		}
	}

	return overlay, nil
}

func randVariableString(r *rand.Rand, maxLen int32) string {
	return randString(r, atLeastOneAtMost(r, maxLen))
}

func randString(r *rand.Rand, length int32) string {
	if length < minStringLength {
		length = minStringLength
	}
	charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	sb := strings.Builder{}
	sb.Grow(int(length))
	for i := 0; i < (int)(length); i++ {
		sb.WriteByte(charset[r.Intn(len(charset))])
	}

	return sb.String()
}

func randChar(r *rand.Rand) string {
	charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	return string(charset[r.Intn(len(charset))])
}

func randBool(r *rand.Rand) bool {
	return randMinMax(r, 0, 1) == 0
}

func randMinMax(r *rand.Rand, min int32, max int32) int32 {
	return r.Int31n(max-min+1) + min
}

func atLeastNAtMostM(r *rand.Rand, min, max int32) int32 {
	return randMinMax(r, min, max)
}

func atLeastOneAtMost(r *rand.Rand, most int32) int32 {
	return atLeastNAtMostM(r, 1, most)
}

func atMost(r *rand.Rand, most int32) int32 {
	return randMinMax(r, 0, most)
}

type generatedConstraints struct {
	containers                       []*securityPolicyContainer
	externalProcesses                []*externalProcess
	fragments                        []*fragment
	allowGetProperties               bool
	allowDumpStacks                  bool
	allowRuntimeLogging              bool
	allowEnvironmentVariableDropping bool
	allowUnencryptedScratch          bool
	namespace                        string
	svn                              string
	allowCapabilityDropping          bool
	ctx                              context.Context
}

type containerInitProcess struct {
	Command          []string
	EnvRules         []EnvRuleConfig
	WorkingDir       string
	AllowStdioAccess bool
}
