//go:build linux && rego
// +build linux,rego

package securitypolicy

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"testing/quick"

	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/open-policy-agent/opa/ast"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

const (
	// variables that influence generated rego-only test fixtures
	maxGeneratedExternalProcesses      = 12
	maxGeneratedSandboxIDLength        = 32
	maxGeneratedEnforcementPointLength = 64
	maxGeneratedPlan9Mounts            = 8
	maxPlan9MountTargetLength          = 64
	maxPlan9MountIndex                 = 16
)

// Validate we do our conversion from Json to rego correctly
func Test_MarshalRego(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		securityPolicy := p.toPolicy()
		defaultMounts := toOCIMounts(generateMounts(testRand))
		privilegedMounts := toOCIMounts(generateMounts(testRand))

		_, err := newRegoPolicy(securityPolicy.marshalRego(), defaultMounts, privilegedMounts)
		if err != nil {
			t.Errorf("unable to convert policy to rego: %v", err)
			return false
		}

		return !t.Failed()
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 4, Rand: testRand}); err != nil {
		t.Errorf("Test_MarshalRego failed: %v", err)
	}
}

// Verify that RegoSecurityPolicyEnforcer.EnforceDeviceMountPolicy will
// return an error when there's no matching root hash in the policy
func Test_Rego_EnforceDeviceMountPolicy_No_Matches(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		securityPolicy := p.toPolicy()
		policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{})
		if err != nil {
			t.Errorf("unable to convert policy to rego: %v", err)
			return false
		}

		target := testDataGenerator.uniqueMountTarget()
		rootHash := generateInvalidRootHash(testRand)

		err = policy.EnforceDeviceMountPolicy(target, rootHash)

		// we expect an error, not getting one means something is broken
		return err != nil && strings.Contains(err.Error(), rootHash) && strings.Contains(err.Error(), "deviceHash not found")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceDeviceMountPolicy_No_Matches failed: %v", err)
	}
}

// Verify that RegoSecurityPolicyEnforcer.EnforceDeviceMountPolicy doesn't
// return an error when there's a matching root hash in the policy
func Test_Rego_EnforceDeviceMountPolicy_Matches(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		securityPolicy := p.toPolicy()
		policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{})
		if err != nil {
			t.Errorf("unable to convert policy to rego: %v", err)
			return false
		}

		target := testDataGenerator.uniqueMountTarget()
		rootHash := selectRootHashFromConstraints(p, testRand)

		err = policy.EnforceDeviceMountPolicy(target, rootHash)

		// getting an error means something is broken
		return err == nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceDeviceMountPolicy_Matches failed: %v", err)
	}
}

func Test_Rego_EnforceDeviceUmountPolicy_Removes_Device_Entries(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		securityPolicy := p.toPolicy()
		policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{})
		if err != nil {
			t.Error(err)
			return false
		}

		target := testDataGenerator.uniqueMountTarget()
		rootHash := selectRootHashFromConstraints(p, testRand)

		err = policy.EnforceDeviceMountPolicy(target, rootHash)
		if err != nil {
			return false
		}

		err = policy.EnforceDeviceUnmountPolicy(target)
		if err != nil {
			return false
		}

		err = policy.EnforceDeviceMountPolicy(target, rootHash)
		if err != nil {
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceDeviceUmountPolicy_Removes_Device_Entries failed: %v", err)
	}
}

func Test_Rego_EnforceDeviceMountPolicy_Duplicate_Device_Target(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		securityPolicy := p.toPolicy()
		policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{})
		if err != nil {
			t.Errorf("unable to convert policy to rego: %v", err)
			return false
		}

		target := testDataGenerator.uniqueMountTarget()
		rootHash := selectRootHashFromConstraints(p, testRand)
		err = policy.EnforceDeviceMountPolicy(target, rootHash)
		if err != nil {
			t.Error("Valid device mount failed. It shouldn't have.")
			return false
		}

		rootHash = selectRootHashFromConstraints(p, testRand)
		err = policy.EnforceDeviceMountPolicy(target, rootHash)
		if err == nil {
			t.Error("Duplicate device mount target was allowed. It shouldn't have been.")
			return false
		}

		return strings.Contains(err.Error(), "device already mounted at path")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceDeviceMountPolicy_Duplicate_Device_Target failed: %v", err)
	}
}

// Verify that RegoSecurityPolicyEnforcer.EnforceOverlayMountPolicy will
// return an error when there's no matching overlay targets.
func Test_Rego_EnforceOverlayMountPolicy_No_Matches(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoOverlayTest(p, false)
		if err != nil {
			t.Error(err)
			return false
		}

		err = tc.policy.EnforceOverlayMountPolicy(tc.containerID, tc.layers, testDataGenerator.uniqueMountTarget())

		if err == nil {
			return false
		}

		if len(tc.layers) > 0 && !strings.Contains(err.Error(), tc.layers[0]) {
			return false
		}

		return strings.Contains(err.Error(), "no matching containers for overlay")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceOverlayMountPolicy_No_Matches failed: %v", err)
	}
}

// Verify that RegoSecurityPolicyEnforcer.EnforceOverlayMountPolicy doesn't
// return an error when there's a valid overlay target.
func Test_Rego_EnforceOverlayMountPolicy_Matches(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoOverlayTest(p, true)
		if err != nil {
			t.Error(err)
			return false
		}

		err = tc.policy.EnforceOverlayMountPolicy(tc.containerID, tc.layers, testDataGenerator.uniqueMountTarget())

		// getting an error means something is broken
		return err == nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceOverlayMountPolicy_Matches: %v", err)
	}
}

// Test that an image that contains layers that share a roothash value can be
// successfully mounted. This was a failure scenario in an earlier policy engine
// implementation.
func Test_Rego_EnforceOverlayMountPolicy_Layers_With_Same_Root_Hash(t *testing.T) {

	container := generateConstraintsContainer(testRand, 2, maxLayersInGeneratedContainer)

	// make the last two layers have the same hash value
	numLayers := len(container.Layers)
	container.Layers[numLayers-2] = container.Layers[numLayers-1]

	constraints := new(generatedConstraints)
	constraints.containers = []*securityPolicyContainer{container}
	constraints.externalProcesses = generateExternalProcesses(testRand)
	securityPolicy := constraints.toPolicy()
	policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{})
	if err != nil {
		t.Fatal("Unable to create security policy")
	}

	containerID := testDataGenerator.uniqueContainerID()

	layers, err := testDataGenerator.createValidOverlayForContainer(policy, container)
	if err != nil {
		t.Fatalf("error creating valid overlay: %v", err)
	}

	err = policy.EnforceOverlayMountPolicy(containerID, layers, testDataGenerator.uniqueMountTarget())
	if err != nil {
		t.Fatalf("Unable to create an overlay where root hashes are the same")
	}
}

// Test that can we mount overlays across containers where some layers are
// shared and on the same device. A common example of this is a base image that
// is used by many containers.
// The setup for this test is rather complicated
func Test_Rego_EnforceOverlayMountPolicy_Layers_Shared_Layers(t *testing.T) {
	containerOne := generateConstraintsContainer(testRand, 1, 2)
	containerTwo := generateConstraintsContainer(testRand, 1, 10)

	sharedLayerIndex := 0

	// Make the two containers have the same base layer
	containerTwo.Layers[sharedLayerIndex] = containerOne.Layers[sharedLayerIndex]
	constraints := new(generatedConstraints)
	constraints.containers = []*securityPolicyContainer{containerOne, containerTwo}
	constraints.externalProcesses = generateExternalProcesses(testRand)

	securityPolicy := constraints.toPolicy()
	policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{})
	if err != nil {
		t.Fatal("Unable to create security policy")
	}

	//
	// Mount our first containers overlay. This should all work.
	//
	containerID := testDataGenerator.uniqueContainerID()

	// Create overlay
	containerOneOverlay := make([]string, len(containerOne.Layers))

	sharedMount := ""
	for i := 0; i < len(containerOne.Layers); i++ {
		mount := testDataGenerator.uniqueMountTarget()
		err := policy.EnforceDeviceMountPolicy(mount, containerOne.Layers[i])
		if err != nil {
			t.Fatalf("Unexpected error mounting overlay device: %v", err)
		}
		if i == sharedLayerIndex {
			sharedMount = mount
		}

		containerOneOverlay[len(containerOneOverlay)-i-1] = mount
	}

	err = policy.EnforceOverlayMountPolicy(containerID, containerOneOverlay, testDataGenerator.uniqueMountTarget())
	if err != nil {
		t.Fatalf("Unexpected error mounting overlay: %v", err)
	}

	//
	// Mount our second contaniers overlay. This should all work.
	//
	containerID = testDataGenerator.uniqueContainerID()

	// Create overlay
	containerTwoOverlay := make([]string, len(containerTwo.Layers))

	for i := 0; i < len(containerTwo.Layers); i++ {
		var mount string
		if i != sharedLayerIndex {
			mount = testDataGenerator.uniqueMountTarget()

			err := policy.EnforceDeviceMountPolicy(mount, containerTwo.Layers[i])
			if err != nil {
				t.Fatalf("Unexpected error mounting overlay device: %v", err)
			}
		} else {
			mount = sharedMount
		}

		containerTwoOverlay[len(containerTwoOverlay)-i-1] = mount
	}

	err = policy.EnforceOverlayMountPolicy(containerID, containerTwoOverlay, testDataGenerator.uniqueMountTarget())
	if err != nil {
		t.Fatalf("Unexpected error mounting overlay: %v", err)
	}

	// A final sanity check that we really had a shared mount
	if containerOneOverlay[len(containerOneOverlay)-1] != containerTwoOverlay[len(containerTwoOverlay)-1] {
		t.Fatal("Ooops. Looks like we botched our test setup.")
	}
}

