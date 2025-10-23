//go:build linux && rego
// +build linux,rego

package securitypolicy

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"testing/quick"

	specInternal "github.com/Microsoft/hcsshim/internal/guest/spec"
	"github.com/Microsoft/hcsshim/internal/guestpath"
	rpi "github.com/Microsoft/hcsshim/internal/regopolicyinterpreter"
	oci "github.com/opencontainers/runtime-spec/specs-go"
)

const testOSType = "linux"

func Test_MarshalRego_Policy(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		p.externalProcesses = generateExternalProcesses(testRand)
		for _, process := range p.externalProcesses {
			// arbitrary environment variable rules for external
			// processes are not currently handled by the config.
			process.envRules = []EnvRuleConfig{{
				Strategy: "string",
				Rule:     "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
				Required: true,
			}}
		}

		p.fragments = generateFragments(testRand, 1)

		securityPolicy := p.toPolicy()
		defaultMounts := toOCIMounts(generateMounts(testRand))
		privilegedMounts := toOCIMounts(generateMounts(testRand))

		expected := securityPolicy.marshalRego()

		containers := make([]*Container, len(p.containers))
		for i, container := range p.containers {
			containers[i] = container.toContainer()
		}

		externalProcesses := make([]ExternalProcessConfig, len(p.externalProcesses))
		for i, process := range p.externalProcesses {
			externalProcesses[i] = process.toConfig()
		}

		fragments := make([]FragmentConfig, len(p.fragments))
		for i, fragment := range p.fragments {
			fragments[i] = fragment.toConfig()
		}

		actual, err := MarshalPolicy(
			"rego",
			false,
			containers,
			externalProcesses,
			fragments,
			p.allowGetProperties,
			p.allowDumpStacks,
			p.allowRuntimeLogging,
			p.allowEnvironmentVariableDropping,
			p.allowUnencryptedScratch,
			p.allowCapabilityDropping,
		)
		if err != nil {
			t.Error(err)
			return false
		}

		if actual != expected {
			start := -1
			end := -1
			for i := 0; i < len(actual) && i < len(expected); i++ {
				if actual[i] != expected[i] {
					if start == -1 {
						start = i
					} else if i-start >= maxDiffLength {
						end = i
						break
					}
				} else if start != -1 {
					end = i
					break
				}
			}
			start = start - 512
			if start < 0 {
				start = 0
			}
			t.Errorf(`MarshalPolicy does not create the expected Rego policy [%d-%d]: "%s" != "%s"`, start, end, actual[start:end], expected[start:end])
			return false
		}

		_, err = newRegoPolicy(expected, defaultMounts, privilegedMounts, testOSType)

		if err != nil {
			t.Errorf("unable to convert policy to rego: %v", err)
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 4, Rand: testRand}); err != nil {
		t.Errorf("Test_MarshalRego_Policy failed: %v", err)
	}
}

func Test_MarshalRego_Fragment(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		p.externalProcesses = generateExternalProcesses(testRand)
		for _, process := range p.externalProcesses {
			// arbitrary environment variable rules for external
			// processes are not currently handled by the config.
			process.envRules = []EnvRuleConfig{{
				Strategy: "string",
				Rule:     "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
				Required: true,
			}}
		}

		p.fragments = generateFragments(testRand, 1)

		fragment := p.toFragment()
		expected := fragment.marshalRego()

		containers := make([]*Container, len(p.containers))
		for i, container := range p.containers {
			containers[i] = container.toContainer()
		}

		externalProcesses := make([]ExternalProcessConfig, len(p.externalProcesses))
		for i, process := range p.externalProcesses {
			externalProcesses[i] = process.toConfig()
		}

		fragments := make([]FragmentConfig, len(p.fragments))
		for i, fragment := range p.fragments {
			fragments[i] = fragment.toConfig()
		}

		actual, err := MarshalFragment(p.namespace, p.svn, containers, externalProcesses, fragments)
		if err != nil {
			t.Error(err)
			return false
		}

		if actual != expected {
			start := -1
			end := -1
			for i := 0; i < len(actual) && i < len(expected); i++ {
				if actual[i] != expected[i] {
					if start == -1 {
						start = i
					} else if i-start >= maxDiffLength {
						end = i
						break
					}
				} else if start != -1 {
					end = i
					break
				}
			}
			t.Errorf("MarshalFragment does not create the expected Rego fragment [%d-%d]: %s != %s", start, end, actual[start:end], expected[start:end])
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 4, Rand: testRand}); err != nil {
		t.Errorf("Test_MarshalRego_Fragment failed: %v", err)
	}
}

// Verify that RegoSecurityPolicyEnforcer.EnforceDeviceMountPolicy will
// return an error when there's no matching root hash in the policy
func Test_Rego_EnforceDeviceMountPolicy_No_Matches(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		securityPolicy := p.toPolicy()
		policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{}, testOSType)

		if err != nil {
			t.Errorf("unable to convert policy to rego: %v", err)
			return false
		}

		target := testDataGenerator.uniqueMountTarget()
		rootHash := generateInvalidRootHash(testRand)

		err = policy.EnforceDeviceMountPolicy(p.ctx, target, rootHash)

		// we expect an error, not getting one means something is broken
		return assertDecisionJSONContains(t, err, rootHash, "deviceHash not found")
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
		policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{}, testOSType)

		if err != nil {
			t.Errorf("unable to convert policy to rego: %v", err)
			return false
		}

		target := testDataGenerator.uniqueMountTarget()
		rootHash := selectRootHashFromConstraints(p, testRand)

		err = policy.EnforceDeviceMountPolicy(p.ctx, target, rootHash)

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
		policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{}, testOSType)

		if err != nil {
			t.Error(err)
			return false
		}

		target := testDataGenerator.uniqueMountTarget()
		rootHash := selectRootHashFromConstraints(p, testRand)

		err = policy.EnforceDeviceMountPolicy(p.ctx, target, rootHash)
		if err != nil {
			t.Errorf("unable to mount device: %v", err)
			return false
		}

		err = policy.EnforceDeviceUnmountPolicy(p.ctx, target)
		if err != nil {
			t.Errorf("unable to unmount device: %v", err)
			return false
		}

		err = policy.EnforceDeviceMountPolicy(p.ctx, target, rootHash)
		if err != nil {
			t.Errorf("unable to remount device: %v", err)
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
		policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{}, testOSType)

		if err != nil {
			t.Errorf("unable to convert policy to rego: %v", err)
			return false
		}

		target := testDataGenerator.uniqueMountTarget()
		rootHash := selectRootHashFromConstraints(p, testRand)
		err = policy.EnforceDeviceMountPolicy(p.ctx, target, rootHash)
		if err != nil {
			t.Error("Valid device mount failed. It shouldn't have.")
			return false
		}

		rootHash = selectRootHashFromConstraints(p, testRand)
		err = policy.EnforceDeviceMountPolicy(p.ctx, target, rootHash)
		if err == nil {
			t.Error("Duplicate device mount target was allowed. It shouldn't have been.")
			return false
		}

		return assertDecisionJSONContains(t, err, "device already mounted at path")
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

		err = tc.policy.EnforceOverlayMountPolicy(p.ctx, tc.containerID, tc.layers, testDataGenerator.uniqueMountTarget())

		if err == nil {
			return false
		}

		toFind := []string{"no matching containers for overlay"}
		if len(tc.layers) > 0 {
			toFind = append(toFind, tc.layers[0])
		}

		return assertDecisionJSONContains(t, err, toFind...)
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

		err = tc.policy.EnforceOverlayMountPolicy(p.ctx, tc.containerID, tc.layers, testDataGenerator.uniqueMountTarget())

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
	constraints.ctx = context.Background()
	constraints.containers = []*securityPolicyContainer{container}
	constraints.externalProcesses = generateExternalProcesses(testRand)
	securityPolicy := constraints.toPolicy()
	policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{}, testOSType)

	if err != nil {
		t.Fatal("Unable to create security policy")
	}

	containerID := testDataGenerator.uniqueContainerID()

	layers, err := testDataGenerator.createValidOverlayForContainer(policy, container)
	if err != nil {
		t.Fatalf("error creating valid overlay: %v", err)
	}

	err = policy.EnforceOverlayMountPolicy(constraints.ctx, containerID, layers, testDataGenerator.uniqueMountTarget())
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
	constraints.ctx = context.Background()
	constraints.containers = []*securityPolicyContainer{containerOne, containerTwo}
	constraints.externalProcesses = generateExternalProcesses(testRand)

	securityPolicy := constraints.toPolicy()
	policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{}, testOSType)

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
		err := policy.EnforceDeviceMountPolicy(constraints.ctx, mount, containerOne.Layers[i])
		if err != nil {
			t.Fatalf("Unexpected error mounting overlay device: %v", err)
		}
		if i == sharedLayerIndex {
			sharedMount = mount
		}

		containerOneOverlay[len(containerOneOverlay)-i-1] = mount
	}

	err = policy.EnforceOverlayMountPolicy(constraints.ctx, containerID, containerOneOverlay, testDataGenerator.uniqueMountTarget())
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

			err := policy.EnforceDeviceMountPolicy(constraints.ctx, mount, containerTwo.Layers[i])
			if err != nil {
				t.Fatalf("Unexpected error mounting overlay device: %v", err)
			}
		} else {
			mount = sharedMount
		}

		containerTwoOverlay[len(containerTwoOverlay)-i-1] = mount
	}

	err = policy.EnforceOverlayMountPolicy(constraints.ctx, containerID, containerTwoOverlay, testDataGenerator.uniqueMountTarget())
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

		if err := tc.policy.EnforceOverlayMountPolicy(p.ctx, tc.containerID, tc.layers, testDataGenerator.uniqueMountTarget()); err != nil {
			t.Errorf("expected nil error got: %v", err)
			return false
		}

		if err := tc.policy.EnforceOverlayMountPolicy(p.ctx, tc.containerID, tc.layers, testDataGenerator.uniqueMountTarget()); err == nil {
			t.Errorf("able to create overlay for the same container twice")
			return false
		} else {
			return assertDecisionJSONContains(t, err, "overlay has already been mounted")
		}
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceOverlayMountPolicy_Overlay_Single_Container_Twice: %v", err)
	}
}

func Test_Rego_EnforceOverlayMountPolicy_Reusing_ID_Across_Overlays(t *testing.T) {
	constraints := new(generatedConstraints)
	constraints.ctx = context.Background()
	for i := 0; i < 2; i++ {
		constraints.containers = append(constraints.containers, generateConstraintsContainer(testRand, 1, maxLayersInGeneratedContainer))
	}

	constraints.externalProcesses = generateExternalProcesses(testRand)

	securityPolicy := constraints.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	policy, err := newRegoPolicy(securityPolicy.marshalRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts), testOSType)
	if err != nil {
		t.Fatal(err)
	}

	containerID := testDataGenerator.uniqueContainerID()

	// First usage should work
	layerPaths, err := testDataGenerator.createValidOverlayForContainer(policy, constraints.containers[0])
	if err != nil {
		t.Fatalf("Unexpected error creating valid overlay: %v", err)
	}

	err = policy.EnforceOverlayMountPolicy(constraints.ctx, containerID, layerPaths, testDataGenerator.uniqueMountTarget())
	if err != nil {
		t.Fatalf("Unexpected error mounting overlay filesystem: %v", err)
	}

	// Reusing container ID with another overlay should fail
	layerPaths, err = testDataGenerator.createValidOverlayForContainer(policy, constraints.containers[1])
	if err != nil {
		t.Fatalf("Unexpected error creating valid overlay: %v", err)
	}

	err = policy.EnforceOverlayMountPolicy(constraints.ctx, containerID, layerPaths, testDataGenerator.uniqueMountTarget())
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
		constraints.ctx = context.Background()
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
		policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{}, testOSType)

		if err != nil {
			t.Fatalf("failed create enforcer")
		}

		for i := 0; i < len(constraints.containers); i++ {
			layerPaths, err := testDataGenerator.createValidOverlayForContainer(policy, constraints.containers[i])
			if err != nil {
				t.Fatal("unexpected error on test setup")
			}

			id := testDataGenerator.uniqueContainerID()
			err = policy.EnforceOverlayMountPolicy(constraints.ctx, id, layerPaths, testDataGenerator.uniqueMountTarget())
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
		err = tc.policy.EnforceOverlayMountPolicy(p.ctx, tc.containerID, tc.layers, target)
		if err != nil {
			t.Errorf("Failure setting up overlay for testing: %v", err)
			return false
		}

		err = tc.policy.EnforceOverlayUnmountPolicy(p.ctx, target)
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
		err = tc.policy.EnforceOverlayMountPolicy(p.ctx, tc.containerID, tc.layers, target)
		if err != nil {
			t.Errorf("Failure setting up overlay for testing: %v", err)
			return false
		}

		badTarget := testDataGenerator.uniqueMountTarget()
		err = tc.policy.EnforceOverlayUnmountPolicy(p.ctx, badTarget)
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

		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, generateCommand(testRand), tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

		if err == nil {
			return false
		}

		return assertDecisionJSONContains(t, err, "invalid command")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_EnforceCommandPolicy_NoMatches: %v", err)
	}
}

func Test_Rego_EnforceEnvironmentVariablePolicy_Re2Match(t *testing.T) {
	testFunc := func(gc *generatedConstraints) bool {
		container := selectContainerFromContainerList(gc.containers, testRand)
		// add a rule to re2 match
		re2MatchRule := EnvRuleConfig{
			Strategy: EnvVarRuleRegex,
			Rule:     "PREFIX_.+=.+",
		}

		// it must pass even if there is leading ^ and trailing $ in the rule
		re2MatchPrefix := EnvRuleConfig{
			Strategy: EnvVarRuleRegex,
			Rule:     "^LEAD_.+=.+_TRAIL$",
		}

		container.EnvRules = append(container.EnvRules, re2MatchRule, re2MatchPrefix)

		tc, err := setupRegoCreateContainerTest(gc, container, false)
		if err != nil {
			t.Error(err)
			return false
		}

		envList := append(tc.envList, "PREFIX_FOO=BAR", "LEAD_FOO=BAR_TRAIL")
		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(gc.ctx, tc.sandboxID, tc.containerID, tc.argList, envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

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

func Test_Rego_EnforceEnvironmentVariablePolicy_Re2MisMatch(t *testing.T) {
	testFunc := func(gc *generatedConstraints) bool {
		container := selectContainerFromContainerList(gc.containers, testRand)
		// add a rule to re2 match
		re2MatchRule := EnvRuleConfig{
			Strategy: EnvVarRuleRegex,
			Rule:     "PREFIX_.+=.+BAR$",
		}

		container.EnvRules = append(container.EnvRules, re2MatchRule)

		tc, err := setupRegoCreateContainerTest(gc, container, false)
		if err != nil {
			t.Error(err)
			return false
		}

		envList := append(tc.envList, "PREFIX_FOO=BAR_FOO")
		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(gc.ctx, tc.sandboxID, tc.containerID, tc.argList, envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

		// not getting an error means something is broken
		if err == nil {
			return false
		}

		return assertDecisionJSONContains(t, err, "invalid env list", "PREFIX_FOO")
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
		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

		// not getting an error means something is broken
		if err == nil {
			return false
		}

		return assertDecisionJSONContains(t, err, "invalid env list", envList[0])
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceEnvironmentVariablePolicy_NotAllMatches: %v", err)
	}
}

func Test_Rego_EnforceEnvironmentVariablePolicy_DropEnvs(t *testing.T) {
	testFunc := func(gc *generatedConstraints) bool {
		gc.allowEnvironmentVariableDropping = true
		container := selectContainerFromContainerList(gc.containers, testRand)

		tc, err := setupRegoCreateContainerTest(gc, container, false)
		if err != nil {
			t.Error(err)
			return false
		}

		extraRules := generateEnvironmentVariableRules(testRand)
		extraEnvs := buildEnvironmentVariablesFromEnvRules(extraRules, testRand)

		envList := append(tc.envList, extraEnvs...)
		actual, _, _, err := tc.policy.EnforceCreateContainerPolicy(gc.ctx, tc.sandboxID, tc.containerID, tc.argList, envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

		// getting an error means something is broken
		if err != nil {
			t.Errorf("Expected container creation to be allowed. It wasn't: %v", err)
			return false
		}

		if !areStringArraysEqual(actual, tc.envList) {
			t.Errorf("environment variables were not dropped correctly.")
			return false
		}

		return true
	}

	if err := quick.Check(testFunc, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceEnvironmentVariablePolicy_DropEnvs: %v", err)
	}
}

func Test_Rego_EnforceEnvironmentVariablePolicy_DropEnvs_Multiple(t *testing.T) {
	tc, err := setupRegoDropEnvsTest(false)
	if err != nil {
		t.Fatalf("error setting up test: %v", err)
	}

	extraRules := generateEnvironmentVariableRules(testRand)
	extraEnvs := buildEnvironmentVariablesFromEnvRules(extraRules, testRand)

	envList := append(tc.envList, extraEnvs...)
	actual, _, _, err := tc.policy.EnforceCreateContainerPolicy(tc.ctx, tc.sandboxID, tc.containerID, tc.argList, envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

	// getting an error means something is broken
	if err != nil {
		t.Errorf("Expected container creation to be allowed. It wasn't: %v", err)
	}

	if !areStringArraysEqual(actual, tc.envList) {
		t.Error("environment variables were not dropped correctly.")
	}
}

func Test_Rego_EnforceEnvironmentVariablePolicy_DropEnvs_Multiple_NoMatch(t *testing.T) {
	tc, err := setupRegoDropEnvsTest(true)
	if err != nil {
		t.Fatalf("error setting up test: %v", err)
	}

	extraRules := generateEnvironmentVariableRules(testRand)
	extraEnvs := buildEnvironmentVariablesFromEnvRules(extraRules, testRand)

	envList := append(tc.envList, extraEnvs...)
	actual, _, _, err := tc.policy.EnforceCreateContainerPolicy(tc.ctx, tc.sandboxID, tc.containerID, tc.argList, envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

	// not getting an error means something is broken
	if err == nil {
		t.Error("expected container creation not to be allowed.")
	}

	if actual != nil {
		t.Error("envList should be nil")
	}
}

func Test_Rego_WorkingDirectoryPolicy_NoMatches(t *testing.T) {
	testFunc := func(gc *generatedConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTest(gc)
		if err != nil {
			t.Error(err)
			return false
		}

		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(tc.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, randString(testRand, 20), tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)
		// not getting an error means something is broken
		if err == nil {
			return false
		}

		return assertDecisionJSONContains(t, err, "invalid working directory")
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
		//t.Logf("Policy: %s", tc.policy.base64policy)
		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

		// getting an error means something is broken
		return err == nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceCreateContainer: %v", err)
	}
}

func Test_Rego_EnforceCreateContainer_Start_All_Containers(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		securityPolicy := p.toPolicy()
		defaultMounts := generateMounts(testRand)
		privilegedMounts := generateMounts(testRand)

		policy, err := newRegoPolicy(securityPolicy.marshalRego(),
			toOCIMounts(defaultMounts),
			toOCIMounts(privilegedMounts), testOSType)
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
			user := buildIDNameFromConfig(container.User.UserIDName, testRand)
			groups := buildGroupIDNamesFromUser(container.User, testRand)

			sandboxID := testDataGenerator.uniqueSandboxID()
			mounts := container.Mounts
			mounts = append(mounts, defaultMounts...)
			if container.AllowElevated {
				mounts = append(mounts, privilegedMounts...)
			}
			mountSpec := buildMountSpecFromMountArray(mounts, sandboxID, testRand)
			capabilities := container.Capabilities.toExternal()
			seccomp := container.SeccompProfileSHA256

			_, _, _, err = policy.EnforceCreateContainerPolicy(p.ctx, sandboxID, containerID, container.Command, envList, container.WorkingDir, mountSpec.Mounts, false, container.NoNewPrivileges, user, groups, container.User.Umask, &capabilities, seccomp)

			// getting an error means something is broken
			if err != nil {
				t.Error(err)
				return false
			}
		}

		return true

	}

	if err := quick.Check(f, &quick.Config{MaxCount: 10, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceCreateContainer_Start_All_Containers: %v", err)
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
		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

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

		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)
		if err != nil {
			t.Error("Unable to start valid container.")
			return false
		}
		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)
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

func Test_Rego_EnforceCreateContainer_Capabilities_NoMatches(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		capabilities := tc.capabilities
		capabilities.Bounding = alterCapabilitySet(testRand, capabilities.Bounding)
		capabilities.Effective = alterCapabilitySet(testRand, capabilities.Effective)
		capabilities.Inheritable = alterCapabilitySet(testRand, capabilities.Inheritable)
		capabilities.Permitted = alterCapabilitySet(testRand, capabilities.Permitted)
		capabilities.Ambient = alterCapabilitySet(testRand, capabilities.Ambient)

		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, capabilities, tc.seccomp)

		if err == nil {
			t.Error("Unexpected success with incorrect capabilities")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceCreateContainer_Capabilities_NoMatches: %v", err)
	}
}

func Test_Rego_EnforceCreateContainer_Capabilities_SubsetDoesntMatch(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		if len(tc.capabilities.Bounding) > 0 {
			capabilities := copyLinuxCapabilities(*tc.capabilities)
			capabilities.Bounding = subsetCapabilitySet(testRand, copyStrings(capabilities.Bounding))

			_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, &capabilities, tc.seccomp)

			if err == nil {
				t.Error("Unexpected success with bounding as a subset of allowed capabilities")
				return false
			}
		}

		if len(tc.capabilities.Effective) > 0 {
			capabilities := copyLinuxCapabilities(*tc.capabilities)
			capabilities.Effective = subsetCapabilitySet(testRand, copyStrings(capabilities.Effective))

			_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, &capabilities, tc.seccomp)

			if err == nil {
				t.Error("Unexpected success with effective as a subset of allowed capabilities")
				return false
			}
		}

		if len(tc.capabilities.Inheritable) > 0 {
			capabilities := copyLinuxCapabilities(*tc.capabilities)
			capabilities.Inheritable = subsetCapabilitySet(testRand, copyStrings(capabilities.Inheritable))

			_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, &capabilities, tc.seccomp)

			if err == nil {
				t.Error("Unexpected success with inheritable as a subset of allowed capabilities")
				return false
			}
		}

		if len(tc.capabilities.Permitted) > 0 {
			capabilities := copyLinuxCapabilities(*tc.capabilities)
			capabilities.Permitted = subsetCapabilitySet(testRand, copyStrings(capabilities.Permitted))

			_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, &capabilities, tc.seccomp)

			if err == nil {
				t.Error("Unexpected success with permitted as a subset of allowed capabilities")
				return false
			}
		}

		if len(tc.capabilities.Ambient) > 0 {
			capabilities := copyLinuxCapabilities(*tc.capabilities)
			capabilities.Ambient = subsetCapabilitySet(testRand, copyStrings(capabilities.Ambient))

			_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, &capabilities, tc.seccomp)

			if err == nil {
				t.Error("Unexpected success with ambient as a subset of allowed capabilities")
				return false
			}
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceCreateContainer_Capabilities_SubsetDoesntMatch: %v", err)
	}
}

