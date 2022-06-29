//go:build linux
// +build linux

package securitypolicy

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"testing/quick"
	"time"

	oci "github.com/opencontainers/runtime-spec/specs-go"
)

func newCommandFromInternal(args []string) CommandArgs {
	command := CommandArgs{}
	command.Length = len(args)
	command.Elements = make(map[string]string)
	for i, arg := range args {
		command.Elements[fmt.Sprint(i)] = arg
	}
	return command
}

func newEnvRulesFromInternal(rules []EnvRuleConfig) EnvRules {
	envRules := EnvRules{}
	envRules.Length = len(rules)
	envRules.Elements = make(map[string]EnvRuleConfig)
	for i, rule := range rules {
		envRules.Elements[fmt.Sprint(i)] = rule
	}
	return envRules
}

func newLayersFromInternal(hashes []string) Layers {
	layers := Layers{}
	layers.Length = len(hashes)
	layers.Elements = make(map[string]string)
	for i, hash := range hashes {
		layers.Elements[fmt.Sprint(i)] = hash
	}
	return layers
}

func newOptionsFromInternal(optionsInternal []string) Options {
	options := Options{}
	options.Length = len(optionsInternal)
	options.Elements = make(map[string]string)
	for i, arg := range optionsInternal {
		options.Elements[fmt.Sprint(i)] = arg
	}
	return options
}

func newMountsFromInternal(mountsInternal []mountInternal) Mounts {
	mounts := Mounts{}
	mounts.Length = len(mountsInternal)
	mounts.Elements = make(map[string]Mount)
	for i, mount := range mountsInternal {
		mounts.Elements[fmt.Sprint(i)] = Mount{
			Source:      mount.Source,
			Destination: mount.Destination,
			Options:     newOptionsFromInternal(mount.Options),
			Type:        mount.Type,
		}
	}

	return mounts
}

func securityPolicyFromInternal(p *generatedContainers) *SecurityPolicy {
	securityPolicy := new(SecurityPolicy)
	securityPolicy.AllowAll = false
	securityPolicy.Containers.Length = len(p.containers)
	securityPolicy.Containers.Elements = make(map[string]Container)
	for i, c := range p.containers {
		container := Container{
			AllowElevated: c.AllowElevated,
			WorkingDir:    c.WorkingDir,
			Command:       newCommandFromInternal(c.Command),
			EnvRules:      newEnvRulesFromInternal(c.EnvRules),
			Layers:        newLayersFromInternal(c.Layers),
			Mounts:        newMountsFromInternal(c.Mounts),
		}
		securityPolicy.Containers.Elements[fmt.Sprint(i)] = container
	}
	return securityPolicy
}

func Test_MarshalRego(t *testing.T) {
	f := func(p *generatedContainers) bool {
		base64policy, err := securityPolicyFromInternal(p).EncodeToString()

		if err != nil {
			t.Errorf("unable to encode policy to base64: %v", err)
		}

		_, err = NewRegoPolicyFromBase64Json(base64policy, []oci.Mount{}, []oci.Mount{})
		if err != nil {
			t.Errorf("unable to convert policy to rego: %v", err)
		}

		return !t.Failed()
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 4}); err != nil {
		t.Errorf("Test_MarshalRego failed: %v", err)
	}
}

// Verify that StandardSecurityPolicyEnforcer.EnforceDeviceMountPolicy doesn't
// return an error when there's a matching root hash in the policy
func Test_Rego_EnforceDeviceMountPolicy_Matches(t *testing.T) {
	f := func(p *generatedContainers) bool {
		securityPolicy := securityPolicyFromInternal(p)
		policy, err := NewRegoPolicyFromSecurityPolicy(securityPolicy, []oci.Mount{}, []oci.Mount{})
		if err != nil {
			t.Errorf("unable to convert policy to rego: %v", err)
		}

		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		target := generateMountTarget(r)
		rootHash := selectRootHashFromContainers(p, r)

		err = policy.EnforceDeviceMountPolicy(target, rootHash)

		// getting an error means something is broken
		return err == nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("Test_Rego_EnforceDeviceMountPolicy_Matches failed: %v", err)
	}
}

// Verify that StandardSecurityPolicyEnforcer.EnforceDeviceMountPolicy will
// return an error when there's no matching root hash in the policy
func Test_Rego_EnforceDeviceMountPolicy_No_Matches(t *testing.T) {
	f := func(p *generatedContainers) bool {
		securityPolicy := securityPolicyFromInternal(p)
		policy, err := NewRegoPolicyFromSecurityPolicy(securityPolicy, []oci.Mount{}, []oci.Mount{})
		if err != nil {
			t.Errorf("unable to convert policy to rego: %v", err)
		}

		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		target := generateMountTarget(r)
		rootHash := generateInvalidRootHash(r)

		err = policy.EnforceDeviceMountPolicy(target, rootHash)

		// we expect an error, not getting one means something is broken
		return err != nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("Test_Rego_EnforceDeviceMountPolicy_No_Matches failed: %v", err)
	}
}

type regoOverlayTestConfig struct {
	layers      []string
	containerID string
	policy      *RegoPolicy
}

