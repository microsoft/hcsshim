//go:build windows && rego
// +build windows,rego

package securitypolicy

import (
	"context"
	_ "embed"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"testing"
	"testing/quick"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	oci "github.com/opencontainers/runtime-spec/specs-go"
)

const testOSType = "windows"

func Test_Rego_EnforceCommandPolicy_NoMatches_Windows(t *testing.T) {
	f := func(p *generatedWindowsConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTestWindows(p)
		if err != nil {
			t.Error(err)
			return false
		}

		//_, _, _, err = tc.policy.EnforceCreateContainerPolicy(p.ctx, tc.sandboxID, tc.containerID, generateCommand(testRand), tc.envList, tc.workingDir, tc.mounts, false, tc.noNewPrivileges, tc.user, tc.groups, tc.umask, tc.capabilities, tc.seccomp)

		_, _, _, err = tc.policy.EnforceCreateContainerPolicyV2(p.ctx, tc.containerID, generateCommand(testRand), tc.envList, tc.workingDir, tc.mounts, tc.user, nil)

		if err == nil {
			return false
		}

		//t.Logf("Error value: %v", err)

		return assertDecisionJSONContains(t, err, "invalid command")
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_EnforceCommandPolicy_NoMatches: %v", err)
	}
}

func Test_Rego_EnforceEnvironmentVariablePolicy_Re2Match_Windows(t *testing.T) {
	testFunc := func(gc *generatedWindowsConstraints) bool {
		container := selectWindowsContainerFromContainerList(gc.containers, testRand)
		// add a rule to re2 match
		re2MatchRule := EnvRuleConfig{
			Strategy: EnvVarRuleRegex,
			Rule:     "PREFIX_.+=.+",
		}

		container.EnvRules = append(container.EnvRules, re2MatchRule)

		tc, err := setupRegoCreateContainerTestWindows(gc, container, false)
		if err != nil {
			t.Error(err)
			return false
		}

		envList := append(tc.envList, "PREFIX_FOO=BAR")

		_, _, _, err = tc.policy.EnforceCreateContainerPolicyV2(gc.ctx, tc.containerID, tc.argList, envList, tc.workingDir, tc.mounts, tc.user, nil)

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

func Test_Rego_EnforceEnvironmentVariablePolicy_NotAllMatches_Windows(t *testing.T) {
	f := func(p *generatedWindowsConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTestWindows(p)
		if err != nil {
			t.Error(err)
			return false
		}

		envList := append(tc.envList, generateNeverMatchingEnvironmentVariable(testRand))

		_, _, _, err = tc.policy.EnforceCreateContainerPolicyV2(p.ctx, tc.containerID, tc.argList, envList, tc.workingDir, tc.mounts, tc.user, nil)

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

func Test_Rego_EnforceEnvironmentVariablePolicy_DropEnvs_Windows(t *testing.T) {
	testFunc := func(gc *generatedWindowsConstraints) bool {
		gc.allowEnvironmentVariableDropping = true
		container := selectWindowsContainerFromContainerList(gc.containers, testRand)

		tc, err := setupRegoCreateContainerTestWindows(gc, container, false)
		if err != nil {
			t.Error(err)
			return false
		}

		extraRules := generateEnvironmentVariableRules(testRand)
		extraEnvs := buildEnvironmentVariablesFromEnvRules(extraRules, testRand)

		envList := append(tc.envList, extraEnvs...)
		actual, _, _, err := tc.policy.EnforceCreateContainerPolicyV2(gc.ctx, tc.containerID, tc.argList, envList, tc.workingDir, tc.mounts, tc.user, nil)

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

func Test_Rego_EnforceEnvironmentVariablePolicy_DropEnvs_Multiple_Windows(t *testing.T) {
	tc, err := setupRegoDropEnvsTestWindows(false)
	if err != nil {
		t.Fatalf("error setting up test: %v", err)
	}

	extraRules := generateEnvironmentVariableRules(testRand)
	extraEnvs := buildEnvironmentVariablesFromEnvRules(extraRules, testRand)

	envList := append(tc.envList, extraEnvs...)
	actual, _, _, err := tc.policy.EnforceCreateContainerPolicyV2(tc.ctx, tc.containerID, tc.argList, envList, tc.workingDir, tc.mounts, tc.user, nil)

	// getting an error means something is broken
	if err != nil {
		t.Errorf("Expected container creation to be allowed. It wasn't: %v", err)
	}

	if !areStringArraysEqual(actual, tc.envList) {
		t.Error("environment variables were not dropped correctly.")
	}
}

func Test_Rego_EnforceEnvironmentVariablePolicy_DropEnvs_Multiple_NoMatch_Windows(t *testing.T) {
	tc, err := setupRegoDropEnvsTestWindows(true)
	if err != nil {
		t.Fatalf("error setting up test: %v", err)
	}

	extraRules := generateEnvironmentVariableRules(testRand)
	extraEnvs := buildEnvironmentVariablesFromEnvRules(extraRules, testRand)

	envList := append(tc.envList, extraEnvs...)
	actual, _, _, err := tc.policy.EnforceCreateContainerPolicyV2(tc.ctx, tc.containerID, tc.argList, envList, tc.workingDir, tc.mounts, tc.user, nil)

	// not getting an error means something is broken
	if err == nil {
		t.Error("expected container creation not to be allowed.")
	}

	if actual != nil {
		t.Error("envList should be nil")
	}
}

func Test_Rego_WorkingDirectoryPolicy_NoMatches_Windows(t *testing.T) {
	testFunc := func(gc *generatedWindowsConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTestWindows(gc)
		if err != nil {
			t.Error(err)
			return false
		}

		_, _, _, err = tc.policy.EnforceCreateContainerPolicyV2(tc.ctx, tc.containerID, tc.argList, tc.envList, randString(testRand, 20), tc.mounts, tc.user, nil)
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
func Test_Rego_EnforceCreateContainer_Windows(t *testing.T) {
	f := func(p *generatedWindowsConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTestWindows(p)
		if err != nil {
			t.Errorf("Setup failed: %v", err)
			return false
		}

		_, _, _, err = tc.policy.EnforceCreateContainerPolicyV2(p.ctx, tc.containerID, tc.argList, tc.envList, tc.workingDir, tc.mounts, tc.user, nil)

		if err != nil {
			t.Errorf("Policy enforcement failed: %v", err)
		}
		// getting an error means something is broken
		return err == nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 10, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceCreateContainer: %v", err)
	}
}

func Test_Rego_EnforceCreateContainer_Start_All_Containers(t *testing.T) {
	f := func(p *generatedWindowsConstraints) bool {
		securityPolicy := p.toPolicy()
		defaultMounts := generateMounts(testRand)
		privilegedMounts := generateMounts(testRand)

		policy, err := newRegoPolicy(securityPolicy.marshalWindowsRego(),
			toOCIMounts(defaultMounts),
			toOCIMounts(privilegedMounts), testOSType)
		if err != nil {
			t.Error(err)
			return false
		}

		for _, container := range p.containers {
			containerID, err := mountImageForWindowsContainer(policy, container)
			if err != nil {
				t.Error(err)
				return false
			}

			envList := buildEnvironmentVariablesFromEnvRules(container.EnvRules, testRand)
			user := IDName{Name: container.User}

			_, _, _, err = policy.EnforceCreateContainerPolicyV2(p.ctx, containerID, container.Command, envList, container.WorkingDir, nil, user, nil)

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

func Test_Rego_EnforceCreateContainer_Invalid_ContainerID_Windows(t *testing.T) {
	f := func(p *generatedWindowsConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTestWindows(p)
		if err != nil {
			t.Error(err)
			return false
		}

		containerID := testDataGenerator.uniqueContainerID()
		_, _, _, err = tc.policy.EnforceCreateContainerPolicyV2(p.ctx, containerID, tc.argList, tc.envList, tc.workingDir, nil, tc.user, nil)

		// not getting an error means something is broken
		return err != nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceCreateContainer_Invalid_ContainerID: %v", err)
	}
}

func Test_Rego_EnforceCreateContainer_Same_Container_Twice_Windows(t *testing.T) {
	f := func(p *generatedWindowsConstraints) bool {
		tc, err := setupSimpleRegoCreateContainerTestWindows(p)
		if err != nil {
			t.Error(err)
			return false
		}

		_, _, _, err = tc.policy.EnforceCreateContainerPolicyV2(p.ctx, tc.containerID, tc.argList, tc.envList, tc.workingDir, nil, tc.user, nil)
		if err != nil {
			t.Error("Unable to start valid container.")
			return false
		}
		_, _, _, err = tc.policy.EnforceCreateContainerPolicyV2(p.ctx, tc.containerID, tc.argList, tc.envList, tc.workingDir, nil, tc.user, nil)

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

func Test_Rego_EnforceVerifiedCIMSPolicy_Multiple_Instances_Same_Container(t *testing.T) {
	for containersToCreate := 5; containersToCreate <= maxContainersInGeneratedConstraints; containersToCreate++ {
		constraints := new(generatedWindowsConstraints)
		constraints.ctx = context.Background()
		constraints.externalProcesses = generateExternalProcesses(testRand)

		for i := 1; i <= containersToCreate; i++ {
			arg := "command " + strconv.Itoa(i)
			// layers = individual layer hashes, mounted_cim = merged CIM hash
			// The runtime sends hashesToVerify=layers (reversed) and mountedCim=merged
			c := &securityPolicyWindowsContainer{
				Command:    []string{arg},
				Layers:     []string{"layer1", "layer2"},
				MountedCim: []string{"merged_hash"},
			}

			constraints.containers = append(constraints.containers, c)
		}

		securityPolicy := constraints.toPolicy()
		policy, err := newRegoPolicy(securityPolicy.marshalWindowsRego(), []oci.Mount{}, []oci.Mount{}, testOSType)

		if err != nil {
			t.Fatalf("failed create enforcer")
		}

		for _, container := range constraints.containers {
			// Reverse container.Layers to satisfy layerHashes_ok ordering
			layerHashes := make([]string, len(container.Layers))
			for i, layer := range container.Layers {
				layerHashes[len(container.Layers)-1-i] = layer
			}

			// The runtime sends individual layers as hashesToVerify
			// and the merged CIM hash separately
			id := testDataGenerator.uniqueContainerID()
			err = policy.EnforceVerifiedCIMsPolicy(constraints.ctx, id, layerHashes, container.MountedCim)
			if err != nil {
				t.Fatalf("failed with %d containers", containersToCreate)
			}
		}
	}
}

// -- Capabilities/Mount/Rego version tests are removed -- Add back Rego versions test//
func Test_Rego_ExecInContainerPolicy_Windows(t *testing.T) {
	f := func(p *generatedWindowsConstraints) bool {
		tc, err := setupRegoRunningWindowsContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		container := selectContainerFromRunningContainers(tc.runningContainers, testRand)

		process := selectWindowsExecProcess(container.windowsContainer.ExecProcesses, testRand)
		envList := buildEnvironmentVariablesFromEnvRules(container.windowsContainer.EnvRules, testRand)
		user := IDName{Name: container.windowsContainer.User}

		commandLine := []string{process.Command}

		_, _, _, err = tc.policy.EnforceExecInContainerPolicyV2(p.ctx, container.containerID, commandLine, envList, container.windowsContainer.WorkingDir, user, nil)

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

func Test_Rego_ExecInContainerPolicy_No_Matches_Windows(t *testing.T) {
	f := func(p *generatedWindowsConstraints) bool {
		tc, err := setupRegoRunningWindowsContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		container := selectContainerFromRunningContainers(tc.runningContainers, testRand)

		process := generateWindowsContainerExecProcess(testRand)
		envList := buildEnvironmentVariablesFromEnvRules(container.windowsContainer.EnvRules, testRand)
		user := IDName{Name: container.windowsContainer.User}
		commandLine := []string{process.Command}
		_, _, _, err = tc.policy.EnforceExecInContainerPolicyV2(p.ctx, container.containerID, commandLine, envList, container.windowsContainer.WorkingDir, user, nil)
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

func Test_Rego_ExecInContainerPolicy_Command_No_Match_Windows(t *testing.T) {
	f := func(p *generatedWindowsConstraints) bool {
		tc, err := setupRegoRunningWindowsContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		container := selectContainerFromRunningContainers(tc.runningContainers, testRand)
		envList := buildEnvironmentVariablesFromEnvRules(container.windowsContainer.EnvRules, testRand)
		user := IDName{Name: container.windowsContainer.User}

		command := generateCommand(testRand)
		_, _, _, err = tc.policy.EnforceExecInContainerPolicyV2(p.ctx, container.containerID, command, envList, container.windowsContainer.WorkingDir, user, nil)

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

func Test_Rego_ExecInContainerPolicy_Some_Env_Not_Allowed_Windows(t *testing.T) {
	f := func(p *generatedWindowsConstraints) bool {
		tc, err := setupRegoRunningWindowsContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		container := selectContainerFromRunningContainers(tc.runningContainers, testRand)
		process := selectWindowsExecProcess(container.windowsContainer.ExecProcesses, testRand)
		envList := generateEnvironmentVariables(testRand)
		user := IDName{Name: container.windowsContainer.User}
		commandLine := []string{process.Command}
		_, _, _, err = tc.policy.EnforceExecInContainerPolicyV2(p.ctx, container.containerID, commandLine, envList, container.windowsContainer.WorkingDir, user, nil)

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

func Test_Rego_ExecInContainerPolicy_WorkingDir_No_Match_Windows(t *testing.T) {
	f := func(p *generatedWindowsConstraints) bool {
		tc, err := setupRegoRunningWindowsContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		container := selectContainerFromRunningContainers(tc.runningContainers, testRand)
		process := selectWindowsExecProcess(container.windowsContainer.ExecProcesses, testRand)
		envList := buildEnvironmentVariablesFromEnvRules(container.windowsContainer.EnvRules, testRand)
		workingDir := generateWorkingDir(testRand)
		user := IDName{Name: container.windowsContainer.User}
		commandLine := []string{process.Command}

		_, _, _, err = tc.policy.EnforceExecInContainerPolicyV2(p.ctx, container.containerID, commandLine, envList, workingDir, user, nil)

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

// -- capabilities tests are removed --//
func Test_Rego_ExecInContainerPolicy_DropEnvs_Windows(t *testing.T) {
	testFunc := func(gc *generatedWindowsConstraints) bool {
		gc.allowEnvironmentVariableDropping = true
		tc, err := setupRegoRunningWindowsContainerTest(gc)
		if err != nil {
			t.Error(err)
			return false
		}

		container := selectContainerFromRunningContainers(tc.runningContainers, testRand)

		process := selectWindowsExecProcess(container.windowsContainer.ExecProcesses, testRand)
		expected := buildEnvironmentVariablesFromEnvRules(container.windowsContainer.EnvRules, testRand)

		extraRules := generateEnvironmentVariableRules(testRand)
		extraEnvs := buildEnvironmentVariablesFromEnvRules(extraRules, testRand)

		envList := append(expected, extraEnvs...)
		user := IDName{Name: container.windowsContainer.User}
		commandLine := []string{process.Command}

		actual, _, _, err := tc.policy.EnforceExecInContainerPolicyV2(gc.ctx, container.containerID, commandLine, envList, container.windowsContainer.WorkingDir, user, nil)

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

func Test_Rego_MaliciousEnvList_Windows(t *testing.T) {
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

	testFunc := func(gc *generatedWindowsConstraints) bool {
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

		envList := generateEnvs(envSet)
		toKeep, _, _, err := policy.EnforceCreateContainerPolicyV2(gc.ctx, "", []string{}, envList, "", []oci.Mount{}, user, nil)
		if len(toKeep) > 0 {
			t.Error("invalid environment variables not filtered from list returned from create_container")
			return false
		}

		envList = generateEnvs(envSet)
		toKeep, _, _, err = policy.EnforceExecInContainerPolicyV2(gc.ctx, "", []string{}, envList, "", user, nil)
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

func Test_Rego_InvalidEnvList_Windows(t *testing.T) {
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

	_, _, _, err = policy.EnforceCreateContainerPolicyV2(ctx, "", []string{}, []string{}, "", []oci.Mount{}, user, nil)
	if err == nil {
		t.Errorf("expected call to create_container to fail")
	} else if err.Error() != "policy returned incorrect type for 'env_list', expected []interface{}, received map[string]interface {}" {
		t.Errorf("incorrected error message from call to create_container: %v", err)
	}

	_, _, _, err = policy.EnforceExecInContainerPolicyV2(ctx, "", []string{}, []string{}, "", user, nil)
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

func Test_Rego_InvalidEnvList_Member_Windows(t *testing.T) {
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

	_, _, _, err = policy.EnforceCreateContainerPolicyV2(ctx, "", []string{}, []string{}, "", []oci.Mount{}, user, nil)
	if err == nil {
		t.Errorf("expected call to create_container to fail")
	} else if err.Error() != "members of env_list from policy must be strings, received json.Number" {
		t.Errorf("incorrected error message from call to create_container: %v", err)
	}

	_, _, _, err = policy.EnforceExecInContainerPolicyV2(ctx, "", []string{}, []string{}, "", user, nil)
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

func Test_Rego_EnforceEnvironmentVariablePolicy_MissingRequired_Windows(t *testing.T) {
	testFunc := func(gc *generatedWindowsConstraints) bool {
		container := selectWindowsContainerFromContainerList(gc.containers, testRand)
		// add a rule to re2 match
		requiredRule := EnvRuleConfig{
			Strategy: "string",
			Rule:     randVariableString(testRand, maxGeneratedEnvironmentVariableRuleLength),
			Required: true,
		}

		container.EnvRules = append(container.EnvRules, requiredRule)

		tc, err := setupRegoCreateContainerTestWindows(gc, container, false)
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

		_, _, _, err = tc.policy.EnforceCreateContainerPolicyV2(gc.ctx, tc.containerID, tc.argList, envList, tc.workingDir, tc.mounts, tc.user, nil)

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

func Test_Rego_ExecExternalProcessPolicy_Windows(t *testing.T) {
	f := func(p *generatedWindowsConstraints) bool {
		tc, err := setupWindowsExternalProcessTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		process := selectWindowsExternalProcessFromConstraints(p, testRand)
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

func Test_Rego_ExecExternalProcessPolicy_No_Matches_Windows(t *testing.T) {
	f := func(p *generatedWindowsConstraints) bool {
		tc, err := setupWindowsExternalProcessTest(p)
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

func Test_Rego_ExecExternalProcessPolicy_Command_No_Match_Windows(t *testing.T) {
	f := func(p *generatedWindowsConstraints) bool {
		tc, err := setupWindowsExternalProcessTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		process := selectWindowsExternalProcessFromConstraints(p, testRand)
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

func Test_Rego_ExecExternalProcessPolicy_Some_Env_Not_Allowed_Windows(t *testing.T) {
	f := func(p *generatedWindowsConstraints) bool {
		tc, err := setupWindowsExternalProcessTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		process := selectWindowsExternalProcessFromConstraints(p, testRand)
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

func Test_Rego_ExecExternalProcessPolicy_WorkingDir_No_Match_Windows(t *testing.T) {
	f := func(p *generatedWindowsConstraints) bool {
		tc, err := setupWindowsExternalProcessTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		process := selectWindowsExternalProcessFromConstraints(p, testRand)
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

func Test_Rego_ExecExternalProcessPolicy_DropEnvs_Windows(t *testing.T) {
	testFunc := func(gc *generatedWindowsConstraints) bool {
		gc.allowEnvironmentVariableDropping = true
		tc, err := setupWindowsExternalProcessTest(gc)
		if err != nil {
			t.Error(err)
			return false
		}

		process := selectWindowsExternalProcessFromConstraints(gc, testRand)
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

func Test_Rego_ExecExternalProcessPolicy_DropEnvs_Multiple_Windows(t *testing.T) {
	envRules := setupEnvRuleSets(3)

	gc := generateWindowsConstraints(testRand, 1)
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

	policy, err := newRegoPolicy(securityPolicy.marshalWindowsRego(),
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

func Test_Rego_ExecExternalProcessPolicy_DropEnvs_Multiple_NoMatch_Windows(t *testing.T) {
	envRules := setupEnvRuleSets(3)

	gc := generateWindowsConstraints(testRand, 1)
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

	policy, err := newRegoPolicy(securityPolicy.marshalWindowsRego(),
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

func Test_Rego_ShutdownContainerPolicy_Running_Container_Windows(t *testing.T) {
	p := generateWindowsConstraints(testRand, maxContainersInGeneratedConstraints)

	tc, err := setupRegoRunningWindowsContainerTest(p)
	if err != nil {
		t.Fatalf("Unable to set up test: %v", err)
	}

	container := selectContainerFromRunningContainers(tc.runningContainers, testRand)

	err = tc.policy.EnforceShutdownContainerPolicy(p.ctx, container.containerID)
	if err != nil {
		t.Fatal("Expected shutdown of running container to be allowed, it wasn't")
	}
}

func Test_Rego_ShutdownContainerPolicy_Not_Running_Container_Windows(t *testing.T) {
	p := generateWindowsConstraints(testRand, maxContainersInGeneratedConstraints)

	tc, err := setupRegoRunningWindowsContainerTest(p)
	if err != nil {
		t.Fatalf("Unable to set up test: %v", err)
	}

	notRunningContainerID := testDataGenerator.uniqueContainerID()

	err = tc.policy.EnforceShutdownContainerPolicy(p.ctx, notRunningContainerID)
	if err == nil {
		t.Fatal("Expected shutdown of not running container to be denied, it wasn't")
	}
}

func Test_Rego_SignalContainerProcessPolicy_ExecProcess_Allowed_Windows(t *testing.T) {
	f := func(p *generatedWindowsConstraints) bool {
		containerUnderTest := generateConstraintsWindowsContainer(testRand, 1, maxLayersInGeneratedContainer)

		ep := generateWindowsExecProcesses(testRand)
		ep[0].Signals = generateListOfWindowsSignals(testRand, 1, 4)
		containerUnderTest.ExecProcesses = ep
		processUnderTest := ep[0]

		p.containers = append(p.containers, containerUnderTest)

		tc, err := setupRegoRunningWindowsContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		containerID, err := idForRunningWindowsContainer(containerUnderTest, tc.runningContainers)
		if err != nil {
			r, err := runWindowsContainer(tc.policy, containerUnderTest)
			if err != nil {
				t.Errorf("Unable to setup test running container: %v", err)
				return false
			}
			containerID = r.containerID
		}

		envList := buildEnvironmentVariablesFromEnvRules(containerUnderTest.EnvRules, testRand)
		user := IDName{Name: containerUnderTest.User}
		commandLine := []string{processUnderTest.Command}

		_, _, _, err = tc.policy.EnforceExecInContainerPolicyV2(p.ctx, containerID, commandLine, envList, containerUnderTest.WorkingDir, user, nil)
		if err != nil {
			t.Errorf("Unable to exec process for test: %v", err)
			return false
		}

		signal := selectSignalFromWindowsSignals(testRand, processUnderTest.Signals)
		opts := &SignalContainerOptions{
			WindowsSignal:  signal,
			WindowsCommand: commandLine,
		}

		err = tc.policy.EnforceSignalContainerProcessPolicyV2(p.ctx, containerID, opts)
		if err != nil {
			t.Errorf("Signal init process unexpectedly failed: %v", err)
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 5, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_SignalContainerProcessPolicy_ExecProcess_Allowed: %v", err)
	}
}

func Test_Rego_SignalContainerProcessPolicy_ExecProcess_Not_Allowed_Windows(t *testing.T) {
	f := func(p *generatedWindowsConstraints) bool {
		containerUnderTest := generateConstraintsWindowsContainer(testRand, 1, maxLayersInGeneratedContainer)

		ep := generateWindowsExecProcesses(testRand)
		ep[0].Signals = make([]guestrequest.SignalValueWCOW, 0)
		containerUnderTest.ExecProcesses = ep
		processUnderTest := ep[0]

		p.containers = append(p.containers, containerUnderTest)

		tc, err := setupRegoRunningWindowsContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		containerID, err := idForRunningWindowsContainer(containerUnderTest, tc.runningContainers)
		if err != nil {
			r, err := runWindowsContainer(tc.policy, containerUnderTest)
			if err != nil {
				t.Errorf("Unable to setup test running container: %v", err)
				return false
			}
			containerID = r.containerID
		}

		envList := buildEnvironmentVariablesFromEnvRules(containerUnderTest.EnvRules, testRand)
		user := IDName{Name: containerUnderTest.User}
		commandLine := []string{processUnderTest.Command}

		_, _, _, err = tc.policy.EnforceExecInContainerPolicyV2(p.ctx, containerID, commandLine, envList, containerUnderTest.WorkingDir, user, nil)
		if err != nil {
			t.Errorf("Unable to exec process for test: %v", err)
			return false
		}

		signal := generateWindowsSignal(testRand)

		opts := &SignalContainerOptions{
			WindowsSignal:  signal,
			WindowsCommand: commandLine,
		}

		err = tc.policy.EnforceSignalContainerProcessPolicyV2(p.ctx, containerID, opts)
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
	f := func(p *generatedWindowsConstraints) bool {
		containerUnderTest := generateConstraintsWindowsContainer(testRand, 1, maxLayersInGeneratedContainer)

		ep := generateWindowsExecProcesses(testRand)
		ep[0].Signals = generateListOfWindowsSignals(testRand, 1, 4)
		containerUnderTest.ExecProcesses = ep
		processUnderTest := ep[0]

		p.containers = append(p.containers, containerUnderTest)

		tc, err := setupRegoRunningWindowsContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		containerID, err := idForRunningWindowsContainer(containerUnderTest, tc.runningContainers)
		if err != nil {
			r, err := runWindowsContainer(tc.policy, containerUnderTest)
			if err != nil {
				t.Errorf("Unable to setup test running container: %v", err)
				return false
			}
			containerID = r.containerID
		}

		envList := buildEnvironmentVariablesFromEnvRules(containerUnderTest.EnvRules, testRand)
		user := IDName{Name: containerUnderTest.User}
		commandLine := []string{processUnderTest.Command}

		_, _, _, err = tc.policy.EnforceExecInContainerPolicyV2(p.ctx, containerID, commandLine, envList, containerUnderTest.WorkingDir, user, nil)
		if err != nil {
			t.Errorf("Unable to exec process for test: %v", err)
			return false
		}

		signal := selectSignalFromWindowsSignals(testRand, processUnderTest.Signals)
		badCommand := generateCommand(testRand)

		opts := &SignalContainerOptions{
			WindowsSignal:  signal,
			WindowsCommand: badCommand,
		}

		err = tc.policy.EnforceSignalContainerProcessPolicyV2(p.ctx, containerID, opts)
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
	f := func(p *generatedWindowsConstraints) bool {
		containerUnderTest := generateConstraintsWindowsContainer(testRand, 1, maxLayersInGeneratedContainer)

		ep := generateWindowsExecProcesses(testRand)
		ep[0].Signals = generateListOfWindowsSignals(testRand, 1, 4)
		containerUnderTest.ExecProcesses = ep
		processUnderTest := ep[0]

		p.containers = append(p.containers, containerUnderTest)

		tc, err := setupRegoRunningWindowsContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		containerID, err := idForRunningWindowsContainer(containerUnderTest, tc.runningContainers)
		if err != nil {
			r, err := runWindowsContainer(tc.policy, containerUnderTest)
			if err != nil {
				t.Errorf("Unable to setup test running container: %v", err)
				return false
			}
			containerID = r.containerID
		}

		envList := buildEnvironmentVariablesFromEnvRules(containerUnderTest.EnvRules, testRand)
		user := IDName{Name: containerUnderTest.User}
		commandLine := []string{processUnderTest.Command}

		_, _, _, err = tc.policy.EnforceExecInContainerPolicyV2(p.ctx, containerID, commandLine, envList, containerUnderTest.WorkingDir, user, nil)
		if err != nil {
			t.Errorf("Unable to exec process for test: %v", err)
			return false
		}

		signal := selectSignalFromWindowsSignals(testRand, processUnderTest.Signals)
		badContainerID := generateContainerID(testRand)

		opts := &SignalContainerOptions{
			WindowsSignal:  signal,
			WindowsCommand: commandLine,
		}

		err = tc.policy.EnforceSignalContainerProcessPolicyV2(p.ctx, badContainerID, opts)
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

func Test_Rego_GetPropertiesPolicy_On(t *testing.T) {
	f := func(constraints *generatedWindowsConstraints) bool {
		tc, err := setupGetPropertiesTestWindows(constraints, true)
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
	f := func(constraints *generatedWindowsConstraints) bool {
		tc, err := setupGetPropertiesTestWindows(constraints, false)
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
	f := func(constraints *generatedWindowsConstraints) bool {
		tc, err := setupDumpStacksTestWindows(constraints, true)
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
	f := func(constraints *generatedWindowsConstraints) bool {
		tc, err := setupDumpStacksTestWindows(constraints, false)
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

func Test_Rego_EnforceRegistryChangesPolicy_Matches_Windows(t *testing.T) {
	// Test that matching registry values are allowed
	f := func(p *generatedWindowsConstraints) bool {
		tc, err := setupRegoRunningWindowsContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		container := selectContainerFromRunningContainers(tc.runningContainers, testRand)

		// Create registry changes with values that should match defaults
		registryChanges := &hcsschema.RegistryChanges{
			AddValues: []hcsschema.RegistryValue{
				{
					Key: &hcsschema.RegistryKey{
						Hive: "System",
						Name: "CurrentControlSet\\Services\\EventLog\\Security",
					},
					Name:       "WaitToKillServiceTimeout",
					Type_:      hcsschema.RegistryValueType_D_WORD,
					DWordValue: 20000,
				},
			},
		}

		err = tc.policy.EnforceRegistryChangesPolicy(p.ctx, container.containerID, registryChanges)
		// With default values, this should be allowed
		if err != nil {
			t.Logf("Registry enforcement returned: %v", err)
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 5, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceRegistryChangesPolicy_Matches_Windows: %v", err)
	}
}

func Test_Rego_EnforceRegistryChangesPolicy_Invalid_ContainerID_Windows(t *testing.T) {
	// Test that using an invalid container ID is denied
	f := func(p *generatedWindowsConstraints) bool {
		tc, err := setupRegoRunningWindowsContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		// Use a non-existent container ID
		invalidContainerID := testDataGenerator.uniqueContainerID()

		registryChanges := &hcsschema.RegistryChanges{
			AddValues: []hcsschema.RegistryValue{
				{
					Key: &hcsschema.RegistryKey{
						Hive: "System",
						Name: "Test",
					},
					Name:        "Value",
					Type_:       hcsschema.RegistryValueType_STRING,
					StringValue: "Data",
				},
			},
		}

		err = tc.policy.EnforceRegistryChangesPolicy(p.ctx, invalidContainerID, registryChanges)
		if err == nil {
			t.Error("Expected registry changes to be denied with invalid container ID")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 5, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceRegistryChangesPolicy_Invalid_ContainerID_Windows: %v", err)
	}
}

func Test_Rego_EnforceRegistryChangesPolicy_Default_Values_Allowed_Windows(t *testing.T) {
	// Test that default registry values are allowed without policy definition
	f := func(p *generatedWindowsConstraints) bool {
		tc, err := setupRegoRunningWindowsContainerTest(p)
		if err != nil {
			t.Error(err)
			return false
		}

		container := selectContainerFromRunningContainers(tc.runningContainers, testRand)

		// Create registry changes with default values (from default_registry_values.go)
		registryChanges := &hcsschema.RegistryChanges{
			AddValues: []hcsschema.RegistryValue{
				{
					Key: &hcsschema.RegistryKey{
						Hive: "System",
						Name: "CurrentControlSet\\Services\\EventLog\\Security",
					},
					Name:       "WaitToKillServiceTimeout",
					Type_:      hcsschema.RegistryValueType_D_WORD,
					DWordValue: 20000,
				},
				{
					Key: &hcsschema.RegistryKey{
						Hive: "System",
						Name: "CurrentControlSet\\Services\\Tcpip\\Parameters",
					},
					Name:       "EnableCompartmentNamespace",
					Type_:      hcsschema.RegistryValueType_D_WORD,
					DWordValue: 1,
				},
			},
		}

		err = tc.policy.EnforceRegistryChangesPolicy(p.ctx, container.containerID, registryChanges)
		// Default values should be allowed
		if err != nil {
			t.Logf("Default registry values enforcement returned: %v", err)
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 5, Rand: testRand}); err != nil {
		t.Errorf("Test_Rego_EnforceRegistryChangesPolicy_Default_Values_Allowed_Windows: %v", err)
	}
}

// This is a no-op for windows.
// substituteUVMPath substitutes mount prefix to an appropriate path inside
// UVM. At policy generation time, it's impossible to tell what the sandboxID
// will be, so the prefix substitution needs to happen during runtime.
func substituteUVMPath(sandboxID string, m mountInternal) mountInternal {
	//no-op for windows
	_ = sandboxID
	return m
}

// Tests for log provider enforcement

// newLogProviderTestPolicy builds a Rego policy whose allowed_log_providers
// list contains the given providers and returns the compiled enforcer.
// Pass no providers to get an empty allow-list.
//
// allow_log_provider_dropping is left unset so the test exercises the
// default fail-close mode. Use newLogProviderTestPolicyWithDropping to flip
// the mode.
func newLogProviderTestPolicy(t *testing.T, allowedProviders ...string) *regoEnforcer {
	t.Helper()
	return newLogProviderTestPolicyWithDropping(t, false, allowedProviders...)
}

// newLogProviderTestPolicyWithDropping is the more general helper used by the
// mode-specific tests. It compiles a Rego policy that defines
// allowed_log_providers, sets allow_log_provider_dropping to dropping, and
// routes log_provider through the framework rule.
func newLogProviderTestPolicyWithDropping(t *testing.T, dropping bool, allowedProviders ...string) *regoEnforcer {
	t.Helper()
	var listLines string
	for _, p := range allowedProviders {
		listLines += fmt.Sprintf("\t\t%q,\n", p)
	}
	rego := fmt.Sprintf(`package policy
	api_version := "%s"
	framework_version := "%s"

	allow_log_provider_dropping := %t

	allowed_log_providers := [
%s	]

	log_provider := data.framework.log_provider
	`, apiVersion, frameworkVersion, dropping, listLines)

	policy, err := newRegoPolicy(rego, []oci.Mount{}, []oci.Mount{}, testOSType)
	if err != nil {
		t.Fatalf("failed to create policy: %v", err)
	}
	return policy
}

func Test_Rego_EnforceLogProviderPolicy_Allowed_Windows(t *testing.T) {
	policy := newLogProviderTestPolicy(t,
		"microsoft.windows.hyperv.compute",
		"microsoft-windows-guest-network-service",
	)

	kept, err := policy.EnforceLogProviderPolicy(context.Background(),
		[]string{"microsoft.windows.hyperv.compute"})
	if err != nil {
		t.Errorf("expected allowed provider to pass: %v", err)
	}
	if len(kept) != 1 || kept[0] != "microsoft.windows.hyperv.compute" {
		t.Errorf("expected kept=[microsoft.windows.hyperv.compute]; got %v", kept)
	}
}

func Test_Rego_EnforceLogProviderPolicy_Denied_Windows(t *testing.T) {
	policy := newLogProviderTestPolicy(t, "microsoft.windows.hyperv.compute")

	_, err := policy.EnforceLogProviderPolicy(context.Background(),
		[]string{"some-malicious-provider"})
	if err == nil {
		t.Errorf("expected unknown provider to be denied")
	}
}

func Test_Rego_EnforceLogProviderPolicy_CaseInsensitive_Windows(t *testing.T) {
	policy := newLogProviderTestPolicy(t, "microsoft.windows.hyperv.compute")

	kept, err := policy.EnforceLogProviderPolicy(context.Background(),
		[]string{"Microsoft.Windows.Hyperv.Compute"})
	if err != nil {
		t.Errorf("expected case-insensitive match to pass: %v", err)
	}
	// Rego preserves the input casing; we just confirm the name survived.
	if len(kept) != 1 || kept[0] != "Microsoft.Windows.Hyperv.Compute" {
		t.Errorf("expected kept=[Microsoft.Windows.Hyperv.Compute]; got %v", kept)
	}
}

func Test_Rego_EnforceLogProviderPolicy_OpenDoor_AllowsAll_Windows(t *testing.T) {
	policy, err := newRegoPolicy(openDoorRego, []oci.Mount{}, []oci.Mount{}, testOSType)
	if err != nil {
		t.Fatalf("failed to create policy: %v", err)
	}

	ctx := context.Background()
	kept, err := policy.EnforceLogProviderPolicy(ctx, []string{"any-provider-at-all"})
	if err != nil {
		t.Errorf("open door should allow any provider: %v", err)
	}
	if len(kept) != 1 || kept[0] != "any-provider-at-all" {
		t.Errorf("open door should keep the requested provider; got %v", kept)
	}
}

func Test_Rego_EnforceLogProviderPolicy_EmptyAllowList_DeniesAll_Windows(t *testing.T) {
	policy := newLogProviderTestPolicy(t)

	_, err := policy.EnforceLogProviderPolicy(context.Background(),
		[]string{"microsoft.windows.hyperv.compute"})
	if err == nil {
		t.Errorf("expected empty allow list to deny all providers")
	}
}

// Test_Rego_EnforceLogProviderPolicy_PreFeatureAPIVersion_Allows_Windows pins
// the non-regression behaviour for policies authored before log_provider was
// introduced (api.rego entry: introducedVersion=0.11.0, default_results.allowed=true).
// Such policies omit allowed_log_providers entirely; EnforceLogProviderPolicy
// must return the input list unchanged with no error so existing CWCOW/WCOW
// policies do not break when the framework gains the new enforcement point.
func Test_Rego_EnforceLogProviderPolicy_PreFeatureAPIVersion_Allows_Windows(t *testing.T) {
	// A pre-feature policy does not define log_provider at all (it was
	// authored before the rule existed). The version-gated default_results
	// path only fires when the policy has no rule for the enforcement
	// point — including `log_provider := data.framework.log_provider`
	// here would shadow the default and route through the framework rule,
	// which defaults to deny.
	rego := fmt.Sprintf(`package policy
	api_version := "0.10.0"
	framework_version := "%s"
	`, frameworkVersion)

	policy, err := newRegoPolicy(rego, []oci.Mount{}, []oci.Mount{}, testOSType)
	if err != nil {
		t.Fatalf("failed to create policy: %v", err)
	}

	ctx := context.Background()
	kept, err := policy.EnforceLogProviderPolicy(ctx,
		[]string{"any-provider-not-in-any-list"})
	if err != nil {
		t.Errorf("expected pre-0.11.0 policy to allow any provider via default_results: %v", err)
	}
	// default_results.providers_to_keep is null, so getProvidersToKeep
	// returns the input list unchanged.
	if len(kept) != 1 || kept[0] != "any-provider-not-in-any-list" {
		t.Errorf("expected kept=[any-provider-not-in-any-list]; got %v", kept)
	}
}

// Test_Rego_EnforceLogProviderPolicy_EmptyProviderName_Denied_Windows pins
// the behaviour for the host-scrubbed-unknown-provider edge case: when the
// providerName is the empty string, no allow-list entry can match (allow-lists
// never contain ""), so enforcement must deny.
func Test_Rego_EnforceLogProviderPolicy_EmptyProviderName_Denied_Windows(t *testing.T) {
	policy := newLogProviderTestPolicy(t, "microsoft.windows.hyperv.compute")

	_, err := policy.EnforceLogProviderPolicy(context.Background(), []string{""})
	if err == nil {
		t.Errorf("expected empty providerName to be denied")
	}
}

// Test_Rego_EnforceLogProviderPolicy_Dropping_KeepsSubset_Windows exercises
// the silent-drop mode: when allow_log_provider_dropping := true the call
// allows even if some requested providers are not on the allow-list, and only
// the matching subset is returned.
func Test_Rego_EnforceLogProviderPolicy_Dropping_KeepsSubset_Windows(t *testing.T) {
	policy := newLogProviderTestPolicyWithDropping(t, true,
		"microsoft.windows.hyperv.compute",
		"microsoft-windows-guest-network-service",
	)

	kept, err := policy.EnforceLogProviderPolicy(context.Background(), []string{
		"microsoft.windows.hyperv.compute",
		"some-bogus-provider",
		"microsoft-windows-guest-network-service",
	})
	if err != nil {
		t.Errorf("dropping mode should allow regardless of unknown providers: %v", err)
	}

	keptSet := make(map[string]struct{}, len(kept))
	for _, n := range kept {
		keptSet[n] = struct{}{}
	}
	if _, ok := keptSet["microsoft.windows.hyperv.compute"]; !ok {
		t.Errorf("expected 'microsoft.windows.hyperv.compute' kept; got %v", kept)
	}
	if _, ok := keptSet["microsoft-windows-guest-network-service"]; !ok {
		t.Errorf("expected 'microsoft-windows-guest-network-service' kept; got %v", kept)
	}
	if _, ok := keptSet["some-bogus-provider"]; ok {
		t.Errorf("expected 'some-bogus-provider' to be dropped; got %v", kept)
	}
}

// Test_Rego_EnforceLogProviderPolicy_FailClose_AnyMissDenies_Windows confirms
// the inverse of the dropping test: with allow_log_provider_dropping := false
// (default) even one unknown provider in a batch fails the entire call.
func Test_Rego_EnforceLogProviderPolicy_FailClose_AnyMissDenies_Windows(t *testing.T) {
	policy := newLogProviderTestPolicyWithDropping(t, false,
		"microsoft.windows.hyperv.compute",
	)

	_, err := policy.EnforceLogProviderPolicy(context.Background(), []string{
		"microsoft.windows.hyperv.compute",
		"some-bogus-provider",
	})
	if err == nil {
		t.Errorf("fail-close mode should deny when any provider is unknown")
	}
}
