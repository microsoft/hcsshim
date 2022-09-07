//go:build linux && rego
// +build linux,rego

package securitypolicy

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"testing"
	"testing/quick"

	oci "github.com/opencontainers/runtime-spec/specs-go"
)

const (
	// variables that influence generated rego-only test fixtures
	maxGeneratedSandboxIDLength        = 32
	maxGeneratedEnforcementPointLength = 64
)

// Validate we do our conversion from Json to rego correctly
func Test_MarshalRego(t *testing.T) {
	f := func(p *generatedContainers) bool {
		securityPolicy := newSecurityPolicyInternal(p.containers)
		defaultMounts := toOCIMounts(generateMounts(testRand))
		privilegedMounts := toOCIMounts(generateMounts(testRand))

		_, err := newRegoPolicy(securityPolicy.marshalRego(), defaultMounts, privilegedMounts)
		if err != nil {
			t.Errorf("unable to convert policy to rego: %v", err)
		}

		return !t.Failed()
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 4}); err != nil {
		t.Errorf("Test_MarshalRego failed: %v", err)
	}
}

// Verify that RegoSecurityPolicyEnforcer.EnforceDeviceMountPolicy will
// return an error when there's no matching root hash in the policy
func Test_Rego_EnforceDeviceMountPolicy_No_Matches(t *testing.T) {
	f := func(p *generatedContainers) bool {
		securityPolicy := newSecurityPolicyInternal(p.containers)
		policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{})
		if err != nil {
			t.Errorf("unable to convert policy to rego: %v", err)
		}

		target := testDataGenerator.uniqueMountTarget()
		rootHash := generateInvalidRootHash(testRand)

		err = policy.EnforceDeviceMountPolicy(target, rootHash)

		// we expect an error, not getting one means something is broken
		return err != nil && strings.Contains(err.Error(), rootHash)
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 250}); err != nil {
		t.Errorf("Test_Rego_EnforceDeviceMountPolicy_No_Matches failed: %v", err)
	}
}

// Verify that RegoSecurityPolicyEnforcer.EnforceDeviceMountPolicy doesn't
// return an error when there's a matching root hash in the policy
func Test_Rego_EnforceDeviceMountPolicy_Matches(t *testing.T) {
	f := func(p *generatedContainers) bool {
		securityPolicy := newSecurityPolicyInternal(p.containers)
		policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{})
		if err != nil {
			t.Errorf("unable to convert policy to rego: %v", err)
		}

		target := testDataGenerator.uniqueMountTarget()
		rootHash := selectRootHashFromContainers(p, testRand)

		err = policy.EnforceDeviceMountPolicy(target, rootHash)

		// getting an error means something is broken
		return err == nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 250}); err != nil {
		t.Errorf("Test_Rego_EnforceDeviceMountPolicy_Matches failed: %v", err)
	}
}

func Test_Rego_EnforceDeviceUmountPolicy_Removes_Device_Entries(t *testing.T) {
	f := func(p *generatedContainers) bool {
		securityPolicy := newSecurityPolicyInternal(p.containers)
		policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{})
		if err != nil {
			t.Error(err)
			return false
		}

		target := testDataGenerator.uniqueMountTarget()
		rootHash := selectRootHashFromContainers(p, testRand)

		err = policy.EnforceDeviceMountPolicy(target, rootHash)
		if err != nil {
			return false
		}

		err = policy.EnforceDeviceUnmountPolicy(target)
		if err != nil {
			return false
		}

		devices := policy.data["devices"].(map[string]string)

		_, found := devices[target]
		return !found
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 250}); err != nil {
		t.Errorf("Test_Rego_EnforceDeviceUmountPolicy_Removes_Device_Entries failed: %v", err)
	}
}