// Tests the specific case of trying to mount the same overlay twice using the
// same container id. This should be disallowed.
func Test_Rego_EnforceOverlayMountPolicy_Overlay_Single_Container_Twice(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoOverlayTest(p, true)
		if err != nil {
			t.Error(err)
			return false
		}

		if err := tc.policy.EnforceOverlayMountPolicy(tc.containerID, tc.layers, testDataGenerator.uniqueMountTarget()); err != nil {
			t.Errorf("expected nil error got: %v", err)
			return false
		}

		if err := tc.policy.EnforceOverlayMountPolicy(tc.containerID, tc.layers, testDataGenerator.uniqueMountTarget()); err == nil {
			t.Errorf("able to create overlay for the same container twice")
			return false
		} else {
			return strings.Contains(err.Error(), "overlay has already been mounted")
		}
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceOverlayMountPolicy_Overlay_Single_Container_Twice: %v", err)
	}
}

func Test_Rego_EnforceOverlayMountPolicy_Reusing_ID_Across_Overlays(t *testing.T) {
	constraints := new(generatedConstraints)
	for i := 0; i < 2; i++ {
		constraints.containers = append(constraints.containers, generateConstraintsContainer(testRand, 1, maxLayersInGeneratedContainer))
	}

	constraints.externalProcesses = generateExternalProcesses(testRand)

	securityPolicy := constraints.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	policy, err := newRegoPolicy(securityPolicy.marshalRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts))
	if err != nil {
		t.Fatal(err)
	}

	containerID := testDataGenerator.uniqueContainerID()

	// First usage should work
	layerPaths, err := testDataGenerator.createValidOverlayForContainer(policy, constraints.containers[0])
	if err != nil {
		t.Fatalf("Unexpected error creating valid overlay: %v", err)
	}

	err = policy.EnforceOverlayMountPolicy(containerID, layerPaths, testDataGenerator.uniqueMountTarget())
	if err != nil {
		t.Fatalf("Unexpected error mounting overlay filesystem: %v", err)
	}

	// Reusing container ID with another overlay should fail
	layerPaths, err = testDataGenerator.createValidOverlayForContainer(policy, constraints.containers[1])
	if err != nil {
		t.Fatalf("Unexpected error creating valid overlay: %v", err)
	}

	err = policy.EnforceOverlayMountPolicy(containerID, layerPaths, testDataGenerator.uniqueMountTarget())
	if err == nil {
		t.Fatalf("Unexpected success mounting overlay filesystem")
	}
}

// work directly on the internal containers
// Test that if more than 1 instance of the same image is started, that we can
// create all the overlays that are required. So for example, if there are
// 13 instances of image X that all share the same overlay of root hashes,
// all 13 should be allowed.
func Test_Rego_EnforceOverlayMountPolicy_Multiple_Instances_Same_Container(t *testing.T) {
	for containersToCreate := 13; containersToCreate <= maxContainersInGeneratedConstraints; containersToCreate++ {
		constraints := new(generatedConstraints)
		constraints.externalProcesses = generateExternalProcesses(testRand)

		for i := 1; i <= containersToCreate; i++ {
			arg := "command " + strconv.Itoa(i)
			c := &securityPolicyContainer{
				Command: []string{arg},
				Layers:  []string{"1", "2"},
			}

			constraints.containers = append(constraints.containers, c)
		}

		securityPolicy := constraints.toPolicy()
		policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{})
		if err != nil {
			t.Fatalf("failed create enforcer")
		}

		for i := 0; i < len(constraints.containers); i++ {
			layerPaths, err := testDataGenerator.createValidOverlayForContainer(policy, constraints.containers[i])
			if err != nil {
				t.Fatal("unexpected error on test setup")
			}

			id := testDataGenerator.uniqueContainerID()
			err = policy.EnforceOverlayMountPolicy(id, layerPaths, testDataGenerator.uniqueMountTarget())
			if err != nil {
				t.Fatalf("failed with %d containers", containersToCreate)
			}
		}
	}
}

func Test_Rego_EnforceOverlayUnmountPolicy(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoOverlayTest(p, true)
		if err != nil {
			t.Error(err)
			return false
		}

		target := testDataGenerator.uniqueMountTarget()
		err = tc.policy.EnforceOverlayMountPolicy(tc.containerID, tc.layers, target)
		if err != nil {
			t.Errorf("Failure setting up overlay for testing: %v", err)
			return false
		}

		err = tc.policy.EnforceOverlayUnmountPolicy(target)
		if err != nil {
			t.Errorf("Unexpected policy enforcement failure: %v", err)
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceOverlayUnmountPolicy: %v", err)
	}
}

func Test_Rego_EnforceOverlayUnmountPolicy_No_Matches(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoOverlayTest(p, true)
		if err != nil {
			t.Error(err)
			return false
		}

		target := testDataGenerator.uniqueMountTarget()
		err = tc.policy.EnforceOverlayMountPolicy(tc.containerID, tc.layers, target)
		if err != nil {
			t.Errorf("Failure setting up overlay for testing: %v", err)
			return false
		}

		badTarget := testDataGenerator.uniqueMountTarget()
		err = tc.policy.EnforceOverlayUnmountPolicy(badTarget)
		if err == nil {
			t.Errorf("Unexpected policy enforcement success: %v", err)
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceOverlayUnmountPolicy: %v", err)
	}
}

func Test_Rego_EnforceCommandPolicy_NoMatches(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, generateCommand(testRand), tc.envList, tc.workingDir, tc.mounts)

		if err == nil {
			return false
		}

		return strings.Contains(err.Error(), "invalid command")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_EnforceCommandPolicy_NoMatches: %v", err)
	}
}

func Test_Rego_EnforceEnvironmentVariablePolicy_Re2Match(t *testing.T) {
	testFunc := func(gc *generatedConstraints) bool {
		container := selectContainerFromConstraints(gc, testRand)
		// add a rule to re2 match
		re2MatchRule := EnvRuleConfig{
			Strategy: EnvVarRuleRegex,
			Rule:     "PREFIX_.+=.+",
		}

		container.EnvRules = append(container.EnvRules, re2MatchRule)

		tc, err := setupRegoCreateContainerTest(gc, container, false)
		if err != nil {
			t.Error(err)
			return false
		}

		envList := append(tc.envList, "PREFIX_FOO=BAR")
		err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, envList, tc.workingDir, tc.mounts)

		// getting an error means something is broken
		if err != nil {
			t.Errorf("Expected container setup to be allowed. It wasn't: %v", err)
			return false
		}

		return true
	}

	if err := quick.Check(testFunc, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceEnvironmentVariablePolicy_Re2Match: %v", err)
	}
}

func Test_Rego_EnforceEnvironmentVariablePolicy_NotAllMatches(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		envList := append(tc.envList, generateNeverMatchingEnvironmentVariable(testRand))
		err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, envList, tc.workingDir, tc.mounts)

		// not getting an error means something is broken
		if err == nil {
			return false
		}

		if !strings.Contains(err.Error(), "invalid env list") {
			t.Error("missing reason")
			return false
		}

		return strings.Contains(err.Error(), envList[0])
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceEnvironmentVariablePolicy_NotAllMatches: %v", err)
	}
}