func setupRegoOverlayTest(gc *generatedContainers, valid bool) (tc *regoOverlayTestConfig, err error) {
	securityPolicy := securityPolicyFromInternal(gc)
	policy, err := NewRegoPolicyFromSecurityPolicy(securityPolicy, []oci.Mount{}, []oci.Mount{})
	if err != nil {
		return nil, err
	}

	containerID := generateContainerID(testRand)
	c := selectContainerFromContainers(gc, testRand)

	var layerPaths []string
	if valid {
		layerPaths, err = createValidOverlayForContainer(policy, c, testRand)
		if err != nil {
			return nil, fmt.Errorf("error creating valid overlay: %w", err)
		}
	} else {
		layerPaths, err = createInvalidOverlayForContainer(policy, c, testRand)
		if err != nil {
			return nil, fmt.Errorf("error creating invalid overlay: %w", err)
		}
	}

	return &regoOverlayTestConfig{
		layers:      layerPaths,
		containerID: containerID,
		policy:      policy,
	}, nil
}

type regoContainerTestConfig struct {
	envList     []string
	argList     []string
	workingDir  string
	containerID string
	policy      *RegoPolicy
}

func setupRegoContainerTest(gc *generatedContainers) (tc *regoContainerTestConfig, err error) {
	securityPolicy := securityPolicyFromInternal(gc)
	policy, err := NewRegoPolicyFromSecurityPolicy(securityPolicy, []oci.Mount{}, []oci.Mount{})
	if err != nil {
		return nil, err
	}

	containerID := generateContainerID(testRand)
	c := selectContainerFromContainers(gc, testRand)

	envList := buildEnvironmentVariablesFromContainerRules(c, testRand)

	return &regoContainerTestConfig{
		envList:     envList,
		argList:     c.Command,
		workingDir:  c.WorkingDir,
		containerID: containerID,
		policy:      policy,
	}, nil
}

// Verify that StandardSecurityPolicyEnforcer.EnforceOverlayMountPolicy will
// return an error when there's no matching overlay targets.
func Test_Rego_EnforceOverlayMountPolicy_No_Matches(t *testing.T) {
	f := func(p *generatedContainers) bool {
		tc, err := setupRegoOverlayTest(p, false)
		if err != nil {
			t.Error(err)
			return false
		}

		err = tc.policy.EnforceOverlayMountPolicy(tc.containerID, tc.layers)

		// not getting an error means something is broken
		return err != nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 10}); err != nil {
		t.Errorf("Test_Rego_EnforceOverlayMountPolicy_No_Matches failed: %v", err)
	}
}

// Verify that StandardSecurityPolicyEnforcer.EnforceOverlayMountPolicy doesn't
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

	if err := quick.Check(f, &quick.Config{MaxCount: 10}); err != nil {
		t.Errorf("Test_Rego_EnforceOverlayMountPolicy_Matches: %v", err)
	}
}

func Test_Rego_EnforceDeviceUmountPolicy_Removes_Device_Entries(t *testing.T) {
	f := func(p *generatedContainers) bool {
		securityPolicy := securityPolicyFromInternal(p)
		policy, err := NewRegoPolicyFromSecurityPolicy(securityPolicy, []oci.Mount{}, []oci.Mount{})
		if err != nil {
			t.Error(err)
			return false
		}

		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		target := generateMountTarget(r)
		rootHash := selectRootHashFromContainers(p, r)

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

	if err := quick.Check(f, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("Test_Rego_EnforceDeviceUmountPolicy_Removes_Device_Entries failed: %v", err)
	}
}

func Test_Rego_EnforceCreateContainer(t *testing.T) {
	f := func(p *generatedContainers) bool {
		tc, err := setupRegoContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		err = tc.policy.EnforceCreateContainerPolicy(tc.containerID, tc.argList, tc.envList, tc.workingDir)

		// getting an error means something is broken
		return err == nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("Test_Rego_EnforceCreateContainer: %v", err)
	}
}

func Test_Rego_EnforceCommandPolicy_NoMatches(t *testing.T) {
	f := func(p *generatedContainers) bool {
		tc, err := setupRegoContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		err = tc.policy.EnforceCreateContainerPolicy(tc.containerID, generateCommand(testRand), tc.envList, tc.workingDir)

		if err == nil {
			return false
		}

		return strings.Contains(err.Error(), "invalid command")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("Test_EnforceCommandPolicy_NoMatches: %v", err)
	}
}

func Test_Rego_EnforceEnvironmentVariablePolicy_NotAllMatches(t *testing.T) {
	f := func(p *generatedContainers) bool {
		tc, err := setupRegoContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		envList := append(tc.envList, generateNeverMatchingEnvironmentVariable(testRand))
		err = tc.policy.EnforceCreateContainerPolicy(tc.containerID, tc.argList, envList, tc.workingDir)

		// not getting an error means something is broken
		if err == nil {
			return false
		}

		return strings.Contains(err.Error(), "invalid env list")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("Test_Rego_EnforceEnvironmentVariablePolicy_NotAllMatches: %v", err)
	}
}

func Test_Rego_WorkingDirectoryPolicy_NoMatches(t *testing.T) {
	testFunc := func(gc *generatedContainers) bool {
		tc, err := setupRegoContainerTest(gc)
		if err != nil {
			t.Error(err)
			return false
		}

		err = tc.policy.EnforceCreateContainerPolicy(tc.containerID, tc.argList, tc.envList, randString(testRand, 20))
		// not getting an error means something is broken
		if err == nil {
			return false
		}

		return strings.Contains(err.Error(), "invalid working directory")
	}

	if err := quick.Check(testFunc, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("Test_Rego_WorkingDirectoryPolicy_NoMatches: %v", err)
	}
}