func Test_Rego_EnforceDeviceMountPolicy_Duplicate_Device_Target(t *testing.T) {
	f := func(p *generatedContainers) bool {
		securityPolicy := newSecurityPolicyInternal(p.containers)
		policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{})
		if err != nil {
			t.Errorf("unable to convert policy to rego: %v", err)
		}

		target := testDataGenerator.uniqueMountTarget()
		rootHash := selectRootHashFromContainers(p, testRand)
		err = policy.EnforceDeviceMountPolicy(target, rootHash)
		if err != nil {
			t.Error("Valid device mount failed. It shouldn't have.")
			return false
		}

		rootHash = selectRootHashFromContainers(p, testRand)
		err = policy.EnforceDeviceMountPolicy(target, rootHash)
		if err == nil {
			t.Error("Duplicate device mount target was allowed. It shouldn't have been.")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 250}); err != nil {
		t.Errorf("Test_Rego_EnforceDeviceMountPolicy_Duplicate_Device_Target failed: %v", err)
	}
}

// Verify that RegoSecurityPolicyEnforcer.EnforceOverlayMountPolicy will
// return an error when there's no matching overlay targets.
func Test_Rego_EnforceOverlayMountPolicy_No_Matches(t *testing.T) {
	f := func(p *generatedContainers) bool {
		tc, err := setupRegoOverlayTest(p, false)
		if err != nil {
			t.Error(err)
			return false
		}

		err = tc.policy.EnforceOverlayMountPolicy(tc.containerID, tc.layers)

		if err == nil {
			return false
		}

		if len(tc.layers) > 0 && !strings.Contains(err.Error(), tc.layers[0]) {
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 250}); err != nil {
		t.Errorf("Test_Rego_EnforceOverlayMountPolicy_No_Matches failed: %v", err)
	}
}

// Verify that RegoSecurityPolicyEnforcer.EnforceOverlayMountPolicy doesn't
// return an error when there's a valid overlay target.
func Test_Rego_EnforceOverlayMountPolicy_Matches(t *testing.T) {
	f := func(p *generatedContainers) bool {
		tc, err := setupRegoOverlayTest(p, true)
		if err != nil {
			t.Error(err)
			return false
		}

		err = tc.policy.EnforceOverlayMountPolicy(tc.containerID, tc.layers)

		// getting an error means something is broken
		return err == nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 250}); err != nil {
		t.Errorf("Test_Rego_EnforceOverlayMountPolicy_Matches: %v", err)
	}
}

// Test that an image that contains layers that share a roothash value can be
// successfully mounted. This was a failure scenario in an earlier policy engine
// implementation.
func Test_Rego_EnforceOverlayMountPolicy_Layers_With_Same_Root_Hash(t *testing.T) {

	container := generateContainersContainer(testRand, 2, maxLayersInGeneratedContainer)

	// make the last two layers have the same hash value
	numLayers := len(container.Layers)
	container.Layers[numLayers-2] = container.Layers[numLayers-1]

	securityPolicy := newSecurityPolicyInternal([]*securityPolicyContainer{container})
	policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{})
	if err != nil {
		t.Fatal("Unable to create security policy")
	}

	containerID := testDataGenerator.uniqueContainerID()

	layers, err := testDataGenerator.createValidOverlayForContainer(policy, container)
	if err != nil {
		t.Fatalf("error creating valid overlay: %v", err)
	}

	err = policy.EnforceOverlayMountPolicy(containerID, layers)
	if err != nil {
		t.Fatalf("Unable to create an overlay where root hashes are the same")
	}
}

// Test that can we mount overlays across containers where some layers are
// shared and on the same device. A common example of this is a base image that
// is used by many containers.
// The setup for this test is rather complicated
func Test_Rego_EnforceOverlayMountPolicy_Layers_Shared_Layers(t *testing.T) {
	containerOne := generateContainersContainer(testRand, 1, 2)
	containerTwo := generateContainersContainer(testRand, 1, 10)

	sharedLayerIndex := 0

	// Make the two containers have the same base layer
	containerTwo.Layers[sharedLayerIndex] = containerOne.Layers[sharedLayerIndex]
	containers := []*securityPolicyContainer{containerOne, containerTwo}

	securityPolicy := newSecurityPolicyInternal(containers)
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

	err = policy.EnforceOverlayMountPolicy(containerID, containerOneOverlay)
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

	err = policy.EnforceOverlayMountPolicy(containerID, containerTwoOverlay)
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
	f := func(p *generatedContainers) bool {
		tc, err := setupRegoOverlayTest(p, true)
		if err != nil {
			t.Error(err)
			return false
		}

		if err := tc.policy.EnforceOverlayMountPolicy(tc.containerID, tc.layers); err != nil {
			t.Fatalf("expected nil error got: %v", err)
		}

		if err := tc.policy.EnforceOverlayMountPolicy(tc.containerID, tc.layers); err == nil {
			t.Fatalf("able to create overlay for the same container twice")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 250}); err != nil {
		t.Errorf("Test_Rego_EnforceOverlayMountPolicy_Overlay_Single_Container_Twice: %v", err)
	}
}