func Test_Rego_EnforceCreateContainer_Capabilities_SupersetDoesntMatch(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		capabilities := copyLinuxCapabilities(*tc.capabilities)
		capabilities.Bounding = superCapabilitySet(testRand, copyStrings(capabilities.Bounding))

		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, &capabilities, tc.seccomp)

		if err == nil {
			t.Error("Unexpected success with bounding as a superset of allowed capabilities")
			return false
		}

		capabilities = copyLinuxCapabilities(*tc.capabilities)
		capabilities.Effective = superCapabilitySet(testRand, copyStrings(capabilities.Effective))

		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, &capabilities, tc.seccomp)

		if err == nil {
			t.Error("Unexpected success with effective as a superset of allowed capabilities")
			return false
		}

		capabilities = copyLinuxCapabilities(*tc.capabilities)
		capabilities.Inheritable = superCapabilitySet(testRand, copyStrings(capabilities.Inheritable))

		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, &capabilities, tc.seccomp)

		if err == nil {
			t.Error("Unexpected success with inheritable as a superset of allowed capabilities")
			return false
		}

		capabilities = copyLinuxCapabilities(*tc.capabilities)
		capabilities.Permitted = superCapabilitySet(testRand, copyStrings(capabilities.Permitted))

		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, &capabilities, tc.seccomp)

		if err == nil {
			t.Error("Unexpected success with permitted as a superset of allowed capabilities")
			return false
		}

		capabilities = copyLinuxCapabilities(*tc.capabilities)
		capabilities.Ambient = superCapabilitySet(testRand, copyStrings(capabilities.Ambient))

		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, &capabilities, tc.seccomp)

		if err == nil {
			t.Error("Unexpected success with ambient as a superset of allowed capabilities")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceCreateContainer_Capabilities_SupersetDoesntMatch: %v", err)
	}
}

func Test_Rego_EnforceCreateContainer_Capabilities_DenialHasErrorMessage(t *testing.T) {
	constraints := generateConstraints(testRand, 1)
	tc, err := setupSimpleRegoCreateContainerTest(constraints)
	if err != nil {
		t.Fatal(err)
	}

	capabilities := tc.capabilities
	capabilities.Bounding = alterCapabilitySet(testRand, capabilities.Bounding)
	capabilities.Effective = alterCapabilitySet(testRand, capabilities.Effective)
	capabilities.Inheritable = alterCapabilitySet(testRand, capabilities.Inheritable)
	capabilities.Permitted = alterCapabilitySet(testRand, capabilities.Permitted)
	capabilities.Ambient = alterCapabilitySet(testRand, capabilities.Ambient)

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(tc.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, capabilities, tc.seccomp)

	if err == nil {
		t.Error("Unexpected success with incorrect capabilities")
	}

	if !assertDecisionJSONContains(t, err, "capabilities don't match") {
		t.Fatal("No error message given for denial by capability mismatch")
	}
}

func Test_Rego_EnforceCreateContainer_Capabilities_UndecidableHasErrorMessage(t *testing.T) {
	constraints := generateConstraints(testRand, 1)

	// Capabilities setup needed to trigger error
	testCaps := []string{"one", "two", "three"}
	firstCaps := []string{"one", "three"}
	secondCaps := []string{"two", "three"}

	incomingCapabilities := &oci.LinuxCapabilities{
		Bounding:    testCaps,
		Effective:   testCaps,
		Inheritable: testCaps,
		Permitted:   testCaps,
		Ambient:     testCaps,
	}

	firstContainerCapabilities := &capabilitiesInternal{
		Bounding:    firstCaps,
		Effective:   firstCaps,
		Inheritable: firstCaps,
		Permitted:   firstCaps,
		Ambient:     firstCaps,
	}

	secondContainerCapabilities := &capabilitiesInternal{
		Bounding:    secondCaps,
		Effective:   secondCaps,
		Inheritable: secondCaps,
		Permitted:   secondCaps,
		Ambient:     secondCaps,
	}

	// setup container one
	constraints.containers[0].Capabilities = firstContainerCapabilities

	// Add a second container that is the same as first container except it
	// differs for "initial create" values only in terms of capabilities
	duplicate := &securityPolicyContainer{
		Command:              constraints.containers[0].Command,
		EnvRules:             constraints.containers[0].EnvRules,
		WorkingDir:           constraints.containers[0].WorkingDir,
		Mounts:               constraints.containers[0].Mounts,
		Layers:               constraints.containers[0].Layers,
		AllowElevated:        constraints.containers[0].AllowElevated,
		AllowStdioAccess:     constraints.containers[0].AllowStdioAccess,
		NoNewPrivileges:      constraints.containers[0].NoNewPrivileges,
		User:                 constraints.containers[0].User,
		SeccompProfileSHA256: constraints.containers[0].SeccompProfileSHA256,
		// Difference here is our test case
		Capabilities: secondContainerCapabilities,
		// Don't care. Can be different
		ExecProcesses: generateExecProcesses(testRand),
		Signals:       generateListOfSignals(testRand, 0, maxSignalNumber),
	}

	constraints.containers = append(constraints.containers, duplicate)

	// Undecidable is only possible for create container if dropping is on
	constraints.allowCapabilityDropping = true

	tc, err := setupSimpleRegoCreateContainerTest(constraints)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(tc.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, incomingCapabilities, tc.seccomp)

	if err == nil {
		t.Fatal("Unexpected success with undecidable capabilities")
	}

	if !assertDecisionJSONContains(t, err, "containers only distinguishable by capabilties") {
		t.Fatal("No error message given for undecidable based on capabilities mismatch")
	}
}

func Test_Rego_EnforceCreateContainer_CapabilitiesIsNil(t *testing.T) {
	constraints := generateConstraints(testRand, 1)
	tc, err := setupSimpleRegoCreateContainerTest(constraints)
	if err != nil {
		t.Fatal(err)
	}

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(tc.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, nil, tc.seccomp)

	if err == nil {
		t.Fatal("Unexpected success with nil capabilities")
	}

	if err.Error() != capabilitiesNilError {
		t.Fatal("No error message given for denial by capability being nil")
	}
}

func Test_Rego_EnforceCreateContainer_Capabilities_Null_Elevated(t *testing.T) {
	constraints := generateConstraints(testRand, 1)
	constraints.containers[0].AllowElevated = true
	constraints.containers[0].Capabilities = nil
	tc, err := setupSimpleRegoCreateContainerTest(constraints)
	if err != nil {
		t.Fatal(err)
	}

	capabilities := capabilitiesInternal{
		Bounding:    DefaultUnprivilegedCapabilities(),
		Effective:   DefaultUnprivilegedCapabilities(),
		Inheritable: []string{},
		Permitted:   DefaultUnprivilegedCapabilities(),
		Ambient:     []string{},
	}.toExternal()

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(tc.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, &capabilities, tc.seccomp)

	if err != nil {
		t.Fatal("Unexpected failure with null capabilities and elevated: %w", err)
	}
}

func Test_Rego_EnforceCreateContainer_Capabilities_Null(t *testing.T) {
	constraints := generateConstraints(testRand, 1)
	constraints.containers[0].AllowElevated = false
	constraints.containers[0].Capabilities = nil
	tc, err := setupSimpleRegoCreateContainerTest(constraints)
	if err != nil {
		t.Fatal(err)
	}

	capabilities := capabilitiesInternal{
		Bounding:    DefaultUnprivilegedCapabilities(),
		Effective:   DefaultUnprivilegedCapabilities(),
		Inheritable: []string{},
		Permitted:   DefaultUnprivilegedCapabilities(),
		Ambient:     []string{},
	}.toExternal()

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(tc.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, &capabilities, tc.seccomp)

	if err != nil {
		t.Fatal("Unexpected failure with null capabilities: %w", err)
	}
}

func Test_Rego_EnforceCreateContainer_Capabilities_Null_Elevated_Privileged(t *testing.T) {
	constraints := generateConstraints(testRand, 1)
	constraints.containers[0].AllowElevated = true
	constraints.containers[0].Capabilities = nil
	tc, err := setupSimpleRegoCreateContainerTest(constraints)
	if err != nil {
		t.Fatal(err)
	}

	capabilities := capabilitiesInternal{
		Bounding:    DefaultPrivilegedCapabilities(),
		Effective:   DefaultPrivilegedCapabilities(),
		Inheritable: DefaultPrivilegedCapabilities(),
		Permitted:   DefaultPrivilegedCapabilities(),
		Ambient:     []string{},
	}.toExternal()

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(tc.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, true, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, &capabilities, tc.seccomp)

	if err != nil {
		t.Fatal("Unexpected failure with null capabilities when elevated and privileged: %w", err)
	}
}

func Test_Rego_EnforceExecInContainer_Capabilities_Null_Elevated(t *testing.T) {
	constraints := generateConstraints(testRand, 1)
	constraints.containers[0].AllowElevated = true
	constraints.containers[0].Capabilities = nil
	tc, err := setupRegoRunningContainerTest(constraints, false)
	if err != nil {
		t.Fatal(err)
	}

	capabilities := capabilitiesInternal{
		Bounding:    DefaultUnprivilegedCapabilities(),
		Effective:   DefaultUnprivilegedCapabilities(),
		Inheritable: []string{},
		Permitted:   DefaultUnprivilegedCapabilities(),
		Ambient:     []string{},
	}.toExternal()

	container := selectContainerFromRunningContainers(tc.runningContainers, testRand)

	process := selectExecProcess(container.container.ExecProcesses, testRand)
	envList := buildEnvironmentVariablesFromEnvRules(container.container.EnvRules, testRand)
	user := buildIDNameFromConfig(container.container.User.UserIDName, testRand)
	groups := buildGroupIDNamesFromUser(container.container.User, testRand)
	umask := container.container.User.Umask

	_, _, _, err = tc.policy.EnforceExecInContainerPolicy(constraints.ctx, container.containerID, process.Command, envList, container.container.WorkingDir, container.container.NoNewPrivileges, user, groups, umask, &capabilities)

	if err != nil {
		t.Fatal("Unexpected failure with null capabilities and elevated: %w", err)
	}
}

func Test_Rego_EnforceExecInContainer_Capabilities_Null(t *testing.T) {
	constraints := generateConstraints(testRand, 1)
	constraints.containers[0].Capabilities = nil
	tc, err := setupRegoRunningContainerTest(constraints, false)
	if err != nil {
		t.Fatal(err)
	}

	capabilities := capabilitiesInternal{
		Bounding:    DefaultUnprivilegedCapabilities(),
		Effective:   DefaultUnprivilegedCapabilities(),
		Inheritable: []string{},
		Permitted:   DefaultUnprivilegedCapabilities(),
		Ambient:     []string{},
	}.toExternal()

	container := selectContainerFromRunningContainers(tc.runningContainers, testRand)

	process := selectExecProcess(container.container.ExecProcesses, testRand)
	envList := buildEnvironmentVariablesFromEnvRules(container.container.EnvRules, testRand)
	user := buildIDNameFromConfig(container.container.User.UserIDName, testRand)
	groups := buildGroupIDNamesFromUser(container.container.User, testRand)
	umask := container.container.User.Umask

	_, _, _, err = tc.policy.EnforceExecInContainerPolicy(constraints.ctx, container.containerID, process.Command, envList, container.container.WorkingDir, container.container.NoNewPrivileges, user, groups, umask, &capabilities)

	if err != nil {
		t.Fatal("Unexpected failure with null capabilities: %w", err)
	}
}

func Test_Rego_EnforceExecInContainer_Capabilities_Null_Elevated_Privileged(t *testing.T) {
	constraints := generateConstraints(testRand, 1)
	constraints.containers[0].AllowElevated = true
	constraints.containers[0].Capabilities = nil
	tc, err := setupRegoRunningContainerTest(constraints, true)
	if err != nil {
		t.Fatal(err)
	}

	capabilities := capabilitiesInternal{
		Bounding:    DefaultPrivilegedCapabilities(),
		Effective:   DefaultPrivilegedCapabilities(),
		Inheritable: DefaultPrivilegedCapabilities(),
		Permitted:   DefaultPrivilegedCapabilities(),
		Ambient:     []string{},
	}.toExternal()

	container := selectContainerFromRunningContainers(tc.runningContainers, testRand)

	process := selectExecProcess(container.container.ExecProcesses, testRand)
	envList := buildEnvironmentVariablesFromEnvRules(container.container.EnvRules, testRand)
	user := buildIDNameFromConfig(container.container.User.UserIDName, testRand)
	groups := buildGroupIDNamesFromUser(container.container.User, testRand)
	umask := container.container.User.Umask

	_, _, _, err = tc.policy.EnforceExecInContainerPolicy(constraints.ctx, container.containerID, process.Command, envList, container.container.WorkingDir, container.container.NoNewPrivileges, user, groups, umask, &capabilities)

	if err != nil {
		t.Fatal("Unexpected failure with null capabilities when elevated and privileged: %w", err)
	}
}

func Test_Rego_EnforceCreateContainer_CapabilitiesAreEmpty(t *testing.T) {
	constraints := generateConstraints(testRand, 1)
	constraints.containers[0].Capabilities.Bounding = make([]string, 0)
	constraints.containers[0].Capabilities.Effective = make([]string, 0)
	constraints.containers[0].Capabilities.Inheritable = make([]string, 0)
	constraints.containers[0].Capabilities.Permitted = make([]string, 0)
	constraints.containers[0].Capabilities.Ambient = make([]string, 0)

	tc, err := setupSimpleRegoCreateContainerTest(constraints)
	if err != nil {
		t.Fatal(err)
	}

	capabilities := oci.LinuxCapabilities{}

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(tc.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, &capabilities, tc.seccomp)

	if err != nil {
		t.Fatal("Unexpected failure")
	}
}

func Test_Rego_EnforceCreateContainer_Capabilities_Drop(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		p.allowCapabilityDropping = true
		tc, err := setupSimpleRegoCreateContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		capabilities := copyLinuxCapabilities(*tc.capabilities)
		extraCapabilities := generateCapabilities(testRand)
		capabilities.Bounding = append(capabilities.Bounding, extraCapabilities.Bounding...)
		capabilities.Effective = append(capabilities.Effective, extraCapabilities.Effective...)
		capabilities.Inheritable = append(capabilities.Inheritable, extraCapabilities.Inheritable...)
		capabilities.Permitted = append(capabilities.Permitted, extraCapabilities.Permitted...)
		capabilities.Ambient = append(capabilities.Ambient, extraCapabilities.Ambient...)

		_, actual, _, err := tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, &capabilities, tc.seccomp)

		if err != nil {
			t.Errorf("Expected container creation to be allowed. It wasn't for extra capabilities: %v", err)
			return false
		}

		if !areStringArraysEqual(actual.Bounding, tc.capabilities.Bounding) {
			t.Errorf("bounding capabilities were not dropped correctly.")
			return false
		}

		if !areStringArraysEqual(actual.Effective, tc.capabilities.Effective) {
			t.Errorf("effective capabilities were not dropped correctly.")
			return false
		}

		if !areStringArraysEqual(actual.Inheritable, tc.capabilities.Inheritable) {
			t.Errorf("inheritable capabilities were not dropped correctly.")
			return false
		}

		if !areStringArraysEqual(actual.Permitted, tc.capabilities.Permitted) {
			t.Errorf("permitted capabilities were not dropped correctly.")
			return false
		}

		if !areStringArraysEqual(actual.Ambient, tc.capabilities.Ambient) {
			t.Errorf("ambient capabilities were not dropped correctly.")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceCreateContainer_Capabilities_Drop: %v", err)
	}
}

func Test_Rego_EnforceCreateContainer_Capabilities_Drop_NoMatches(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		p.allowCapabilityDropping = true
		tc, err := setupSimpleRegoCreateContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		capabilities := generateCapabilities(testRand)

		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, capabilities, tc.seccomp)

		if err == nil {
			t.Errorf("Unexpected success with non matching capabilities set and dropping")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceCreateContainer_Capabilities_Drop_NoMatches: %v", err)
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

		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

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

		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

		// not getting an error means something is broken
		if err == nil {
			t.Error("We added additional mounts not in policyS and it didn't result in an error")
			return false
		}

		return assertDecisionJSONContains(t, err, "invalid mount list")
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

		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

		// not getting an error means something is broken
		if err == nil {
			return false
		}

		return assertDecisionJSONContains(t, err, "invalid mount list")
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

		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

		// not getting an error means something is broken
		if err == nil {
			return false
		}

		return assertDecisionJSONContains(t, err, "invalid mount list")
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

		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

		// not getting an error means something is broken
		if err == nil {
			return false
		}

		return assertDecisionJSONContains(t, err, "invalid mount list")
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

		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

		// not getting an error means something is broken
		if err == nil {
			return false
		}

		return assertDecisionJSONContains(t, err, "invalid mount list")
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

		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

		// not getting an error means something is broken
		if err == nil {
			t.Error("We changed a mount option and it didn't result in an error")
			return false
		}

		return assertDecisionJSONContains(t, err, "invalid mount list")
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

		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

		// not getting an error means something is broken
		if err == nil {
			t.Error("We tried to mount a privileged mount when not allowed and it didn't result in an error")
			return false
		}

		return assertDecisionJSONContains(t, err, "invalid mount list")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_MountPolicy_BadOption: %v", err)
	}
}

// Tests whether an error is raised if support information is requested for
// an enforcement point which does not have stored version information.
func Test_Rego_Version_Unregistered_Enforcement_Point(t *testing.T) {
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints)
	securityPolicy := gc.toPolicy()
	policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{}, testOSType)

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
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints)
	securityPolicy := gc.toPolicy()
	policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{}, testOSType)

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
	code := "package policy\n\napi_version := \"0.0.1\""
	policy, err := newRegoPolicy(code, []oci.Mount{}, []oci.Mount{}, testOSType)

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

	allowed, err := info.defaultResults.Bool("allowed")

	if err != nil {
		t.Error(err)
	}

	if !allowed {
		t.Error("default behavior was incorrect for unavailable enforcement point")
	}
}

func Test_Rego_Enforcement_Point_Allowed(t *testing.T) {
	code := "package policy\n\napi_version := \"0.0.1\""
	policy, err := newRegoPolicy(code, []oci.Mount{}, []oci.Mount{}, testOSType)

	if err != nil {
		t.Fatalf("unable to create a new Rego policy: %v", err)
	}

	err = policy.injectTestAPI()
	if err != nil {
		t.Fatal(err)
	}

	input := make(map[string]interface{})
	results, err := policy.applyDefaults("__fixture_for_allowed_test_false__", input)
	if err != nil {
		t.Fatalf("applied defaults for an enforcement point receieved an error: %v", err)
	}

	allowed, err := results.Bool("allowed")

	if err != nil {
		t.Error(err)
	}

	if allowed {
		t.Fatal("result of allowed for an available enforcement point was not the specified default (false)")
	}

	input = make(map[string]interface{})
	results, err = policy.applyDefaults("__fixture_for_allowed_test_true__", input)
	if err != nil {
		t.Fatalf("applied defaults for an enforcement point receieved an error: %v", err)
	}

	allowed, err = results.Bool("allowed")

	if err != nil {
		t.Error(err)
	}

	if !allowed {
		t.Error("result of allowed for an available enforcement point was not the specified default (true)")
	}
}

func Test_Rego_Enforcement_Point_Extra(t *testing.T) {
	ctx := context.Background()
	code := `package policy

api_version := "0.0.1"

__fixture_for_allowed_extra__ := {"allowed": true}
`
	policy, err := newRegoPolicy(code, []oci.Mount{}, []oci.Mount{}, testOSType)

	if err != nil {
		t.Fatalf("unable to create a new Rego policy: %v", err)
	}

	err = policy.injectTestAPI()
	if err != nil {
		t.Fatal(err)
	}

	input := make(map[string]interface{})
	results, err := policy.enforce(ctx, "__fixture_for_allowed_extra__", input)
	if err != nil {
		t.Fatalf("enforcement produced an error: %v", err)
	}

	allowed, err := results.Bool("allowed")

	if err != nil {
		t.Error(err)
	}

	if !allowed {
		t.Error("result of allowed for an available enforcement point was not the policy value (true)")
	}

	if extra, ok := results["__test__"]; ok {
		if extra != "test" {
			t.Errorf("extra value was not specified default: %s != test", extra)
		}
	} else {
		t.Error("extra value is missing from enforcement result")
	}
}

