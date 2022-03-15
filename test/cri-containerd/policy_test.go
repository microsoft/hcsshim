//go:build functional
// +build functional

package cri_containerd

import (
	"context"
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

func Test_RunContainers_WithSyncHooks_Positive(t *testing.T) {
	requireFeatures(t, featureLCOW, featureLCOWIntegrity)

	type config struct {
		name              string
		waiterSideEffect  func(containerConfig *securitypolicy.ContainerConfig)
		shouldError       bool
		expectedErrString string
	}

	for _, testConfig := range []config{
		{
			name:             "ValidWaitPath",
			waiterSideEffect: nil,
			shouldError:      false,
		},
		{
			// This is a long test that will wait for a timeout
			name: "InvalidWaitPath",
			waiterSideEffect: func(cfg *securitypolicy.ContainerConfig) {
				cfg.ExpectedMounts = []string{"/mnt/shared/container-B/wrong-sync-file"}
			},
			shouldError:       true,
			expectedErrString: "timeout while waiting for path",
		},
	} {
		t.Run(testConfig.name, func(t *testing.T) {
			// create container #1 that writes a file
			touchCmdArgs := []string{"ash", "-c", "touch /mnt/shared/container-A/sync-file && while true; do echo hello; sleep 1; done"}
			configWriter := securitypolicy.ContainerConfig{
				ImageName: "alpine:latest",
				Command:   touchCmdArgs,
			}
			// create container #2 that waits for a path to appear
			echoCmdArgs := []string{"ash", "-c", "while true; do echo hello2; sleep 1; done"}
			configWaiter := securitypolicy.ContainerConfig{
				ImageName:      "alpine:latest",
				Command:        echoCmdArgs,
				ExpectedMounts: []string{"/mnt/shared/container-B/sync-file"},
			}
			if testConfig.waiterSideEffect != nil {
				testConfig.waiterSideEffect(&configWaiter)
			}

			// create appropriate policies for the two containers
			containerConfigs := append(helpers.DefaultContainerConfigs(), configWriter, configWaiter)
			policyContainers, err := helpers.PolicyContainersFromConfigs(containerConfigs)
			if err != nil {
				t.Fatalf("failed to create security policy containers: %s", err)
			}
			policy := securitypolicy.NewSecurityPolicy(false, policyContainers)
			policyString, err := policy.EncodeToString()
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

			sbMountWriter := v1alpha2.Mount{
				HostPath:      "sandbox://host/path",
				ContainerPath: "/mnt/shared/container-A",
				Readonly:      false,
			}
			// start containers async and make sure that both of the start
			requestWriter := getCreateContainerRequest(
				podID,
				"alpine-writer",
				"alpine:latest",
				touchCmdArgs,
				podRequest.Config,
			)
			requestWriter.Config.Mounts = append(requestWriter.Config.Mounts, &sbMountWriter)

			sbMountWaiter := v1alpha2.Mount{
				HostPath:      "sandbox://host/path",
				ContainerPath: "/mnt/shared/container-B",
				Readonly:      false,
			}
			requestWaiter := getCreateContainerRequest(
				podID,
				"alpine-waiter",
				"alpine:latest",
				echoCmdArgs,
				podRequest.Config,
			)
			requestWaiter.Config.Mounts = append(requestWaiter.Config.Mounts, &sbMountWaiter)

			cidWriter := createContainer(t, client, ctx, requestWriter)
			cidWaiter := createContainer(t, client, ctx, requestWaiter)

			startContainer(t, client, ctx, cidWriter)
			defer removeContainer(t, client, ctx, cidWriter)
			defer stopContainer(t, client, ctx, cidWriter)

			if !testConfig.shouldError {
				startContainer(t, client, ctx, cidWaiter)
				defer removeContainer(t, client, ctx, cidWaiter)
				defer stopContainer(t, client, ctx, cidWaiter)
			} else {
				_, err := client.StartContainer(ctx, &v1alpha2.StartContainerRequest{
					ContainerId: cidWaiter,
				})
				if err == nil {
					defer removeContainer(t, client, ctx, cidWaiter)
					defer stopContainer(t, client, ctx, cidWaiter)
					t.Fatalf("should fail, succeeded instead")
				} else {
					if !strings.Contains(err.Error(), testConfig.expectedErrString) {
						t.Fatalf("expected error: %q, got: %q", testConfig.expectedErrString, err)
					}
				}
			}
		})
	}
}