func Test_Rego_EnforceOverlayMountPolicy_Reusing_ID_Across_Overlays(t *testing.T) {
	var containers []*securityPolicyContainer

	for i := 0; i < 2; i++ {
		containers = append(containers, generateContainersContainer(testRand, 1, maxLayersInGeneratedContainer))
	}

	securityPolicy := newSecurityPolicyInternal(containers)
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
	layerPaths, err := testDataGenerator.createValidOverlayForContainer(policy, containers[0])
	if err != nil {
		t.Fatalf("Unexpected error creating valid overlay: %v", err)
	}

	err = policy.EnforceOverlayMountPolicy(containerID, layerPaths)
	if err != nil {
		t.Fatalf("Unexpected error mounting overlay filesystem: %v", err)
	}

	// Reusing container ID with another overlay should fail
	layerPaths, err = testDataGenerator.createValidOverlayForContainer(policy, containers[1])
	if err != nil {
		t.Fatalf("Unexpected error creating valid overlay: %v", err)
	}

	err = policy.EnforceOverlayMountPolicy(containerID, layerPaths)
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
	for containersToCreate := 13; containersToCreate <= maxContainersInGeneratedPolicy; containersToCreate++ {
		var containers []*securityPolicyContainer

		for i := 1; i <= containersToCreate; i++ {
			arg := "command " + strconv.Itoa(i)
			c := &securityPolicyContainer{
				Command: []string{arg},
				Layers:  []string{"1", "2"},
			}

			containers = append(containers, c)
		}

		securityPolicy := newSecurityPolicyInternal(containers)
		policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{})
		if err != nil {
			t.Fatalf("failed create enforcer")
		}

		for i := 0; i < len(containers); i++ {
			layerPaths, err := testDataGenerator.createValidOverlayForContainer(policy, containers[i])
			if err != nil {
				t.Fatal("unexpected error on test setup")
			}

			id := testDataGenerator.uniqueContainerID()
			err = policy.EnforceOverlayMountPolicy(id, layerPaths)
			if err != nil {
				t.Fatalf("failed with %d containers", containersToCreate)
			}
		}
	}
}

