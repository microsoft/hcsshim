//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"testing"

	securityPolicyTest "github.com/Microsoft/hcsshim/test/pkg/securitypolicy"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

var validPolicyAlpineCommand = []string{"ash", "-c", "echo 'Hello'"}

type configSideEffect func(*runtime.CreateContainerRequest) error

var defaultExternalProcesses = []securitypolicy.ExternalProcessConfig{
	{
		Command:          []string{"ls", "-l", "/dev/mapper"},
		WorkingDir:       "/",
		AllowStdioAccess: true,
	},
	{
		Command:          []string{"bash"},
		WorkingDir:       "/",
		AllowStdioAccess: true,
	},
}

func alpineSecurityPolicy(t *testing.T, policyType string, allowEnvironmentVariableDropping bool, allowCapabilityDropping bool, opts ...securitypolicy.ContainerConfigOpt) string {
	t.Helper()
	containerConfigOpts := append(
		[]securitypolicy.ContainerConfigOpt{
			securitypolicy.WithCommand(validPolicyAlpineCommand),
			securitypolicy.WithAllowPrivilegeEscalation(true),
		},
		opts...,
	)
	return securityPolicyTest.PolicyFromImageWithOpts(
		t,
		imageLcowAlpine,
		policyType,
		containerConfigOpts,
		[]securitypolicy.PolicyConfigOpt{
			securitypolicy.WithAllowEnvVarDropping(allowEnvironmentVariableDropping),
			securitypolicy.WithAllowCapabilityDropping(allowCapabilityDropping),
			securitypolicy.WithAllowUnencryptedScratch(!*flagSevSnp),
		},
	)
}

func sandboxRequestWithPolicy(t *testing.T, policy string) *runtime.RunPodSandboxRequest {
	t.Helper()
	return getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		WithSandboxAnnotations(
			map[string]string{
				annotations.NoSecurityHardware:  strconv.FormatBool(!*flagSevSnp),
				annotations.SecurityPolicy:      policy,
				annotations.VPMemNoMultiMapping: "true",
				annotations.VPMemCount:          "0",
				// This allows for better experience for policy only tests.
				annotations.EncryptedScratchDisk: strconv.FormatBool(*flagSevSnp),
			},
		),
	)
}

type policyConfig struct {
	enforcer string
	input    string
}

var policyTestMatrix = []policyConfig{
	{
		enforcer: "rego",
		input:    "rego",
	},
	{
		enforcer: "rego",
		input:    "json",
	},
	{
		enforcer: "standard",
		input:    "json",
	},
}