func Test_Rego_WorkingDirectoryPolicy_NoMatches(t *testing.T) {
	testFunc := func(gc *generatedConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTest(gc)
		if err != nil {
			t.Error(err)
			return false
		}

		err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, tc.envList, randString(testRand, 20), tc.mounts)
		// not getting an error means something is broken
		if err == nil {
			return false
		}

		return strings.Contains(err.Error(), "invalid working directory")
	}

	if err := quick.Check(testFunc, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_WorkingDirectoryPolicy_NoMatches: %v", err)
	}
}

func Test_Rego_EnforceCreateContainer(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts)

		// getting an error means something is broken
		return err == nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceCreateContainer: %v", err)
	}
}

func Test_Rego_Enforce_CreateContainer_Start_All_Containers(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		securityPolicy := p.toPolicy()
		defaultMounts := generateMounts(testRand)
		privilegedMounts := generateMounts(testRand)

		policy, err := newRegoPolicy(securityPolicy.marshalRego(),
			toOCIMounts(defaultMounts),
			toOCIMounts(privilegedMounts))
		if err != nil {
			t.Error(err)
			return false
		}

		for _, container := range p.containers {
			containerID, err := mountImageForContainer(policy, container)
			if err != nil {
				t.Error(err)
				return false
			}

			envList := buildEnvironmentVariablesFromEnvRules(container.EnvRules, testRand)

			sandboxID := testDataGenerator.uniqueSandboxID()
			mounts := container.Mounts
			mounts = append(mounts, defaultMounts...)
			if container.AllowElevated {
				mounts = append(mounts, privilegedMounts...)
			}
			mountSpec := buildMountSpecFromMountArray(mounts, sandboxID, testRand)

			err = policy.EnforceCreateContainerPolicy(sandboxID, containerID, container.Command, envList, container.WorkingDir, mountSpec.Mounts)

			// getting an error means something is broken
			if err != nil {
				t.Error(err)
				return false
			}
		}

		return true

	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_Enforce_CreateContainer_Start_All_Containers: %v", err)
	}
}

func Test_Rego_EnforceCreateContainer_Invalid_ContainerID(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		containerID := testDataGenerator.uniqueContainerID()
		err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts)

		// not getting an error means something is broken
		return err != nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceCreateContainer_Invalid_ContainerID: %v", err)
	}
}

func Test_Rego_EnforceCreateContainer_Same_Container_Twice(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts)
		if err != nil {
			t.Error("Unable to start valid container.")
			return false
		}
		err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts)
		if err == nil {
			t.Error("Able to start a container with already used id.")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceCreateContainer_Same_Container_Twice: %v", err)
	}
}

func Test_Rego_ExtendDefaultMounts(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		defaultMounts := generateMounts(testRand)
		_ = tc.policy.ExtendDefaultMounts(toOCIMounts(defaultMounts))

		additionalMounts := buildMountSpecFromMountArray(defaultMounts, tc.sandboxID, testRand)
		tc.mounts = append(tc.mounts, additionalMounts.Mounts...)

		err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts)

		if err != nil {
			t.Error(err)
			return false
		} else {
			return true
		}
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_ExtendDefaultMounts: %v", err)
	}
}

func Test_Rego_MountPolicy_NoMatches(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		invalidMounts := generateMounts(testRand)
		additionalMounts := buildMountSpecFromMountArray(invalidMounts, tc.sandboxID, testRand)
		tc.mounts = append(tc.mounts, additionalMounts.Mounts...)

		err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts)

		// not getting an error means something is broken
		if err == nil {
			t.Error("We added additional mounts not in policyS and it didn't result in an error")
			return false
		}

		return strings.Contains(err.Error(), "invalid mount list")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_MountPolicy_NoMatches: %v", err)
	}
}

func Test_Rego_MountPolicy_NotAllOptionsFromConstraints(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		inputMounts := tc.mounts
		mindex := randMinMax(testRand, 0, int32(len(tc.mounts)-1))
		options := inputMounts[mindex].Options
		inputMounts[mindex].Options = options[:len(options)-1]

		err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts)

		// not getting an error means something is broken
		if err == nil {
			return false
		}

		return strings.Contains(err.Error(), "invalid mount list")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_MountPolicy_NotAllOptionsFromConstraints: %v", err)
	}
}

func Test_Rego_MountPolicy_BadSource(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		index := randMinMax(testRand, 0, int32(len(tc.mounts)-1))
		tc.mounts[index].Source = randString(testRand, maxGeneratedMountSourceLength)

		err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts)

		// not getting an error means something is broken
		if err == nil {
			return false
		}

		return strings.Contains(err.Error(), "invalid mount list")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_MountPolicy_BadSource: %v", err)
	}
}

func Test_Rego_MountPolicy_BadDestination(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		index := randMinMax(testRand, 0, int32(len(tc.mounts)-1))
		tc.mounts[index].Destination = randString(testRand, maxGeneratedMountDestinationLength)

		err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts)

		// not getting an error means something is broken
		if err == nil {
			return false
		}

		return strings.Contains(err.Error(), "invalid mount list")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_MountPolicy_BadDestination: %v", err)
	}
}

func Test_Rego_MountPolicy_BadType(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		index := randMinMax(testRand, 0, int32(len(tc.mounts)-1))
		tc.mounts[index].Type = randString(testRand, 4)

		err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts)

		// not getting an error means something is broken
		if err == nil {
			return false
		}

		return strings.Contains(err.Error(), "invalid mount list")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_MountPolicy_BadType: %v", err)
	}
}

func Test_Rego_MountPolicy_BadOption(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		mindex := randMinMax(testRand, 0, int32(len(tc.mounts)-1))
		mountToChange := tc.mounts[mindex]
		oindex := randMinMax(testRand, 0, int32(len(mountToChange.Options)-1))
		tc.mounts[mindex].Options[oindex] = randString(testRand, maxGeneratedMountOptionLength)

		err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts)

		// not getting an error means something is broken
		if err == nil {
			t.Error("We changed a mount option and it didn't result in an error")
			return false
		}

		return strings.Contains(err.Error(), "invalid mount list")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_MountPolicy_BadOption: %v", err)
	}
}

func Test_Rego_MountPolicy_MountPrivilegedWhenNotAllowed(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoPrivilegedMountTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		mindex := randMinMax(testRand, 0, int32(len(tc.mounts)-1))
		mountToChange := tc.mounts[mindex]
		oindex := randMinMax(testRand, 0, int32(len(mountToChange.Options)-1))
		tc.mounts[mindex].Options[oindex] = randString(testRand, maxGeneratedMountOptionLength)

		err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts)

		// not getting an error means something is broken
		if err == nil {
			t.Error("We tried to mount a privileged mount when not allowed and it didn't result in an error")
			return false
		}

		return strings.Contains(err.Error(), "invalid mount list")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_MountPolicy_BadOption: %v", err)
	}
}

// Tests whether an error is raised if support information is requested for
// an enforcement point which does not have stored version information.
func Test_Rego_Version_Unregistered_Enforcement_Point(t *testing.T) {
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints, maxExternalProcessesInGeneratedConstraints)
	securityPolicy := gc.toPolicy()
	policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{})
	if err != nil {
		t.Fatalf("unable to create a new Rego policy: %v", err)
	}

	enforcementPoint := testDataGenerator.uniqueEnforcementPoint()
	_, err = policy.queryEnforcementPoint(enforcementPoint)

	// we expect an error, not getting one means something is broken
	if err == nil {
		t.Fatal("an error was not thrown when asking whether an unregistered enforcement point was available")
	}
}

// Tests whether an error is raised if support information is requested for
// a version of an enforcement point which is of a later version than the
// framework. This should not happen, but may occur during development if
// version numbers have been entered incorrectly.
func Test_Rego_Version_Future_Enforcement_Point(t *testing.T) {
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints, maxExternalProcessesInGeneratedConstraints)
	securityPolicy := gc.toPolicy()
	policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{})
	if err != nil {
		t.Fatalf("unable to create a new Rego policy: %v", err)
	}

	err = policy.injectTestAPI()
	if err != nil {
		t.Fatal(err)
	}

	_, err = policy.queryEnforcementPoint("__fixture_for_future_test__")

	// we expect an error, not getting one means something is broken
	if err == nil {
		t.Fatalf("an error was not thrown when asking whether an enforcement point from the future was available")
	}

	expected_error := "enforcement point rule __fixture_for_future_test__ is invalid"
	if err.Error() != expected_error {
		t.Fatalf("error message when asking for a future enforcement point was incorrect.")
	}
}