func Test_Rego_EnforceCommandPolicy_NoMatches(t *testing.T) {
	f := func(p *generatedContainers) bool {
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

	if err := quick.Check(f, &quick.Config{MaxCount: 250}); err != nil {
		t.Errorf("Test_EnforceCommandPolicy_NoMatches: %v", err)
	}
}

func Test_Rego_EnforceEnvironmentVariablePolicy_Re2Match(t *testing.T) {
	testFunc := func(gc *generatedContainers) bool {
		container := selectContainerFromContainers(gc, testRand)
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

	if err := quick.Check(testFunc, &quick.Config{MaxCount: 250}); err != nil {
		t.Errorf("Test_Rego_EnforceEnvironmentVariablePolicy_Re2Match: %v", err)
	}
}

func Test_Rego_EnforceEnvironmentVariablePolicy_NotAllMatches(t *testing.T) {
	f := func(p *generatedContainers) bool {
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

	if err := quick.Check(f, &quick.Config{MaxCount: 250}); err != nil {
		t.Errorf("Test_Rego_EnforceEnvironmentVariablePolicy_NotAllMatches: %v", err)
	}
}

func Test_Rego_WorkingDirectoryPolicy_NoMatches(t *testing.T) {
	testFunc := func(gc *generatedContainers) bool {
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

	if err := quick.Check(testFunc, &quick.Config{MaxCount: 250}); err != nil {
		t.Errorf("Test_Rego_WorkingDirectoryPolicy_NoMatches: %v", err)
	}
}

func Test_Rego_EnforceCreateContainer(t *testing.T) {
	f := func(p *generatedContainers) bool {
		tc, err := setupSimpleRegoCreateContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts)

		// getting an error means something is broken
		return err == nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 250}); err != nil {
		t.Errorf("Test_Rego_EnforceCreateContainer: %v", err)
	}
}

func Test_Rego_Enforce_CreateContainer_Start_All_Containers(t *testing.T) {
	f := func(p *generatedContainers) bool {
		securityPolicy := newSecurityPolicyInternal(p.containers)
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

			envList := buildEnvironmentVariablesFromContainerRules(container, testRand)

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

	if err := quick.Check(f, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("Test_Rego_Enforce_CreateContainer_Start_All_Containers: %v", err)
	}
}

func Test_Rego_EnforceCreateContainer_Invalid_ContainerID(t *testing.T) {
	f := func(p *generatedContainers) bool {
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

	if err := quick.Check(f, &quick.Config{MaxCount: 250}); err != nil {
		t.Errorf("Test_Rego_EnforceCreateContainer_Invalid_ContainerID: %v", err)
	}
}

func Test_Rego_EnforceCreateContainer_Same_Container_Twice(t *testing.T) {
	f := func(p *generatedContainers) bool {
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

	if err := quick.Check(f, &quick.Config{MaxCount: 250}); err != nil {
		t.Errorf("Test_Rego_EnforceCreateContainer_Same_Container_Twice: %v", err)
	}
}

func Test_Rego_ExtendDefaultMounts(t *testing.T) {
	f := func(p *generatedContainers) bool {
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

	if err := quick.Check(f, &quick.Config{MaxCount: 250}); err != nil {
		t.Errorf("Test_Rego_ExtendDefaultMounts: %v", err)
	}
}

func Test_Rego_MountPolicy_NoMatches(t *testing.T) {
	f := func(p *generatedContainers) bool {
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

	if err := quick.Check(f, &quick.Config{MaxCount: 250}); err != nil {
		t.Errorf("Test_Rego_MountPolicy_NoMatches: %v", err)
	}
}

func Test_Rego_MountPolicy_NotAllOptionsFromConstraints(t *testing.T) {
	f := func(p *generatedContainers) bool {
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

	if err := quick.Check(f, &quick.Config{MaxCount: 250}); err != nil {
		t.Errorf("Test_Rego_MountPolicy_NotAllOptionsFromConstraints: %v", err)
	}
}

func Test_Rego_MountPolicy_BadSource(t *testing.T) {
	f := func(p *generatedContainers) bool {
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

	if err := quick.Check(f, &quick.Config{MaxCount: 250}); err != nil {
		t.Errorf("Test_Rego_MountPolicy_BadSource: %v", err)
	}
}

func Test_Rego_MountPolicy_BadDestination(t *testing.T) {
	f := func(p *generatedContainers) bool {
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

	if err := quick.Check(f, &quick.Config{MaxCount: 250}); err != nil {
		t.Errorf("Test_Rego_MountPolicy_BadDestination: %v", err)
	}
}

func Test_Rego_MountPolicy_BadType(t *testing.T) {
	f := func(p *generatedContainers) bool {
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

	if err := quick.Check(f, &quick.Config{MaxCount: 250}); err != nil {
		t.Errorf("Test_Rego_MountPolicy_BadType: %v", err)
	}
}

func Test_Rego_MountPolicy_BadOption(t *testing.T) {
	f := func(p *generatedContainers) bool {
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

	if err := quick.Check(f, &quick.Config{MaxCount: 250}); err != nil {
		t.Errorf("Test_Rego_MountPolicy_BadOption: %v", err)
	}
}

func Test_Rego_MountPolicy_MountPrivilegedWhenNotAllowed(t *testing.T) {
	f := func(p *generatedContainers) bool {
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

	if err := quick.Check(f, &quick.Config{MaxCount: 250}); err != nil {
		t.Errorf("Test_Rego_MountPolicy_BadOption: %v", err)
	}
}

// Tests whether an error is raised if support information is requested for
// an enforcement point which does not have stored version information.
func Test_Rego_Version_Unregistered_Enforcement_Point(t *testing.T) {
	gc := generateContainers(testRand, maxContainersInGeneratedPolicy)
	securityPolicy := newSecurityPolicyInternal(gc.containers)
	policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{})
	if err != nil {
		t.Errorf("unable to create a new Rego policy: %v", err)
	}

	enforcementPoint := testDataGenerator.uniqueEnforcementPoint()
	_, err = policy.queryEnforcementPoint(enforcementPoint)

	// we expect an error, not getting one means something is broken
	if err == nil {
		t.Error("an error was not thrown when asking whether an unregistered enforcement point was available")
	}
}

// Tests whether an error is raised if support information is requested for
// a version of an enforcement point which is of a later version than the
// framework. This should not happen, but may occur during development if
// version numbers have been entered incorrectly.
func Test_Rego_Version_Future_Enforcement_Point(t *testing.T) {
	gc := generateContainers(testRand, maxContainersInGeneratedPolicy)
	securityPolicy := newSecurityPolicyInternal(gc.containers)
	policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{})
	if err != nil {
		t.Errorf("unable to create a new Rego policy: %v", err)
	}

	_, err = policy.queryEnforcementPoint("__fixture_for_future_test__")

	// we expect an error, not getting one means something is broken
	if err == nil {
		t.Errorf("an error was not thrown when asking whether an enforcement point from the future was available")
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
		t.Errorf("unable to create a new Rego policy: %v", err)
	}

	info, err := policy.queryEnforcementPoint("__fixture_for_allowed_test_true__")
	// we do not expect an error, getting one means something is broken
	if err != nil {
		t.Errorf("unable to query whether enforcement point is available: %v", err)
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
		t.Errorf("unable to create a new Rego policy: %v", err)
	}

	input := make(map[string]interface{})
	allowed, err := policy.allowed("__fixture_for_allowed_test_false__", input)
	if err != nil {
		t.Errorf("asked whether an enforcement point was allowed and receieved an error: %v", err)
	}

	if allowed {
		t.Error("result of allowed for an unavailable enforcement point was not the specified default (false)")
	}

	allowed, err = policy.allowed("__fixture_for_allowed_test_true__", input)
	if err != nil {
		t.Errorf("asked whether an enforcement point was allowed and receieved an error: %v", err)
	}

	if !allowed {
		t.Error("result of allowed for an unavailable enforcement point was not the specified default (true)")
	}
}

//
// Setup and "fixtures" follow...
//

func newSecurityPolicyInternal(containers []*securityPolicyContainer) *securityPolicyInternal {
	securityPolicy := new(securityPolicyInternal)
	securityPolicy.Containers = containers
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

type regoOverlayTestConfig struct {
	layers      []string
	containerID string
	policy      *regoEnforcer
}

func setupRegoOverlayTest(gc *generatedContainers, valid bool) (tc *regoOverlayTestConfig, err error) {
	securityPolicy := newSecurityPolicyInternal(gc.containers)
	policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{})
	if err != nil {
		return nil, err
	}

	containerID := testDataGenerator.uniqueContainerID()
	c := selectContainerFromContainers(gc, testRand)

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

func setupSimpleRegoCreateContainerTest(gc *generatedContainers) (tc *regoContainerTestConfig, err error) {
	c := selectContainerFromContainers(gc, testRand)
	return setupRegoCreateContainerTest(gc, c, false)
}

func setupRegoPrivilegedMountTest(gc *generatedContainers) (tc *regoContainerTestConfig, err error) {
	c := selectContainerFromContainers(gc, testRand)
	return setupRegoCreateContainerTest(gc, c, true)
}

func setupRegoCreateContainerTest(gc *generatedContainers, testContainer *securityPolicyContainer, privilegedError bool) (tc *regoContainerTestConfig, err error) {
	securityPolicy := newSecurityPolicyInternal(gc.containers)
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

	envList := buildEnvironmentVariablesFromContainerRules(testContainer, testRand)
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

func mountImageForContainer(policy *regoEnforcer, container *securityPolicyContainer) (string, error) {
	containerID := testDataGenerator.uniqueContainerID()

	layerPaths, err := testDataGenerator.createValidOverlayForContainer(policy, container)
	if err != nil {
		return "", fmt.Errorf("error creating valid overlay: %w", err)
	}

	// see NOTE_TESTCOPY
	err = policy.EnforceOverlayMountPolicy(containerID, copyStrings(layerPaths))
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
