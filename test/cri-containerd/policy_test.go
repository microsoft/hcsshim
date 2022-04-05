//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Microsoft/hcsshim/internal/tools/securitypolicy/helpers"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
	"k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

var (
	validPolicyAlpineCommand = []string{"ash", "-c", "echo 'Hello'"}
)

type configOpt func(*securitypolicy.ContainerConfig) error

func withExpectedMounts(em []string) configOpt {
	return func(conf *securitypolicy.ContainerConfig) error {
		conf.ExpectedMounts = append(conf.ExpectedMounts, em...)
		return nil
	}
}

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
		[]string{},
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

func syncContainerConfigs(writePath, waitPath string) (writer, waiter *securitypolicy.ContainerConfig) {
	writerCmdArgs := []string{"ash", "-c", fmt.Sprintf("touch %s && while true; do echo hello1; sleep 1; done", writePath)}
	writer = &securitypolicy.ContainerConfig{
		ImageName: "alpine:latest",
		Command:   writerCmdArgs,
	}
	// create container #2 that waits for a path to appear
	echoCmdArgs := []string{"ash", "-c", "while true; do echo hello2; sleep 1; done"}
	waiter = &securitypolicy.ContainerConfig{
		ImageName:      "alpine:latest",
		Command:        echoCmdArgs,
		ExpectedMounts: []string{waitPath},
	}
	return writer, waiter
}

func syncContainerRequests(
	writer, waiter *securitypolicy.ContainerConfig,
	podID string,
	podConfig *v1alpha2.PodSandboxConfig,
) (writerReq, waiterReq *v1alpha2.CreateContainerRequest) {
	writerReq = getCreateContainerRequest(
		podID,
		"alpine-writer",
		"alpine:latest",
		writer.Command,
		podConfig,
	)
	writerReq.Config.Mounts = append(writerReq.Config.Mounts, &v1alpha2.Mount{
		HostPath:      "sandbox://host/path",
		ContainerPath: "/mnt/shared/container-A",
	})

	waiterReq = getCreateContainerRequest(
		podID,
		"alpine-waiter",
		"alpine:latest",
		waiter.Command,
		podConfig,
	)
	waiterReq.Config.Mounts = append(waiterReq.Config.Mounts, &v1alpha2.Mount{
		// The HostPath must be the same as for the "writer" container
		HostPath:      "sandbox://host/path",
		ContainerPath: "/mnt/shared/container-B",
	})

	return writerReq, waiterReq
}

func Test_RunContainers_WithSyncHooks_ValidWaitPath(t *testing.T) {
	requireFeatures(t, featureLCOW, featureLCOWIntegrity)

	writerCfg, waiterCfg := syncContainerConfigs(
		"/mnt/shared/container-A/sync-file", "/mnt/shared/container-B/sync-file")

	containerConfigs := append(helpers.DefaultContainerConfigs(), *writerCfg, *waiterCfg)
	policyString, err := securityPolicyFromContainers(containerConfigs)
	if err != nil {
		t.Fatalf("failed to generate security policy string: %s", err)
	}

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// create pod with security policy
	podRequest := sandboxRequestWithPolicy(t, policyString)
	podID := runPodSandbox(t, client, ctx, podRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	writerReq, waiterReq := syncContainerRequests(writerCfg, waiterCfg, podID, podRequest.Config)

	cidWriter := createContainer(t, client, ctx, writerReq)
	cidWaiter := createContainer(t, client, ctx, waiterReq)

	startContainer(t, client, ctx, cidWriter)
	defer removeContainer(t, client, ctx, cidWriter)
	defer stopContainer(t, client, ctx, cidWriter)

	startContainer(t, client, ctx, cidWaiter)
	defer removeContainer(t, client, ctx, cidWaiter)
	defer stopContainer(t, client, ctx, cidWaiter)
}

func Test_RunContainers_WithSyncHooks_InvalidWaitPath(t *testing.T) {
	requireFeatures(t, featureLCOW, featureLCOWIntegrity)

	writerCfg, waiterCfg := syncContainerConfigs(
		"/mnt/shared/container-A/sync-file",
		"/mnt/shared/container-B/sync-file-invalid", // NOTE: this is an invalid wait path
	)

	containerConfigs := append(helpers.DefaultContainerConfigs(), *writerCfg, *waiterCfg)
	policyString, err := securityPolicyFromContainers(containerConfigs)
	if err != nil {
		t.Fatalf("failed to generate security policy string: %s", policyString)
	}

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// create pod with security policy
	podRequest := sandboxRequestWithPolicy(t, policyString)
	podID := runPodSandbox(t, client, ctx, podRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	writerReq, waiterReq := syncContainerRequests(writerCfg, waiterCfg, podID, podRequest.Config)
	cidWriter := createContainer(t, client, ctx, writerReq)
	cidWaiter := createContainer(t, client, ctx, waiterReq)

	startContainer(t, client, ctx, cidWriter)
	defer removeContainer(t, client, ctx, cidWriter)
	defer stopContainer(t, client, ctx, cidWriter)

	_, err = client.StartContainer(ctx, &v1alpha2.StartContainerRequest{
		ContainerId: cidWaiter,
	})
	expectedErrString := "timeout while waiting for path"
	if err == nil {
		defer removeContainer(t, client, ctx, cidWaiter)
		defer stopContainer(t, client, ctx, cidWaiter)
		t.Fatalf("should fail, succeeded instead")
	} else {
		if !strings.Contains(err.Error(), expectedErrString) {
			t.Fatalf("expected error: %q, got: %q", expectedErrString, err)
		}
	}
}
