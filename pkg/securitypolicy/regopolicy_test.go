//go:build linux && rego
// +build linux,rego

package securitypolicy

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"testing/quick"

	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/blang/semver/v4"
	"github.com/open-policy-agent/opa/ast"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

const (
	// variables that influence generated rego-only test fixtures
	maxDiffLength                              = 64
	maxExternalProcessesInGeneratedConstraints = 16
	maxFragmentsInGeneratedConstraints         = 4
	maxGeneratedExternalProcesses              = 12
	maxGeneratedSandboxIDLength                = 32
	maxGeneratedEnforcementPointLength         = 64
	maxGeneratedPlan9Mounts                    = 8
	maxGeneratedFragmentFeedLength             = 256
	maxGeneratedFragmentIssuerLength           = 16
	maxPlan9MountTargetLength                  = 64
	maxPlan9MountIndex                         = 16
)

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
			t.Errorf("MarshalPolicy does not create the expected Rego policy [%d-%d]: %s != %s", start, end, actual[start:end], expected[start:end])
			return false
		}

		_, err = newRegoPolicy(expected, defaultMounts, privilegedMounts)
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

		_, err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, generateCommand(testRand), tc.envList, tc.workingDir, tc.mounts)

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
		_, err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, envList, tc.workingDir, tc.mounts)

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
		_, err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, envList, tc.workingDir, tc.mounts)

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

func Test_Rego_EnforceEnvironmentVariablePolicy_DropEnvs(t *testing.T) {
	testFunc := func(gc *generatedConstraints) bool {
		gc.allowEnvironmentVariableDropping = true
		container := selectContainerFromConstraints(gc, testRand)

		tc, err := setupRegoCreateContainerTest(gc, container, false)
		if err != nil {
			t.Error(err)
			return false
		}

		extraRules := generateEnvironmentVariableRules(testRand)
		extraEnvs := buildEnvironmentVariablesFromEnvRules(extraRules, testRand)

		envList := append(tc.envList, extraEnvs...)
		actual, err := tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, envList, tc.workingDir, tc.mounts)

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
	actual, err := tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, envList, tc.workingDir, tc.mounts)

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
	actual, err := tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, envList, tc.workingDir, tc.mounts)

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

		_, err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, tc.envList, randString(testRand, 20), tc.mounts)
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

		_, err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts)

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

			_, err = policy.EnforceCreateContainerPolicy(sandboxID, containerID, container.Command, envList, container.WorkingDir, mountSpec.Mounts)

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
		_, err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts)

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

		_, err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts)
		if err != nil {
			t.Error("Unable to start valid container.")
			return false
		}
		_, err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts)
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

		_, err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts)

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

		_, err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts)

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

		_, err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts)

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

		_, err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts)

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

		_, err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts)

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

		_, err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts)

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

		_, err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts)

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

		_, err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts)

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
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints)
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
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints)
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

		_, err = tc.policy.EnforceExecInContainerPolicy(container.containerID, process.Command, envList, container.container.WorkingDir)

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

		_, err = tc.policy.EnforceExecInContainerPolicy(container.containerID, process.Command, envList, container.container.WorkingDir)
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
		_, err = tc.policy.EnforceExecInContainerPolicy(container.containerID, command, envList, container.container.WorkingDir)

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

		_, err = tc.policy.EnforceExecInContainerPolicy(container.containerID, process.Command, envList, container.container.WorkingDir)

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

		_, err = tc.policy.EnforceExecInContainerPolicy(container.containerID, process.Command, envList, workingDir)

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