func Test_Rego_No_API_Version(t *testing.T) {
	code := "package policy"
	policy, err := newRegoPolicy(code, []oci.Mount{}, []oci.Mount{}, testOSType)

	if err != nil {
		t.Fatalf("unable to create a new Rego policy: %v", err)
	}

	err = policy.injectTestAPI()
	if err != nil {
		t.Fatal(err)
	}

	_, err = policy.queryEnforcementPoint("__fixture_for_allowed_test_true__")

	if err == nil {
		t.Error("querying an enforcement point without an api_version did not produce an error")
	}

	if err.Error() != noAPIVersionError {
		t.Errorf("querying an enforcement point without an api_version produced an incorrect error: %s", err)
	}
}

func Test_Rego_ExecInContainerPolicy(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoRunningContainerTest(p, false)
		if err != nil {
			t.Error(err)
			return false
		}

		container := selectContainerFromRunningContainers(tc.runningContainers, testRand)
		capabilities := container.container.Capabilities.toExternal()

		process := selectExecProcess(container.container.ExecProcesses, testRand)
		envList := buildEnvironmentVariablesFromEnvRules(container.container.EnvRules, testRand)
		user := buildIDNameFromConfig(container.container.User.UserIDName, testRand)
		groups := buildGroupIDNamesFromUser(container.container.User, testRand)
		umask := container.container.User.Umask

		_, _, _, err = tc.policy.EnforceExecInContainerPolicy(p.ctx, container.containerID, process.Command, envList, container.container.WorkingDir, container.container.NoNewPrivileges, user, groups, umask, &capabilities)

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
		tc, err := setupRegoRunningContainerTest(p, false)
		if err != nil {
			t.Error(err)
			return false
		}

		container := selectContainerFromRunningContainers(tc.runningContainers, testRand)
		capabilities := container.container.Capabilities.toExternal()

		process := generateContainerExecProcess(testRand)
		envList := buildEnvironmentVariablesFromEnvRules(container.container.EnvRules, testRand)
		user := buildIDNameFromConfig(container.container.User.UserIDName, testRand)
		groups := buildGroupIDNamesFromUser(container.container.User, testRand)
		umask := container.container.User.Umask

		_, _, _, err = tc.policy.EnforceExecInContainerPolicy(p.ctx, container.containerID, process.Command, envList, container.container.WorkingDir, container.container.NoNewPrivileges, user, groups, umask, &capabilities)
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
		tc, err := setupRegoRunningContainerTest(p, false)
		if err != nil {
			t.Error(err)
			return false
		}

		container := selectContainerFromRunningContainers(tc.runningContainers, testRand)
		envList := buildEnvironmentVariablesFromEnvRules(container.container.EnvRules, testRand)

		user := buildIDNameFromConfig(container.container.User.UserIDName, testRand)
		groups := buildGroupIDNamesFromUser(container.container.User, testRand)
		umask := container.container.User.Umask
		capabilities := container.container.Capabilities.toExternal()

		command := generateCommand(testRand)
		_, _, _, err = tc.policy.EnforceExecInContainerPolicy(p.ctx, container.containerID, command, envList, container.container.WorkingDir, container.container.NoNewPrivileges, user, groups, umask, &capabilities)

		// not getting an error means something is broken
		if err == nil {
			t.Error("Unexpected success when enforcing policy")
			return false
		}

		return assertDecisionJSONContains(t, err, "invalid command")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_ExecInContainerPolicy_Command_No_Match: %v", err)
	}
}

func Test_Rego_ExecInContainerPolicy_Some_Env_Not_Allowed(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoRunningContainerTest(p, false)
		if err != nil {
			t.Error(err)
			return false
		}

		container := selectContainerFromRunningContainers(tc.runningContainers, testRand)
		process := selectExecProcess(container.container.ExecProcesses, testRand)
		envList := generateEnvironmentVariables(testRand)
		user := buildIDNameFromConfig(container.container.User.UserIDName, testRand)
		groups := buildGroupIDNamesFromUser(container.container.User, testRand)
		umask := container.container.User.Umask
		capabilities := container.container.Capabilities.toExternal()

		_, _, _, err = tc.policy.EnforceExecInContainerPolicy(p.ctx, container.containerID, process.Command, envList, container.container.WorkingDir, container.container.NoNewPrivileges, user, groups, umask, &capabilities)

		// not getting an error means something is broken
		if err == nil {
			t.Error("Unexpected success when enforcing policy")
			return false
		}

		return assertDecisionJSONContains(t, err, "invalid env list")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_ExecInContainerPolicy_Some_Env_Not_Allowed: %v", err)
	}
}

func Test_Rego_ExecInContainerPolicy_WorkingDir_No_Match(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoRunningContainerTest(p, false)
		if err != nil {
			t.Error(err)
			return false
		}

		container := selectContainerFromRunningContainers(tc.runningContainers, testRand)
		process := selectExecProcess(container.container.ExecProcesses, testRand)
		envList := buildEnvironmentVariablesFromEnvRules(container.container.EnvRules, testRand)
		workingDir := generateWorkingDir(testRand)
		user := buildIDNameFromConfig(container.container.User.UserIDName, testRand)
		groups := buildGroupIDNamesFromUser(container.container.User, testRand)
		umask := container.container.User.Umask
		capabilities := container.container.Capabilities.toExternal()

		_, _, _, err = tc.policy.EnforceExecInContainerPolicy(p.ctx, container.containerID, process.Command, envList, workingDir, container.container.NoNewPrivileges, user, groups, umask, &capabilities)

		// not getting an error means something is broken
		if err == nil {
			t.Error("Unexpected success when enforcing policy")
			return false
		}

		return assertDecisionJSONContains(t, err, "invalid working directory")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_ExecInContainerPolicy_WorkingDir_No_Match: %v", err)
	}
}

func Test_Rego_ExecInContainerPolicy_Capabilities_No_Match(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoRunningContainerTest(p, false)
		if err != nil {
			t.Error(err)
			return false
		}

		container := selectContainerFromRunningContainers(tc.runningContainers, testRand)
		process := selectExecProcess(container.container.ExecProcesses, testRand)
		envList := buildEnvironmentVariablesFromEnvRules(container.container.EnvRules, testRand)
		user := buildIDNameFromConfig(container.container.User.UserIDName, testRand)
		groups := buildGroupIDNamesFromUser(container.container.User, testRand)
		umask := container.container.User.Umask

		capabilities := copyLinuxCapabilities(container.container.Capabilities.toExternal())
		capabilities.Bounding = superCapabilitySet(testRand, capabilities.Bounding)

		_, _, _, err = tc.policy.EnforceExecInContainerPolicy(p.ctx, container.containerID, process.Command, envList, container.container.WorkingDir, container.container.NoNewPrivileges, user, groups, umask, &capabilities)

		// not getting an error means something is broken
		if err == nil {
			t.Error("Unexpected success with bounding as a superset of allowed capabilities")
			return false
		}

		if !assertDecisionJSONContains(t, err, "capabilities don't match") {
			t.Errorf("Didn't find expected error message\n%v\n", err)
			return false
		}

		capabilities = copyLinuxCapabilities(container.container.Capabilities.toExternal())
		capabilities.Effective = superCapabilitySet(testRand, capabilities.Effective)

		_, _, _, err = tc.policy.EnforceExecInContainerPolicy(p.ctx, container.containerID, process.Command, envList, container.container.WorkingDir, container.container.NoNewPrivileges, user, groups, umask, &capabilities)

		// not getting an error means something is broken
		if err == nil {
			t.Error("Unexpected success with effective as a superset of allowed capabilities")
			return false
		}

		if !assertDecisionJSONContains(t, err, "capabilities don't match") {
			t.Errorf("Didn't find expected error message\n%v\n", err)
			return false
		}

		capabilities = copyLinuxCapabilities(container.container.Capabilities.toExternal())
		capabilities.Inheritable = superCapabilitySet(testRand, capabilities.Inheritable)

		_, _, _, err = tc.policy.EnforceExecInContainerPolicy(p.ctx, container.containerID, process.Command, envList, container.container.WorkingDir, container.container.NoNewPrivileges, user, groups, umask, &capabilities)

		// not getting an error means something is broken
		if err == nil {
			t.Error("Unexpected success with inheritable as a superset of allowed capabilities")
			return false
		}

		if !assertDecisionJSONContains(t, err, "capabilities don't match") {
			t.Errorf("Didn't find expected error message\n%v\n", err)
			return false
		}

		capabilities = copyLinuxCapabilities(container.container.Capabilities.toExternal())
		capabilities.Permitted = superCapabilitySet(testRand, capabilities.Permitted)

		_, _, _, err = tc.policy.EnforceExecInContainerPolicy(p.ctx, container.containerID, process.Command, envList, container.container.WorkingDir, container.container.NoNewPrivileges, user, groups, umask, &capabilities)

		// not getting an error means something is broken
		if err == nil {
			t.Error("Unexpected success with permitted as a superset of allowed capabilities")
			return false
		}

		if !assertDecisionJSONContains(t, err, "capabilities don't match") {
			t.Errorf("Didn't find expected error message\n%v\n", err)
			return false
		}

		capabilities = copyLinuxCapabilities(container.container.Capabilities.toExternal())
		capabilities.Ambient = superCapabilitySet(testRand, capabilities.Ambient)

		_, _, _, err = tc.policy.EnforceExecInContainerPolicy(p.ctx, container.containerID, process.Command, envList, container.container.WorkingDir, container.container.NoNewPrivileges, user, groups, umask, &capabilities)

		// not getting an error means something is broken
		if err == nil {
			t.Error("Unexpected success with ambient as a superset of allowed capabilities")
			return false
		}

		if !assertDecisionJSONContains(t, err, "capabilities don't match") {
			t.Errorf("Didn't find expected error message\n%v\n", err)
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_ExecInContainerPolicy_Capabilities_No_Match: %v", err)
	}
}

func Test_Rego_ExecInContainerPolicy_CapabilitiesIsNil(t *testing.T) {
	constraints := generateConstraints(testRand, 1)
	tc, err := setupRegoRunningContainerTest(constraints, false)
	if err != nil {
		t.Fatal(err)
	}

	container := selectContainerFromRunningContainers(tc.runningContainers, testRand)
	process := selectExecProcess(container.container.ExecProcesses, testRand)
	envList := buildEnvironmentVariablesFromEnvRules(container.container.EnvRules, testRand)
	user := buildIDNameFromConfig(container.container.User.UserIDName, testRand)
	groups := buildGroupIDNamesFromUser(container.container.User, testRand)
	umask := container.container.User.Umask

	_, _, _, err = tc.policy.EnforceExecInContainerPolicy(constraints.ctx, container.containerID, process.Command, envList, container.container.WorkingDir, container.container.NoNewPrivileges, user, groups, umask, nil)

	if err == nil {
		t.Fatal("Unexpected success with nil capabilities")
	}

	if err.Error() != capabilitiesNilError {
		t.Fatal("No error message given for denial by capability being nil")
	}
}

func Test_Rego_ExecInContainerPolicy_CapabilitiesAreEmpty(t *testing.T) {
	constraints := generateConstraints(testRand, 1)
	constraints.containers[0].Capabilities.Bounding = make([]string, 0)
	constraints.containers[0].Capabilities.Effective = make([]string, 0)
	constraints.containers[0].Capabilities.Inheritable = make([]string, 0)
	constraints.containers[0].Capabilities.Permitted = make([]string, 0)
	constraints.containers[0].Capabilities.Ambient = make([]string, 0)

	tc, err := setupRegoRunningContainerTest(constraints, false)
	if err != nil {
		t.Fatal(err)
	}

	capabilities := oci.LinuxCapabilities{}

	container := selectContainerFromRunningContainers(tc.runningContainers, testRand)
	process := selectExecProcess(container.container.ExecProcesses, testRand)
	envList := buildEnvironmentVariablesFromEnvRules(container.container.EnvRules, testRand)
	user := buildIDNameFromConfig(container.container.User.UserIDName, testRand)
	groups := buildGroupIDNamesFromUser(container.container.User, testRand)
	umask := container.container.User.Umask

	_, _, _, err = tc.policy.EnforceExecInContainerPolicy(constraints.ctx, container.containerID, process.Command, envList, container.container.WorkingDir, container.container.NoNewPrivileges, user, groups, umask, &capabilities)

	if err != nil {
		t.Fatal("Unexpected failure")
	}
}

func Test_Rego_ExecInContainerPolicy_Capabilities_Drop(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		p.allowCapabilityDropping = true
		tc, err := setupRegoRunningContainerTest(p, false)
		if err != nil {
			t.Error(err)
			return false
		}

		container := selectContainerFromRunningContainers(tc.runningContainers, testRand)
		process := selectExecProcess(container.container.ExecProcesses, testRand)
		envList := buildEnvironmentVariablesFromEnvRules(container.container.EnvRules, testRand)
		user := buildIDNameFromConfig(container.container.User.UserIDName, testRand)
		groups := buildGroupIDNamesFromUser(container.container.User, testRand)
		umask := container.container.User.Umask

		capabilities := copyLinuxCapabilities(container.container.Capabilities.toExternal())
		capabilities.Bounding = superCapabilitySet(testRand, capabilities.Bounding)

		extraCapabilities := generateCapabilities(testRand)
		capabilities.Bounding = append(capabilities.Bounding, extraCapabilities.Bounding...)
		capabilities.Effective = append(capabilities.Effective, extraCapabilities.Effective...)
		capabilities.Inheritable = append(capabilities.Inheritable, extraCapabilities.Inheritable...)
		capabilities.Permitted = append(capabilities.Permitted, extraCapabilities.Permitted...)
		capabilities.Ambient = append(capabilities.Ambient, extraCapabilities.Ambient...)

		_, actual, _, err := tc.policy.EnforceExecInContainerPolicy(p.ctx, container.containerID, process.Command, envList, container.container.WorkingDir, container.container.NoNewPrivileges, user, groups, umask, &capabilities)

		if err != nil {
			t.Errorf("Expected exec in container to be allowed. It wasn't for extra capabilities: %v", err)
			return false
		}

		if !areStringArraysEqual(actual.Bounding, container.container.Capabilities.Bounding) {
			t.Errorf("bounding capabilities were not dropped correctly.")
			return false
		}

		if !areStringArraysEqual(actual.Effective, container.container.Capabilities.Effective) {
			t.Errorf("effective capabilities were not dropped correctly.")
			return false
		}

		if !areStringArraysEqual(actual.Inheritable, container.container.Capabilities.Inheritable) {
			t.Errorf("inheritable capabilities were not dropped correctly.")
			return false
		}

		if !areStringArraysEqual(actual.Permitted, container.container.Capabilities.Permitted) {
			t.Errorf("permitted capabilities were not dropped correctly.")
			return false
		}

		if !areStringArraysEqual(actual.Ambient, container.container.Capabilities.Ambient) {
			t.Errorf("ambient capabilities were not dropped correctly.")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_ExecInContainerPolicy_Capabilities_Drop: %v", err)
	}
}

func Test_Rego_ExecInContainerPolicy_DropEnvs(t *testing.T) {
	testFunc := func(gc *generatedConstraints) bool {
		gc.allowEnvironmentVariableDropping = true
		tc, err := setupRegoRunningContainerTest(gc, false)
		if err != nil {
			t.Error(err)
			return false
		}

		container := selectContainerFromRunningContainers(tc.runningContainers, testRand)
		capabilities := container.container.Capabilities.toExternal()

		process := selectExecProcess(container.container.ExecProcesses, testRand)
		expected := buildEnvironmentVariablesFromEnvRules(container.container.EnvRules, testRand)

		extraRules := generateEnvironmentVariableRules(testRand)
		extraEnvs := buildEnvironmentVariablesFromEnvRules(extraRules, testRand)

		envList := append(expected, extraEnvs...)
		user := buildIDNameFromConfig(container.container.User.UserIDName, testRand)
		groups := buildGroupIDNamesFromUser(container.container.User, testRand)
		umask := container.container.User.Umask

		actual, _, _, err := tc.policy.EnforceExecInContainerPolicy(gc.ctx, container.containerID, process.Command, envList, container.container.WorkingDir, container.container.NoNewPrivileges, user, groups, umask, &capabilities)

		if err != nil {
			t.Errorf("expected exec in container process to be allowed. It wasn't: %v", err)
			return false
		}

		if !areStringArraysEqual(actual, expected) {
			t.Errorf("environment variables were not dropped correctly.")
			return false
		}

		return true
	}

	if err := quick.Check(testFunc, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_ExecInContainerPolicy_DropEnvs: %v", err)
	}
}

func Test_Rego_MaliciousEnvList(t *testing.T) {
	template := `package policy
create_container := {
	"allowed": true,
	"env_list": ["%s"]
}

exec_in_container := {
	"allowed": true,
	"env_list": ["%s"]
}

exec_external := {
	"allowed": true,
	"env_list": ["%s"]
}`

	generateEnv := func(r *rand.Rand) string {
		return randVariableString(r, maxGeneratedEnvironmentVariableRuleLength)
	}

	generateEnvs := func(envSet stringSet) []string {
		numVars := atLeastOneAtMost(testRand, maxGeneratedEnvironmentVariableRules)
		return envSet.randUniqueArray(testRand, generateEnv, numVars)
	}

	testFunc := func(gc *generatedConstraints) bool {
		envSet := make(stringSet)
		rego := fmt.Sprintf(
			template,
			strings.Join(generateEnvs(envSet), `","`),
			strings.Join(generateEnvs(envSet), `","`),
			strings.Join(generateEnvs(envSet), `","`))

		policy, err := newRegoPolicy(rego, []oci.Mount{}, []oci.Mount{}, testOSType)

		if err != nil {
			t.Errorf("error creating policy: %v", err)
			return false
		}

		user := generateIDName(testRand)
		capabilities := &oci.LinuxCapabilities{}
		seccomp := ""

		envList := generateEnvs(envSet)
		toKeep, _, _, err := policy.EnforceCreateContainerPolicy(gc.ctx, "", "", []string{}, envList, "", []oci.Mount{}, false, true, user, nil, "", capabilities, seccomp)
		if len(toKeep) > 0 {
			t.Error("invalid environment variables not filtered from list returned from create_container")
			return false
		}

		envList = generateEnvs(envSet)
		toKeep, _, _, err = policy.EnforceExecInContainerPolicy(gc.ctx, "", []string{}, envList, "", true, user, nil, "", capabilities)
		if len(toKeep) > 0 {
			t.Error("invalid environment variables not filtered from list returned from exec_in_container")
			return false
		}

		envList = generateEnvs(envSet)
		toKeep, _, err = policy.EnforceExecExternalProcessPolicy(gc.ctx, []string{}, envList, "")
		if len(toKeep) > 0 {
			t.Error("invalid environment variables not filtered from list returned from exec_external")
			return false
		}

		return true
	}

	if err := quick.Check(testFunc, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_MaliciousEnvList: %v", err)
	}
}

func Test_Rego_InvalidEnvList(t *testing.T) {
	rego := fmt.Sprintf(`package policy
	api_version := "%s"
	framework_version := "%s"

	create_container := {
		"allowed": true,
		"env_list": {"an_object": 1}
	}
	exec_in_container := {
		"allowed": true,
		"env_list": "string"
	}
	exec_external := {
		"allowed": true,
		"env_list": true
	}`, apiVersion, frameworkVersion)

	policy, err := newRegoPolicy(rego, []oci.Mount{}, []oci.Mount{}, testOSType)

	if err != nil {
		t.Fatalf("error creating policy: %v", err)
	}

	ctx := context.Background()
	user := generateIDName(testRand)
	capabilities := &oci.LinuxCapabilities{}
	seccomp := ""
	_, _, _, err = policy.EnforceCreateContainerPolicy(ctx, "", "", []string{}, []string{}, "", []oci.Mount{}, false, true, user, nil, "", capabilities, seccomp)
	if err == nil {
		t.Errorf("expected call to create_container to fail")
	} else if err.Error() != "policy returned incorrect type for 'env_list', expected []interface{}, received map[string]interface {}" {
		t.Errorf("incorrected error message from call to create_container: %v", err)
	}

	_, _, _, err = policy.EnforceExecInContainerPolicy(ctx, "", []string{}, []string{}, "", true, user, nil, "", capabilities)
	if err == nil {
		t.Errorf("expected call to exec_in_container to fail")
	} else if err.Error() != "policy returned incorrect type for 'env_list', expected []interface{}, received string" {
		t.Errorf("incorrected error message from call to exec_in_container: %v", err)
	}

	_, _, err = policy.EnforceExecExternalProcessPolicy(ctx, []string{}, []string{}, "")
	if err == nil {
		t.Errorf("expected call to exec_external to fail")
	} else if err.Error() != "policy returned incorrect type for 'env_list', expected []interface{}, received bool" {
		t.Errorf("incorrected error message from call to exec_external: %v", err)
	}
}

