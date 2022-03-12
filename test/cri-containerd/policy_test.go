//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"fmt"
	"strings"
	"testing"

	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	"github.com/Microsoft/hcsshim/internal/tools/securitypolicy/helpers"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
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

func withEnvVarRules(envRules []securitypolicy.EnvRuleConfig) configOpt {
	return func(config *securitypolicy.ContainerConfig) error {
		config.EnvRules = append(config.EnvRules, envRules...)
		return nil
	}
}

func withWorkingDir(workingDir string) configOpt {
	return func(config *securitypolicy.ContainerConfig) error {
		config.WorkingDir = workingDir
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

func alpineSecurityPolicy(t *testing.T, opts ...configOpt) string {
	defaultContainers := helpers.DefaultContainerConfigs()
	alpineContainer := securitypolicy.NewContainerConfig(
		"alpine:latest",
		validPolicyAlpineCommand,
		[]securitypolicy.EnvRuleConfig{},
		securitypolicy.AuthConfig{},
		"",
		[]string{},
	)

	for _, o := range opts {
		if err := o(&alpineContainer); err != nil {
			t.Fatalf("failed to apply config opt: %s", err)
		}
	}

	containers := append(defaultContainers, alpineContainer)
	policyString, err := securityPolicyFromContainers(containers)
	if err != nil {
		t.Fatalf("failed to create security policy string: %s", err)
	}
	return policyString
}

func sandboxRequestWithPolicy(t *testing.T, policy string) *runtime.RunPodSandboxRequest {
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
	podConfig *runtime.PodSandboxConfig,
) (writerReq, waiterReq *runtime.CreateContainerRequest) {
	writerReq = getCreateContainerRequest(
		podID,
		"alpine-writer",
		"alpine:latest",
		writer.Command,
		podConfig,
	)
	writerReq.Config.Mounts = append(writerReq.Config.Mounts, &runtime.Mount{
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
	waiterReq.Config.Mounts = append(waiterReq.Config.Mounts, &runtime.Mount{
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

	_, err = client.StartContainer(ctx, &runtime.StartContainerRequest{
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

func Test_RunContainer_ValidContainerConfigs_Allowed(t *testing.T) {
	type sideEffect func(*runtime.CreateContainerRequest)
	type config struct {
		name string
		sf   sideEffect
		opts []configOpt
	}

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	requireFeatures(t, featureLCOW)
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	for _, testConfig := range []config{
		{
			name: "WorkingDir",
			sf: func(req *runtime.CreateContainerRequest) {
				req.Config.WorkingDir = "/root"
			},
			opts: []configOpt{withWorkingDir("/root")},
		},
		{
			name: "EnvironmentVariable",
			sf: func(req *runtime.CreateContainerRequest) {
				req.Config.Envs = append(req.Config.Envs, &runtime.KeyValue{
					Key:   "KEY",
					Value: "VALUE",
				})
			},
			opts: []configOpt{
				withEnvVarRules(
					[]securitypolicy.EnvRuleConfig{
						{
							Strategy: securitypolicy.EnvVarRuleString,
							Rule:     "KEY=VALUE",
						},
					}),
			},
		},
	} {
		t.Run(testConfig.name, func(t *testing.T) {
			alpinePolicy := alpineSecurityPolicy(t, testConfig.opts...)
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
			testConfig.sf(containerRequest)

			containerID := createContainer(t, client, ctx, containerRequest)
			startContainer(t, client, ctx, containerID)
			defer removeContainer(t, client, ctx, containerID)
			defer stopContainer(t, client, ctx, containerID)
		})
	}
}

func Test_RunContainer_InvalidContainerConfigs_NotAllowed(t *testing.T) {
	type sideEffect func(*runtime.CreateContainerRequest)
	type config struct {
		name          string
		sf            sideEffect
		expectedError string
	}

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	requireFeatures(t, featureLCOW)
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	alpinePolicy := alpineSecurityPolicy(t)
	for _, testConfig := range []config{
		{
			name: "InvalidWorkingDir",
			sf: func(req *runtime.CreateContainerRequest) {
				req.Config.WorkingDir = "/non/existent"
			},
			expectedError: "working_dir \"/non/existent\" unmatched by policy rule",
		},
		{
			name: "InvalidCommand",
			sf: func(req *runtime.CreateContainerRequest) {
				req.Config.Command = []string{"ash", "-c", "echo 'invalid command'"}
			},
			expectedError: "command [ash -c echo 'invalid command'] doesn't match policy",
		},
		{
			name: "InvalidEnvironmentVariable",
			sf: func(req *runtime.CreateContainerRequest) {
				req.Config.Envs = append(req.Config.Envs, &runtime.KeyValue{
					Key:   "KEY",
					Value: "VALUE",
				})
			},
			expectedError: "env variable KEY=VALUE unmatched by policy rule",
		},
	} {
		t.Run(testConfig.name, func(t *testing.T) {
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
			testConfig.sf(containerRequest)

			containerID := createContainer(t, client, ctx, containerRequest)
			_, err := client.StartContainer(ctx, &runtime.StartContainerRequest{
				ContainerId: containerID,
			})
			if err == nil {
				t.Fatal("expected container start failure")
			}
			if !strings.Contains(err.Error(), testConfig.expectedError) {
				t.Fatalf("expected %q in error message, got: %q", testConfig.expectedError, err)
			}
		})
	}
}