// Tests whether the enforcement point logic returns the default behavior
// and "unsupported" when the enforcement point was added in a version of the
// framework that was released after the policy was authored as indicated
// by their respective version information.
func Test_Rego_Version_Unavailable_Enforcement_Point(t *testing.T) {
	code := "package policy\n\napi_svn := \"0.0.1\""
	policy, err := newRegoPolicy(code, []oci.Mount{}, []oci.Mount{})
	if err != nil {
		t.Fatalf("unable to create a new Rego policy: %v", err)
	}

	err = policy.injectTestAPI()
	if err != nil {
		t.Fatal(err)
	}

	info, err := policy.queryEnforcementPoint("__fixture_for_allowed_test_true__")
	// we do not expect an error, getting one means something is broken
	if err != nil {
		t.Fatalf("unable to query whether enforcement point is available: %v", err)
	}

	if info.availableByPolicyVersion {
		t.Error("unavailable enforcement incorrectly indicated as available")
	}

	if !info.allowedByDefault {
		t.Error("default behavior was incorrect for unavailable enforcement point")
	}
}

func Test_Rego_Enforcement_Point_Allowed(t *testing.T) {
	code := "package policy\n\napi_svn := \"0.0.1\""
	policy, err := newRegoPolicy(code, []oci.Mount{}, []oci.Mount{})
	if err != nil {
		t.Fatalf("unable to create a new Rego policy: %v", err)
	}

	err = policy.injectTestAPI()
	if err != nil {
		t.Fatal(err)
	}

	input := make(map[string]interface{})
	allowed, err := policy.allowed("__fixture_for_allowed_test_false__", input)
	if err != nil {
		t.Fatalf("asked whether an enforcement point was allowed and receieved an error: %v", err)
	}

	if allowed {
		t.Fatal("result of allowed for an unavailable enforcement point was not the specified default (false)")
	}

	allowed, err = policy.allowed("__fixture_for_allowed_test_true__", input)
	if err != nil {
		t.Fatalf("asked whether an enforcement point was allowed and receieved an error: %v", err)
	}

	if !allowed {
		t.Error("result of allowed for an unavailable enforcement point was not the specified default (true)")
	}
}

func Test_Rego_ExecInContainerPolicy(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoRunningContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		container := selectContainerFromRunningContainers(tc.runningContainers, testRand)
		process := selectExecProcess(container.container.ExecProcesses, testRand)
		envList := buildEnvironmentVariablesFromEnvRules(container.container.EnvRules, testRand)

		err = tc.policy.EnforceExecInContainerPolicy(container.containerID, process.Command, envList, container.container.WorkingDir)

		// getting an error means something is broken
		if err != nil {
			t.Error(err)
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_ExecInContainerPolicy: %v", err)
	}
}

func Test_Rego_ExecInContainerPolicy_No_Matches(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoRunningContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		container := selectContainerFromRunningContainers(tc.runningContainers, testRand)

		process := generateContainerExecProcess(testRand)
		envList := buildEnvironmentVariablesFromEnvRules(container.container.EnvRules, testRand)

		err = tc.policy.EnforceExecInContainerPolicy(container.containerID, process.Command, envList, container.container.WorkingDir)
		if err == nil {
			t.Error("Test unexpectedly passed")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_ExecInContainerPolicy_No_Matches: %v", err)
	}
}

func Test_Rego_ExecInContainerPolicy_Command_No_Match(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoRunningContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		container := selectContainerFromRunningContainers(tc.runningContainers, testRand)
		envList := buildEnvironmentVariablesFromEnvRules(container.container.EnvRules, testRand)

		command := generateCommand(testRand)
		err = tc.policy.EnforceExecInContainerPolicy(container.containerID, command, envList, container.container.WorkingDir)

		// not getting an error means something is broken
		if err == nil {
			t.Error("Unexpected success when enforcing policy")
			return false
		}

		return strings.Contains(err.Error(), "invalid command")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_ExecInContainerPolicy_Command_No_Match: %v", err)
	}
}

func Test_Rego_ExecInContainerPolicy_Some_Env_Not_Allowed(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoRunningContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		container := selectContainerFromRunningContainers(tc.runningContainers, testRand)
		process := selectExecProcess(container.container.ExecProcesses, testRand)

		envList := generateEnvironmentVariables(testRand)

		err = tc.policy.EnforceExecInContainerPolicy(container.containerID, process.Command, envList, container.container.WorkingDir)

		// not getting an error means something is broken
		if err == nil {
			t.Error("Unexpected success when enforcing policy")
			return false
		}

		return strings.Contains(err.Error(), "invalid env list")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_ExecInContainerPolicy_Some_Env_Not_Allowed: %v", err)
	}
}

func Test_Rego_ExecInContainerPolicy_WorkingDir_No_Match(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoRunningContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		container := selectContainerFromRunningContainers(tc.runningContainers, testRand)
		process := selectExecProcess(container.container.ExecProcesses, testRand)
		envList := buildEnvironmentVariablesFromEnvRules(container.container.EnvRules, testRand)
		workingDir := generateWorkingDir(testRand)

		err = tc.policy.EnforceExecInContainerPolicy(container.containerID, process.Command, envList, workingDir)

		// not getting an error means something is broken
		if err == nil {
			t.Error("Unexpected success when enforcing policy")
			return false
		}

		return strings.Contains(err.Error(), "invalid working directory")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_ExecInContainerPolicy_WorkingDir_No_Match: %v", err)
	}
}

func Test_Rego_EnforceEnvironmentVariablePolicy_MissingRequired(t *testing.T) {
	testFunc := func(gc *generatedConstraints) bool {
		container := selectContainerFromConstraints(gc, testRand)
		// add a rule to re2 match
		requiredRule := EnvRuleConfig{
			Strategy: "string",
			Rule:     randVariableString(testRand, maxGeneratedEnvironmentVariableRuleLength),
			Required: true,
		}

		container.EnvRules = append(container.EnvRules, requiredRule)

		tc, err := setupRegoCreateContainerTest(gc, container, false)
		if err != nil {
			t.Error(err)
			return false
		}

		envList := make([]string, 0, len(container.EnvRules))
		for _, env := range tc.envList {
			if env != requiredRule.Rule {
				envList = append(envList, env)
			}
		}

		err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, envList, tc.workingDir, tc.mounts)

		// not getting an error means something is broken
		if err == nil {
			t.Errorf("Expected container setup to fail.")
			return false
		}

		return true
	}

	if err := quick.Check(testFunc, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceEnvironmentVariablePolicy_MissingRequired: %v", err)
	}
}

func Test_Rego_ExecExternalProcessPolicy(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupExternalProcessTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		process := selectExternalProcessFromConstraints(p, testRand)
		envList := buildEnvironmentVariablesFromEnvRules(process.envRules, testRand)

		err = tc.policy.EnforceExecExternalProcessPolicy(process.command, envList, process.workingDir)
		if err != nil {
			t.Error("Policy enforcement unexpectedly was denied")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_ExecExternalProcessPolicy: %v", err)
	}
}

func Test_Rego_ExecExternalProcessPolicy_No_Matches(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupExternalProcessTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		process := generateExternalProcess(testRand)
		envList := buildEnvironmentVariablesFromEnvRules(process.envRules, testRand)

		err = tc.policy.EnforceExecExternalProcessPolicy(process.command, envList, process.workingDir)
		if err == nil {
			t.Error("Policy was unexpectedly not enforced")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_ExecExternalProcessPolicy_No_Matches: %v", err)
	}
}

func Test_Rego_ExecExternalProcessPolicy_Command_No_Match(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupExternalProcessTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		process := selectExternalProcessFromConstraints(p, testRand)
		envList := buildEnvironmentVariablesFromEnvRules(process.envRules, testRand)
		command := generateCommand(testRand)

		err = tc.policy.EnforceExecExternalProcessPolicy(command, envList, process.workingDir)
		if err == nil {
			t.Error("Policy was unexpectedly not enforced")
			return false
		}

		return strings.Contains(err.Error(), "invalid command")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_ExecExternalProcessPolicy_Command_No_Match: %v", err)
	}
}

func Test_Rego_ExecExternalProcessPolicy_Some_Env_Not_Allowed(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupExternalProcessTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		process := selectExternalProcessFromConstraints(p, testRand)
		envList := generateEnvironmentVariables(testRand)

		err = tc.policy.EnforceExecExternalProcessPolicy(process.command, envList, process.workingDir)
		if err == nil {
			t.Error("Policy was unexpectedly not enforced")
			return false
		}

		return strings.Contains(err.Error(), "invalid env list")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_ExecExternalProcessPolicy_Some_Env_Not_Allowed: %v", err)
	}
}