func Test_Rego_InvalidEnvList_Member(t *testing.T) {
	rego := fmt.Sprintf(`package policy
	api_version := "%s"
	framework_version := "%s"

	create_container := {
		"allowed": true,
		"env_list": ["one", "two", 3]
	}
	exec_in_container := {
		"allowed": true,
		"env_list": ["one", true, "three"]
	}
	exec_external := {
		"allowed": true,
		"env_list": ["one", ["two"], "three"]
	}`, apiVersion, frameworkVersion)

	policy, err := newRegoPolicy(rego, []oci.Mount{}, []oci.Mount{}, testOSType)

	if err != nil {
		t.Fatalf("error creating policy: %v", err)
	}

	ctx := context.Background()
	user := generateIDName(testRand)
	capabilities := &oci.LinuxCapabilities{}
	seccomp := ""

	_, _, _, err = policy.EnforceCreateContainerPolicy(ctx, "", "", []string{}, []string{}, "", []oci.Mount{}, false, true, user, nil, "", capabilities, seccomp)
	if err == nil {
		t.Errorf("expected call to create_container to fail")
	} else if err.Error() != "members of env_list from policy must be strings, received json.Number" {
		t.Errorf("incorrected error message from call to create_container: %v", err)
	}

	_, _, _, err = policy.EnforceExecInContainerPolicy(ctx, "", []string{}, []string{}, "", true, user, nil, "", capabilities)
	if err == nil {
		t.Errorf("expected call to exec_in_container to fail")
	} else if err.Error() != "members of env_list from policy must be strings, received bool" {
		t.Errorf("incorrected error message from call to exec_in_container: %v", err)
	}

	_, _, err = policy.EnforceExecExternalProcessPolicy(ctx, []string{}, []string{}, "")
	if err == nil {
		t.Errorf("expected call to exec_external to fail")
	} else if err.Error() != "members of env_list from policy must be strings, received []interface {}" {
		t.Errorf("incorrected error message from call to exec_external: %v", err)
	}
}

func Test_Rego_EnforceEnvironmentVariablePolicy_MissingRequired(t *testing.T) {
	testFunc := func(gc *generatedConstraints) bool {
		container := selectContainerFromContainerList(gc.containers, testRand)
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

		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(gc.ctx, tc.sandboxID, tc.containerID, tc.argList, envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

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

		_, _, err = tc.policy.EnforceExecExternalProcessPolicy(p.ctx, process.command, envList, process.workingDir)
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

		_, _, err = tc.policy.EnforceExecExternalProcessPolicy(p.ctx, process.command, envList, process.workingDir)
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

		_, _, err = tc.policy.EnforceExecExternalProcessPolicy(p.ctx, command, envList, process.workingDir)
		if err == nil {
			t.Error("Policy was unexpectedly not enforced")
			return false
		}

		return assertDecisionJSONContains(t, err, "invalid command")
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

		_, _, err = tc.policy.EnforceExecExternalProcessPolicy(p.ctx, process.command, envList, process.workingDir)
		if err == nil {
			t.Error("Policy was unexpectedly not enforced")
			return false
		}

		return assertDecisionJSONContains(t, err, "invalid env list")
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

		_, _, err = tc.policy.EnforceExecExternalProcessPolicy(p.ctx, process.command, envList, workingDir)
		if err == nil {
			t.Error("Policy was unexpectedly not enforced")
			return false
		}

		return assertDecisionJSONContains(t, err, "invalid working directory")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_ExecExternalProcessPolicy_WorkingDir_No_Match: %v", err)
	}
}

func Test_Rego_ExecExternalProcessPolicy_DropEnvs(t *testing.T) {
	testFunc := func(gc *generatedConstraints) bool {
		gc.allowEnvironmentVariableDropping = true
		tc, err := setupExternalProcessTest(gc)
		if err != nil {
			t.Error(err)
			return false
		}

		process := selectExternalProcessFromConstraints(gc, testRand)
		expected := buildEnvironmentVariablesFromEnvRules(process.envRules, testRand)

		extraRules := generateEnvironmentVariableRules(testRand)
		extraEnvs := buildEnvironmentVariablesFromEnvRules(extraRules, testRand)

		envList := append(expected, extraEnvs...)

		actual, _, err := tc.policy.EnforceExecExternalProcessPolicy(gc.ctx, process.command, envList, process.workingDir)

		if err != nil {
			t.Errorf("expected exec in container process to be allowed. It wasn't: %v", err)
			return false
		}

		if !areStringArraysEqual(actual, expected) {
			t.Errorf("environment variables were not dropped correctly.")
			return false
		}

		return true
	}

	if err := quick.Check(testFunc, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_ExecExternalProcessPolicy_DropEnvs: %v", err)
	}
}

func Test_Rego_ExecExternalProcessPolicy_DropEnvs_Multiple(t *testing.T) {
	envRules := setupEnvRuleSets(3)

	gc := generateConstraints(testRand, 1)
	gc.allowEnvironmentVariableDropping = true
	process0 := generateExternalProcess(testRand)

	process1 := process0.clone()
	process1.envRules = append(envRules[0], envRules[1]...)

	process2 := process0.clone()
	process2.envRules = append(process1.envRules, envRules[2]...)

	gc.externalProcesses = []*externalProcess{process0, process1, process2}
	securityPolicy := gc.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	policy, err := newRegoPolicy(securityPolicy.marshalRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts),
		testOSType)
	if err != nil {
		t.Fatal(err)
	}

	envs0 := buildEnvironmentVariablesFromEnvRules(envRules[0], testRand)
	envs1 := buildEnvironmentVariablesFromEnvRules(envRules[1], testRand)
	envs2 := buildEnvironmentVariablesFromEnvRules(envRules[2], testRand)
	envList := append(envs0, envs1...)
	envList = append(envList, envs2...)

	actual, _, err := policy.EnforceExecExternalProcessPolicy(gc.ctx, process2.command, envList, process2.workingDir)

	// getting an error means something is broken
	if err != nil {
		t.Errorf("Expected container creation to be allowed. It wasn't: %v", err)
	}

	if !areStringArraysEqual(actual, envList) {
		t.Error("environment variables were not dropped correctly.")
	}
}

func Test_Rego_ExecExternalProcessPolicy_DropEnvs_Multiple_NoMatch(t *testing.T) {
	envRules := setupEnvRuleSets(3)

	gc := generateConstraints(testRand, 1)
	gc.allowEnvironmentVariableDropping = true

	process0 := generateExternalProcess(testRand)

	process1 := process0.clone()
	process1.envRules = append(envRules[0], envRules[1]...)

	process2 := process0.clone()
	process2.envRules = append(envRules[0], envRules[2]...)

	gc.externalProcesses = []*externalProcess{process0, process1, process2}
	securityPolicy := gc.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	policy, err := newRegoPolicy(securityPolicy.marshalRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts),
		testOSType)
	if err != nil {
		t.Fatal(err)
	}

	envs0 := buildEnvironmentVariablesFromEnvRules(envRules[0], testRand)
	envs1 := buildEnvironmentVariablesFromEnvRules(envRules[1], testRand)
	envs2 := buildEnvironmentVariablesFromEnvRules(envRules[2], testRand)
	var extraLen int
	if len(envs1) > len(envs2) {
		extraLen = len(envs2)
	} else {
		extraLen = len(envs1)
	}
	envList := append(envs0, envs1[:extraLen]...)
	envList = append(envList, envs2[:extraLen]...)

	actual, _, err := policy.EnforceExecExternalProcessPolicy(gc.ctx, process2.command, envList, process2.workingDir)

	// not getting an error means something is broken
	if err == nil {
		t.Error("expected container creation to not be allowed.")
	}

	if actual != nil {
		t.Error("envList should be nil.")
	}
}

func Test_Rego_ShutdownContainerPolicy_Running_Container(t *testing.T) {
	p := generateConstraints(testRand, maxContainersInGeneratedConstraints)

	tc, err := setupRegoRunningContainerTest(p, false)
	if err != nil {
		t.Fatalf("Unable to set up test: %v", err)
	}

	container := selectContainerFromRunningContainers(tc.runningContainers, testRand)

	err = tc.policy.EnforceShutdownContainerPolicy(p.ctx, container.containerID)
	if err != nil {
		t.Fatal("Expected shutdown of running container to be allowed, it wasn't")
	}
}

func Test_Rego_ShutdownContainerPolicy_Not_Running_Container(t *testing.T) {
	p := generateConstraints(testRand, maxContainersInGeneratedConstraints)

	tc, err := setupRegoRunningContainerTest(p, false)
	if err != nil {
		t.Fatalf("Unable to set up test: %v", err)
	}

	notRunningContainerID := testDataGenerator.uniqueContainerID()

	err = tc.policy.EnforceShutdownContainerPolicy(p.ctx, notRunningContainerID)
	if err == nil {
		t.Fatal("Expected shutdown of not running container to be denied, it wasn't")
	}
}

func Test_Rego_SignalContainerProcessPolicy_InitProcess_Allowed(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		hasAllowedSignals := generateConstraintsContainer(testRand, 1, maxLayersInGeneratedContainer)
		hasAllowedSignals.Signals = generateListOfSignals(testRand, 1, maxSignalNumber)
		p.containers = append(p.containers, hasAllowedSignals)

		tc, err := setupRegoRunningContainerTest(p, false)
		if err != nil {
			t.Error(err)
			return false
		}

		containerID, err := idForRunningContainer(hasAllowedSignals, tc.runningContainers)
		if err != nil {
			r, err := runContainer(tc.policy, hasAllowedSignals, tc.defaultMounts, tc.privilegedMounts, false)
			if err != nil {
				t.Errorf("Unable to setup test running container: %v", err)
				return false
			}
			containerID = r.containerID
		}

		signal := selectSignalFromSignals(testRand, hasAllowedSignals.Signals)
		err = tc.policy.EnforceSignalContainerProcessPolicy(p.ctx, containerID, signal, true, hasAllowedSignals.Command)

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

		tc, err := setupRegoRunningContainerTest(p, false)
		if err != nil {
			t.Error(err)
			return false
		}

		containerID, err := idForRunningContainer(hasNoAllowedSignals, tc.runningContainers)
		if err != nil {
			r, err := runContainer(tc.policy, hasNoAllowedSignals, tc.defaultMounts, tc.privilegedMounts, false)
			if err != nil {
				t.Errorf("Unable to setup test running container: %v", err)
				return false
			}
			containerID = r.containerID
		}

		signal := generateSignal(testRand)
		err = tc.policy.EnforceSignalContainerProcessPolicy(p.ctx, containerID, signal, true, hasNoAllowedSignals.Command)

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

		tc, err := setupRegoRunningContainerTest(p, false)
		if err != nil {
			t.Error(err)
			return false
		}

		_, err = idForRunningContainer(hasAllowedSignals, tc.runningContainers)
		if err != nil {
			_, err := runContainer(tc.policy, hasAllowedSignals, tc.defaultMounts, tc.privilegedMounts, false)
			if err != nil {
				t.Errorf("Unable to setup test running container: %v", err)
				return false
			}
		}

		signal := selectSignalFromSignals(testRand, hasAllowedSignals.Signals)
		badContainerID := generateContainerID(testRand)
		err = tc.policy.EnforceSignalContainerProcessPolicy(p.ctx, badContainerID, signal, true, hasAllowedSignals.Command)

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

		tc, err := setupRegoRunningContainerTest(p, false)
		if err != nil {
			t.Error(err)
			return false
		}

		containerID, err := idForRunningContainer(containerUnderTest, tc.runningContainers)
		if err != nil {
			r, err := runContainer(tc.policy, containerUnderTest, tc.defaultMounts, tc.privilegedMounts, false)
			if err != nil {
				t.Errorf("Unable to setup test running container: %v", err)
				return false
			}
			containerID = r.containerID
		}

		envList := buildEnvironmentVariablesFromEnvRules(containerUnderTest.EnvRules, testRand)
		user := buildIDNameFromConfig(containerUnderTest.User.UserIDName, testRand)
		groups := buildGroupIDNamesFromUser(containerUnderTest.User, testRand)
		umask := containerUnderTest.User.Umask
		capabilities := containerUnderTest.Capabilities.toExternal()

		_, _, _, err = tc.policy.EnforceExecInContainerPolicy(p.ctx, containerID, processUnderTest.Command, envList, containerUnderTest.WorkingDir, containerUnderTest.NoNewPrivileges, user, groups, umask, &capabilities)
		if err != nil {
			t.Errorf("Unable to exec process for test: %v", err)
			return false
		}

		signal := selectSignalFromSignals(testRand, processUnderTest.Signals)

		err = tc.policy.EnforceSignalContainerProcessPolicy(p.ctx, containerID, signal, false, processUnderTest.Command)
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

		tc, err := setupRegoRunningContainerTest(p, false)
		if err != nil {
			t.Error(err)
			return false
		}

		containerID, err := idForRunningContainer(containerUnderTest, tc.runningContainers)
		if err != nil {
			r, err := runContainer(tc.policy, containerUnderTest, tc.defaultMounts, tc.privilegedMounts, false)
			if err != nil {
				t.Errorf("Unable to setup test running container: %v", err)
				return false
			}
			containerID = r.containerID
		}

		envList := buildEnvironmentVariablesFromEnvRules(containerUnderTest.EnvRules, testRand)
		user := buildIDNameFromConfig(containerUnderTest.User.UserIDName, testRand)
		groups := buildGroupIDNamesFromUser(containerUnderTest.User, testRand)
		umask := containerUnderTest.User.Umask
		capabilities := containerUnderTest.Capabilities.toExternal()

		_, _, _, err = tc.policy.EnforceExecInContainerPolicy(p.ctx, containerID, processUnderTest.Command, envList, containerUnderTest.WorkingDir, containerUnderTest.NoNewPrivileges, user, groups, umask, &capabilities)
		if err != nil {
			t.Errorf("Unable to exec process for test: %v", err)
			return false
		}

		signal := generateSignal(testRand)

		err = tc.policy.EnforceSignalContainerProcessPolicy(p.ctx, containerID, signal, false, processUnderTest.Command)
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

		tc, err := setupRegoRunningContainerTest(p, false)
		if err != nil {
			t.Error(err)
			return false
		}

		containerID, err := idForRunningContainer(containerUnderTest, tc.runningContainers)
		if err != nil {
			r, err := runContainer(tc.policy, containerUnderTest, tc.defaultMounts, tc.privilegedMounts, false)
			if err != nil {
				t.Errorf("Unable to setup test running container: %v", err)
				return false
			}
			containerID = r.containerID
		}

		envList := buildEnvironmentVariablesFromEnvRules(containerUnderTest.EnvRules, testRand)
		user := buildIDNameFromConfig(containerUnderTest.User.UserIDName, testRand)
		groups := buildGroupIDNamesFromUser(containerUnderTest.User, testRand)
		umask := containerUnderTest.User.Umask
		capabilities := containerUnderTest.Capabilities.toExternal()

		_, _, _, err = tc.policy.EnforceExecInContainerPolicy(p.ctx, containerID, processUnderTest.Command, envList, containerUnderTest.WorkingDir, containerUnderTest.NoNewPrivileges, user, groups, umask, &capabilities)
		if err != nil {
			t.Errorf("Unable to exec process for test: %v", err)
			return false
		}

		signal := selectSignalFromSignals(testRand, processUnderTest.Signals)
		badCommand := generateCommand(testRand)

		err = tc.policy.EnforceSignalContainerProcessPolicy(p.ctx, containerID, signal, false, badCommand)
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

		tc, err := setupRegoRunningContainerTest(p, false)
		if err != nil {
			t.Error(err)
			return false
		}

		containerID, err := idForRunningContainer(containerUnderTest, tc.runningContainers)
		if err != nil {
			r, err := runContainer(tc.policy, containerUnderTest, tc.defaultMounts, tc.privilegedMounts, false)
			if err != nil {
				t.Errorf("Unable to setup test running container: %v", err)
				return false
			}
			containerID = r.containerID
		}

		envList := buildEnvironmentVariablesFromEnvRules(containerUnderTest.EnvRules, testRand)
		user := buildIDNameFromConfig(containerUnderTest.User.UserIDName, testRand)
		groups := buildGroupIDNamesFromUser(containerUnderTest.User, testRand)
		umask := containerUnderTest.User.Umask
		capabilities := containerUnderTest.Capabilities.toExternal()

		_, _, _, err = tc.policy.EnforceExecInContainerPolicy(p.ctx, containerID, processUnderTest.Command, envList, containerUnderTest.WorkingDir, containerUnderTest.NoNewPrivileges, user, groups, umask, &capabilities)
		if err != nil {
			t.Errorf("Unable to exec process for test: %v", err)
			return false
		}

		signal := selectSignalFromSignals(testRand, processUnderTest.Signals)
		badContainerID := generateContainerID(testRand)

		err = tc.policy.EnforceSignalContainerProcessPolicy(p.ctx, badContainerID, signal, false, processUnderTest.Command)
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
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints)

	tc, err := setupPlan9MountTest(gc)
	if err != nil {
		t.Fatalf("unable to setup test: %v", err)
	}

	err = tc.policy.EnforcePlan9MountPolicy(gc.ctx, tc.uvmPathForShare)
	if err != nil {
		t.Fatalf("Policy enforcement unexpectedly was denied: %v", err)
	}

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(
		gc.ctx,
		tc.sandboxID,
		tc.containerID,
		tc.argList,
		tc.envList,
		tc.workingDir,
		tc.mounts,
		false,
		tc.noNewPrivileges,
		tc.user,
		tc.groups,
		tc.umask,
		tc.capabilities,
		tc.seccomp,
	)

	if err != nil {
		t.Fatalf("Policy enforcement unexpectedly was denied: %v", err)
	}
}

func Test_Rego_Plan9MountPolicy_No_Matches(t *testing.T) {
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints)

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

	err = tc.policy.EnforcePlan9MountPolicy(gc.ctx, mount)
	if err != nil {
		t.Fatalf("Policy enforcement unexpectedly was denied: %v", err)
	}

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(
		gc.ctx,
		tc.sandboxID,
		tc.containerID,
		tc.argList,
		tc.envList,
		tc.workingDir,
		tc.mounts,
		false,
		tc.noNewPrivileges,
		tc.user,
		tc.groups,
		tc.umask,
		tc.capabilities,
		tc.seccomp,
	)

	if err == nil {
		t.Fatal("Policy enforcement unexpectedly was allowed")
	}
}

func Test_Rego_Plan9MountPolicy_Invalid(t *testing.T) {
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints)

	tc, err := setupPlan9MountTest(gc)
	if err != nil {
		t.Fatalf("unable to setup test: %v", err)
	}

	mount := randString(testRand, maxGeneratedMountSourceLength)
	err = tc.policy.EnforcePlan9MountPolicy(gc.ctx, mount)
	if err == nil {
		t.Fatal("Policy enforcement unexpectedly was allowed", err)
	}
}

func Test_Rego_Plan9UnmountPolicy(t *testing.T) {
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints)

	tc, err := setupPlan9MountTest(gc)
	if err != nil {
		t.Fatalf("unable to setup test: %v", err)
	}

	err = tc.policy.EnforcePlan9MountPolicy(gc.ctx, tc.uvmPathForShare)
	if err != nil {
		t.Fatalf("Couldn't mount as part of setup: %v", err)
	}

	err = tc.policy.EnforcePlan9UnmountPolicy(gc.ctx, tc.uvmPathForShare)
	if err != nil {
		t.Fatalf("Policy enforcement unexpectedly was denied: %v", err)
	}

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(
		gc.ctx,
		tc.sandboxID,
		tc.containerID,
		tc.argList,
		tc.envList,
		tc.workingDir,
		tc.mounts,
		false,
		tc.noNewPrivileges,
		tc.user,
		tc.groups,
		tc.umask,
		tc.capabilities,
		tc.seccomp,
	)

	if err == nil {
		t.Fatal("Policy enforcement unexpectedly was allowed")
	}
}

func Test_Rego_Plan9UnmountPolicy_No_Matches(t *testing.T) {
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints)

	tc, err := setupPlan9MountTest(gc)
	if err != nil {
		t.Fatalf("unable to setup test: %v", err)
	}

	mount := generateUVMPathForShare(testRand, tc.containerID)
	err = tc.policy.EnforcePlan9MountPolicy(gc.ctx, mount)
	if err != nil {
		t.Fatalf("Couldn't mount as part of setup: %v", err)
	}

	badMount := randString(testRand, maxPlan9MountTargetLength)
	err = tc.policy.EnforcePlan9UnmountPolicy(gc.ctx, badMount)
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

		err = tc.policy.EnforceGetPropertiesPolicy(constraints.ctx)
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

		err = tc.policy.EnforceGetPropertiesPolicy(constraints.ctx)
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

		err = tc.policy.EnforceDumpStacksPolicy(constraints.ctx)
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

		err = tc.policy.EnforceDumpStacksPolicy(constraints.ctx)
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

func Test_EnforceRuntimeLogging_Allowed(t *testing.T) {
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints)
	gc.allowRuntimeLogging = true

	tc, err := setupRegoPolicyOnlyTest(gc)
	if err != nil {
		t.Fatalf("unable to setup test: %v", err)
	}

	err = tc.policy.EnforceRuntimeLoggingPolicy(gc.ctx)
	if err != nil {
		t.Fatalf("Policy enforcement unexpectedly was denied: %v", err)
	}
}

func Test_EnforceRuntimeLogging_Not_Allowed(t *testing.T) {
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints)
	gc.allowRuntimeLogging = false

	tc, err := setupRegoPolicyOnlyTest(gc)
	if err != nil {
		t.Fatalf("unable to setup test: %v", err)
	}

	err = tc.policy.EnforceRuntimeLoggingPolicy(gc.ctx)
	if err == nil {
		t.Fatalf("Policy enforcement unexpectedly was allowed")
	}
}