func Test_RunContainer_WithPolicy_And_InvalidConfigs(t *testing.T) {
	type config struct {
		name          string
		sf            configSideEffect
		expectedError string
	}

	requireFeatures(t, featureLCOW, featureLCOWIntegrity)
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, testConfig := range []config{
		{
			name: "InvalidWorkingDir",
			sf: func(req *runtime.CreateContainerRequest) error {
				req.Config.WorkingDir = "/non/existent"
				return nil
			},
			expectedError: "invalid working directory",
		},
		{
			name: "InvalidCommand",
			sf: func(req *runtime.CreateContainerRequest) error {
				req.Config.Command = []string{"ash", "-c", "echo 'invalid command'"}
				return nil
			},
			expectedError: "invalid command",
		},
		{
			name: "InvalidEnvironmentVariable",
			sf: func(req *runtime.CreateContainerRequest) error {
				req.Config.Envs = append(
					req.Config.Envs, &runtime.KeyValue{
						Key:   "KEY",
						Value: "VALUE",
					},
				)
				return nil
			},
			expectedError: "invalid env list",
		},
	} {
		t.Run(testConfig.name, func(t *testing.T) {
			alpinePolicy := alpineSecurityPolicy(t, "rego", false, false)
			sandboxRequest := sandboxRequestWithPolicy(t, alpinePolicy)

			podID := runPodSandbox(t, client, ctx, sandboxRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			containerRequest := getCreateContainerRequest(
				podID,
				"alpine-with-policy",
				imageLcowAlpine,
				validPolicyAlpineCommand,
				sandboxRequest.Config,
			)

			if err := testConfig.sf(containerRequest); err != nil {
				t.Fatalf("failed to apply containerRequest side effect: %s", err)
			}

			containerID := createContainer(t, client, ctx, containerRequest)
			_, err := client.StartContainer(
				ctx, &runtime.StartContainerRequest{
					ContainerId: containerID,
				},
			)
			if err == nil {
				t.Fatal("expected container start failure")
			}
			if !securityPolicyTest.AssertErrorContains(t, err, testConfig.expectedError) {
				t.Fatalf("expected %q in error message, got: %q", testConfig.expectedError, err)
			}
		})
	}
}

func userConfig(uid, gid int64) securitypolicy.UserConfig {
	return securitypolicy.UserConfig{
		UserIDName: securitypolicy.IDNameConfig{
			Strategy: securitypolicy.IDNameStrategyID,
			Rule:     strconv.FormatInt(uid, 10),
		},
		GroupIDNames: []securitypolicy.IDNameConfig{
			{
				Strategy: securitypolicy.IDNameStrategyID,
				Rule:     strconv.FormatInt(gid, 10),
			},
		},
		Umask: "0022",
	}
}

func capabilitiesConfig() *securitypolicy.CapabilitiesConfig {
	return &securitypolicy.CapabilitiesConfig{
		Bounding:    securitypolicy.DefaultUnprivilegedCapabilities(),
		Effective:   securitypolicy.DefaultUnprivilegedCapabilities(),
		Inheritable: securitypolicy.EmptyCapabiltiesSet(),
		Permitted:   securitypolicy.DefaultUnprivilegedCapabilities(),
		Ambient:     securitypolicy.EmptyCapabiltiesSet(),
	}
}

//go:embed seccomp_valid.json
var validSeccomp []byte

//go:embed seccomp_invalid.json
var invalidSeccomp []byte

//go:embed policy-v0.1.0.rego
var oldPolicy []byte

// todo: PORT TO AZCRI?
func Test_GetProperties_WithPolicy(t *testing.T) {
	requireFeatures(t, featureLCOWIntegrity)
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, allowGetProperties := range []bool{true, false} {
		t.Run(fmt.Sprintf("Allowed_%t", allowGetProperties), func(t *testing.T) {
			policy := securityPolicyTest.PolicyFromImageWithOpts(
				t,
				imageLcowAlpine,
				"rego",
				[]securitypolicy.ContainerConfigOpt{
					securitypolicy.WithCommand(validPolicyAlpineCommand),
					securitypolicy.WithAllowPrivilegeEscalation(true),
				},
				[]securitypolicy.PolicyConfigOpt{
					securitypolicy.WithAllowUnencryptedScratch(true),
					securitypolicy.WithAllowPropertiesAccess(allowGetProperties),
				},
			)
			sandboxRequest := sandboxRequestWithPolicy(t, policy)
			sandboxRequest.Config.Annotations[annotations.EncryptedScratchDisk] = "false"

			podID := runPodSandbox(t, client, ctx, sandboxRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			containerRequest := getCreateContainerRequest(
				podID,
				"alpine-get-properties",
				imageLcowAlpine,
				validPolicyAlpineCommand,
				sandboxRequest.Config,
			)
			containerID := createContainer(t, client, ctx, containerRequest)
			defer removeContainer(t, client, ctx, containerID)
			startContainer(t, client, ctx, containerID)
			defer stopContainer(t, client, ctx, containerID)

			_, err := client.ContainerStats(ctx, &runtime.ContainerStatsRequest{ContainerId: containerID})
			if err != nil {
				if allowGetProperties {
					t.Fatalf("container stats should be allowed: %s", err)
				}
				// unfortunately the errors returned during stats collection
				// are not bubbled up, so we can only rely on the fact that
				// the metrics response is invalid.
				if !strings.Contains(err.Error(), " unexpected metrics response: []") {
					t.Fatalf("expected different error: %s", err)
				}
			} else {
				if !allowGetProperties {
					t.Fatal("container stats should not be allowed")
				}
			}
		})
	}
}
