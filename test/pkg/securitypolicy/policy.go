package securitypolicy

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/Microsoft/hcsshim/internal/tools/securitypolicy/helpers"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

func PolicyFromImageWithOpts(
	tb testing.TB,
	imageName string,
	policyType string,
	cOpts []securitypolicy.ContainerConfigOpt,
	pOpts []securitypolicy.PolicyConfigOpt,
) string {
	tb.Helper()
	containerConfig := securitypolicy.ContainerConfig{
		ImageName: imageName,
	}
	for _, co := range cOpts {
		if err := co(&containerConfig); err != nil {
			tb.Fatal(err)
		}
	}

	policyOpts := []securitypolicy.PolicyConfigOpt{
		securitypolicy.WithContainers([]securitypolicy.ContainerConfig{
			containerConfig,
		}),
	}
	policyOpts = append(policyOpts, pOpts...)

	return PolicyWithOpts(tb, policyType, policyOpts...)
}

func PolicyWithOpts(tb testing.TB, policyType string, pOpts ...securitypolicy.PolicyConfigOpt) string {
	tb.Helper()
	policyOpts := []securitypolicy.PolicyConfigOpt{
		securitypolicy.WithContainers(helpers.DefaultContainerConfigs()),
	}
	policyOpts = append(policyOpts, pOpts...)

	config, err := securitypolicy.NewPolicyConfig(policyOpts...)
	if err != nil {
		tb.Fatal(err)
	}

	pc, err := helpers.PolicyContainersFromConfigs(config.Containers)
	if err != nil {
		tb.Fatal(err)
	}
	policyString, err := securitypolicy.MarshalPolicy(
		policyType,
		config.AllowAll,
		pc,
		config.ExternalProcesses,
		config.Fragments,
		config.AllowPropertiesAccess,
		config.AllowDumpStacks,
		config.AllowRuntimeLogging,
		config.AllowHostNetwork,
		config.AllowEnvironmentVariableDropping,
		config.AllowUnencryptedScratch,
		config.AllowCapabilityDropping,
		config.AllowRegistryChangesDropping,
		config.AllowLogProviderDropping,
	)
	if err != nil {
		tb.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString([]byte(policyString))

}

func PolicyFromWindowsContainerWithOpts(
	tb testing.TB,
	policyType string,
	cOpts []securitypolicy.WindowsContainerConfigOpt,
	pOpts []securitypolicy.PolicyConfigOpt,
) string {
	tb.Helper()
	containerConfig := securitypolicy.WindowsContainerConfig{}
	for _, option := range cOpts {
		if err := option(&containerConfig); err != nil {
			tb.Fatal(err)
		}
	}

	policyOpts := []securitypolicy.PolicyConfigOpt{
		securitypolicy.WithWindowsContainers([]securitypolicy.WindowsContainerConfig{
			containerConfig,
		}),
	}
	policyOpts = append(policyOpts, pOpts...)

	return WindowsPolicyWithOpts(tb, policyType, policyOpts...)
}

func WindowsPolicyWithOpts(tb testing.TB, policyType string, pOpts ...securitypolicy.PolicyConfigOpt) string {
	tb.Helper()
	config, err := securitypolicy.NewPolicyConfig(pOpts...)
	if err != nil {
		tb.Fatal(err)
	}

	policyContainers, err := helpers.PolicyWindowsContainersFromConfigs(context.Background(), config.WindowsContainers)
	if err != nil {
		tb.Fatal(err)
	}
	policyString, err := securitypolicy.MarshalWindowsPolicy(
		policyType,
		config.AllowAll,
		policyContainers,
		config.ExternalProcesses,
		config.Fragments,
		config.AllowPropertiesAccess,
		config.AllowDumpStacks,
		config.AllowRuntimeLogging,
		config.AllowHostNetwork,
		config.AllowEnvironmentVariableDropping,
		config.AllowUnencryptedScratch,
		config.AllowCapabilityDropping,
		config.AllowRegistryChangesDropping,
		config.AllowLogProviderDropping,
	)
	if err != nil {
		tb.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString([]byte(policyString))
}

func AssertErrorContains(t *testing.T, err error, expected string) bool {
	t.Helper()
	if err == nil {
		t.Error("expected error but got nil")
		return false
	}

	if strings.Contains(err.Error(), expected) {
		return true
	}

	policyDecisionJSON, err := securitypolicy.ExtractPolicyDecision(err.Error())
	if err != nil {
		t.Error(err)
		return false
	}

	if !strings.Contains(policyDecisionJSON, expected) {
		t.Errorf("expected policy decision JSON to contain %q", expected)
		return false
	}

	return true
}