func Test_Rego_LoadFragment_Container(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoFragmentTestConfigWithIncludes(p, []string{"containers"})
		if err != nil {
			t.Error(err)
			return false
		}

		fragment := tc.fragments[0]
		container := tc.containers[0]

		err = tc.policy.LoadFragment(p.ctx, fragment.info.issuer, fragment.info.feed, fragment.code)
		if err != nil {
			t.Error("unable to load fragment: %w", err)
			return false
		}

		containerID, err := mountImageForContainer(tc.policy, container.container)
		if err != nil {
			t.Error("unable to mount image for fragment container: %w", err)
			return false
		}

		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx,
			container.sandboxID,
			containerID,
			copyStrings(container.container.Command),
			copyStrings(container.envList),
			container.container.WorkingDir,
			copyMounts(container.mounts),
			false,
			container.container.NoNewPrivileges,
			container.user,
			container.groups,
			container.container.User.Umask,
			container.capabilities,
			container.seccomp,
		)

		if err != nil {
			t.Error("unable to create container from fragment: %w", err)
			return false
		}

		if tc.policy.rego.IsModuleActive(rpi.ModuleID(fragment.info.issuer, fragment.info.feed)) {
			t.Error("module not removed after load")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_LoadFragment_Container: %v", err)
	}
}

func Test_Rego_LoadFragment_Fragment(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoFragmentTestConfigWithIncludes(p, []string{"fragments"})
		if err != nil {
			t.Error(err)
			return false
		}

		fragment := tc.fragments[0]
		subFragment := tc.subFragments[0]

		err = tc.policy.LoadFragment(p.ctx, fragment.info.issuer, fragment.info.feed, fragment.code)
		if err != nil {
			t.Error("unable to load fragment: %w", err)
			return false
		}

		err = tc.policy.LoadFragment(p.ctx, subFragment.info.issuer, subFragment.info.feed, subFragment.code)
		if err != nil {
			t.Error("unable to load sub-fragment from fragment: %w", err)
			return false
		}

		container := selectContainerFromContainerList(subFragment.constraints.containers, testRand)
		_, err = mountImageForContainer(tc.policy, container)
		if err != nil {
			t.Error("unable to mount image for sub-fragment container: %w", err)
			return false
		}

		if tc.policy.rego.IsModuleActive(rpi.ModuleID(fragment.info.issuer, fragment.info.feed)) {
			t.Error("module not removed after load")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 15, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_LoadFragment_Fragment: %v", err)
	}
}

func Test_Rego_LoadFragment_ExternalProcess(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoFragmentTestConfigWithIncludes(p, []string{"external_processes"})
		if err != nil {
			t.Error(err)
			return false
		}

		fragment := tc.fragments[0]
		process := tc.externalProcesses[0]

		err = tc.policy.LoadFragment(p.ctx, fragment.info.issuer, fragment.info.feed, fragment.code)
		if err != nil {
			t.Error("unable to load fragment: %w", err)
			return false
		}

		envList := buildEnvironmentVariablesFromEnvRules(process.envRules, testRand)
		_, _, err = tc.policy.EnforceExecExternalProcessPolicy(p.ctx, process.command, envList, process.workingDir)
		if err != nil {
			t.Error("unable to execute external process from fragment: %w", err)
			return false
		}

		if tc.policy.rego.IsModuleActive(rpi.ModuleID(fragment.info.issuer, fragment.info.feed)) {
			t.Error("module not removed after load")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_LoadFragment_ExternalProcess: %v", err)
	}
}

func Test_Rego_LoadFragment_BadIssuer(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupSimpleRegoFragmentTestConfig(p)
		if err != nil {
			t.Error(err)
			return false
		}

		fragment := tc.fragments[0]
		issuer := testDataGenerator.uniqueFragmentIssuer()
		err = tc.policy.LoadFragment(p.ctx, issuer, fragment.info.feed, fragment.code)
		if err == nil {
			t.Error("expected to be unable to load fragment due to bad issuer")
			return false
		}

		if !assertDecisionJSONContains(t, err, "invalid fragment issuer") {
			t.Error("expected error string to contain 'invalid fragment issuer'")
			return false
		}

		if tc.policy.rego.IsModuleActive(rpi.ModuleID(issuer, fragment.info.feed)) {
			t.Error("module not removed upon failure")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_LoadFragment_BadIssuer: %v", err)
	}
}

func Test_Rego_LoadFragment_BadFeed(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupSimpleRegoFragmentTestConfig(p)
		if err != nil {
			t.Error(err)
			return false
		}

		fragment := tc.fragments[0]
		feed := testDataGenerator.uniqueFragmentFeed()
		err = tc.policy.LoadFragment(p.ctx, fragment.info.issuer, feed, fragment.code)
		if err == nil {
			t.Error("expected to be unable to load fragment due to bad feed")
			return false
		}

		if !assertDecisionJSONContains(t, err, "invalid fragment feed") {
			t.Error("expected error string to contain 'invalid fragment feed'")
			return false
		}

		if tc.policy.rego.IsModuleActive(rpi.ModuleID(fragment.info.issuer, feed)) {
			t.Error("module not removed upon failure")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_LoadFragment_BadFeed: %v", err)
	}
}

func Test_Rego_LoadFragment_InvalidSVN(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoFragmentSVNErrorTestConfig(p)
		if err != nil {
			t.Error(err)
			return false
		}

		fragment := tc.fragments[0]
		err = tc.policy.LoadFragment(p.ctx, fragment.info.issuer, fragment.info.feed, fragment.code)
		if err == nil {
			t.Error("expected to be unable to load fragment due to invalid svn")
			return false
		}

		if !assertDecisionJSONContains(t, err, "fragment svn is below the specified minimum") {
			t.Error("expected error string to contain 'fragment svn is below the specified minimum'")
			return false
		}

		if tc.policy.rego.IsModuleActive(rpi.ModuleID(fragment.info.issuer, fragment.info.feed)) {
			t.Error("module not removed upon failure")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_LoadFragment_InvalidSVN: %v", err)
	}
}

func Test_Rego_LoadFragment_Fragment_InvalidSVN(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoSubfragmentSVNErrorTestConfig(p)
		if err != nil {
			t.Error(err)
			return false
		}

		fragment := tc.fragments[0]
		err = tc.policy.LoadFragment(p.ctx, fragment.info.issuer, fragment.info.feed, fragment.code)
		if err != nil {
			t.Error("unable to load fragment: %w", err)
			return false
		}

		subFragment := tc.subFragments[0]
		err = tc.policy.LoadFragment(p.ctx, subFragment.info.issuer, subFragment.info.feed, subFragment.code)
		if err == nil {
			t.Error("expected to be unable to load subfragment due to invalid svn")
			return false
		}

		if !assertDecisionJSONContains(t, err, "fragment svn is below the specified minimum") {
			t.Error("expected error string to contain 'fragment svn is below the specified minimum'")
			return false
		}

		if tc.policy.rego.IsModuleActive(rpi.ModuleID(subFragment.info.issuer, fragment.info.feed)) {
			t.Error("module not removed upon failure")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_LoadFragment_Fragment_InvalidSVN: %v", err)
	}
}

func Test_Rego_LoadFragment_SemverVersion(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		p.fragments = generateFragments(testRand, 1)
		p.fragments[0].minimumSVN = generateSemver(testRand)
		securityPolicy := p.toPolicy()

		defaultMounts := toOCIMounts(generateMounts(testRand))
		privilegedMounts := toOCIMounts(generateMounts(testRand))
		policy, err := newRegoPolicy(securityPolicy.marshalRego(), defaultMounts, privilegedMounts, testOSType)

		if err != nil {
			t.Fatalf("error compiling policy: %v", err)
		}

		issuer := p.fragments[0].issuer
		feed := p.fragments[0].feed

		fragmentConstraints := generateConstraints(testRand, 1)
		fragmentConstraints.svn = mustIncrementSVN(p.fragments[0].minimumSVN)
		code := fragmentConstraints.toFragment().marshalRego()

		err = policy.LoadFragment(p.ctx, issuer, feed, code)
		if err != nil {
			t.Error("unable to load fragment: %w", err)
			return false
		}

		if policy.rego.IsModuleActive(rpi.ModuleID(issuer, feed)) {
			t.Error("module not removed after load")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_LoadFragment_SemverVersion: %v", err)
	}
}

func Test_Rego_LoadFragment_SVNMismatch(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoFragmentSVNMismatchTestConfig(p)
		if err != nil {
			t.Error(err)
			return false
		}

		fragment := tc.fragments[0]
		err = tc.policy.LoadFragment(p.ctx, fragment.info.issuer, fragment.info.feed, fragment.code)
		if err == nil {
			t.Error("expected to be unable to load fragment due to invalid version")
			return false
		}

		if !assertDecisionJSONContains(t, err, "fragment svn and the specified minimum are different types") {
			t.Error("expected error string to contain 'fragment svn and the specified minimum are different types'")
			return false
		}

		if tc.policy.rego.IsModuleActive(rpi.ModuleID(fragment.info.issuer, fragment.info.feed)) {
			t.Error("module not removed upon failure")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_LoadFragment_SVNMismatch: %v", err)
	}
}

func Test_Rego_LoadFragment_SameIssuerTwoFeeds(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoFragmentTwoFeedTestConfig(p, true, false)
		if err != nil {
			t.Error(err)
			return false
		}

		for _, fragment := range tc.fragments {
			err = tc.policy.LoadFragment(p.ctx, fragment.info.issuer, fragment.info.feed, fragment.code)
			if err != nil {
				t.Error("unable to load fragment: %w", err)
				return false
			}
		}

		for _, container := range tc.containers {
			containerID, err := mountImageForContainer(tc.policy, container.container)
			if err != nil {
				t.Error("unable to mount image for fragment container: %w", err)
				return false
			}

			_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx,
				container.sandboxID,
				containerID,
				copyStrings(container.container.Command),
				copyStrings(container.envList),
				container.container.WorkingDir,
				copyMounts(container.mounts),
				false,
				container.container.NoNewPrivileges,
				container.user,
				container.groups,
				container.container.User.Umask,
				container.capabilities,
				container.seccomp,
			)

			if err != nil {
				t.Error("unable to create container from fragment: %w", err)
				return false
			}
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 15, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_LoadFragment_SameIssuerTwoFeeds: %v", err)
	}
}

func Test_Rego_LoadFragment_TwoFeeds(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoFragmentTwoFeedTestConfig(p, false, false)
		if err != nil {
			t.Error(err)
			return false
		}

		for _, fragment := range tc.fragments {
			err = tc.policy.LoadFragment(p.ctx, fragment.info.issuer, fragment.info.feed, fragment.code)
			if err != nil {
				t.Error("unable to load fragment: %w", err)
				return false
			}
		}

		for _, container := range tc.containers {
			containerID, err := mountImageForContainer(tc.policy, container.container)
			if err != nil {
				t.Error("unable to mount image for fragment container: %w", err)
				return false
			}

			_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx,
				container.sandboxID,
				containerID,
				copyStrings(container.container.Command),
				copyStrings(container.envList),
				container.container.WorkingDir,
				copyMounts(container.mounts),
				false,
				container.container.NoNewPrivileges,
				container.user,
				container.groups,
				container.container.User.Umask,
				container.capabilities,
				container.seccomp,
			)

			if err != nil {
				t.Error("unable to create container from fragment: %w", err)
				return false
			}
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 15, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_LoadFragment_TwoFeeds: %v", err)
	}
}

func Test_Rego_LoadFragment_SameFeedTwice(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoFragmentTwoFeedTestConfig(p, true, true)
		if err != nil {
			t.Error(err)
			return false
		}

		err = tc.policy.LoadFragment(p.ctx, tc.fragments[0].info.issuer, tc.fragments[0].info.feed, tc.fragments[0].code)
		if err != nil {
			t.Error("unable to load fragment the first time: %w", err)
			return false
		}

		err = tc.policy.LoadFragment(p.ctx, tc.fragments[1].info.issuer, tc.fragments[1].info.feed, tc.fragments[1].code)
		if err != nil {
			t.Error("expected to be able to load the same issuer/feed twice: %w", err)
			return false
		}

		for _, container := range tc.containers {
			containerID, err := mountImageForContainer(tc.policy, container.container)
			if err != nil {
				t.Error("unable to mount image for fragment container: %w", err)
				return false
			}

			_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx,
				container.sandboxID,
				containerID,
				copyStrings(container.container.Command),
				copyStrings(container.envList),
				container.container.WorkingDir,
				copyMounts(container.mounts),
				false,
				container.container.NoNewPrivileges,
				container.user,
				container.groups,
				container.container.User.Umask,
				container.capabilities,
				container.seccomp,
			)

			if err != nil {
				t.Error("unable to create container from fragment: %w", err)
				return false
			}
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 15, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_LoadFragment_SameFragmentTwice: %v", err)
	}
}

func Test_Rego_LoadFragment_ExcludedContainer(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoFragmentTestConfigWithExcludes(p, []string{"containers"})
		if err != nil {
			t.Error(err)
			return false
		}

		fragment := tc.fragments[0]
		container := tc.containers[0]

		err = tc.policy.LoadFragment(p.ctx, fragment.info.issuer, fragment.info.feed, fragment.code)
		if err != nil {
			t.Error("unable to load fragment: %w", err)
			return false
		}

		_, err = mountImageForContainer(tc.policy, container.container)
		if err == nil {
			t.Error("expected to be unable to mount image for fragment container")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 15, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_LoadFragment_ExcludedContainer: %v", err)
	}
}

func Test_Rego_LoadFragment_ExcludedFragment(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoFragmentTestConfigWithExcludes(p, []string{"fragments"})
		if err != nil {
			t.Error(err)
			return false
		}

		fragment := tc.fragments[0]
		subFragment := tc.subFragments[0]

		err = tc.policy.LoadFragment(p.ctx, fragment.info.issuer, fragment.info.feed, fragment.code)
		if err != nil {
			t.Error("unable to load fragment: %w", err)
			return false
		}

		err = tc.policy.LoadFragment(p.ctx, subFragment.info.issuer, subFragment.info.feed, subFragment.code)
		if err == nil {
			t.Error("expected to be unable to load a sub-fragment from a fragment")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 15, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_LoadFragment_ExcludedFragment: %v", err)
	}
}

func Test_Rego_LoadFragment_ExcludedExternalProcess(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoFragmentTestConfigWithExcludes(p, []string{"external_processes"})
		if err != nil {
			t.Error(err)
			return false
		}

		fragment := tc.fragments[0]
		process := tc.externalProcesses[0]

		err = tc.policy.LoadFragment(p.ctx, fragment.info.issuer, fragment.info.feed, fragment.code)
		if err != nil {
			t.Error("unable to load fragment: %w", err)
			return false
		}

		envList := buildEnvironmentVariablesFromEnvRules(process.envRules, testRand)

		_, _, err = tc.policy.EnforceExecExternalProcessPolicy(p.ctx, process.command, envList, process.workingDir)
		if err == nil {
			t.Error("expected to be unable to execute external process from a fragment")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_LoadFragment_ExcludedExternalProcess: %v", err)
	}
}

func Test_Rego_LoadFragment_FragmentNamespace(t *testing.T) {
	ctx := context.Background()
	deviceHash := generateRootHash(testRand)
	key := randVariableString(testRand, 32)
	value := randVariableString(testRand, 32)
	fragmentCode := fmt.Sprintf(`package fragment

svn := 1
framework_version := "%s"

layer := "%s"

mount_device := {"allowed": allowed, "metadata": [addCustom]} {
	allowed := input.deviceHash == layer
	addCustom := {
		"name": "custom",
        "action": "add",
        "key": "%s",
        "value": "%s"
	}
}`, frameworkVersion, deviceHash, key, value)

	issuer := testDataGenerator.uniqueFragmentIssuer()
	feed := testDataGenerator.uniqueFragmentFeed()
	policyCode := fmt.Sprintf(`package policy

api_version := "%s"
framework_version := "%s"

default load_fragment := {"allowed": false}

load_fragment := {"allowed": true, "add_module": true} {
	input.issuer == "%s"
	input.feed == "%s"
	data[input.namespace].svn >= 1
}

mount_device := data.fragment.mount_device
	`, apiVersion, frameworkVersion, issuer, feed)

	policy, err := newRegoPolicy(policyCode, []oci.Mount{}, []oci.Mount{}, testOSType)

	if err != nil {
		t.Fatalf("unable to create Rego policy: %v", err)
	}

	err = policy.LoadFragment(ctx, issuer, feed, fragmentCode)
	if err != nil {
		t.Fatalf("unable to load fragment: %v", err)
	}

	err = policy.EnforceDeviceMountPolicy(ctx, "/mnt/foo", deviceHash)
	if err != nil {
		t.Fatalf("unable to mount device: %v", err)
	}

	if test, err := policy.rego.GetMetadata("custom", key); err == nil {
		if test != value {
			t.Error("incorrect metadata value stored by fragment")
		}
	} else {
		t.Errorf("unable to located metadata key stored by fragment: %v", err)
	}
}

func Test_Rego_Scratch_Mount_Policy(t *testing.T) {
	for _, tc := range []struct {
		unencryptedAllowed bool
		encrypted          bool
		failureExpected    bool
	}{
		{
			unencryptedAllowed: false,
			encrypted:          false,
			failureExpected:    true,
		},
		{
			unencryptedAllowed: false,
			encrypted:          true,
			failureExpected:    false,
		},
		{
			unencryptedAllowed: true,
			encrypted:          false,
			failureExpected:    false,
		},
		{
			unencryptedAllowed: true,
			encrypted:          true,
			failureExpected:    false,
		},
	} {
		t.Run(fmt.Sprintf("UnencryptedAllowed_%t_And_Encrypted_%t", tc.unencryptedAllowed, tc.encrypted), func(t *testing.T) {
			gc := generateConstraints(testRand, maxContainersInGeneratedConstraints)
			smConfig, err := setupRegoScratchMountTest(gc, tc.unencryptedAllowed)
			if err != nil {
				t.Fatalf("unable to setup test: %s", err)
			}

			scratchPath := generateMountTarget(testRand)
			err = smConfig.policy.EnforceScratchMountPolicy(gc.ctx, scratchPath, tc.encrypted)
			if tc.failureExpected {
				if err == nil {
					t.Fatal("policy enforcement should've been denied")
				}
			} else {
				if err != nil {
					t.Fatalf("policy enforcement unexpectedly was denied: %s", err)
				}
			}
		})
	}
}

// We only test cases where scratch mount should succeed, so that we can test
// the scratch unmount policy. The negative cases for scratch mount are covered
// in Test_Rego_Scratch_Mount_Policy test.
func Test_Rego_Scratch_Unmount_Policy(t *testing.T) {
	for _, tc := range []struct {
		unencryptedAllowed bool
		encrypted          bool
		failureExpected    bool
	}{
		{
			unencryptedAllowed: true,
			encrypted:          false,
			failureExpected:    false,
		},
		{
			unencryptedAllowed: true,
			encrypted:          true,
			failureExpected:    false,
		},
		{
			unencryptedAllowed: false,
			encrypted:          true,
			failureExpected:    false,
		},
	} {
		t.Run(fmt.Sprintf("UnencryptedAllowed_%t_And_Encrypted_%t", tc.unencryptedAllowed, tc.encrypted), func(t *testing.T) {
			gc := generateConstraints(testRand, maxContainersInGeneratedConstraints)
			smConfig, err := setupRegoScratchMountTest(gc, tc.unencryptedAllowed)
			if err != nil {
				t.Fatalf("unable to setup test: %s", err)
			}

			scratchPath := generateMountTarget(testRand)
			err = smConfig.policy.EnforceScratchMountPolicy(gc.ctx, scratchPath, tc.encrypted)
			if err != nil {
				t.Fatalf("scratch_mount policy enforcement unexpectedly was denied: %s", err)
			}

			err = smConfig.policy.EnforceScratchUnmountPolicy(gc.ctx, scratchPath)
			if err != nil {
				t.Fatalf("scratch_unmount policy enforcement unexpectedly was denied: %s", err)
			}
		})
	}
}

func Test_Rego_StdioAccess_Allowed(t *testing.T) {
	gc := generateConstraints(testRand, 1)
	gc.containers[0].AllowStdioAccess = true
	gc.externalProcesses = generateExternalProcesses(testRand)
	gc.externalProcesses[0].allowStdioAccess = true
	tc, err := setupRegoCreateContainerTest(gc, gc.containers[0], false)
	if err != nil {
		t.Fatalf("error setting up test: %v", err)
	}

	_, _, allow_stdio_access, err := tc.policy.EnforceCreateContainerPolicy(
		gc.ctx,
		tc.sandboxID,
		tc.containerID,
		tc.argList,
		tc.envList,
		tc.workingDir,
		tc.mounts,
		false,
		tc.noNewPrivileges,
		tc.user,
		tc.groups,
		tc.umask,
		tc.capabilities,
		tc.seccomp,
	)

	if err != nil {
		t.Errorf("create_container not allowed: %v", err)
	}

	if !allow_stdio_access {
		t.Errorf("expected allow_stdio_access to be true")
	}

	// stdio access is inherited from the container and should be the same
	_, _, allow_stdio_access, err = tc.policy.EnforceExecInContainerPolicy(
		gc.ctx,
		tc.containerID,
		gc.containers[0].ExecProcesses[0].Command,
		tc.envList,
		tc.workingDir,
		tc.noNewPrivileges,
		tc.user,
		tc.groups,
		tc.umask,
		tc.capabilities,
	)

	if err != nil {
		t.Errorf("exec_in_container not allowed: %v", err)
	}

	if !allow_stdio_access {
		t.Errorf("expected allow_stdio_access to be true")
	}

	envList := buildEnvironmentVariablesFromEnvRules(gc.externalProcesses[0].envRules, testRand)
	_, allow_stdio_access, err = tc.policy.EnforceExecExternalProcessPolicy(
		gc.ctx,
		gc.externalProcesses[0].command,
		envList,
		gc.externalProcesses[0].workingDir,
	)

	if err != nil {
		t.Errorf("exec_external not allowed: %v", err)
	}

	if !allow_stdio_access {
		t.Errorf("expected allow_stdio_access to be true")
	}
}