func Test_Rego_ExecExternalProcessPolicy_WorkingDir_No_Match(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupExternalProcessTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		process := selectExternalProcessFromConstraints(p, testRand)
		envList := buildEnvironmentVariablesFromEnvRules(process.envRules, testRand)
		workingDir := generateWorkingDir(testRand)

		err = tc.policy.EnforceExecExternalProcessPolicy(process.command, envList, workingDir)
		if err == nil {
			t.Error("Policy was unexpectedly not enforced")
			return false
		}

		return strings.Contains(err.Error(), "invalid working directory")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_ExecExternalProcessPolicy_WorkingDir_No_Match: %v", err)
	}
}

func Test_Rego_ShutdownContainerPolicy_Running_Container(t *testing.T) {
	p := generateConstraints(testRand, maxContainersInGeneratedConstraints, maxExternalProcessesInGeneratedConstraints)

	tc, err := setupRegoRunningContainerTest(p)
	if err != nil {
		t.Fatalf("Unable to set up test: %v", err)
	}

	container := selectContainerFromRunningContainers(tc.runningContainers, testRand)

	err = tc.policy.EnforceShutdownContainerPolicy(container.containerID)
	if err != nil {
		t.Fatal("Expected shutdown of running container to be allowed, it wasn't")
	}
}

func Test_Rego_ShutdownContainerPolicy_Not_Running_Container(t *testing.T) {
	p := generateConstraints(testRand, maxContainersInGeneratedConstraints, maxExternalProcessesInGeneratedConstraints)

	tc, err := setupRegoRunningContainerTest(p)
	if err != nil {
		t.Fatalf("Unable to set up test: %v", err)
	}

	notRunningContainerID := testDataGenerator.uniqueContainerID()

	err = tc.policy.EnforceShutdownContainerPolicy(notRunningContainerID)
	if err == nil {
		t.Fatal("Expected shutdown of not running container to be denied, it wasn't")
	}
}

func Test_Rego_SignalContainerProcessPolicy_InitProcess_Allowed(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		hasAllowedSignals := generateConstraintsContainer(testRand, 1, maxLayersInGeneratedContainer)
		hasAllowedSignals.Signals = generateListOfSignals(testRand, 1, maxSignalNumber)
		p.containers = append(p.containers, hasAllowedSignals)

		tc, err := setupRegoRunningContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		containerID, err := idForRunningContainer(hasAllowedSignals, tc.runningContainers)
		if err != nil {
			r, err := runContainer(tc.policy, hasAllowedSignals, tc.defaultMounts, tc.privilegedMounts)
			if err != nil {
				t.Errorf("Unable to setup test running container: %v", err)
				return false
			}
			containerID = r.containerID
		}

		signal := selectSignalFromSignals(testRand, hasAllowedSignals.Signals)
		err = tc.policy.EnforceSignalContainerProcessPolicy(containerID, signal, true, hasAllowedSignals.Command)

		if err != nil {
			t.Errorf("Signal init process unexpectedly failed: %v", err)
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_SignalContainerProcessPolicy_InitProcess_Allowed: %v", err)
	}
}

func Test_Rego_SignalContainerProcessPolicy_InitProcess_Not_Allowed(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		hasNoAllowedSignals := generateConstraintsContainer(testRand, 1, maxLayersInGeneratedContainer)
		hasNoAllowedSignals.Signals = make([]syscall.Signal, 0)

		p.containers = append(p.containers, hasNoAllowedSignals)

		tc, err := setupRegoRunningContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		containerID, err := idForRunningContainer(hasNoAllowedSignals, tc.runningContainers)
		if err != nil {
			r, err := runContainer(tc.policy, hasNoAllowedSignals, tc.defaultMounts, tc.privilegedMounts)
			if err != nil {
				t.Errorf("Unable to setup test running container: %v", err)
				return false
			}
			containerID = r.containerID
		}

		signal := generateSignal(testRand)
		err = tc.policy.EnforceSignalContainerProcessPolicy(containerID, signal, true, hasNoAllowedSignals.Command)

		if err == nil {
			t.Errorf("Signal init process unexpectedly passed: %v", err)
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_SignalContainerProcessPolicy_InitProcess_Not_Allowed: %v", err)
	}
}

func Test_Rego_SignalContainerProcessPolicy_InitProcess_Bad_ContainerID(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		hasAllowedSignals := generateConstraintsContainer(testRand, 1, maxLayersInGeneratedContainer)
		hasAllowedSignals.Signals = generateListOfSignals(testRand, 1, maxSignalNumber)
		p.containers = append(p.containers, hasAllowedSignals)

		tc, err := setupRegoRunningContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		_, err = idForRunningContainer(hasAllowedSignals, tc.runningContainers)
		if err != nil {
			_, err := runContainer(tc.policy, hasAllowedSignals, tc.defaultMounts, tc.privilegedMounts)
			if err != nil {
				t.Errorf("Unable to setup test running container: %v", err)
				return false
			}
		}

		signal := selectSignalFromSignals(testRand, hasAllowedSignals.Signals)
		badContainerID := generateContainerID(testRand)
		err = tc.policy.EnforceSignalContainerProcessPolicy(badContainerID, signal, true, hasAllowedSignals.Command)

		if err == nil {
			t.Errorf("Signal init process unexpectedly succeeded: %v", err)
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_SignalContainerProcessPolicy_InitProcess_Bad_ContainerID: %v", err)
	}
}

func Test_Rego_SignalContainerProcessPolicy_ExecProcess_Allowed(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		containerUnderTest := generateConstraintsContainer(testRand, 1, maxLayersInGeneratedContainer)

		ep := generateExecProcesses(testRand)
		ep[0].Signals = generateListOfSignals(testRand, 1, 4)
		containerUnderTest.ExecProcesses = ep
		processUnderTest := ep[0]

		p.containers = append(p.containers, containerUnderTest)

		tc, err := setupRegoRunningContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		containerID, err := idForRunningContainer(containerUnderTest, tc.runningContainers)
		if err != nil {
			r, err := runContainer(tc.policy, containerUnderTest, tc.defaultMounts, tc.privilegedMounts)
			if err != nil {
				t.Errorf("Unable to setup test running container: %v", err)
				return false
			}
			containerID = r.containerID
		}

		envList := buildEnvironmentVariablesFromEnvRules(containerUnderTest.EnvRules, testRand)

		err = tc.policy.EnforceExecInContainerPolicy(containerID, processUnderTest.Command, envList, containerUnderTest.WorkingDir)
		if err != nil {
			t.Errorf("Unable to exec process for test: %v", err)
			return false
		}

		signal := selectSignalFromSignals(testRand, processUnderTest.Signals)

		err = tc.policy.EnforceSignalContainerProcessPolicy(containerID, signal, false, processUnderTest.Command)
		if err != nil {
			t.Errorf("Signal init process unexpectedly failed: %v", err)
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_SignalContainerProcessPolicy_ExecProcess_Allowed: %v", err)
	}
}

func Test_Rego_SignalContainerProcessPolicy_ExecProcess_Not_Allowed(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		containerUnderTest := generateConstraintsContainer(testRand, 1, maxLayersInGeneratedContainer)

		ep := generateExecProcesses(testRand)
		ep[0].Signals = make([]syscall.Signal, 0)
		containerUnderTest.ExecProcesses = ep
		processUnderTest := ep[0]

		p.containers = append(p.containers, containerUnderTest)

		tc, err := setupRegoRunningContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		containerID, err := idForRunningContainer(containerUnderTest, tc.runningContainers)
		if err != nil {
			r, err := runContainer(tc.policy, containerUnderTest, tc.defaultMounts, tc.privilegedMounts)
			if err != nil {
				t.Errorf("Unable to setup test running container: %v", err)
				return false
			}
			containerID = r.containerID
		}

		envList := buildEnvironmentVariablesFromEnvRules(containerUnderTest.EnvRules, testRand)

		err = tc.policy.EnforceExecInContainerPolicy(containerID, processUnderTest.Command, envList, containerUnderTest.WorkingDir)
		if err != nil {
			t.Errorf("Unable to exec process for test: %v", err)
			return false
		}

		signal := generateSignal(testRand)

		err = tc.policy.EnforceSignalContainerProcessPolicy(containerID, signal, false, processUnderTest.Command)
		if err == nil {
			t.Errorf("Signal init process unexpectedly succeeded: %v", err)
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_SignalContainerProcessPolicy_ExecProcess_Not_Allowed: %v", err)
	}
}