func Test_Rego_ExecInContainerPolicy_DropEnvs(t *testing.T) {
	testFunc := func(gc *generatedConstraints) bool {
		gc.allowEnvironmentVariableDropping = true
		tc, err := setupRegoRunningContainerTest(gc)
		if err != nil {
			t.Error(err)
			return false
		}

		container := selectContainerFromRunningContainers(tc.runningContainers, testRand)
		process := selectExecProcess(container.container.ExecProcesses, testRand)
		expected := buildEnvironmentVariablesFromEnvRules(container.container.EnvRules, testRand)

		extraRules := generateEnvironmentVariableRules(testRand)
		extraEnvs := buildEnvironmentVariablesFromEnvRules(extraRules, testRand)

		envList := append(expected, extraEnvs...)
		actual, err := tc.policy.EnforceExecInContainerPolicy(container.containerID, process.Command, envList, container.container.WorkingDir)

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

		policy, err := newRegoPolicy(rego, []oci.Mount{}, []oci.Mount{})
		if err != nil {
			t.Errorf("error creating policy: %v", err)
			return false
		}

		envList := generateEnvs(envSet)
		toKeep, err := policy.EnforceCreateContainerPolicy("", "", []string{}, envList, "", []oci.Mount{})
		if len(toKeep) > 0 {
			t.Error("invalid environment variables not filtered from list returned from create_container")
			return false
		}

		envList = generateEnvs(envSet)
		toKeep, err = policy.EnforceExecInContainerPolicy("", []string{}, envList, "")
		if len(toKeep) > 0 {
			t.Error("invalid environment variables not filtered from list returned from exec_in_container")
			return false
		}

		envList = generateEnvs(envSet)
		toKeep, err = policy.EnforceExecExternalProcessPolicy([]string{}, envList, "")
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
	rego := `package policy
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
	}`

	policy, err := newRegoPolicy(rego, []oci.Mount{}, []oci.Mount{})
	if err != nil {
		t.Fatalf("error creating policy: %v", err)
	}

	_, err = policy.EnforceCreateContainerPolicy("", "", []string{}, []string{}, "", []oci.Mount{})
	if err == nil {
		t.Errorf("expected call to create_container to fail")
	} else if err.Error() != "policy returned incorrect type for 'env_list', expected []interface{}, received map[string]interface {}" {
		t.Errorf("incorrected error message from call to create_container")
	}

	_, err = policy.EnforceExecInContainerPolicy("", []string{}, []string{}, "")
	if err == nil {
		t.Errorf("expected call to exec_in_container to fail")
	} else if err.Error() != "policy returned incorrect type for 'env_list', expected []interface{}, received string" {
		t.Errorf("incorrected error message from call to exec_in_container")
	}

	_, err = policy.EnforceExecExternalProcessPolicy([]string{}, []string{}, "")
	if err == nil {
		t.Errorf("expected call to exec_external to fail")
	} else if err.Error() != "policy returned incorrect type for 'env_list', expected []interface{}, received bool" {
		t.Errorf("incorrected error message from call to exec_external")
	}
}

func Test_Rego_InvalidEnvList_Member(t *testing.T) {
	rego := `package policy
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
	}`

	policy, err := newRegoPolicy(rego, []oci.Mount{}, []oci.Mount{})
	if err != nil {
		t.Fatalf("error creating policy: %v", err)
	}

	_, err = policy.EnforceCreateContainerPolicy("", "", []string{}, []string{}, "", []oci.Mount{})
	if err == nil {
		t.Errorf("expected call to create_container to fail")
	} else if err.Error() != "members of env_list from policy must be strings, received json.Number" {
		t.Errorf("incorrected error message from call to create_container")
	}

	_, err = policy.EnforceExecInContainerPolicy("", []string{}, []string{}, "")
	if err == nil {
		t.Errorf("expected call to exec_in_container to fail")
	} else if err.Error() != "members of env_list from policy must be strings, received bool" {
		t.Errorf("incorrected error message from call to exec_in_container")
	}

	_, err = policy.EnforceExecExternalProcessPolicy([]string{}, []string{}, "")
	if err == nil {
		t.Errorf("expected call to exec_external to fail")
	} else if err.Error() != "members of env_list from policy must be strings, received []interface {}" {
		t.Errorf("incorrected error message from call to exec_external")
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

		_, err = tc.policy.EnforceCreateContainerPolicy(tc.sandboxID, tc.containerID, tc.argList, envList, tc.workingDir, tc.mounts)

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

		_, err = tc.policy.EnforceExecExternalProcessPolicy(process.command, envList, process.workingDir)
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

		_, err = tc.policy.EnforceExecExternalProcessPolicy(process.command, envList, process.workingDir)
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

		_, err = tc.policy.EnforceExecExternalProcessPolicy(command, envList, process.workingDir)
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

		_, err = tc.policy.EnforceExecExternalProcessPolicy(process.command, envList, process.workingDir)
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

		_, err = tc.policy.EnforceExecExternalProcessPolicy(process.command, envList, workingDir)
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
		actual, err := tc.policy.EnforceExecExternalProcessPolicy(process.command, envList, process.workingDir)

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
	gc := generateConstraints(testRand, 1)
	gc.allowEnvironmentVariableDropping = true
	process0 := generateExternalProcess(testRand)
	envRules0 := process0.envRules

	process1 := process0.clone()
	envRules1 := generateEnvironmentVariableRules(testRand)
	process1.envRules = append(envRules0, envRules1...)

	process2 := process0.clone()
	envRules2 := generateEnvironmentVariableRules(testRand)
	process2.envRules = append(process1.envRules, envRules2...)

	gc.externalProcesses = []*externalProcess{process0, process1, process2}
	securityPolicy := gc.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	policy, err := newRegoPolicy(securityPolicy.marshalRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts))
	if err != nil {
		t.Fatal(err)
	}

	envs0 := buildEnvironmentVariablesFromEnvRules(envRules0, testRand)
	envs1 := buildEnvironmentVariablesFromEnvRules(envRules1, testRand)
	envs2 := buildEnvironmentVariablesFromEnvRules(envRules2, testRand)
	envList := append(envs0, envs1...)
	envList = append(envList, envs2...)

	actual, err := policy.EnforceExecExternalProcessPolicy(process2.command, envList, process2.workingDir)

	// getting an error means something is broken
	if err != nil {
		t.Errorf("Expected container creation to be allowed. It wasn't: %v", err)
	}

	if !areStringArraysEqual(actual, envList) {
		t.Error("environment variables were not dropped correctly.")
	}
}

func Test_Rego_ExecExternalProcessPolicy_DropEnvs_Multiple_NoMatch(t *testing.T) {
	gc := generateConstraints(testRand, 1)
	gc.allowEnvironmentVariableDropping = true
	process0 := generateExternalProcess(testRand)
	envRules0 := process0.envRules

	process1 := process0.clone()
	envRules1 := generateEnvironmentVariableRules(testRand)
	process1.envRules = append(envRules0, envRules1...)

	process2 := process0.clone()
	envRules2 := generateEnvironmentVariableRules(testRand)
	process2.envRules = append(envRules0, envRules2...)

	gc.externalProcesses = []*externalProcess{process0, process1, process2}
	securityPolicy := gc.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	policy, err := newRegoPolicy(securityPolicy.marshalRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts))
	if err != nil {
		t.Fatal(err)
	}

	envs0 := buildEnvironmentVariablesFromEnvRules(envRules0, testRand)
	envs1 := buildEnvironmentVariablesFromEnvRules(envRules1, testRand)
	envs2 := buildEnvironmentVariablesFromEnvRules(envRules2, testRand)
	var extraLen int
	if len(envs1) > len(envs2) {
		extraLen = len(envs2)
	} else {
		extraLen = len(envs1)
	}
	envList := append(envs0, envs1[:extraLen]...)
	envList = append(envList, envs2[:extraLen]...)

	actual, err := policy.EnforceExecExternalProcessPolicy(process2.command, envList, process2.workingDir)

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
	p := generateConstraints(testRand, maxContainersInGeneratedConstraints)

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

		_, err = tc.policy.EnforceExecInContainerPolicy(containerID, processUnderTest.Command, envList, containerUnderTest.WorkingDir)
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

		_, err = tc.policy.EnforceExecInContainerPolicy(containerID, processUnderTest.Command, envList, containerUnderTest.WorkingDir)
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

		_, err = tc.policy.EnforceExecInContainerPolicy(containerID, processUnderTest.Command, envList, containerUnderTest.WorkingDir)
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

		_, err = tc.policy.EnforceExecInContainerPolicy(containerID, processUnderTest.Command, envList, containerUnderTest.WorkingDir)
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
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints)

	tc, err := setupPlan9MountTest(gc)
	if err != nil {
		t.Fatalf("unable to setup test: %v", err)
	}

	err = tc.policy.EnforcePlan9MountPolicy(tc.uvmPathForShare)
	if err != nil {
		t.Fatalf("Policy enforcement unexpectedly was denied: %v", err)
	}

	_, err = tc.policy.EnforceCreateContainerPolicy(
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

	err = tc.policy.EnforcePlan9MountPolicy(mount)
	if err != nil {
		t.Fatalf("Policy enforcement unexpectedly was denied: %v", err)
	}

	_, err = tc.policy.EnforceCreateContainerPolicy(
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
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints)

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
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints)

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

	_, err = tc.policy.EnforceCreateContainerPolicy(
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
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints)

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

