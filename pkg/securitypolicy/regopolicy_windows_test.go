//go:build windows && rego
// +build windows,rego

package securitypolicy

import (
	"context"
	_ "embed"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"testing/quick"

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

// This is a no-op for windows.
// substituteUVMPath substitutes mount prefix to an appropriate path inside
// UVM. At policy generation time, it's impossible to tell what the sandboxID
// will be, so the prefix substitution needs to happen during runtime.
func substituteUVMPath(sandboxID string, m mountInternal) mountInternal {
	//no-op for windows
	_ = sandboxID
	return m
}