func Test_Rego_SignalContainerProcessPolicy_ExecProcess_Bad_Command(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		containerUnderTest := generateConstraintsContainer(testRand, 1, maxLayersInGeneratedContainer)

		ep := generateExecProcesses(testRand)
		ep[0].Signals = generateListOfSignals(testRand, 1, 4)
		containerUnderTest.ExecProcesses = ep
		processUnderTest := ep[0]

		p.containers = append(p.containers, containerUnderTest)

		tc, err := setupRegoRunningContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		containerID, err := idForRunningContainer(containerUnderTest, tc.runningContainers)
		if err != nil {
			r, err := runContainer(tc.policy, containerUnderTest, tc.defaultMounts, tc.privilegedMounts)
			if err != nil {
				t.Errorf("Unable to setup test running container: %v", err)
				return false
			}
			containerID = r.containerID
		}

		envList := buildEnvironmentVariablesFromEnvRules(containerUnderTest.EnvRules, testRand)

		err = tc.policy.EnforceExecInContainerPolicy(containerID, processUnderTest.Command, envList, containerUnderTest.WorkingDir)
		if err != nil {
			t.Errorf("Unable to exec process for test: %v", err)
			return false
		}

		signal := selectSignalFromSignals(testRand, processUnderTest.Signals)
		badCommand := generateCommand(testRand)

		err = tc.policy.EnforceSignalContainerProcessPolicy(containerID, signal, false, badCommand)
		if err == nil {
			t.Errorf("Signal init process unexpectedly succeeded: %v", err)
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_SignalContainerProcessPolicy_ExecProcess_Bad_Command: %v", err)
	}
}

func Test_Rego_SignalContainerProcessPolicy_ExecProcess_Bad_ContainerID(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		containerUnderTest := generateConstraintsContainer(testRand, 1, maxLayersInGeneratedContainer)

		ep := generateExecProcesses(testRand)
		ep[0].Signals = generateListOfSignals(testRand, 1, 4)
		containerUnderTest.ExecProcesses = ep
		processUnderTest := ep[0]

		p.containers = append(p.containers, containerUnderTest)

		tc, err := setupRegoRunningContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		containerID, err := idForRunningContainer(containerUnderTest, tc.runningContainers)
		if err != nil {
			r, err := runContainer(tc.policy, containerUnderTest, tc.defaultMounts, tc.privilegedMounts)
			if err != nil {
				t.Errorf("Unable to setup test running container: %v", err)
				return false
			}
			containerID = r.containerID
		}

		envList := buildEnvironmentVariablesFromEnvRules(containerUnderTest.EnvRules, testRand)

		err = tc.policy.EnforceExecInContainerPolicy(containerID, processUnderTest.Command, envList, containerUnderTest.WorkingDir)
		if err != nil {
			t.Errorf("Unable to exec process for test: %v", err)
			return false
		}

		signal := selectSignalFromSignals(testRand, processUnderTest.Signals)
		badContainerID := generateContainerID(testRand)

		err = tc.policy.EnforceSignalContainerProcessPolicy(badContainerID, signal, false, processUnderTest.Command)
		if err == nil {
			t.Errorf("Signal init process unexpectedly succeeded: %v", err)
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_SignalContainerProcessPolicy_ExecProcess_Bad_ContainerID: %v", err)
	}
}

func Test_Rego_Plan9MountPolicy(t *testing.T) {
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints, maxExternalProcessesInGeneratedConstraints)

	tc, err := setupPlan9MountTest(gc)
	if err != nil {
		t.Fatalf("unable to setup test: %v", err)
	}

	err = tc.policy.EnforcePlan9MountPolicy(tc.uvmPathForShare)
	if err != nil {
		t.Fatalf("Policy enforcement unexpectedly was denied: %v", err)
	}

	err = tc.policy.EnforceCreateContainerPolicy(
		tc.sandboxID,
		tc.containerID,
		tc.argList,
		tc.envList,
		tc.workingDir,
		tc.mounts)

	if err != nil {
		t.Fatalf("Policy enforcement unexpectedly was denied: %v", err)
	}
}

func Test_Rego_Plan9MountPolicy_No_Matches(t *testing.T) {
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints, maxExternalProcessesInGeneratedConstraints)

	tc, err := setupPlan9MountTest(gc)
	if err != nil {
		t.Fatalf("unable to setup test: %v", err)
	}

	mount := generateUVMPathForShare(testRand, tc.containerID)
	for {
		if mount != tc.uvmPathForShare {
			break
		}
		mount = generateUVMPathForShare(testRand, tc.containerID)
	}

	err = tc.policy.EnforcePlan9MountPolicy(mount)
	if err != nil {
		t.Fatalf("Policy enforcement unexpectedly was denied: %v", err)
	}

	err = tc.policy.EnforceCreateContainerPolicy(
		tc.sandboxID,
		tc.containerID,
		tc.argList,
		tc.envList,
		tc.workingDir,
		tc.mounts)

	if err == nil {
		t.Fatal("Policy enforcement unexpectedly was allowed")
	}
}

func Test_Rego_Plan9MountPolicy_Invalid(t *testing.T) {
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints, maxExternalProcessesInGeneratedConstraints)

	tc, err := setupPlan9MountTest(gc)
	if err != nil {
		t.Fatalf("unable to setup test: %v", err)
	}

	mount := randString(testRand, maxGeneratedMountSourceLength)
	err = tc.policy.EnforcePlan9MountPolicy(mount)
	if err == nil {
		t.Fatal("Policy enforcement unexpectedly was allowed", err)
	}
}

func Test_Rego_Plan9UnmountPolicy(t *testing.T) {
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints, maxExternalProcessesInGeneratedConstraints)

	tc, err := setupPlan9MountTest(gc)
	if err != nil {
		t.Fatalf("unable to setup test: %v", err)
	}

	err = tc.policy.EnforcePlan9MountPolicy(tc.uvmPathForShare)
	if err != nil {
		t.Fatalf("Couldn't mount as part of setup: %v", err)
	}

	err = tc.policy.EnforcePlan9UnmountPolicy(tc.uvmPathForShare)
	if err != nil {
		t.Fatalf("Policy enforcement unexpectedly was denied: %v", err)
	}

	err = tc.policy.EnforceCreateContainerPolicy(
		tc.sandboxID,
		tc.containerID,
		tc.argList,
		tc.envList,
		tc.workingDir,
		tc.mounts)

	if err == nil {
		t.Fatal("Policy enforcement unexpectedly was allowed")
	}
}

func Test_Rego_Plan9UnmountPolicy_No_Matches(t *testing.T) {
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints, maxExternalProcessesInGeneratedConstraints)

	tc, err := setupPlan9MountTest(gc)
	if err != nil {
		t.Fatalf("unable to setup test: %v", err)
	}

	mount := generateUVMPathForShare(testRand, tc.containerID)
	err = tc.policy.EnforcePlan9MountPolicy(mount)
	if err != nil {
		t.Fatalf("Couldn't mount as part of setup: %v", err)
	}

	badMount := randString(testRand, maxPlan9MountTargetLength)
	err = tc.policy.EnforcePlan9UnmountPolicy(badMount)
	if err == nil {
		t.Fatalf("Policy enforcement unexpectedly was allowed")
	}
}