func Test_Rego_EnforeCreateContainerPolicy_StdioAccess_NotAllowed(t *testing.T) {
	gc := generateConstraints(testRand, 1)
	gc.containers[0].AllowStdioAccess = false
	tc, err := setupRegoCreateContainerTest(gc, gc.containers[0], false)
	if err != nil {
		t.Fatalf("error setting up test: %v", err)
	}

	_, _, allow_stdio_access, err := tc.policy.EnforceCreateContainerPolicy(
		tc.ctx,
		tc.sandboxID,
		tc.containerID,
		tc.argList,
		tc.envList,
		tc.workingDir,
		tc.mounts,
		false,
		tc.noNewPrivileges,
		tc.user,
		tc.groups,
		tc.umask,
		tc.capabilities,
		tc.seccomp,
	)

	if err != nil {
		t.Errorf("create_container not allowed: %v", err)
	}

	if allow_stdio_access {
		t.Errorf("expected allow_stdio_access to be false")
	}
}

func Test_Rego_Container_StdioAccess_NotDecidable(t *testing.T) {
	gc := generateConstraints(testRand, 1)
	container0 := gc.containers[0]
	container0.AllowStdioAccess = true
	container1, err := container0.clone()
	if err != nil {
		t.Fatalf("unable to clone container: %v", err)
	}

	container1.AllowStdioAccess = false
	gc.containers = append(gc.containers, container1)

	container0.ExecProcesses = append(container0.ExecProcesses, container0.ExecProcesses[0].clone())

	gc.externalProcesses = generateExternalProcesses(testRand)
	gc.externalProcesses = append(gc.externalProcesses, gc.externalProcesses[0].clone())
	gc.externalProcesses[0].allowStdioAccess = true

	tc, err := setupRegoCreateContainerTest(gc, gc.containers[0], false)
	if err != nil {
		t.Fatalf("error setting up test: %v", err)
	}

	_, _, allow_stdio_access, err := tc.policy.EnforceCreateContainerPolicy(
		tc.ctx,
		tc.sandboxID,
		tc.containerID,
		tc.argList,
		tc.envList,
		tc.workingDir,
		tc.mounts,
		false,
		tc.noNewPrivileges,
		tc.user,
		tc.groups,
		tc.umask,
		tc.capabilities,
		tc.seccomp,
	)

	if err == nil {
		t.Errorf("expected create_container to not be allowed")
	}

	if allow_stdio_access {
		t.Errorf("expected allow_stdio_access to be false")
	}
}

func Test_Rego_ExecExternal_StdioAccess_NotAllowed(t *testing.T) {
	gc := generateConstraints(testRand, 1)
	gc.externalProcesses = generateExternalProcesses(testRand)
	gc.externalProcesses = append(gc.externalProcesses, gc.externalProcesses[0].clone())
	gc.externalProcesses[0].allowStdioAccess = !gc.externalProcesses[0].allowStdioAccess

	policy, err := newRegoPolicy(gc.toPolicy().marshalRego(), []oci.Mount{}, []oci.Mount{}, testOSType)

	if err != nil {
		t.Fatalf("error marshaling policy: %v", err)
	}

	envList := buildEnvironmentVariablesFromEnvRules(gc.externalProcesses[0].envRules, testRand)
	_, allow_stdio_access, err := policy.EnforceExecExternalProcessPolicy(
		gc.ctx,
		gc.externalProcesses[0].command,
		envList,
		gc.externalProcesses[0].workingDir,
	)

	if err == nil {
		t.Errorf("expected exec_external to not be allowed")
	}

	if allow_stdio_access {
		t.Errorf("expected allow_stdio_access to be false")
	}
}

func Test_Rego_EnforceCreateContainerPolicy_AllowElevatedAllowsPrivilegedContainer(t *testing.T) {
	gc := generateConstraints(testRand, 1)
	gc.containers[0].AllowElevated = true
	tc, err := setupRegoCreateContainerTest(gc, gc.containers[0], false)
	if err != nil {
		t.Fatalf("error setting up test: %v", err)
	}

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(
		tc.ctx,
		tc.sandboxID,
		tc.containerID,
		tc.argList,
		tc.envList,
		tc.workingDir,
		tc.mounts,
		false,
		tc.noNewPrivileges,
		tc.user,
		tc.groups,
		tc.umask,
		tc.capabilities,
		tc.seccomp,
	)

	if err != nil {
		t.Fatalf("expected privilege escalation to be allowed: %s", err)
	}
}

func Test_Rego_EnforceCreateContainerPolicy_AllowElevatedAllowsUnprivilegedContainer(t *testing.T) {
	gc := generateConstraints(testRand, 1)
	gc.containers[0].AllowElevated = true
	tc, err := setupRegoCreateContainerTest(gc, gc.containers[0], false)
	if err != nil {
		t.Fatalf("error setting up test: %v", err)
	}

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(
		tc.ctx,
		tc.sandboxID,
		tc.containerID,
		tc.argList,
		tc.envList,
		tc.workingDir,
		tc.mounts,
		false,
		tc.noNewPrivileges,
		tc.user,
		tc.groups,
		tc.umask,
		tc.capabilities,
		tc.seccomp,
	)

	if err != nil {
		t.Fatalf("expected lack of escalation to be fine: %s", err)
	}
}

func Test_Rego_EnforceCreateContainerPolicy_NoAllowElevatedDenysPrivilegedContainer(t *testing.T) {
	gc := generateConstraints(testRand, 1)
	gc.containers[0].AllowElevated = false
	tc, err := setupRegoCreateContainerTest(gc, gc.containers[0], false)
	if err != nil {
		t.Fatalf("error setting up test: %v", err)
	}

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(
		tc.ctx,
		tc.sandboxID,
		tc.containerID,
		tc.argList,
		tc.envList,
		tc.workingDir,
		tc.mounts,
		true,
		tc.noNewPrivileges,
		tc.user,
		tc.groups,
		tc.umask,
		tc.capabilities,
		tc.seccomp,
	)

	if err == nil {
		t.Fatal("expected escalation to be denied")
	}
}

func Test_Rego_EnforceCreateContainerPolicy_NoAllowElevatedAllowsUnprivilegedContainer(t *testing.T) {
	gc := generateConstraints(testRand, 1)
	gc.containers[0].AllowElevated = false
	tc, err := setupRegoCreateContainerTest(gc, gc.containers[0], false)
	if err != nil {
		t.Fatalf("error setting up test: %v", err)
	}

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(
		tc.ctx,
		tc.sandboxID,
		tc.containerID,
		tc.argList,
		tc.envList,
		tc.workingDir,
		tc.mounts,
		false,
		tc.noNewPrivileges,
		tc.user,
		tc.groups,
		tc.umask,
		tc.capabilities,
		tc.seccomp,
	)

	if err != nil {
		t.Fatalf("expected lack of escalation to be fine: %s", err)
	}
}

func Test_Rego_CreateContainer_NoNewPrivileges_Default(t *testing.T) {
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints)
	tc, err := setupFrameworkVersionSimpleTest(gc, "0.1.0", frameworkVersion)
	if err != nil {
		t.Fatalf("error setting up test: %v", err)
	}

	input := map[string]interface{}{}
	result, err := tc.policy.rego.RawQuery("data.framework.candidate_containers", input)

	if err != nil {
		t.Fatalf("unable to query containers: %v", err)
	}

	containers, ok := result[0].Expressions[0].Value.([]interface{})
	if !ok {
		t.Fatal("unable to extract containers from result")
	}

	if len(containers) != len(gc.containers) {
		t.Error("incorrect number of candidate containers.")
	}

	for _, container := range containers {
		object := container.(map[string]interface{})
		err := assertKeyValue(object, "no_new_privileges", false)
		if err != nil {
			t.Error(err)
		}
	}
}

func Test_Rego_CreateContainer_User_Default(t *testing.T) {
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints)
	tc, err := setupFrameworkVersionSimpleTest(gc, "0.1.0", frameworkVersion)
	if err != nil {
		t.Fatalf("error setting up test: %v", err)
	}

	input := map[string]interface{}{}
	result, err := tc.policy.rego.RawQuery("data.framework.candidate_containers", input)

	if err != nil {
		t.Fatalf("unable to query containers: %v", err)
	}

	containers, ok := result[0].Expressions[0].Value.([]interface{})
	if !ok {
		t.Fatal("unable to extract containers from result")
	}

	if len(containers) != len(gc.containers) {
		t.Error("incorrect number of candidate containers.")
	}

	for _, container := range containers {
		object := container.(map[string]interface{})
		user, ok := object["user"].(map[string]interface{})
		if !ok {
			t.Error("unable to extract user from container")
			continue
		}

		err := assertKeyValue(user, "umask", "0022")
		if err != nil {
			t.Error(err)
		}

		if user_idname, ok := user["user_idname"].(map[string]interface{}); ok {
			err = assertKeyValue(user_idname, "strategy", "any")
			if err != nil {
				t.Error(err)
			}
		} else {
			t.Error("unable to extract user_idname from user")
		}

		if group_idnames, ok := user["group_idnames"].([]interface{}); ok {
			if len(group_idnames) != 1 {
				t.Error("incorrect number of group_idnames")
			} else {
				group_idname := group_idnames[0].(map[string]interface{})
				err = assertKeyValue(group_idname, "strategy", "any")
				if err != nil {
					t.Error(err)
				}
			}
		}
	}
}

func Test_Rego_CreateContainer_Capabilities_Default(t *testing.T) {
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints)
	tc, err := setupFrameworkVersionSimpleTest(gc, "0.1.0", frameworkVersion)
	if err != nil {
		t.Fatalf("error setting up test: %v", err)
	}

	input := map[string]interface{}{
		"privileged": false,
	}
	result, err := tc.policy.rego.RawQuery("data.framework.candidate_containers", input)

	if err != nil {
		t.Fatalf("unable to query containers: %v", err)
	}

	containers, ok := result[0].Expressions[0].Value.([]interface{})
	if !ok {
		t.Fatal("unable to extract containers from result")
	}

	if len(containers) != len(gc.containers) {
		t.Error("incorrect number of candidate containers.")
	}

	for _, container := range containers {
		object := container.(map[string]interface{})
		capabilities, ok := object["capabilities"]
		if !ok {
			t.Error("unable to extract capabilities from container")
			continue
		}

		if capabilities != nil {
			t.Error("capabilities should be nil by default")
		}
	}
}

func Test_Rego_CreateContainer_AllowCapabilityDropping_Default(t *testing.T) {
	gc := generateConstraints(testRand, 1)
	tc, err := setupFrameworkVersionSimpleTest(gc, "0.1.0", frameworkVersion)
	if err != nil {
		t.Fatalf("error setting up test: %v", err)
	}

	input := map[string]interface{}{}
	result, err := tc.policy.rego.RawQuery("data.framework.allow_capability_dropping", input)

	if err != nil {
		t.Fatalf("unable to query allow_capability_dropping: %v", err)
	}

	actualValue, ok := result[0].Expressions[0].Value.(bool)
	if !ok {
		t.Fatal("unable to extract allow_capability_dropping from result")
	}

	if actualValue != false {
		t.Error("unexpected allow_capability_dropping value")
	}
}

func Test_Rego_CreateContainer_Seccomp_Default(t *testing.T) {
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints)
	tc, err := setupFrameworkVersionSimpleTest(gc, "0.1.0", frameworkVersion)
	if err != nil {
		t.Fatalf("error setting up test: %v", err)
	}

	input := map[string]interface{}{}
	result, err := tc.policy.rego.RawQuery("data.framework.candidate_containers", input)

	if err != nil {
		t.Fatalf("unable to query containers: %v", err)
	}

	containers, ok := result[0].Expressions[0].Value.([]interface{})
	if !ok {
		t.Fatal("unable to extract containers from result")
	}

	if len(containers) != len(gc.containers) {
		t.Error("incorrect number of candidate containers.")
	}

	for _, container := range containers {
		object := container.(map[string]interface{})
		err := assertKeyValue(object, "seccomp_profile_sha256", "")
		if err != nil {
			t.Error(err)
		}
	}
}

func Test_FrameworkVersion_Missing(t *testing.T) {
	gc := generateConstraints(testRand, 1)
	tc, err := setupFrameworkVersionSimpleTest(gc, "", frameworkVersion)
	if err != nil {
		t.Fatalf("unable to setup test: %v", err)
	}

	containerID := testDataGenerator.uniqueContainerID()
	c := selectContainerFromContainerList(gc.containers, testRand)

	layerPaths, err := testDataGenerator.createValidOverlayForContainer(tc.policy, c)

	err = tc.policy.EnforceOverlayMountPolicy(gc.ctx, containerID, layerPaths, testDataGenerator.uniqueMountTarget())
	if err == nil {
		t.Error("unexpected success. Missing framework_version should trigger an error.")
	}

	assertDecisionJSONContains(t, err, fmt.Sprintf("framework_version is missing. Current version: %s", frameworkVersion))
}

func Test_Fragment_FrameworkVersion_Missing(t *testing.T) {
	gc := generateConstraints(testRand, 1)
	tc, err := setupFrameworkVersionTest(gc, frameworkVersion, frameworkVersion, 1, "", []string{})
	if err != nil {
		t.Fatalf("unable to setup test: %v", err)
	}

	fragment := tc.fragments[0]
	err = tc.policy.LoadFragment(gc.ctx, fragment.info.issuer, fragment.info.feed, fragment.code)
	if err == nil {
		t.Error("unexpected success. Missing framework_version should trigger an error.")
	}

	assertDecisionJSONContains(t, err, fmt.Sprintf("fragment framework_version is missing. Current version: %s", frameworkVersion))
}

func Test_FrameworkVersion_In_Future(t *testing.T) {
	gc := generateConstraints(testRand, 1)
	tc, err := setupFrameworkVersionSimpleTest(gc, "100.0.0", frameworkVersion)
	if err != nil {
		t.Fatalf("unable to setup test: %v", err)
	}

	containerID := testDataGenerator.uniqueContainerID()
	c := selectContainerFromContainerList(gc.containers, testRand)

	layerPaths, err := testDataGenerator.createValidOverlayForContainer(tc.policy, c)

	err = tc.policy.EnforceOverlayMountPolicy(gc.ctx, containerID, layerPaths, testDataGenerator.uniqueMountTarget())
	if err == nil {
		t.Error("unexpected success. Future framework_version should trigger an error.")
	}

	assertDecisionJSONContains(t, err, fmt.Sprintf("framework_version is ahead of the current version: 100.0.0 is greater than %s", frameworkVersion))
}

func Test_Fragment_FrameworkVersion_In_Future(t *testing.T) {
	gc := generateConstraints(testRand, 1)
	tc, err := setupFrameworkVersionTest(gc, frameworkVersion, frameworkVersion, 1, "100.0.0", []string{})
	if err != nil {
		t.Fatalf("unable to setup test: %v", err)
	}

	fragment := tc.fragments[0]
	err = tc.policy.LoadFragment(gc.ctx, fragment.info.issuer, fragment.info.feed, fragment.code)
	if err == nil {
		t.Error("unexpected success. Future framework_version should trigger an error.")
	}

	assertDecisionJSONContains(t, err, fmt.Sprintf("fragment framework_version is ahead of the current version: 100.0.0 is greater than %s", frameworkVersion))
}

func Test_Rego_MissingEnvList(t *testing.T) {
	code := fmt.Sprintf(`package policy

	api_version := "%s"

	create_container := {"allowed": true}
	exec_in_container := {"allowed": true}
	exec_external := {"allowed": true}
	`, apiVersion)

	policy, err := newRegoPolicy(code, []oci.Mount{}, []oci.Mount{}, testOSType)

	if err != nil {
		t.Fatalf("error compiling the rego policy: %v", err)
	}

	ctx := context.Background()
	sandboxID := generateSandboxID(testRand)
	containerID := generateContainerID(testRand)
	command := generateCommand(testRand)
	expectedEnvs := generateEnvironmentVariables(testRand)
	workingDir := generateWorkingDir(testRand)
	privileged := randBool(testRand)
	noNewPrivileges := randBool(testRand)
	user := generateIDName(testRand)
	groups := []IDName{}
	umask := generateUmask(testRand)
	capabilities := generateCapabilities(testRand)
	seccomp := ""

	actualEnvs, _, _, err := policy.EnforceCreateContainerPolicy(
		ctx,
		sandboxID,
		containerID,
		command,
		expectedEnvs,
		workingDir,
		[]oci.Mount{},
		privileged,
		noNewPrivileges,
		user,
		groups,
		umask,
		capabilities,
		seccomp,
	)

	if err != nil {
		t.Errorf("unexpected error when calling EnforceCreateContainerPolicy: %v", err)
	}

	if !areStringArraysEqual(actualEnvs, expectedEnvs) {
		t.Error("invalid envList returned from EnforceCreateContainerPolicy")
	}

	actualEnvs, _, _, err = policy.EnforceExecInContainerPolicy(ctx, containerID, command, expectedEnvs, workingDir, noNewPrivileges, user, groups, umask, capabilities)

	if err != nil {
		t.Errorf("unexpected error when calling EnforceExecInContainerPolicy: %v", err)
	}

	if !areStringArraysEqual(actualEnvs, expectedEnvs) {
		t.Error("invalid envList returned from EnforceExecInContainerPolicy")
	}

	actualEnvs, _, err = policy.EnforceExecExternalProcessPolicy(ctx, command, expectedEnvs, workingDir)

	if err != nil {
		t.Errorf("unexpected error when calling EnfForceExecExternalProcessPolicy: %v", err)
	}

	if !areStringArraysEqual(actualEnvs, expectedEnvs) {
		t.Error("invalid envList returned from EnforceExecExternalProcessPolicy")
	}
}