func Test_EnforceRuntimeLogging_Allowed(t *testing.T) {
	gc := generateConstraints(testRand, maxContainersInGeneratedConstraints)
	gc.allowRuntimeLogging = true

	tc, err := setupRegoPolicyOnlyTest(gc)
	if err != nil {
		t.Fatalf("unable to setup test: %v", err)
	}

	err = tc.policy.EnforceRuntimeLoggingPolicy()
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

	err = tc.policy.EnforceRuntimeLoggingPolicy()
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

		err = tc.policy.LoadFragment(fragment.info.issuer, fragment.info.feed, fragment.code)
		if err != nil {
			t.Error("unable to load fragment: %w", err)
			return false
		}

		containerID, err := mountImageForContainer(tc.policy, container.container)
		if err != nil {
			t.Error("unable to mount image for fragment container: %w", err)
			return false
		}

		_, err = tc.policy.EnforceCreateContainerPolicy(
			container.sandboxID,
			containerID,
			copyStrings(container.container.Command),
			copyStrings(container.envList),
			container.container.WorkingDir,
			copyMounts(container.mounts))

		if err != nil {
			t.Error("unable to create container from fragment: %w", err)
			return false
		}

		if _, ok := tc.policy.modules[moduleID(fragment.info.issuer, fragment.info.feed)]; ok {
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

		err = tc.policy.LoadFragment(fragment.info.issuer, fragment.info.feed, fragment.code)
		if err != nil {
			t.Error("unable to load fragment: %w", err)
			return false
		}

		err = tc.policy.LoadFragment(subFragment.info.issuer, subFragment.info.feed, subFragment.code)
		if err != nil {
			t.Error("unable to load sub-fragment from fragment: %w", err)
			return false
		}

		container := selectContainerFromConstraints(subFragment.constraints, testRand)
		_, err = mountImageForContainer(tc.policy, container)
		if err != nil {
			t.Error("unable to mount image for sub-fragment container: %w", err)
			return false
		}

		if _, ok := tc.policy.modules[moduleID(fragment.info.issuer, fragment.info.feed)]; ok {
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

		err = tc.policy.LoadFragment(fragment.info.issuer, fragment.info.feed, fragment.code)
		if err != nil {
			t.Error("unable to load fragment: %w", err)
			return false
		}

		envList := buildEnvironmentVariablesFromEnvRules(process.envRules, testRand)
		_, err = tc.policy.EnforceExecExternalProcessPolicy(process.command, envList, process.workingDir)
		if err != nil {
			t.Error("unable to execute external process from fragment: %w", err)
			return false
		}

		if _, ok := tc.policy.modules[moduleID(fragment.info.issuer, fragment.info.feed)]; ok {
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
		err = tc.policy.LoadFragment(issuer, fragment.info.feed, fragment.code)
		if err == nil {
			t.Error("expected to be unable to load fragment due to bad issuer")
			return false
		}

		if !strings.Contains(err.Error(), "invalid fragment issuer") {
			t.Error("expected error string to contain 'invalid fragment issuer'")
			return false
		}

		if _, ok := tc.policy.modules[moduleID(issuer, fragment.info.feed)]; ok {
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
		err = tc.policy.LoadFragment(fragment.info.issuer, feed, fragment.code)
		if err == nil {
			t.Error("expected to be unable to load fragment due to bad feed")
			return false
		}

		if !strings.Contains(err.Error(), "invalid fragment feed") {
			t.Error("expected error string to contain 'invalid fragment feed'")
			return false
		}

		if _, ok := tc.policy.modules[moduleID(fragment.info.issuer, feed)]; ok {
			t.Error("module not removed upon failure")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_LoadFragment_BadFeed: %v", err)
	}
}

func Test_Rego_LoadFragment_InvalidVersion(t *testing.T) {
	f := func(p *generatedConstraints) bool {
		tc, err := setupRegoFragmentVersionErrorTestConfig(p)
		if err != nil {
			t.Error(err)
			return false
		}

		fragment := tc.fragments[0]
		err = tc.policy.LoadFragment(fragment.info.issuer, fragment.info.feed, fragment.code)
		if err == nil {
			t.Error("expected to be unable to load fragment due to invalid version")
			return false
		}

		if !strings.Contains(err.Error(), "fragment version is below the specified minimum") {
			t.Error("expected error string to contain 'fragment version is below the specified minimum'")
			return false
		}

		if _, ok := tc.policy.modules[moduleID(fragment.info.issuer, fragment.info.feed)]; ok {
			t.Error("module not removed upon failure")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 25, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_LoadFragment_InvalidVersion: %v", err)
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
			err = tc.policy.LoadFragment(fragment.info.issuer, fragment.info.feed, fragment.code)
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

			_, err = tc.policy.EnforceCreateContainerPolicy(
				container.sandboxID,
				containerID,
				copyStrings(container.container.Command),
				copyStrings(container.envList),
				container.container.WorkingDir,
				copyMounts(container.mounts))

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
			err = tc.policy.LoadFragment(fragment.info.issuer, fragment.info.feed, fragment.code)
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

			_, err = tc.policy.EnforceCreateContainerPolicy(
				container.sandboxID,
				containerID,
				copyStrings(container.container.Command),
				copyStrings(container.envList),
				container.container.WorkingDir,
				copyMounts(container.mounts))

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

		err = tc.policy.LoadFragment(tc.fragments[0].info.issuer, tc.fragments[0].info.feed, tc.fragments[0].code)
		if err != nil {
			t.Error("unable to load fragment the first time: %w", err)
			return false
		}

		err = tc.policy.LoadFragment(tc.fragments[1].info.issuer, tc.fragments[1].info.feed, tc.fragments[1].code)
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

			_, err = tc.policy.EnforceCreateContainerPolicy(
				container.sandboxID,
				containerID,
				copyStrings(container.container.Command),
				copyStrings(container.envList),
				container.container.WorkingDir,
				copyMounts(container.mounts))

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

		err = tc.policy.LoadFragment(fragment.info.issuer, fragment.info.feed, fragment.code)
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

		err = tc.policy.LoadFragment(fragment.info.issuer, fragment.info.feed, fragment.code)
		if err != nil {
			t.Error("unable to load fragment: %w", err)
			return false
		}

		err = tc.policy.LoadFragment(subFragment.info.issuer, subFragment.info.feed, subFragment.code)
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

		err = tc.policy.LoadFragment(fragment.info.issuer, fragment.info.feed, fragment.code)
		if err != nil {
			t.Error("unable to load fragment: %w", err)
			return false
		}

		envList := buildEnvironmentVariablesFromEnvRules(process.envRules, testRand)
		_, err = tc.policy.EnforceExecExternalProcessPolicy(process.command, envList, process.workingDir)
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
	deviceHash := generateRootHash(testRand)
	key := randVariableString(testRand, 32)
	value := randVariableString(testRand, 32)
	fragmentCode := fmt.Sprintf(`package fragment

svn := "1.0.0"

layer := "%s"

mount_device := {"allowed": allowed, "custom": custom} {
	allowed := input.deviceHash == layer
	custom := {
        "action": "add",
        "key": "%s",
        "value": "%s"
	}
}`, deviceHash, key, value)

	issuer := testDataGenerator.uniqueFragmentIssuer()
	feed := testDataGenerator.uniqueFragmentFeed()
	policyCode := fmt.Sprintf(`package policy

default load_fragment := {"allowed": false}

load_fragment := {"allowed": true, "add_module": true} {
	input.issuer == "%s"
	input.feed == "%s"
	semver.compare(data[input.namespace].svn, "1.0.0") >= 0
}

mount_device := data.fragment.mount_device
	`, issuer, feed)

	policy, err := newRegoPolicy(policyCode, []oci.Mount{}, []oci.Mount{})
	if err != nil {
		t.Fatalf("unable to create Rego policy: %v", err)
	}

	err = policy.LoadFragment(issuer, feed, fragmentCode)
	if err != nil {
		t.Fatalf("unable to load fragment: %v", err)
	}

	err = policy.EnforceDeviceMountPolicy("/mnt/foo", deviceHash)
	if err != nil {
		t.Fatalf("unable to mount device: %v", err)
	}

	custom, err := policy.getMetadata("custom")
	if err != nil {
		t.Error("expected metadata stored by fragment is missing")
	}

	if test, ok := custom[key]; ok {
		if test != value {
			t.Error("incorrect metadata value stored by fragment")
		}
	} else {
		t.Error("unable to located metadata key stored by fragment")
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
			err = smConfig.policy.EnforceScratchMountPolicy(scratchPath, tc.encrypted)
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
			err = smConfig.policy.EnforceScratchMountPolicy(scratchPath, tc.encrypted)
			if err != nil {
				t.Fatalf("scratch_mount policy enforcement unexpectedly was denied: %s", err)
			}

			err = smConfig.policy.EnforceScratchUnmountPolicy(scratchPath)
			if err != nil {
				t.Fatalf("scratch_unmount policy enforcement unexpectedly was denied: %s", err)
			}
		})
	}
}

//
// Setup and "fixtures" follow...
//

func generateExternalProcesses(r *rand.Rand) []*externalProcess {
	var processes []*externalProcess

	numProcesses := atLeastOneAtMost(r, maxExternalProcessesInGeneratedConstraints)
	for i := 0; i < int(numProcesses); i++ {
		processes = append(processes, generateExternalProcess(r))
	}

	return processes
}

func generateExternalProcess(r *rand.Rand) *externalProcess {
	return &externalProcess{
		command:    generateCommand(r),
		envRules:   generateEnvironmentVariableRules(r),
		workingDir: generateWorkingDir(r),
	}
}

func randChoices(r *rand.Rand, numChoices int, numItems int) []int {
	shuffle := r.Perm(numItems)
	if numChoices > numItems {
		return shuffle
	}

	return shuffle[:numChoices]
}

func randChoicesWithReplacement(r *rand.Rand, numChoices int, numItems int) []int {
	choices := make([]int, numChoices)
	for i := 0; i < numChoices; i++ {
		choices[i] = r.Intn(numItems)
	}

	return choices
}

func randChooseStrings(r *rand.Rand, items []string, numChoices int) []string {
	numItems := len(items)
	choiceIndices := randChoices(r, numChoices, numItems)
	choices := make([]string, numChoices)
	for i, index := range choiceIndices {
		choices[i] = items[index]
	}
	return choices
}

func randChooseStringsWithReplacement(r *rand.Rand, items []string, numChoices int) []string {
	numItems := len(items)
	choiceIndices := randChoicesWithReplacement(r, numChoices, numItems)
	choices := make([]string, numChoices)
	for i, index := range choiceIndices {
		choices[i] = items[index]
	}
	return choices
}

func selectExternalProcessFromConstraints(constraints *generatedConstraints, r *rand.Rand) *externalProcess {
	numberOfProcessesInConstraints := len(constraints.externalProcesses)
	return constraints.externalProcesses[r.Intn(numberOfProcessesInConstraints)]
}

func (constraints *generatedConstraints) toPolicy() *securityPolicyInternal {
	return &securityPolicyInternal{
		Containers:                       constraints.containers,
		ExternalProcesses:                constraints.externalProcesses,
		Fragments:                        constraints.fragments,
		AllowPropertiesAccess:            constraints.allowGetProperties,
		AllowDumpStacks:                  constraints.allowDumpStacks,
		AllowRuntimeLogging:              constraints.allowRuntimeLogging,
		AllowEnvironmentVariableDropping: constraints.allowEnvironmentVariableDropping,
		AllowUnencryptedScratch:          constraints.allowUnencryptedScratch,
	}
}

func (constraints *generatedConstraints) toFragment() *securityPolicyFragment {
	return &securityPolicyFragment{
		Namespace:         constraints.namespace,
		SVN:               constraints.svn,
		Containers:        constraints.containers,
		ExternalProcesses: constraints.externalProcesses,
		Fragments:         constraints.fragments,
	}
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
	containersToRun := randChoicesWithReplacement(testRand, numOfRunningContainers, len(gc.containers))
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

	_, err = enforcer.EnforceCreateContainerPolicy(sandboxID, containerID, container.Command, envList, container.WorkingDir, mountSpec.Mounts)
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
	gc.externalProcesses = generateExternalProcesses(testRand)
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

type regoPolicyOnlyTestConfig struct {
	policy *regoEnforcer
}

func setupRegoPolicyOnlyTest(gc *generatedConstraints) (tc *regoPolicyOnlyTestConfig, err error) {
	securityPolicy := gc.toPolicy()
	policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{})
	if err != nil {
		return nil, err
	}

	// see NOTE_TESTCOPY
	return &regoPolicyOnlyTestConfig{
		policy: policy,
	}, nil
}

type regoFragmentTestConfig struct {
	fragments         []*regoFragment
	containers        []*regoFragmentContainer
	externalProcesses []*externalProcess
	subFragments      []*regoFragment
	plan9Mounts       []string
	mountSpec         []string
	policy            *regoEnforcer
}

type regoFragmentContainer struct {
	container *securityPolicyContainer
	envList   []string
	sandboxID string
	mounts    []oci.Mount
}

func setupSimpleRegoFragmentTestConfig(gc *generatedConstraints) (*regoFragmentTestConfig, error) {
	return setupRegoFragmentTestConfig(gc, 1, []string{"containers"}, []string{}, false, false, false)
}

func setupRegoFragmentTestConfigWithIncludes(gc *generatedConstraints, includes []string) (*regoFragmentTestConfig, error) {
	return setupRegoFragmentTestConfig(gc, 1, includes, []string{}, false, false, false)
}

func setupRegoFragmentTestConfigWithExcludes(gc *generatedConstraints, excludes []string) (*regoFragmentTestConfig, error) {
	return setupRegoFragmentTestConfig(gc, 1, []string{}, excludes, false, false, false)
}

func setupRegoFragmentVersionErrorTestConfig(gc *generatedConstraints) (*regoFragmentTestConfig, error) {
	return setupRegoFragmentTestConfig(gc, 1, []string{"containers"}, []string{}, true, false, false)
}

func setupRegoFragmentTwoFeedTestConfig(gc *generatedConstraints, sameIssuer bool, sameFeed bool) (*regoFragmentTestConfig, error) {
	return setupRegoFragmentTestConfig(gc, 2, []string{"containers"}, []string{}, false, sameIssuer, sameFeed)
}

func setupRegoFragmentTestConfig(gc *generatedConstraints, numFragments int, includes []string, excludes []string, versionError bool, sameIssuer bool, sameFeed bool) (tc *regoFragmentTestConfig, err error) {
	gc.fragments = generateFragments(testRand, int32(numFragments))

	if sameIssuer {
		for _, fragment := range gc.fragments {
			fragment.issuer = gc.fragments[0].issuer
			if sameFeed {
				fragment.feed = gc.fragments[0].feed
			}
		}
	}

	fragments := selectFragmentsFromConstraints(gc, numFragments, includes, excludes, versionError)

	containers := make([]*regoFragmentContainer, numFragments)
	subFragments := make([]*regoFragment, numFragments)
	externalProcesses := make([]*externalProcess, numFragments)
	plan9Mounts := make([]string, numFragments)
	for i, fragment := range fragments {
		container := fragment.selectContainer()

		envList := buildEnvironmentVariablesFromEnvRules(container.EnvRules, testRand)
		sandboxID := testDataGenerator.uniqueSandboxID()

		mounts := container.Mounts
		mountSpec := buildMountSpecFromMountArray(mounts, sandboxID, testRand)
		containers[i] = &regoFragmentContainer{
			container: container,
			envList:   envList,
			sandboxID: sandboxID,
			mounts:    mountSpec.Mounts,
		}

		for _, include := range fragment.info.includes {
			switch include {
			case "fragments":
				subFragments[i] = selectFragmentsFromConstraints(fragment.constraints, 1, []string{"containers"}, []string{}, false)[0]
				break

			case "external_processes":
				externalProcesses[i] = selectExternalProcessFromConstraints(fragment.constraints, testRand)
				break
			}
		}

		// now that we've explicitly added the excluded items to the fragment
		// we remove the include string so that the generated policy
		// does not include them.
		fragment.info.includes = removeStringsFromArray(fragment.info.includes, excludes)
	}

	if sameFeed {
		includeSet := make(map[string]bool)
		minSVN := semver.MustParse("9.9.9")
		for _, fragment := range gc.fragments {
			svn := semver.MustParse(fragment.minimumSVN)
			if svn.LT(minSVN) {
				minSVN = svn
			}
			for _, include := range fragment.includes {
				includeSet[include] = true
			}
		}
		frag := gc.fragments[0]
		frag.minimumSVN = minSVN.String()
		frag.includes = make([]string, 0, len(includeSet))
		for include := range includeSet {
			frag.includes = append(frag.includes, include)
		}

		gc.fragments = []*fragment{frag}

	}

	securityPolicy := gc.toPolicy()
	defaultMounts := toOCIMounts(generateMounts(testRand))
	privilegedMounts := toOCIMounts(generateMounts(testRand))
	policy, err := newRegoPolicy(securityPolicy.marshalRego(), defaultMounts, privilegedMounts)

	if err != nil {
		return nil, err
	}

	return &regoFragmentTestConfig{
		fragments:         fragments,
		containers:        containers,
		subFragments:      subFragments,
		externalProcesses: externalProcesses,
		plan9Mounts:       plan9Mounts,
		policy:            policy,
	}, nil
}

type regoDropEnvsTestConfig struct {
	envList     []string
	expected    []string
	argList     []string
	workingDir  string
	containerID string
	sandboxID   string
	mounts      []oci.Mount
	policy      *regoEnforcer
}

func setupRegoDropEnvsTest(disjoint bool) (*regoContainerTestConfig, error) {
	gc := generateConstraints(testRand, 1)
	gc.allowEnvironmentVariableDropping = true

	const numContainers int = 3
	numEnvRules := []int{int(randMinMax(testRand, 1, 4)),
		int(randMinMax(testRand, 1, 4)),
		int(randMinMax(testRand, 1, 4))}
	envRuleLookup := make(stringSet)
	envRules := make([][]EnvRuleConfig, numContainers)

	containers := make([]*securityPolicyContainer, numContainers)
	envs := make([][]string, numContainers)

	for i := 0; i < numContainers; i++ {
		c, err := gc.containers[0].clone()
		if err != nil {
			return nil, err
		}
		containers[i] = c

		rules := envRuleLookup.randUniqueArray(testRand, func(r *rand.Rand) string {
			return randVariableString(r, 10)
		}, int32(numEnvRules[i]))

		envRules[i] = make([]EnvRuleConfig, numEnvRules[i])
		for j, rule := range rules {
			envRules[i][j] = EnvRuleConfig{
				Strategy: "string",
				Rule:     rule,
			}
		}

		envs[i] = buildEnvironmentVariablesFromEnvRules(envRules[i], testRand)
		if i == 0 {
			c.EnvRules = envRules[i]
		} else if disjoint {
			c.EnvRules = append(envRules[0], envRules[i]...)
		} else {
			c.EnvRules = append(containers[i-1].EnvRules, envRules[i]...)
		}
	}

	gc.containers = containers
	securityPolicy := gc.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	policy, err := newRegoPolicy(securityPolicy.marshalRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts))

	if err != nil {
		return nil, err
	}

	containerIDs := make([]string, numContainers)
	for i, c := range gc.containers {
		containerID, err := mountImageForContainer(policy, c)
		if err != nil {
			return nil, err
		}

		containerIDs[i] = containerID
	}

	var envList []string
	if disjoint {
		var extraLen int
		if len(envs[1]) < len(envs[2]) {
			extraLen = len(envs[1])
		} else {
			extraLen = len(envs[2])
		}
		envList = append(envs[0], envs[1][:extraLen]...)
		envList = append(envList, envs[2][:extraLen]...)
	} else {
		envList = append(envs[0], envs[1]...)
		envList = append(envList, envs[2]...)
	}

	sandboxID := testDataGenerator.uniqueSandboxID()

	mounts := containers[2].Mounts
	mounts = append(mounts, defaultMounts...)
	if containers[2].AllowElevated {
		mounts = append(mounts, privilegedMounts...)
	}

	mountSpec := buildMountSpecFromMountArray(mounts, sandboxID, testRand)
	// see NOTE_TESTCOPY
	return &regoContainerTestConfig{
		envList:     copyStrings(envList),
		argList:     copyStrings(containers[2].Command),
		workingDir:  containers[2].WorkingDir,
		containerID: containerIDs[2],
		sandboxID:   sandboxID,
		mounts:      copyMounts(mountSpec.Mounts),
		policy:      policy,
	}, nil
}

type regoFragment struct {
	info        *fragment
	constraints *generatedConstraints
	code        string
}

func (f *regoFragment) selectContainer() *securityPolicyContainer {
	return selectContainerFromConstraints(f.constraints, testRand)
}

func selectFragmentsFromConstraints(gc *generatedConstraints, numFragments int, includes []string, excludes []string, versionError bool) []*regoFragment {
	choices := randChoices(testRand, numFragments, len(gc.fragments))
	fragments := make([]*regoFragment, numFragments)
	for i, choice := range choices {
		config := gc.fragments[choice]
		config.includes = addStringsToArray(config.includes, includes)
		// since we want to test that the policy cannot include an excluded
		// quantity, we must first ensure they are in the fragment
		config.includes = addStringsToArray(config.includes, excludes)

		constraints := generateConstraints(testRand, maxContainersInGeneratedConstraints)
		for _, include := range config.includes {
			switch include {
			case "fragments":
				constraints.fragments = generateFragments(testRand, 1)
				for _, fragment := range constraints.fragments {
					fragment.includes = addStringsToArray(fragment.includes, []string{"containers"})
				}
				break

			case "external_processes":
				constraints.externalProcesses = generateExternalProcesses(testRand)
				break
			}
		}
		code := constraints.toPolicy().marshalRego()

		version := config.minimumSVN
		if versionError {
			sv := semver.MustParse(version)
			sv.IncrementMajor()
			config.minimumSVN = sv.String()
		}

		namespace := testDataGenerator.uniqueFragmentNamespace()
		fragmentHeader := fmt.Sprintf("package %s\n\nsvn := \"%s\"\n", namespace, version)
		code = strings.Replace(code, "package policy", fragmentHeader, 1)
		fragments[i] = &regoFragment{
			info:        config,
			constraints: constraints,
			code:        code,
		}
	}

	return fragments
}

func generateSandboxID(r *rand.Rand) string {
	return randVariableString(r, maxGeneratedSandboxIDLength)
}

func generateEnforcementPoint(r *rand.Rand) string {
	first := randChar(r)
	return first + randString(r, atMost(r, maxGeneratedEnforcementPointLength))
}

func (gen *dataGenerator) uniqueSandboxID() string {
	return gen.sandboxIDs.randUnique(gen.rng, generateSandboxID)
}

func (gen *dataGenerator) uniqueEnforcementPoint() string {
	return gen.enforcementPoints.randUnique(gen.rng, generateEnforcementPoint)
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

func (policy *regoEnforcer) injectTestAPI() error {
	modules := map[string]string{
		"policy.rego":    policy.code,
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
		policy.compiledModules = compiled
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

func generateUVMPathForShare(r *rand.Rand, containerID string) string {
	return fmt.Sprintf("%s/%s%s",
		guestpath.LCOWRootPrefixInUVM,
		containerID,
		fmt.Sprintf(guestpath.LCOWMountPathPrefixFmt, atMost(r, maxPlan9MountIndex)))
}

func generateFragments(r *rand.Rand, minFragments int32) []*fragment {
	numFragments := randMinMax(r, minFragments, maxFragmentsInGeneratedConstraints)

	fragments := make([]*fragment, numFragments)
	for i := 0; i < int(numFragments); i++ {
		fragments[i] = generateFragment(r)
	}

	return fragments
}

func generateFragmentIssuer(r *rand.Rand) string {
	return randString(r, maxGeneratedFragmentIssuerLength)
}

func generateFragmentFeed(r *rand.Rand) string {
	return randString(r, maxGeneratedFragmentFeedLength)
}

func (gen *dataGenerator) uniqueFragmentNamespace() string {
	return gen.fragmentNamespaces.randUnique(gen.rng, generateFragmentNamespace)
}

func (gen *dataGenerator) uniqueFragmentIssuer() string {
	return gen.fragmentIssuers.randUnique(gen.rng, generateFragmentIssuer)
}

func (gen *dataGenerator) uniqueFragmentFeed() string {
	return gen.fragmentFeeds.randUnique(gen.rng, generateFragmentFeed)
}

func generateFragment(r *rand.Rand) *fragment {
	possibleIncludes := []string{"containers", "fragments", "external_processes"}
	numChoices := int(atLeastOneAtMost(r, int32(len(possibleIncludes))))
	includes := randChooseStrings(r, possibleIncludes, numChoices)
	return &fragment{
		issuer:     testDataGenerator.uniqueFragmentIssuer(),
		feed:       testDataGenerator.uniqueFragmentFeed(),
		minimumSVN: generateSVN(r),
		includes:   includes,
	}
}

func addStringsToArray(values []string, valuesToAdd []string) []string {
	toAdd := []string{}
	for _, valueToAdd := range valuesToAdd {
		add := true
		for _, value := range values {
			if value == valueToAdd {
				add = false
				break
			}
		}
		if add {
			toAdd = append(toAdd, valueToAdd)
		}
	}

	return append(values, toAdd...)
}

func removeStringsFromArray(values []string, valuesToRemove []string) []string {
	remain := make([]string, 0, len(values))
	for _, value := range values {
		keep := true
		for _, toRemove := range valuesToRemove {
			if value == toRemove {
				keep = false
				break
			}
		}
		if keep {
			remain = append(remain, value)
		}
	}

	return remain
}

func areStringArraysEqual(lhs []string, rhs []string) bool {
	if len(lhs) != len(rhs) {
		return false
	}

	sort.Strings(lhs)
	sort.Strings(rhs)

	for i, a := range lhs {
		if a != rhs[i] {
			return false
		}
	}

	return true
}

func (c securityPolicyContainer) clone() (*securityPolicyContainer, error) {
	contents, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}

	var clone securityPolicyContainer
	err = json.Unmarshal(contents, &clone)
	if err != nil {
		return nil, err
	}

	return &clone, nil
}

func (p externalProcess) clone() *externalProcess {
	envRules := make([]EnvRuleConfig, len(p.envRules))
	copy(envRules, p.envRules)

	return &externalProcess{
		command:    copyStrings(p.command),
		envRules:   envRules,
		workingDir: p.workingDir,
	}
}

func (c *securityPolicyContainer) toContainer() *Container {
	execProcesses := make([]ExecProcessConfig, len(c.ExecProcesses))
	for i, ep := range c.ExecProcesses {
		execProcesses[i] = ExecProcessConfig(ep)
	}

	return &Container{
		Command:       CommandArgs(stringArrayToStringMap(c.Command)),
		EnvRules:      envRuleArrayToEnvRules(c.EnvRules),
		Layers:        Layers(stringArrayToStringMap(c.Layers)),
		WorkingDir:    c.WorkingDir,
		Mounts:        mountArrayToMounts(c.Mounts),
		AllowElevated: c.AllowElevated,
		ExecProcesses: execProcesses,
		Signals:       c.Signals,
	}
}

func envRuleArrayToEnvRules(envRules []EnvRuleConfig) EnvRules {
	elements := make(map[string]EnvRuleConfig)
	for i, envRule := range envRules {
		elements[strconv.Itoa(i)] = envRule
	}
	return EnvRules{
		Elements: elements,
		Length:   len(envRules),
	}
}

func mountArrayToMounts(mounts []mountInternal) Mounts {
	elements := make(map[string]Mount)
	for i, mount := range mounts {
		elements[strconv.Itoa(i)] = Mount{
			Source:      mount.Source,
			Destination: mount.Destination,
			Type:        mount.Type,
			Options:     Options(stringArrayToStringMap(mount.Options)),
		}
	}

	return Mounts{
		Elements: elements,
		Length:   len(mounts),
	}
}

func (p externalProcess) toConfig() ExternalProcessConfig {
	return ExternalProcessConfig{
		Command:    p.command,
		WorkingDir: p.workingDir,
	}
}

func (f fragment) toConfig() FragmentConfig {
	return FragmentConfig{
		Issuer:     f.issuer,
		Feed:       f.feed,
		MinimumSVN: f.minimumSVN,
		Includes:   f.includes,
	}
}

func stringArrayToStringMap(values []string) StringArrayMap {
	elements := make(map[string]string)
	for i, value := range values {
		elements[strconv.Itoa(i)] = value
	}

	return StringArrayMap{
		Elements: elements,
		Length:   len(values),
	}
}

func (s *stringSet) randUniqueArray(r *rand.Rand, generator func(*rand.Rand) string, numItems int32) []string {
	items := make([]string, numItems)
	for i := 0; i < int(numItems); i++ {
		items[i] = s.randUnique(r, generator)
	}
	return items
}

type regoScratchMountPolicyTestConfig struct {
	policy *regoEnforcer
}

func setupRegoScratchMountTest(
	gc *generatedConstraints,
	unencryptedScratch bool,
) (tc *regoScratchMountPolicyTestConfig, err error) {
	securityPolicy := gc.toPolicy()
	securityPolicy.AllowUnencryptedScratch = unencryptedScratch

	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)
	policy, err := newRegoPolicy(securityPolicy.marshalRego(), toOCIMounts(defaultMounts), toOCIMounts(privilegedMounts))
	if err != nil {
		return nil, err
	}
	return &regoScratchMountPolicyTestConfig{
		policy: policy,
	}, nil
}