func Test_Rego_GetPropertiesPolicy_On(t *testing.T) {
	f := func(constraints *generatedConstraints) bool {
		tc, err := setupGetPropertiesTest(constraints, true)
		if err != nil {
			t.Error(err)
			return false
		}

		err = tc.policy.EnforceGetPropertiesPolicy()
		if err != nil {
			t.Error("Policy enforcement unexpectedly was denied")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("Test_Rego_GetPropertiesPolicy_On: %v", err)
	}
}

func Test_Rego_GetPropertiesPolicy_Off(t *testing.T) {
	f := func(constraints *generatedConstraints) bool {
		tc, err := setupGetPropertiesTest(constraints, false)
		if err != nil {
			t.Error(err)
			return false
		}

		err = tc.policy.EnforceGetPropertiesPolicy()
		if err == nil {
			t.Error("Policy enforcement unexpectedly was allowed")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("Test_Rego_GetPropertiesPolicy_Off: %v", err)
	}
}

func Test_Rego_DumpStacksPolicy_On(t *testing.T) {
	f := func(constraints *generatedConstraints) bool {
		tc, err := setupDumpStacksTest(constraints, true)
		if err != nil {
			t.Error(err)
			return false
		}

		err = tc.policy.EnforceDumpStacksPolicy()
		if err != nil {
			t.Errorf("Policy enforcement unexpectedly was denied: %v", err)
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("Test_Rego_DumpStacksPolicy_On: %v", err)
	}
}

func Test_Rego_DumpStacksPolicy_Off(t *testing.T) {
	f := func(constraints *generatedConstraints) bool {
		tc, err := setupDumpStacksTest(constraints, false)
		if err != nil {
			t.Error(err)
			return false
		}

		err = tc.policy.EnforceDumpStacksPolicy()
		if err == nil {
			t.Error("Policy enforcement unexpectedly was allowed")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("Test_Rego_DumpStacksPolicy_Off: %v", err)
	}
}

//
// Setup and "fixtures" follow...
//

func generateExternalProcesses(r *rand.Rand) []*externalProcess {
	var processes []*externalProcess

	numProcesses := atLeastOneAtMost(r, maxGeneratedExternalProcesses)
	for i := 0; i < int(numProcesses); i++ {
		processes = append(processes, generateExternalProcess(r))
	}

	return processes
}

func selectExternalProcessFromConstraints(constraints *generatedConstraints, r *rand.Rand) *externalProcess {
	numberOfProcessesInConstraints := len(constraints.externalProcesses)
	return constraints.externalProcesses[r.Intn(numberOfProcessesInConstraints)]
}

func (constraints *generatedConstraints) toPolicy() *securityPolicyInternal {
	securityPolicy := new(securityPolicyInternal)
	securityPolicy.Containers = constraints.containers
	securityPolicy.ExternalProcesses = constraints.externalProcesses
	securityPolicy.AllowPropertiesAccess = constraints.allowGetProperties
	securityPolicy.AllowDumpStacks = constraints.allowDumpStacks
	return securityPolicy
}

func toOCIMounts(mounts []mountInternal) []oci.Mount {
	result := make([]oci.Mount, len(mounts))
	for i, mount := range mounts {
		result[i] = oci.Mount{
			Source:      mount.Source,
			Destination: mount.Destination,
			Options:     mount.Options,
			Type:        mount.Type,
		}
	}
	return result
}

/**
 * NOTE_TESTCOPY: the following "copy*" functions are provided to ensure that
 * everything passed to the policy is a new object which will not be shared in
 * any way with other policy objects in other tests. In any additional fixture
 * setup routines these functions (or others like them) should be used.
 */

func copyStrings(values []string) []string {
	valuesCopy := make([]string, len(values))
	copy(valuesCopy, values)
	return valuesCopy
}

func copyMounts(mounts []oci.Mount) []oci.Mount {
	bytes, err := json.Marshal(mounts)
	if err != nil {
		panic(err)
	}

	mountsCopy := make([]oci.Mount, len(mounts))
	err = json.Unmarshal(bytes, &mountsCopy)
	if err != nil {
		panic(err)
	}

	return mountsCopy
}

func copyMountsInternal(mounts []mountInternal) []mountInternal {
	var mountsCopy []mountInternal

	for _, in := range mounts {
		out := mountInternal{
			Source:      in.Source,
			Destination: in.Destination,
			Type:        in.Type,
			Options:     copyStrings(in.Options),
		}

		mountsCopy = append(mountsCopy, out)
	}

	return mountsCopy
}

type regoOverlayTestConfig struct {
	layers      []string
	containerID string
	policy      *regoEnforcer
}

func setupRegoOverlayTest(gc *generatedConstraints, valid bool) (tc *regoOverlayTestConfig, err error) {
	securityPolicy := gc.toPolicy()
	policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{})
	if err != nil {
		return nil, err
	}

	containerID := testDataGenerator.uniqueContainerID()
	c := selectContainerFromConstraints(gc, testRand)

	var layerPaths []string
	if valid {
		layerPaths, err = testDataGenerator.createValidOverlayForContainer(policy, c)
		if err != nil {
			return nil, fmt.Errorf("error creating valid overlay: %w", err)
		}
	} else {
		layerPaths, err = testDataGenerator.createInvalidOverlayForContainer(policy, c)
		if err != nil {
			return nil, fmt.Errorf("error creating invalid overlay: %w", err)
		}
	}

	// see NOTE_TESTCOPY
	return &regoOverlayTestConfig{
		layers:      copyStrings(layerPaths),
		containerID: containerID,
		policy:      policy,
	}, nil
}

type regoContainerTestConfig struct {
	envList     []string
	argList     []string
	workingDir  string
	containerID string
	sandboxID   string
	mounts      []oci.Mount
	policy      *regoEnforcer
}

func setupSimpleRegoCreateContainerTest(gc *generatedConstraints) (tc *regoContainerTestConfig, err error) {
	c := selectContainerFromConstraints(gc, testRand)
	return setupRegoCreateContainerTest(gc, c, false)
}

func setupRegoPrivilegedMountTest(gc *generatedConstraints) (tc *regoContainerTestConfig, err error) {
	c := selectContainerFromConstraints(gc, testRand)
	return setupRegoCreateContainerTest(gc, c, true)
}

func setupRegoCreateContainerTest(gc *generatedConstraints, testContainer *securityPolicyContainer, privilegedError bool) (tc *regoContainerTestConfig, err error) {
	securityPolicy := gc.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	policy, err := newRegoPolicy(securityPolicy.marshalRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts))
	if err != nil {
		return nil, err
	}

	containerID, err := mountImageForContainer(policy, testContainer)
	if err != nil {
		return nil, err
	}

	envList := buildEnvironmentVariablesFromEnvRules(testContainer.EnvRules, testRand)
	sandboxID := testDataGenerator.uniqueSandboxID()

	mounts := testContainer.Mounts
	mounts = append(mounts, defaultMounts...)
	if privilegedError {
		testContainer.AllowElevated = false
	}

	if testContainer.AllowElevated || privilegedError {
		mounts = append(mounts, privilegedMounts...)
	}
	mountSpec := buildMountSpecFromMountArray(mounts, sandboxID, testRand)

	// see NOTE_TESTCOPY
	return &regoContainerTestConfig{
		envList:     copyStrings(envList),
		argList:     copyStrings(testContainer.Command),
		workingDir:  testContainer.WorkingDir,
		containerID: containerID,
		sandboxID:   sandboxID,
		mounts:      copyMounts(mountSpec.Mounts),
		policy:      policy,
	}, nil
}

func setupRegoRunningContainerTest(gc *generatedConstraints) (tc *regoRunningContainerTestConfig, err error) {
	securityPolicy := gc.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	policy, err := newRegoPolicy(securityPolicy.marshalRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts))
	if err != nil {
		return nil, err
	}

	var runningContainers []regoRunningContainer
	numOfRunningContainers := int(atLeastOneAtMost(testRand, int32(len(gc.containers))))
	containersToRun := randChoices(testRand, numOfRunningContainers, len(gc.containers), true)
	for _, i := range containersToRun {
		containerToStart := gc.containers[i]
		r, err := runContainer(policy, containerToStart, defaultMounts, privilegedMounts)
		if err != nil {
			return nil, err
		}
		runningContainers = append(runningContainers, *r)
	}

	return &regoRunningContainerTestConfig{
		runningContainers: runningContainers,
		policy:            policy,
		defaultMounts:     copyMountsInternal(defaultMounts),
		privilegedMounts:  copyMountsInternal(privilegedMounts),
	}, nil
}

func runContainer(enforcer *regoEnforcer, container *securityPolicyContainer, defaultMounts []mountInternal, privilegedMounts []mountInternal) (*regoRunningContainer, error) {
	containerID, err := mountImageForContainer(enforcer, container)
	if err != nil {
		return nil, err
	}

	envList := buildEnvironmentVariablesFromEnvRules(container.EnvRules, testRand)
	sandboxID := generateSandboxID(testRand)

	mounts := container.Mounts
	mounts = append(mounts, defaultMounts...)
	if container.AllowElevated {
		mounts = append(mounts, privilegedMounts...)
	}
	mountSpec := buildMountSpecFromMountArray(mounts, sandboxID, testRand)

	err = enforcer.EnforceCreateContainerPolicy(sandboxID, containerID, container.Command, envList, container.WorkingDir, mountSpec.Mounts)
	if err != nil {
		return nil, err
	}

	return &regoRunningContainer{
		container:   container,
		containerID: containerID,
	}, nil
}

type regoRunningContainerTestConfig struct {
	runningContainers []regoRunningContainer
	policy            *regoEnforcer
	defaultMounts     []mountInternal
	privilegedMounts  []mountInternal
}

type regoRunningContainer struct {
	container   *securityPolicyContainer
	containerID string
}

func setupExternalProcessTest(gc *generatedConstraints) (tc *regoExternalPolicyTestConfig, err error) {
	securityPolicy := gc.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	policy, err := newRegoPolicy(securityPolicy.marshalRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts))
	if err != nil {
		return nil, err
	}

	return &regoExternalPolicyTestConfig{
		policy: policy,
	}, nil
}