func Test_Rego_EnvListGetsRedacted(t *testing.T) {
	c := generateConstraints(testRand, 1)
	// don't allow env dropping. with dropping we aren't testing the
	// "invalid env list" message, only the input
	c.allowEnvironmentVariableDropping = false

	// No environment variables are allowed
	tc, err := setupRegoCreateContainerTest(c, c.containers[0], false)
	if err != nil {
		t.Fatal(err)
	}

	var envList []string
	envVar := "FOO=BAR"
	envList = append(envList, envVar)

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(tc.ctx, tc.sandboxID, tc.containerID, tc.argList, envList, "bunk", tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

	// not getting an error means something is broken
	if err == nil {
		t.Fatal("Unexpected success when enforcing policy")
	}

	if !assertDecisionJSONDoesNotContain(t, err, envVar) {
		t.Fatal("EnvList wasn't redacted in error message")
	}

	if !assertDecisionJSONContains(t, err, `FOO=\u003c\u003credacted\u003e\u003e`) {
		t.Fatal("EnvList redaction format wasn't as expected")
	}
}

func Test_Rego_EnforceCreateContainer_ConflictingAllowStdioAccessHasErrorMessage(t *testing.T) {
	constraints := generateConstraints(testRand, 1)
	constraints.containers[0].AllowStdioAccess = true

	// create a "duplicate" as far as create container is concerned except for
	// a different "AllowStdioAccess" value
	duplicate := &securityPolicyContainer{
		Command:              constraints.containers[0].Command,
		EnvRules:             constraints.containers[0].EnvRules,
		WorkingDir:           constraints.containers[0].WorkingDir,
		Mounts:               constraints.containers[0].Mounts,
		Layers:               constraints.containers[0].Layers,
		Capabilities:         constraints.containers[0].Capabilities,
		ExecProcesses:        generateExecProcesses(testRand),
		Signals:              generateListOfSignals(testRand, 0, maxSignalNumber),
		AllowElevated:        constraints.containers[0].AllowElevated,
		AllowStdioAccess:     false,
		NoNewPrivileges:      constraints.containers[0].NoNewPrivileges,
		User:                 constraints.containers[0].User,
		SeccompProfileSHA256: constraints.containers[0].SeccompProfileSHA256,
	}

	constraints.containers = append(constraints.containers, duplicate)

	tc, err := setupRegoCreateContainerTest(constraints, constraints.containers[0], false)
	if err != nil {
		t.Fatal(err)
	}

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(tc.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

	// not getting an error means something is broken
	if err == nil {
		t.Fatalf("Unexpected success when enforcing policy")
	}

	if !assertDecisionJSONContains(t, err, "containers only distinguishable by allow_stdio_access") {
		t.Fatal("No error message given for conflicting allow_stdio_access on otherwise 'same' containers")
	}
}

func Test_Rego_ExecExternalProcessPolicy_ConflictingAllowStdioAccessHasErrorMessage(t *testing.T) {
	constraints := generateConstraints(testRand, 1)
	process := generateExternalProcess(testRand)
	process.allowStdioAccess = false
	duplicate := process.clone()
	duplicate.allowStdioAccess = true

	constraints.externalProcesses = append(constraints.externalProcesses, process)
	constraints.externalProcesses = append(constraints.externalProcesses, duplicate)
	securityPolicy := constraints.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	policy, err := newRegoPolicy(securityPolicy.marshalRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts),
		testOSType)
	if err != nil {
		t.Fatal(err)
	}

	envList := buildEnvironmentVariablesFromEnvRules(process.envRules, testRand)

	_, _, err = policy.EnforceExecExternalProcessPolicy(constraints.ctx, process.command, envList, process.workingDir)
	if err == nil {
		t.Fatal("Policy was unexpectedly not enforced")
	}

	if !assertDecisionJSONContains(t, err, "external processes only distinguishable by allow_stdio_access") {
		t.Fatal("No error message given for conflicting allow_stdio_access on otherwise 'same' external processes")
	}
}

func Test_Rego_Enforce_CreateContainer_RequiredEnvMissingHasErrorMessage(t *testing.T) {
	constraints := generateConstraints(testRand, 1)
	container := selectContainerFromContainerList(constraints.containers, testRand)
	requiredRule := EnvRuleConfig{
		Strategy: "string",
		Rule:     randVariableString(testRand, maxGeneratedEnvironmentVariableRuleLength),
		Required: true,
	}

	container.EnvRules = append(container.EnvRules, requiredRule)

	tc, err := setupRegoCreateContainerTest(constraints, container, false)
	if err != nil {
		t.Fatal(err)
	}

	envList := make([]string, 0, len(container.EnvRules))
	for _, env := range tc.envList {
		if env != requiredRule.Rule {
			envList = append(envList, env)
		}
	}

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(tc.ctx, tc.sandboxID, tc.containerID, tc.argList, envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

	// not getting an error means something is broken
	if err == nil {
		t.Fatalf("Unexpected success when enforcing policy")
	}

	if !assertDecisionJSONContains(t, err, "missing required environment variable") {
		t.Fatal("No error message given for missing required environment variable")
	}
}

func Test_Rego_ExecInContainerPolicy_RequiredEnvMissingHasErrorMessage(t *testing.T) {
	constraints := generateConstraints(testRand, 1)
	container := selectContainerFromContainerList(constraints.containers, testRand)
	neededEnv := randVariableString(testRand, maxGeneratedEnvironmentVariableRuleLength)
	requiredRule := EnvRuleConfig{
		Strategy: "string",
		Rule:     neededEnv,
		Required: true,
	}

	container.EnvRules = append(container.EnvRules, requiredRule)

	tc, err := setupRegoRunningContainerTest(constraints, false)
	if err != nil {
		t.Fatal(err)
	}

	running := selectContainerFromRunningContainers(tc.runningContainers, testRand)

	process := selectExecProcess(running.container.ExecProcesses, testRand)

	allEnvs := buildEnvironmentVariablesFromEnvRules(running.container.EnvRules, testRand)
	envList := make([]string, 0, len(container.EnvRules))
	for _, env := range allEnvs {
		if env != requiredRule.Rule {
			envList = append(envList, env)
		}
	}
	user := buildIDNameFromConfig(running.container.User.UserIDName, testRand)
	groups := buildGroupIDNamesFromUser(running.container.User, testRand)
	umask := running.container.User.Umask
	capabilities := running.container.Capabilities.toExternal()

	_, _, _, err = tc.policy.EnforceExecInContainerPolicy(constraints.ctx, running.containerID, process.Command, envList, running.container.WorkingDir, running.container.NoNewPrivileges, user, groups, umask, &capabilities)

	// not getting an error means something is broken
	if err == nil {
		t.Fatal("Unexpected success when enforcing policy")
	}

	if !assertDecisionJSONContains(t, err, "missing required environment variable") {
		fmt.Print(err.Error())
		t.Fatal("No error message given for missing required environment variable")
	}
}

func Test_Rego_ExecExternalProcessPolicy_RequiredEnvMissingHasErrorMessage(t *testing.T) {
	constraints := generateConstraints(testRand, 1)
	process := generateExternalProcess(testRand)
	neededEnv := randVariableString(testRand, maxGeneratedEnvironmentVariableRuleLength)
	requiredRule := EnvRuleConfig{
		Strategy: "string",
		Rule:     neededEnv,
		Required: true,
	}

	process.envRules = append(process.envRules, requiredRule)

	constraints.externalProcesses = append(constraints.externalProcesses, process)
	securityPolicy := constraints.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	policy, err := newRegoPolicy(securityPolicy.marshalRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts),
		testOSType)
	if err != nil {
		t.Fatal(err)
	}

	allEnvs := buildEnvironmentVariablesFromEnvRules(process.envRules, testRand)
	envList := make([]string, 0, len(process.envRules))
	for _, env := range allEnvs {
		if env != requiredRule.Rule {
			envList = append(envList, env)
		}
	}

	_, _, err = policy.EnforceExecExternalProcessPolicy(constraints.ctx, process.command, envList, process.workingDir)
	if err == nil {
		t.Fatal("Policy was unexpectedly not enforced")
	}

	if !assertDecisionJSONContains(t, err, "missing required environment variable") {
		fmt.Print(err.Error())
		t.Fatal("No error message given for missing required environment variable")
	}
}

func Test_Rego_EnforceContainerNoNewPrivilegesPolicy_FalseAllowsFalse(t *testing.T) {
	p := generateConstraints(testRand, 1)
	p.containers[0].NoNewPrivileges = false

	tc, err := setupSimpleRegoCreateContainerTest(p)
	if err != nil {
		t.Fatal(err)
	}

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(tc.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, false, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

	if err != nil {
		t.Fatal("Unexpected failure with false")
	}
}

func Test_Rego_EnforceContainerNoNewPrivilegesPolicy_FalseAllowsTrue(t *testing.T) {
	p := generateConstraints(testRand, 1)
	p.containers[0].NoNewPrivileges = false

	tc, err := setupSimpleRegoCreateContainerTest(p)
	if err != nil {
		t.Fatal(err)
	}

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, true, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

	if err != nil {
		t.Fatal("Unexpected failure with true")
	}
}

func Test_Rego_EnforceContainerNoNewPrivilegesPolicy_TrueDisallowsFalse(t *testing.T) {
	p := generateConstraints(testRand, 1)
	p.containers[0].NoNewPrivileges = true

	tc, err := setupSimpleRegoCreateContainerTest(p)
	if err != nil {
		t.Fatal(err)
	}

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, false, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

	if err == nil {
		t.Fatal("Unexpected success with false")
	}

	if !assertDecisionJSONContains(t, err, "invalid noNewPrivileges") {
		t.Fatal("Expected error message is missing")
	}
}

func Test_Rego_EnforceContainerNoNewPrivilegesPolicy_TrueAllowsTrue(t *testing.T) {
	p := generateConstraints(testRand, 1)
	p.containers[0].NoNewPrivileges = true

	tc, err := setupSimpleRegoCreateContainerTest(p)
	if err != nil {
		t.Fatal(err)
	}

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, true, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

	if err != nil {
		t.Fatal("Unexpected failure with true")
	}
}

func Test_Rego_EnforceExecInContainerNoNewPrivilegesPolicy_FalseAllowsFalse(t *testing.T) {
	p := generateConstraints(testRand, 1)
	p.containers[0].NoNewPrivileges = false

	tc, err := setupRegoRunningContainerTest(p, false)
	if err != nil {
		t.Fatal(err)
	}

	container := selectContainerFromRunningContainers(tc.runningContainers, testRand)
	process := selectExecProcess(container.container.ExecProcesses, testRand)
	envList := buildEnvironmentVariablesFromEnvRules(container.container.EnvRules, testRand)
	user := buildIDNameFromConfig(container.container.User.UserIDName, testRand)
	groups := buildGroupIDNamesFromUser(container.container.User, testRand)
	umask := container.container.User.Umask
	capabilities := container.container.Capabilities.toExternal()

	_, _, _, err = tc.policy.EnforceExecInContainerPolicy(p.ctx, container.containerID, process.Command, envList, container.container.WorkingDir, false, user, groups, umask, &capabilities)

	if err != nil {
		t.Fatal("Unexpected failure with false")
	}
}

func Test_Rego_EnforceExecInContainerNoNewPrivilegesPolicy_FalseAllowsTrue(t *testing.T) {
	p := generateConstraints(testRand, 1)
	p.containers[0].NoNewPrivileges = false

	tc, err := setupRegoRunningContainerTest(p, false)
	if err != nil {
		t.Fatal(err)
	}

	container := selectContainerFromRunningContainers(tc.runningContainers, testRand)
	process := selectExecProcess(container.container.ExecProcesses, testRand)
	envList := buildEnvironmentVariablesFromEnvRules(container.container.EnvRules, testRand)
	user := buildIDNameFromConfig(container.container.User.UserIDName, testRand)
	groups := buildGroupIDNamesFromUser(container.container.User, testRand)
	umask := container.container.User.Umask
	capabilities := container.container.Capabilities.toExternal()

	_, _, _, err = tc.policy.EnforceExecInContainerPolicy(p.ctx, container.containerID, process.Command, envList, container.container.WorkingDir, true, user, groups, umask, &capabilities)

	if err != nil {
		t.Fatal("Unexpected failure with true")
	}
}

func Test_Rego_EnforceExecInContainerNoNewPrivilegesPolicy_TrueDisallowsFalse(t *testing.T) {
	p := generateConstraints(testRand, 1)
	p.containers[0].NoNewPrivileges = true

	tc, err := setupRegoRunningContainerTest(p, false)
	if err != nil {
		t.Fatal(err)
	}

	container := selectContainerFromRunningContainers(tc.runningContainers, testRand)
	process := selectExecProcess(container.container.ExecProcesses, testRand)
	envList := buildEnvironmentVariablesFromEnvRules(container.container.EnvRules, testRand)
	user := buildIDNameFromConfig(container.container.User.UserIDName, testRand)
	groups := buildGroupIDNamesFromUser(container.container.User, testRand)
	umask := container.container.User.Umask
	capabilities := container.container.Capabilities.toExternal()

	_, _, _, err = tc.policy.EnforceExecInContainerPolicy(p.ctx, container.containerID, process.Command, envList, container.container.WorkingDir, false, user, groups, umask, &capabilities)

	if err == nil {
		t.Fatal("Unexpected success with false")
	}

	if !assertDecisionJSONContains(t, err, "invalid noNewPrivileges") {
		t.Fatal("Expected error message is missing")
	}
}

func Test_Rego_EnforceExecInContainerNoNewPrivilegesPolicy_TrueAllowsTrue(t *testing.T) {
	p := generateConstraints(testRand, 1)
	p.containers[0].NoNewPrivileges = true

	tc, err := setupRegoRunningContainerTest(p, false)
	if err != nil {
		t.Fatal(err)
	}

	container := selectContainerFromRunningContainers(tc.runningContainers, testRand)
	process := selectExecProcess(container.container.ExecProcesses, testRand)
	envList := buildEnvironmentVariablesFromEnvRules(container.container.EnvRules, testRand)
	user := buildIDNameFromConfig(container.container.User.UserIDName, testRand)
	groups := buildGroupIDNamesFromUser(container.container.User, testRand)
	umask := container.container.User.Umask
	capabilities := container.container.Capabilities.toExternal()

	_, _, _, err = tc.policy.EnforceExecInContainerPolicy(p.ctx, container.containerID, process.Command, envList, container.container.WorkingDir, true, user, groups, umask, &capabilities)

	if err != nil {
		t.Fatal("Unexpected failure with true")
	}
}

func Test_Rego_EnforceContainerUserPolicy_UserName_NoMatches(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		user := generateIDName(testRand)

		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

		if err == nil {
			return false
		}

		return assertDecisionJSONContains(t, err, "invalid user")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceContainerUserPolicy_UserName_NoMatches: %v", err)
	}
}

func Test_Rego_EnforceContainerUserPolicy_GroupNames_NoMatches(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		groups := append(tc.groups, generateIDName(testRand))

		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, groups, tc.umask, tc.capabilities, tc.seccomp)

		if err == nil {
			return false
		}

		return assertDecisionJSONContains(t, err, "invalid user")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceContainerUserPolicy_GroupNames_NoMatches: %v", err)
	}
}

func Test_Rego_EnforceContainerUserPolicy_Umask_NoMatches(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		umask := "0888"

		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, umask, tc.capabilities, tc.seccomp)

		if err == nil {
			return false
		}

		return assertDecisionJSONContains(t, err, "invalid user")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceContainerUserPolicy_Umask_NoMatches: %v", err)
	}
}

func Test_Rego_EnforceExecInContainerUserPolicy_Username_NoMatches(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoRunningContainerTest(p, false)
		if err != nil {
			t.Error(err)
			return false
		}

		container := selectContainerFromRunningContainers(tc.runningContainers, testRand)
		process := selectExecProcess(container.container.ExecProcesses, testRand)
		envList := buildEnvironmentVariablesFromEnvRules(container.container.EnvRules, testRand)
		groups := buildGroupIDNamesFromUser(container.container.User, testRand)
		umask := container.container.User.Umask
		capabilities := container.container.Capabilities.toExternal()

		user := generateIDName(testRand)

		_, _, _, err = tc.policy.EnforceExecInContainerPolicy(p.ctx, container.containerID, process.Command, envList, container.container.WorkingDir, container.container.NoNewPrivileges, user, groups, umask, &capabilities)

		// not getting an error means something is broken
		if err == nil {
			return false
		}

		return assertDecisionJSONContains(t, err, "invalid user")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 10, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceExecInContainerUserPolicy_Username_NoMatches: %v", err)
	}
}

func Test_Rego_EnforceExecInContainerUserPolicy_GroupNames_NoMatches(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoRunningContainerTest(p, false)
		if err != nil {
			t.Error(err)
			return false
		}

		container := selectContainerFromRunningContainers(tc.runningContainers, testRand)
		process := selectExecProcess(container.container.ExecProcesses, testRand)
		envList := buildEnvironmentVariablesFromEnvRules(container.container.EnvRules, testRand)
		user := buildIDNameFromConfig(container.container.User.UserIDName, testRand)
		groups := buildGroupIDNamesFromUser(container.container.User, testRand)
		umask := container.container.User.Umask
		capabilities := container.container.Capabilities.toExternal()

		groups = append(groups, generateIDName(testRand))

		_, _, _, err = tc.policy.EnforceExecInContainerPolicy(p.ctx, container.containerID, process.Command, envList, container.container.WorkingDir, container.container.NoNewPrivileges, user, groups, umask, &capabilities)

		// not getting an error means something is broken
		if err == nil {
			return false
		}

		return assertDecisionJSONContains(t, err, "invalid user")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 10, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceExecInContainerUserPolicy_GroupNames_NoMatches: %v", err)
	}
}

func Test_Rego_EnforceExecInContainerUserPolicy_Umask_NoMatches(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoRunningContainerTest(p, false)
		if err != nil {
			t.Error(err)
			return false
		}

		container := selectContainerFromRunningContainers(tc.runningContainers, testRand)
		process := selectExecProcess(container.container.ExecProcesses, testRand)
		envList := buildEnvironmentVariablesFromEnvRules(container.container.EnvRules, testRand)
		user := buildIDNameFromConfig(container.container.User.UserIDName, testRand)
		groups := buildGroupIDNamesFromUser(container.container.User, testRand)
		capabilities := container.container.Capabilities.toExternal()

		// This value will never be generated by our generators as it isn't valid.
		// We don't care about valid for this test, only that it won't match any
		// generated value.
		umask := "8888"

		_, _, _, err = tc.policy.EnforceExecInContainerPolicy(p.ctx, container.containerID, process.Command, envList, container.container.WorkingDir, container.container.NoNewPrivileges, user, groups, umask, &capabilities)

		// not getting an error means something is broken
		if err == nil {
			return false
		}

		return assertDecisionJSONContains(t, err, "invalid user")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 10, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceExecInContainerUserPolicy_Umask_NoMatches: %v", err)
	}
}

func Test_Rego_EnforceCreateContainerUserPolicy_UserIDName_Re2Match(t *testing.T) {
	gc := generateConstraints(testRand, 1)
	gc.containers[0].User.UserIDName = IDNameConfig{
		Strategy: IDNameStrategyRegex,
		Rule:     "foo\\d+",
	}

	tc, err := setupSimpleRegoCreateContainerTest(gc)
	if err != nil {
		t.Fatalf("unable to setup test: %v", err)
	}

	user := IDName{
		ID:   "1000",
		Name: "foo123",
	}

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(tc.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)
	if err != nil {
		t.Errorf("Expected container setup to be allowed. It wasn't: %v", err)
	}
}

func Test_Rego_EnforceCreateContainerUserPolicy_UserIDName_AnyMatch(t *testing.T) {
	gc := generateConstraints(testRand, 1)
	gc.containers[0].User.UserIDName = IDNameConfig{
		Strategy: IDNameStrategyAny,
		Rule:     "",
	}

	tc, err := setupSimpleRegoCreateContainerTest(gc)
	if err != nil {
		t.Fatalf("unable to setup test: %v", err)
	}

	user := generateIDName(testRand)

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(tc.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)
	if err != nil {
		t.Errorf("Expected container setup to be allowed. It wasn't: %v", err)
	}
}

func Test_Rego_EnforceCreateContainerUserPolicy_GroupIDName_Re2Match(t *testing.T) {
	gc := generateConstraints(testRand, 1)
	gc.containers[0].User.GroupIDNames = append(gc.containers[0].User.GroupIDNames, IDNameConfig{
		Strategy: IDNameStrategyRegex,
		Rule:     "foo\\d+",
	})

	tc, err := setupSimpleRegoCreateContainerTest(gc)
	if err != nil {
		t.Fatalf("unable to setup test: %v", err)
	}

	groups := append(tc.groups, IDName{ID: "1000", Name: "foo123"})

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(tc.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, groups, tc.umask, tc.capabilities, tc.seccomp)
	if err != nil {
		t.Errorf("Expected container setup to be allowed. It wasn't: %v", err)
	}
}

func Test_Rego_EnforceCreateContainerUserPolicy_GroupIDName_AnyMatch(t *testing.T) {
	gc := generateConstraints(testRand, 1)
	gc.containers[0].User.GroupIDNames = append(gc.containers[0].User.GroupIDNames, IDNameConfig{
		Strategy: IDNameStrategyAny,
		Rule:     "",
	})

	tc, err := setupSimpleRegoCreateContainerTest(gc)
	if err != nil {
		t.Fatalf("unable to setup test: %v", err)
	}

	groups := append(tc.groups, generateIDName(testRand))

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(tc.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, groups, tc.umask, tc.capabilities, tc.seccomp)
	if err != nil {
		t.Errorf("Expected container setup to be allowed. It wasn't: %v", err)
	}
}

func Test_Rego_EnforceCreateContainerSeccompPolicy_NoMatch(t *testing.T) {
	gc := generateConstraints(testRand, 1)

	tc, err := setupSimpleRegoCreateContainerTest(gc)
	if err != nil {
		t.Fatalf("unable to setup test: %v", err)
	}

	seccomp := generateRootHash(testRand)

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(tc.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, seccomp)
	if err == nil {
		t.Error("Expected container setup to not be allowed.")
	} else if !assertDecisionJSONContains(t, err, "invalid seccomp") {
		t.Error("`invalid seccomp` missing from error message")
	}
}

func Test_Rego_FrameworkSVN(t *testing.T) {
	gc := generateConstraints(testRand, 1)
	securityPolicy := gc.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	code := securityPolicy.marshalRego()
	code = strings.Replace(code, "framework_version", "framework_svn", 1)

	policy, err := newRegoPolicy(code,
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts),
		testOSType)
	if err != nil {
		t.Fatalf("unable to create policy: %v", err)
	}

	value, err := policy.rego.RawQuery("data.framework.policy_framework_version", map[string]interface{}{})
	if err != nil {
		t.Fatalf("unable to query policy: %v", err)
	}

	policyFrameworkVersion, ok := value[0].Expressions[0].Value.(string)
	if ok {
		if policyFrameworkVersion != frameworkVersion {
			t.Error("policy_framework_version is not set correctly from framework_svn")
		}
	} else {
		t.Error("no result set from querying data.framework.policy_framework_version")
	}
}

func Test_Rego_Fragment_FrameworkSVN(t *testing.T) {
	gc := generateConstraints(testRand, 1)
	gc.fragments = generateFragments(testRand, 1)

	gc.fragments = generateFragments(testRand, 1)
	gc.fragments[0].minimumSVN = generateSemver(testRand)
	securityPolicy := gc.toPolicy()

	defaultMounts := toOCIMounts(generateMounts(testRand))
	privilegedMounts := toOCIMounts(generateMounts(testRand))
	policy, err := newRegoPolicy(securityPolicy.marshalRego(), defaultMounts, privilegedMounts, testOSType)

	if err != nil {
		t.Fatalf("error compiling policy: %v", err)
	}

	fragmentConstraints := generateConstraints(testRand, 1)
	fragmentConstraints.svn = mustIncrementSVN(gc.fragments[0].minimumSVN)
	code := fragmentConstraints.toFragment().marshalRego()

	policy.rego.AddModule(fragmentConstraints.namespace, &rpi.RegoModule{
		Namespace: fragmentConstraints.namespace,
		Feed:      gc.fragments[0].feed,
		Issuer:    gc.fragments[0].issuer,
		Code:      code,
	})

	input := map[string]interface{}{
		"namespace": fragmentConstraints.namespace,
	}
	result, err := policy.rego.RawQuery("data.framework.fragment_framework_version", input)

	if err != nil {
		t.Fatalf("error querying policy: %v", err)
	}

	fragmentFrameworkVersion, ok := result[0].Expressions[0].Value.(string)

	if ok {
		if fragmentFrameworkVersion != frameworkVersion {
			t.Error("fragment_framework_version is not set correctly from framework_svn")
		}
	} else {
		t.Error("no result set from querying data.framework.fragment_framework_version")
	}
}

