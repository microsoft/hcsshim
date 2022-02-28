//go:build functional
// +build functional

package cri_containerd

import (
	"context"
	"testing"

	"github.com/Microsoft/hcsshim/internal/tools/securitypolicy/helpers"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
	"k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

var (
	validPolicyAlpineCommand = []string{"ash", "-c", "echo 'Hello'"}
)

func securityPolicyFromContainers(containers []securitypolicy.ContainerConfig) (string, error) {
	pc, err := helpers.PolicyContainersFromConfigs(containers)
	if err != nil {
		return "", err
	}
	p := securitypolicy.NewSecurityPolicy(false, pc)
	pString, err := p.EncodeToString()
	if err != nil {
		return "", err
	}
	return pString, nil
}

func sandboxSecurityPolicy(t *testing.T) string {
	defaultContainers := helpers.DefaultContainerConfigs()
	policyString, err := securityPolicyFromContainers(defaultContainers)
	if err != nil {
		t.Fatalf("failed to create security policy string: %s", err)
	}
	return policyString
}

func alpineSecurityPolicy(t *testing.T) string {
	defaultContainers := helpers.DefaultContainerConfigs()
	alpineContainer := securitypolicy.NewContainerConfig(
		"alpine:latest",
		validPolicyAlpineCommand,
		[]securitypolicy.EnvRuleConfig{},
		securitypolicy.AuthConfig{},
		"",
	)

	containers := append(defaultContainers, alpineContainer)
	policyString, err := securityPolicyFromContainers(containers)
	if err != nil {
		t.Fatalf("failed to create security policy string: %s", err)
	}
	return policyString
}

func sandboxRequestWithPolicy(t *testing.T, policy string) *v1alpha2.RunPodSandboxRequest {
	return getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.NoSecurityHardware:  "true",
			annotations.SecurityPolicy:      policy,
			annotations.VPMemNoMultiMapping: "true",
		}),
	)
}

func Test_RunPodSandbox_WithPolicy_Allowed(t *testing.T) {
	requireFeatures(t, featureLCOW)
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	sandboxPolicy := sandboxSecurityPolicy(t)

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sandboxRequest := sandboxRequestWithPolicy(t, sandboxPolicy)

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)
}

func Test_RunSimpleAlpineContainer_WithPolicy_Allowed(t *testing.T) {
	requireFeatures(t, featureLCOW)
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	alpinePolicy := alpineSecurityPolicy(t)

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sandboxRequest := sandboxRequestWithPolicy(t, alpinePolicy)

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	containerRequest := getCreateContainerRequest(
		podID,
		"alpine-with-policy",
		"alpine:latest",
		validPolicyAlpineCommand,
		sandboxRequest.Config,
	)

	containerID := createContainer(t, client, ctx, containerRequest)
	defer removeContainer(t, client, ctx, containerID)

	startContainer(t, client, ctx, containerID)
	stopContainer(t, client, ctx, containerID)
}