type regoExternalPolicyTestConfig struct {
	policy *regoEnforcer
}

func setupPlan9MountTest(gc *generatedConstraints) (tc *regoPlan9MountTestConfig, err error) {
	securityPolicy := gc.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	testContainer := selectContainerFromConstraints(gc, testRand)
	mountIndex := atMost(testRand, int32(len(testContainer.Mounts)-1))
	testMount := &testContainer.Mounts[mountIndex]
	testMount.Source = plan9Prefix
	testMount.Type = "secret"

	policy, err := newRegoPolicy(securityPolicy.marshalRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts))
	if err != nil {
		return nil, err
	}

	containerID, err := mountImageForContainer(policy, testContainer)
	if err != nil {
		return nil, err
	}

	uvmPathForShare := generateUVMPathForShare(testRand, containerID)

	envList := buildEnvironmentVariablesFromEnvRules(testContainer.EnvRules, testRand)
	sandboxID := testDataGenerator.uniqueSandboxID()

	mounts := testContainer.Mounts
	mounts = append(mounts, defaultMounts...)

	if testContainer.AllowElevated {
		mounts = append(mounts, privilegedMounts...)
	}
	mountSpec := buildMountSpecFromMountArray(mounts, sandboxID, testRand)
	mountSpec.Mounts = append(mountSpec.Mounts, oci.Mount{
		Source:      uvmPathForShare,
		Destination: testMount.Destination,
		Options:     testMount.Options,
		Type:        testMount.Type,
	})

	// see NOTE_TESTCOPY
	return &regoPlan9MountTestConfig{
		envList:         copyStrings(envList),
		argList:         copyStrings(testContainer.Command),
		workingDir:      testContainer.WorkingDir,
		containerID:     containerID,
		sandboxID:       sandboxID,
		mounts:          copyMounts(mountSpec.Mounts),
		uvmPathForShare: uvmPathForShare,
		policy:          policy,
	}, nil
}

type regoPlan9MountTestConfig struct {
	envList         []string
	argList         []string
	workingDir      string
	containerID     string
	sandboxID       string
	mounts          []oci.Mount
	uvmPathForShare string
	policy          *regoEnforcer
}

func setupGetPropertiesTest(gc *generatedConstraints, allowPropertiesAccess bool) (tc *regoGetPropertiesTestConfig, err error) {
	gc.allowGetProperties = allowPropertiesAccess

	securityPolicy := gc.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	policy, err := newRegoPolicy(securityPolicy.marshalRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts))
	if err != nil {
		return nil, err
	}

	return &regoGetPropertiesTestConfig{
		policy: policy,
	}, nil
}

type regoGetPropertiesTestConfig struct {
	policy *regoEnforcer
}

func setupDumpStacksTest(constraints *generatedConstraints, allowDumpStacks bool) (tc *regoGetPropertiesTestConfig, err error) {
	constraints.allowDumpStacks = allowDumpStacks

	securityPolicy := constraints.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	policy, err := newRegoPolicy(securityPolicy.marshalRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts))
	if err != nil {
		return nil, err
	}

	return &regoGetPropertiesTestConfig{
		policy: policy,
	}, nil
}

type regoDumpStacksTestConfig struct {
	policy *regoEnforcer
}

func mountImageForContainer(policy *regoEnforcer, container *securityPolicyContainer) (string, error) {
	containerID := testDataGenerator.uniqueContainerID()

	layerPaths, err := testDataGenerator.createValidOverlayForContainer(policy, container)
	if err != nil {
		return "", fmt.Errorf("error creating valid overlay: %w", err)
	}

	// see NOTE_TESTCOPY
	err = policy.EnforceOverlayMountPolicy(containerID, copyStrings(layerPaths), testDataGenerator.uniqueMountTarget())
	if err != nil {
		return "", fmt.Errorf("error mounting filesystem: %w", err)
	}

	return containerID, nil
}

func generateSandboxID(r *rand.Rand) string {
	return randVariableString(r, maxGeneratedSandboxIDLength)
}

func generateEnforcementPoint(r *rand.Rand) string {
	first := randChar(r)
	return first + randString(r, atMost(r, maxGeneratedEnforcementPointLength))
}

func randChar(r *rand.Rand) string {
	charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	return string(charset[r.Intn(len(charset))])
}

func (gen *dataGenerator) uniqueSandboxID() string {
	for {
		t := generateSandboxID(gen.rng)
		if _, ok := gen.sandboxIDs[t]; !ok {
			gen.sandboxIDs[t] = struct{}{}
			return t
		}
	}
}

func (gen *dataGenerator) uniqueEnforcementPoint() string {
	for {
		t := generateEnforcementPoint(gen.rng)
		if _, ok := gen.enforcementPoints[t]; !ok {
			gen.enforcementPoints[t] = struct{}{}
			return t
		}
	}
}

func buildMountSpecFromMountArray(mounts []mountInternal, sandboxID string, r *rand.Rand) *oci.Spec {
	mountSpec := new(oci.Spec)

	// Select some number of the valid, matching rules to be environment
	// variable
	numberOfMounts := int32(len(mounts))
	numberOfMatches := randMinMax(r, 1, numberOfMounts)
	usedIndexes := map[int]struct{}{}
	for numberOfMatches > 0 {
		anIndex := -1
		if (numberOfMatches * 2) > numberOfMounts {
			// if we have a lot of matches, randomly select
			exists := true

			for exists {
				anIndex = int(randMinMax(r, 0, numberOfMounts-1))
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

		mount := mounts[anIndex]

		source := substituteUVMPath(sandboxID, mount).Source
		mountSpec.Mounts = append(mountSpec.Mounts, oci.Mount{
			Source:      source,
			Destination: mount.Destination,
			Options:     mount.Options,
			Type:        mount.Type,
		})
		usedIndexes[anIndex] = struct{}{}

		numberOfMatches--
	}

	return mountSpec
}

//go:embed api_test.rego
var apiTestCode string

func (p *regoEnforcer) injectTestAPI() error {
	modules := map[string]string{
		"policy.rego":    p.code,
		"api.rego":       apiTestCode,
		"framework.rego": frameworkCode,
	}

	// TODO temporary hack for debugging policies until GCS logging design
	// and implementation is finalized. This option should be changed to
	// "true" if debugging is desired.
	options := ast.CompileOpts{
		EnablePrintStatements: false,
	}

	if compiled, err := ast.CompileModulesWithOpt(modules, options); err == nil {
		p.compiledModules = compiled
		return nil
	} else {
		return fmt.Errorf("rego compilation failed: %w", err)
	}
}

func selectContainerFromRunningContainers(containers []regoRunningContainer, r *rand.Rand) regoRunningContainer {
	numContainers := len(containers)
	return containers[r.Intn(numContainers)]
}

func selectExecProcess(processes []containerExecProcess, r *rand.Rand) containerExecProcess {
	numProcesses := len(processes)
	return processes[r.Intn(numProcesses)]
}

func idForRunningContainer(container *securityPolicyContainer, running []regoRunningContainer) (string, error) {
	for _, c := range running {
		if c.container == container {
			return c.containerID, nil
		}
	}

	return "", errors.New("Container isn't running")
}

func selectSignalFromSignals(r *rand.Rand, signals []syscall.Signal) syscall.Signal {
	numSignals := len(signals)
	return signals[r.Intn(numSignals)]
}

func randChoices(r *rand.Rand, numChoices int, numItems int, replacement bool) []int {
	if !replacement {
		shuffle := r.Perm(numItems)
		if numChoices > numItems {
			return shuffle
		}

		return shuffle[:numChoices]
	}

	choices := make([]int, numChoices)
	for i := 0; i < numChoices; i++ {
		choices[i] = r.Intn(numItems)
	}

	return choices
}

func generateUVMPathForShare(r *rand.Rand, containerID string) string {
	return fmt.Sprintf("%s/%s%s",
		guestpath.LCOWRootPrefixInUVM,
		containerID,
		fmt.Sprintf(guestpath.LCOWMountPathPrefixFmt, atMost(r, maxPlan9MountIndex)))
}