func Test_Rego_APISVN(t *testing.T) {
	gc := generateConstraints(testRand, 1)
	securityPolicy := gc.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	code := securityPolicy.marshalRego()
	code = strings.Replace(code, "api_version", "api_svn", 1)

	policy, err := newRegoPolicy(code,
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts),
		testOSType)
	if err != nil {
		t.Fatalf("unable to create policy: %v", err)
	}

	value, err := policy.rego.RawQuery("data.framework.policy_api_version", map[string]interface{}{})
	if err != nil {
		t.Fatalf("unable to query policy: %v", err)
	}

	policyAPIVersion, ok := value[0].Expressions[0].Value.(string)
	if ok {
		if policyAPIVersion != apiVersion {
			t.Error("policy_api_version is not set correctly from api_svn")
		}
	} else {
		t.Error("no result set from querying data.framework.policy_api_version")
	}
}

func Test_Rego_NoReason(t *testing.T) {
	code := `package policy

	api_version := "0.0.1"

	mount_device := {"allowed": false}
`
	policy, err := newRegoPolicy(code, []oci.Mount{}, []oci.Mount{}, testOSType)

	if err != nil {
		t.Fatalf("unable to create policy: %v", err)
	}

	ctx := context.Background()
	err = policy.EnforceDeviceMountPolicy(ctx, generateMountTarget(testRand), generateRootHash(testRand))
	if err == nil {
		t.Error("expected error, got nil")
	}

	assertDecisionJSONContains(t, err, noReasonMessage)
}

func Test_Rego_ErrorTruncation(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		maxErrorMessageLength := int(randMinMax(testRand, 128, 4*1024))
		tc.policy.maxErrorMessageLength = maxErrorMessageLength

		_, _, _, err = tc.policy.EnforceCreateContainerPolicy(tc.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, randString(testRand, 20), tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)
		// not getting an error means something is broken
		if err == nil {
			return false
		}

		if len(err.Error()) > maxErrorMessageLength {
			return assertDecisionJSONContains(t, err, `"reason.error_objects","input","reason"`)
		}

		policyDecisionJSON, err := ExtractPolicyDecision(err.Error())
		if err != nil {
			t.Errorf("unable to extract policy decision JSON: %v", err)
			return false
		}

		var policyDecision map[string]interface{}
		err = json.Unmarshal([]byte(policyDecisionJSON), &policyDecision)
		if err != nil {
			t.Errorf("unable to unmarshal policy decision: %v", err)
		}

		if truncated, ok := policyDecision["truncated"].([]interface{}); ok {
			if truncated[0].(string) != "reason.error_objects" {
				t.Error("first item to be truncated should be reason.error_objects")
				return false
			} else if len(truncated) > 1 && truncated[1].(string) != "input" {
				t.Error("second item to be truncated should be input")
				return false
			}
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceOverlayMountPolicy_No_Matches failed: %v", err)
	}
}

func Test_Rego_ErrorTruncation_Unable(t *testing.T) {
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints)
	tc, err := setupRegoOverlayTest(gc, false)
	if err != nil {
		t.Fatal(err)
	}

	maxErrorMessageLength := 32
	tc.policy.maxErrorMessageLength = maxErrorMessageLength
	err = tc.policy.EnforceOverlayMountPolicy(gc.ctx, tc.containerID, tc.layers, testDataGenerator.uniqueMountTarget())

	if err == nil {
		t.Fatal("Policy did not throw the expected error")
	}

	assertDecisionJSONContains(t, err, `"reason.error_objects","input","reason"`)
}

func Test_Rego_ErrorTruncation_CustomPolicy(t *testing.T) {
	code := fmt.Sprintf(`package policy

	api_version := "0.1.0"

	mount_device := {"allowed": false}

	reason := {"custom_error": "%s"}
`, randString(testRand, 2048))

	policy, err := newRegoPolicy(code, []oci.Mount{}, []oci.Mount{}, testOSType)

	if err != nil {
		t.Fatalf("unable to create policy: %v", err)
	}

	policy.maxErrorMessageLength = 512
	ctx := context.Background()
	err = policy.EnforceDeviceMountPolicy(ctx, generateMountTarget(testRand), generateRootHash(testRand))
	if err == nil {
		t.Error("expected error, got nil")
	}

	assertDecisionJSONContains(t, err, `"input","reason"`)
}

func Test_Rego_Missing_Enforcement_Point(t *testing.T) {
	code := `package policy

	api_svn := "0.10.0"

	mount_device := {"allowed": true}
	unmount_device := {"allowed": true}
	mount_overlay := {"allowed": true}
	unmount_overlay := {"allowed": true}

	reason := {"errors": data.framework.errors}
`

	policy, err := newRegoPolicy(code, []oci.Mount{}, []oci.Mount{}, testOSType)

	if err != nil {
		t.Fatalf("unable to create policy: %v", err)
	}

	sandboxID := generateSandboxID(testRand)
	containerID := generateContainerID(testRand)
	argList := generateCommand(testRand)
	envList := generateEnvironmentVariables(testRand)
	workingDir := generateWorkingDir(testRand)
	mounts := []oci.Mount{}
	user := generateIDName(testRand)
	groups := []IDName{}
	umask := generateUmask(testRand)
	capabilities := &oci.LinuxCapabilities{
		Bounding:    DefaultUnprivilegedCapabilities(),
		Effective:   DefaultUnprivilegedCapabilities(),
		Inheritable: EmptyCapabiltiesSet(),
		Permitted:   DefaultUnprivilegedCapabilities(),
		Ambient:     EmptyCapabiltiesSet(),
	}

	ctx := context.Background()
	_, _, _, err = policy.EnforceCreateContainerPolicy(
		ctx,
		sandboxID,
		containerID,
		argList,
		envList,
		workingDir,
		mounts,
		false,
		false,
		user,
		groups,
		umask,
		capabilities,
		"",
	)

	assertDecisionJSONContains(t, err, "rule for create_container is missing from policy")
}

func Test_Rego_Capabiltiies_Placeholder_Object_Privileged(t *testing.T) {
	gc := generateConstraints(testRand, 1)
	gc.containers[0].Capabilities = &capabilitiesInternal{
		Bounding:    DefaultPrivilegedCapabilities(),
		Effective:   DefaultPrivilegedCapabilities(),
		Inheritable: DefaultPrivilegedCapabilities(),
		Permitted:   DefaultPrivilegedCapabilities(),
		Ambient:     EmptyCapabiltiesSet(),
	}
	tc, err := setupSimpleRegoCreateContainerTest(gc)
	if err != nil {
		t.Fatal(err)
	}

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(tc.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, randString(testRand, 20), tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

	if err == nil {
		t.Fatal("Policy did not throw the expected error")
	}

	assertDecisionJSONContains(t, err, "[privileged]")
	assertDecisionJSONDoesNotContain(t, err, DefaultPrivilegedCapabilities()...)
}

func Test_Rego_Capabiltiies_Placeholder_Object_Unprivileged(t *testing.T) {
	gc := generateConstraints(testRand, 1)
	gc.containers[0].Capabilities = &capabilitiesInternal{
		Bounding:    DefaultUnprivilegedCapabilities(),
		Effective:   DefaultUnprivilegedCapabilities(),
		Inheritable: EmptyCapabiltiesSet(),
		Permitted:   DefaultUnprivilegedCapabilities(),
		Ambient:     EmptyCapabiltiesSet(),
	}
	tc, err := setupSimpleRegoCreateContainerTest(gc)
	if err != nil {
		t.Fatal(err)
	}

	_, _, _, err = tc.policy.EnforceCreateContainerPolicy(tc.ctx, tc.sandboxID, tc.containerID, tc.argList, tc.envList, randString(testRand, 20), tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

	if err == nil {
		t.Fatal("Policy did not throw the expected error")
	}

	assertDecisionJSONContains(t, err, "[unprivileged]")
	assertDecisionJSONDoesNotContain(t, err, DefaultUnprivilegedCapabilities()...)
}

func Test_Rego_GetUserInfo_WithEtcPasswdAndGroup(t *testing.T) {
	etcPasswdString := strings.Join([]string{
		"root:x:0:0:root:/root:/bin/bash",
		"daemon:x:1:1:daemon:/usr/sbin:/usr/sbin/nologin",
		"postgres:x:105:111:PostgreSQL administrator,,,:/var/lib/postgresql:/bin/bash",
		"ihavenogroup:x:1000:1000:ihavenogroup:/home/ihavenogroup:/bin/bash",
		"nobody:x:65534:65534:nobody:/nonexistent:/usr/sbin/nologin",
		"",
	}, "\n")
	etcGroupsString := strings.Join([]string{
		"root:x:0:",
		"daemon:x:1:",
		"postgres:x:111:",
		"nogroup:x:65534:",
		"",
	}, "\n")

	testDir := t.TempDir()
	os.MkdirAll(filepath.Join(testDir, "etc"), 0755)
	if err := os.WriteFile(filepath.Join(testDir, "etc/passwd"), []byte(etcPasswdString), 0644); err != nil {
		t.Fatalf("Failed to write /etc/passwd: %v", err)
	}
	if err := os.WriteFile(filepath.Join(testDir, "etc/group"), []byte(etcGroupsString), 0644); err != nil {
		t.Fatalf("Failed to write /etc/group: %v", err)
	}

	regoEnforcer, err := newRegoPolicy(openDoorRego, []oci.Mount{}, []oci.Mount{}, testOSType)
	if err != nil {
		t.Errorf("cannot compile open door rego policy: %v", err)
		return
	}

	testCases := []getUserInfoTestCase{
		{
			userStrs:           []string{"0:0", "root:root", "0:root", "root:0", "root", "0", ""},
			additionalGIDs:     []uint32{},
			expectedUID:        0,
			expectedGIDs:       []int{0},
			expectedUsername:   "root",
			expectedGroupNames: []string{"root"},
		},
		{
			userStrs:           []string{"1:1", "daemon:daemon", "1:daemon", "daemon:1", "daemon", "1"},
			additionalGIDs:     []uint32{},
			expectedUID:        1,
			expectedGIDs:       []int{1},
			expectedUsername:   "daemon",
			expectedGroupNames: []string{"daemon"},
		},
		{
			userStrs:           []string{"1:0", "daemon:root", "daemon:0", "1:root"},
			additionalGIDs:     []uint32{},
			expectedUID:        1,
			expectedGIDs:       []int{0},
			expectedUsername:   "daemon",
			expectedGroupNames: []string{"root"},
		},
		{
			userStrs:           []string{"105:111", "postgres:postgres", "105", "postgres", "105:postgres", "postgres:111"},
			additionalGIDs:     []uint32{},
			expectedUID:        105,
			expectedGIDs:       []int{111},
			expectedUsername:   "postgres",
			expectedGroupNames: []string{"postgres"},
		},
		{
			userStrs:           []string{"postgres:daemon", "105:1", "postgres:1", "105:daemon"},
			additionalGIDs:     []uint32{},
			expectedUID:        105,
			expectedGIDs:       []int{1},
			expectedUsername:   "postgres",
			expectedGroupNames: []string{"daemon"},
		},
		{
			userStrs:           []string{"1234", "1234:0"},
			additionalGIDs:     []uint32{},
			expectedUID:        1234,
			expectedGIDs:       []int{0},
			expectedUsername:   "",
			expectedGroupNames: []string{"root"},
		},
		{
			userStrs:           []string{"0:1234"},
			additionalGIDs:     []uint32{},
			expectedUID:        0,
			expectedGIDs:       []int{1234},
			expectedUsername:   "root",
			expectedGroupNames: []string{""},
		},
		{
			userStrs:           []string{"1234:1234"},
			additionalGIDs:     []uint32{},
			expectedUID:        1234,
			expectedGIDs:       []int{1234},
			expectedUsername:   "",
			expectedGroupNames: []string{""},
		},
		{
			userStrs:           []string{"1234:postgres", "1234:111"},
			additionalGIDs:     []uint32{},
			expectedUID:        1234,
			expectedGIDs:       []int{111},
			expectedUsername:   "",
			expectedGroupNames: []string{"postgres"},
		},
		{
			userStrs:           []string{"postgres:1234", "105:1234"},
			additionalGIDs:     []uint32{},
			expectedUID:        105,
			expectedGIDs:       []int{1234},
			expectedUsername:   "postgres",
			expectedGroupNames: []string{""},
		},
		{
			userStrs:           []string{"ihavenogroup:nogroup", "ihavenogroup:65534", "1000:65534"},
			additionalGIDs:     []uint32{},
			expectedUID:        1000,
			expectedGIDs:       []int{65534},
			expectedUsername:   "ihavenogroup",
			expectedGroupNames: []string{"nogroup"},
		},
		{
			userStrs:           []string{"ihavenogroup", "ihavenogroup:1000", "1000:1000"},
			additionalGIDs:     []uint32{},
			expectedUID:        1000,
			expectedGIDs:       []int{1000},
			expectedUsername:   "ihavenogroup",
			expectedGroupNames: []string{""},
		},
		{
			userStrs:       []string{"nobody", "nobody:nogroup", "nobody:65534", "65534:65534", "65534:nogroup"},
			additionalGIDs: []uint32{100},
			expectErr:      true, // non-existent additionalGIDs not allowed
		},
		{
			userStrs:           []string{"nobody", "nobody:nogroup", "nobody:65534", "65534:65534", "65534:nogroup"},
			additionalGIDs:     []uint32{111},
			expectedUID:        65534,
			expectedGIDs:       []int{65534, 111},
			expectedUsername:   "nobody",
			expectedGroupNames: []string{"nogroup", "postgres"},
		},
		{
			userStrs:           []string{"1234", "1234:0"},
			additionalGIDs:     []uint32{111},
			expectedUID:        1234,
			expectedGIDs:       []int{0, 111},
			expectedUsername:   "",
			expectedGroupNames: []string{"root", "postgres"},
		},
		{
			userStrs:       []string{"nonexistentuser", "nonexistentuser:0", "nonexistentuser:nonexistentgroup"},
			additionalGIDs: []uint32{},
			expectErr:      true, // non-existent username will fail since we don't know the UID
		},
		{
			userStrs:       []string{"postgres:nonexistentgroup", "105:nonexistentgroup"},
			additionalGIDs: []uint32{},
			expectErr:      true, // non-existent group will fail since we don't know the GID
		},
		{
			userStrs:       []string{":", "malformed:b:c", ":root", "root:", ":0", "0:", "2a01:110::/32"},
			additionalGIDs: []uint32{},
			expectErr:      true,
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("TestCase%d", i), func(t *testing.T) {
			for _, userStr := range tc.userStrs {
				testGetUserInfo(t, tc, userStr, regoEnforcer, testDir)
			}
		})
	}
}

func Test_Rego_GetUserInfo_NoEtc(t *testing.T) {
	testDir := t.TempDir()

	regoEnforcer, err := newRegoPolicy(openDoorRego, []oci.Mount{}, []oci.Mount{}, testOSType)
	if err != nil {
		t.Errorf("cannot compile open door rego policy: %v", err)
		return
	}

	testCases := []getUserInfoTestCase{
		{
			userStrs:           []string{"0:0", "0", ""},
			additionalGIDs:     []uint32{},
			expectedUID:        0,
			expectedGIDs:       []int{0},
			expectedUsername:   "",
			expectedGroupNames: []string{""},
		},
		{
			userStrs:           []string{"1:1"},
			additionalGIDs:     []uint32{},
			expectedUID:        1,
			expectedGIDs:       []int{1},
			expectedUsername:   "",
			expectedGroupNames: []string{""},
		},
		{
			userStrs:       []string{"root:root", "root", "daemon:daemon", "1:daemon", "daemon:1", "daemon", "daemon:root", "daemon:0"},
			additionalGIDs: []uint32{},
			expectErr:      true, // no /etc/passwd or /etc/group, so we cannot resolve any names
		},
		{
			userStrs:           []string{"1:0", "1"},
			additionalGIDs:     []uint32{},
			expectedUID:        1,
			expectedGIDs:       []int{0},
			expectedUsername:   "",
			expectedGroupNames: []string{""},
		},
		{
			userStrs:           []string{"1234:4321"},
			additionalGIDs:     []uint32{},
			expectedUID:        1234,
			expectedGIDs:       []int{4321},
			expectedUsername:   "",
			expectedGroupNames: []string{""},
		},
		{
			userStrs:       []string{"1234:4321"},
			additionalGIDs: []uint32{5678, 9012},
			expectErr:      true, // non-existent additionalGIDs not allowed
		},
		{
			userStrs:       []string{":", "malformed:b:c", ":root", "root:", ":0", "0:", "2a01:110::/32"},
			additionalGIDs: []uint32{},
			expectErr:      true,
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("TestCase%d", i), func(t *testing.T) {
			for _, userStr := range tc.userStrs {
				testGetUserInfo(t, tc, userStr, regoEnforcer, testDir)
			}
		})
	}
}

func Test_Rego_GetUserInfo_EtcPasswdOnly(t *testing.T) {
	etcPasswdString := strings.Join([]string{
		"root:x:0:0:root:/root:/bin/bash",
		"daemon:x:1:1:daemon:/usr/sbin:/usr/sbin/nologin",
		"postgres:x:105:111:PostgreSQL administrator,,,:/var/lib/postgresql:/bin/bash",
		"ihavenogroup:x:1000:1000:ihavenogroup:/home/ihavenogroup:/bin/bash",
		"nobody:x:65534:65534:nobody:/nonexistent:/usr/sbin/nologin",
		"",
	}, "\n")

	testDir := t.TempDir()
	os.MkdirAll(filepath.Join(testDir, "etc"), 0755)
	if err := os.WriteFile(filepath.Join(testDir, "etc/passwd"), []byte(etcPasswdString), 0644); err != nil {
		t.Fatalf("Failed to write /etc/passwd: %v", err)
	}

	regoEnforcer, err := newRegoPolicy(openDoorRego, []oci.Mount{}, []oci.Mount{}, testOSType)
	if err != nil {
		t.Errorf("cannot compile open door rego policy: %v", err)
		return
	}

	testCases := []getUserInfoTestCase{
		{
			userStrs:           []string{"0:0", "root:0", "0", "root", ""},
			additionalGIDs:     []uint32{},
			expectedUID:        0,
			expectedGIDs:       []int{0},
			expectedUsername:   "root",
			expectedGroupNames: []string{""},
		},
		{
			userStrs:           []string{"1000:1234", "ihavenogroup:1234"},
			additionalGIDs:     []uint32{},
			expectedUID:        1000,
			expectedGIDs:       []int{1234},
			expectedUsername:   "ihavenogroup",
			expectedGroupNames: []string{""},
		},
		{
			userStrs:       []string{"nonexistentuser"},
			additionalGIDs: []uint32{},
			expectErr:      true, // non-existent username will fail since we don't know the UID
		},
		{
			userStrs:       []string{"1000:root", "0:root"},
			additionalGIDs: []uint32{},
			expectErr:      true, // No /etc/group, can't resolve group names
		},
		{
			userStrs:           []string{"1234"},
			additionalGIDs:     []uint32{},
			expectedUID:        1234,
			expectedGIDs:       []int{0},
			expectedUsername:   "",
			expectedGroupNames: []string{""},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("TestCase%d", i), func(t *testing.T) {
			for _, userStr := range tc.userStrs {
				testGetUserInfo(t, tc, userStr, regoEnforcer, testDir)
			}
		})
	}
}

type getUserInfoTestCase struct {
	userStrs           []string
	additionalGIDs     []uint32
	expectErr          bool
	expectedUID        int
	expectedGIDs       []int
	expectedUsername   string
	expectedGroupNames []string
}

func testGetUserInfo(t *testing.T, tc getUserInfoTestCase, userStr string, regoEnforcer *regoEnforcer, testDir string) {
	testName := userStr
	if userStr == "" {
		testName = "(empty)"
	}
	if len(tc.additionalGIDs) > 0 {
		testName += " additionalGIDs="
		for i, gid := range tc.additionalGIDs {
			if i > 0 {
				testName += ","
			}
			testName += fmt.Sprintf("%d", gid)
		}
	}
	t.Run(testName, func(t *testing.T) {
		ociProcess := &oci.Process{
			User: oci.User{
				UID:            0,
				GID:            0,
				Umask:          nil,
				Username:       userStr,
				AdditionalGids: tc.additionalGIDs,
			},
		}
		userIDName, groupIDNames, umask, err := regoEnforcer.GetUserInfo(ociProcess, testDir)
		if tc.expectErr {
			if err == nil {
				t.Errorf("Expected error for userStr %q, but succeed", userStr)
			}
			return
		}
		if err != nil {
			t.Errorf("GetUserInfo failed for userStr %q: %v", userStr, err)
			return
		}
		if userIDName.ID != fmt.Sprintf("%d", tc.expectedUID) {
			t.Errorf("Expected UID %d, got %s", tc.expectedUID, userIDName.ID)
		}
		if userIDName.Name != tc.expectedUsername {
			t.Errorf("Expected username %s, got %s", tc.expectedUsername, userIDName.Name)
		}
		groupIDs := make([]int, 0, len(groupIDNames))
		for _, groupIDName := range groupIDNames {
			gid, err := strconv.Atoi(groupIDName.ID)
			if err != nil {
				t.Errorf("Returned group ID %s is not a valid integer", groupIDName.ID)
			}
			groupIDs = append(groupIDs, gid)
		}
		if !slices.Equal(groupIDs, tc.expectedGIDs) {
			t.Errorf("Expected group IDs %v, got %v", tc.expectedGIDs, groupIDs)
		}
		groupNames := make([]string, 0, len(groupIDNames))
		for _, groupIDName := range groupIDNames {
			groupNames = append(groupNames, groupIDName.Name)
		}
		if !slices.Equal(groupNames, tc.expectedGroupNames) {
			t.Errorf("Expected group names %v, got %v", tc.expectedGroupNames, groupNames)
		}
		defaultUmask := "0022"
		if umask != defaultUmask {
			t.Errorf("Expected umask '%s', got '%s'", defaultUmask, umask)
		}
	})
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
