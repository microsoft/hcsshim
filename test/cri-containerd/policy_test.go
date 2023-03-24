//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	_ "embed"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	"github.com/Microsoft/hcsshim/internal/tools/securitypolicy/helpers"
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

func policyFromOpts(t *testing.T, policyType string, opts ...securitypolicy.PolicyConfigOpt) string {
	t.Helper()
	defaultOpts := []securitypolicy.PolicyConfigOpt{
		securitypolicy.WithContainers(helpers.DefaultContainerConfigs()),
	}

	defaultOpts = append(defaultOpts, opts...)
	config, err := securitypolicy.NewPolicyConfig(defaultOpts...)
	if err != nil {
		t.Fatal(err)
	}

	pc, err := helpers.PolicyContainersFromConfigs(config.Containers)
	if err != nil {
		t.Fatal(err)
	}
	policyString, err := securitypolicy.MarshalPolicy(
		policyType,
		false,
		pc,
		config.ExternalProcesses,
		config.Fragments,
		config.AllowPropertiesAccess,
		config.AllowDumpStacks,
		config.AllowRuntimeLogging,
		config.AllowEnvironmentVariableDropping,
		config.AllowUnencryptedScratch,
		config.AllowCapabilityDropping,
	)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString([]byte(policyString))
}

func securityPolicyFromImageWithOpts(t *testing.T, imageName string, policyType string, allowEnvironmentVariableDropping bool, allowCapabilityDropping bool, opts ...securitypolicy.ContainerConfigOpt) string {
	t.Helper()
	alpineContainer := securitypolicy.ContainerConfig{
		ImageName:                imageName,
		Command:                  validPolicyAlpineCommand,
		AllowPrivilegeEscalation: true,
	}

	for _, o := range opts {
		if err := o(&alpineContainer); err != nil {
			t.Fatalf("failed to apply config opt: %s", err)
		}
	}

	return policyFromOpts(
		t,
		policyType,
		securitypolicy.WithContainers([]securitypolicy.ContainerConfig{alpineContainer}),
		securitypolicy.WithExternalProcesses(defaultExternalProcesses),
		securitypolicy.WithAllowUnencryptedScratch(true),
		securitypolicy.WithAllowEnvVarDropping(allowEnvironmentVariableDropping),
		securitypolicy.WithAllowCapabilityDropping(allowCapabilityDropping),
		securitypolicy.WithAllowRuntimeLogging(true),
		securitypolicy.WithAllowPropertiesAccess(true),
		securitypolicy.WithAllowDumpStacks(true),
	)
}

func alpineSecurityPolicy(t *testing.T, policyType string, allowEnvironmentVariableDropping bool, allowCapabilityDropping bool, opts ...securitypolicy.ContainerConfigOpt) string {
	t.Helper()
	return securityPolicyFromImageWithOpts(t, imageLcowAlpine, policyType, allowEnvironmentVariableDropping, allowCapabilityDropping, opts...)
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

func Test_RunPodSandbox_WithPolicy_Allowed(t *testing.T) {
	requireFeatures(t, featureLCOW, featureLCOWIntegrity)
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, pc := range policyTestMatrix {
		t.Run(t.Name()+fmt.Sprintf("_Enforcer_%s_Input_%s", pc.enforcer, pc.input), func(t *testing.T) {
			sandboxPolicy := policyFromOpts(
				t,
				pc.input,
				securitypolicy.WithAllowUnencryptedScratch(true),
				securitypolicy.WithAllowEnvVarDropping(true),
				securitypolicy.WithAllowCapabilityDropping(true),
			)
			sandboxRequest := sandboxRequestWithPolicy(t, sandboxPolicy)
			sandboxRequest.Config.Annotations[annotations.SecurityPolicyEnforcer] = pc.enforcer

			podID := runPodSandbox(t, client, ctx, sandboxRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)
		})
	}
}

func Test_RunSimpleAlpineContainer_WithPolicy_Allowed(t *testing.T) {
	requireFeatures(t, featureLCOW, featureLCOWIntegrity)
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, pc := range policyTestMatrix {
		t.Run(t.Name()+fmt.Sprintf("_Enforcer_%s_Input_%s", pc.enforcer, pc.input), func(t *testing.T) {
			alpinePolicy := alpineSecurityPolicy(t, pc.input, false, false)
			sandboxRequest := sandboxRequestWithPolicy(t, alpinePolicy)
			sandboxRequest.Config.Annotations[annotations.SecurityPolicyEnforcer] = pc.enforcer

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

			containerID := createContainer(t, client, ctx, containerRequest)
			defer removeContainer(t, client, ctx, containerID)

			startContainer(t, client, ctx, containerID)
			stopContainer(t, client, ctx, containerID)
		})
	}
}

func Test_RunContainer_WithPolicy_And_ValidConfigs(t *testing.T) {
	type sideEffect func(*runtime.CreateContainerRequest)
	type config struct {
		name string
		sf   sideEffect
		opts []securitypolicy.ContainerConfigOpt
	}

	requireFeatures(t, featureLCOW, featureLCOWIntegrity)
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, testConfig := range []config{
		{
			name: "WorkingDir",
			sf: func(req *runtime.CreateContainerRequest) {
				req.Config.WorkingDir = "/root"
			},
			opts: []securitypolicy.ContainerConfigOpt{securitypolicy.WithWorkingDir("/root")},
		},
		{
			name: "EnvironmentVariable",
			sf: func(req *runtime.CreateContainerRequest) {
				req.Config.Envs = append(
					req.Config.Envs, &runtime.KeyValue{
						Key:   "KEY",
						Value: "VALUE",
					},
				)
			},
			opts: []securitypolicy.ContainerConfigOpt{
				securitypolicy.WithEnvVarRules(
					[]securitypolicy.EnvRuleConfig{
						{
							Strategy: securitypolicy.EnvVarRuleString,
							Rule:     "KEY=VALUE",
						},
					},
				),
			},
		},
	} {
		for _, pc := range policyTestMatrix {
			t.Run(testConfig.name+fmt.Sprintf("_Enforcer_%s_Input_%s", pc.enforcer, pc.input), func(t *testing.T) {
				alpinePolicy := alpineSecurityPolicy(t, pc.input, false, false, testConfig.opts...)
				sandboxRequest := sandboxRequestWithPolicy(t, alpinePolicy)
				sandboxRequest.Config.Annotations[annotations.SecurityPolicyEnforcer] = pc.enforcer

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
				testConfig.sf(containerRequest)

				containerID := createContainer(t, client, ctx, containerRequest)
				startContainer(t, client, ctx, containerID)
				defer removeContainer(t, client, ctx, containerID)
				defer stopContainer(t, client, ctx, containerID)
			})
		}
	}
}

// todo (maksiman): add coverage for rego enforcer
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
			expectedError: "working_dir \"/non/existent\" unmatched by policy rule",
		},
		{
			name: "InvalidCommand",
			sf: func(req *runtime.CreateContainerRequest) error {
				req.Config.Command = []string{"ash", "-c", "echo 'invalid command'"}
				return nil
			},
			expectedError: "command [ash -c echo 'invalid command'] doesn't match policy",
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
			expectedError: "env variable KEY=VALUE unmatched by policy rule",
		},
	} {
		t.Run(testConfig.name, func(t *testing.T) {
			alpinePolicy := alpineSecurityPolicy(t, "json", false, false)
			sandboxRequest := sandboxRequestWithPolicy(t, alpinePolicy)
			sandboxRequest.Config.Annotations[annotations.SecurityPolicyEnforcer] = "standard"

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
			if !strings.Contains(err.Error(), testConfig.expectedError) {
				t.Fatalf("expected %q in error message, got: %q", testConfig.expectedError, err)
			}
		})
	}
}

func Test_RunContainer_WithPolicy_And_MountConstraints_Allowed(t *testing.T) {
	requireFeatures(t, featureLCOW, featureLCOWIntegrity)
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type config struct {
		name       string
		sideEffect configSideEffect
		opts       []securitypolicy.ContainerConfigOpt
	}

	for _, testConfig := range []config{
		{
			name: "DefaultMounts",
			sideEffect: func(_ *runtime.CreateContainerRequest) error {
				return nil
			},
			opts: []securitypolicy.ContainerConfigOpt{},
		},
		{
			name: "SandboxMountRW",
			sideEffect: func(req *runtime.CreateContainerRequest) error {
				req.Config.Mounts = append(
					req.Config.Mounts, &runtime.Mount{
						HostPath:      "sandbox:///sandbox/path",
						ContainerPath: "/container/path",
						Propagation:   runtime.MountPropagation_PROPAGATION_BIDIRECTIONAL,
					},
				)
				return nil
			},
			opts: []securitypolicy.ContainerConfigOpt{
				securitypolicy.WithMountConstraints(
					[]securitypolicy.MountConfig{
						{
							HostPath:      "sandbox:///sandbox/path",
							ContainerPath: "/container/path",
						},
					},
				)},
		},
		{
			name: "SandboxMountRO",
			sideEffect: func(req *runtime.CreateContainerRequest) error {
				req.Config.Mounts = append(
					req.Config.Mounts, &runtime.Mount{
						HostPath:      "sandbox:///sandbox/path",
						ContainerPath: "/container/path",
						Propagation:   runtime.MountPropagation_PROPAGATION_BIDIRECTIONAL,
						Readonly:      true,
					},
				)
				return nil
			},
			opts: []securitypolicy.ContainerConfigOpt{
				securitypolicy.WithMountConstraints(
					[]securitypolicy.MountConfig{
						{
							HostPath:      "sandbox:///sandbox/path",
							ContainerPath: "/container/path",
							Readonly:      true,
						},
					},
				)},
		},
		{
			name: "SandboxMountRegex",
			sideEffect: func(req *runtime.CreateContainerRequest) error {
				req.Config.Mounts = append(
					req.Config.Mounts, &runtime.Mount{
						HostPath:      "sandbox:///sandbox/path/regexp",
						ContainerPath: "/container/path",
						Propagation:   runtime.MountPropagation_PROPAGATION_BIDIRECTIONAL,
					},
				)
				return nil
			},
			opts: []securitypolicy.ContainerConfigOpt{
				securitypolicy.WithMountConstraints(
					[]securitypolicy.MountConfig{
						{
							HostPath:      "sandbox:///sandbox/path/r.+",
							ContainerPath: "/container/path",
						},
					},
				)},
		},
	} {
		for _, pc := range policyTestMatrix {
			t.Run(testConfig.name+fmt.Sprintf("_Enforcer_%s_Input_%s", pc.enforcer, pc.input), func(t *testing.T) {
				alpinePolicy := alpineSecurityPolicy(t, pc.input, false, false, testConfig.opts...)
				sandboxRequest := sandboxRequestWithPolicy(t, alpinePolicy)
				sandboxRequest.Config.Annotations[annotations.SecurityPolicyEnforcer] = pc.enforcer

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

				if err := testConfig.sideEffect(containerRequest); err != nil {
					t.Fatalf("failed to apply containerRequest side effect: %s", err)
				}

				containerID := createContainer(t, client, ctx, containerRequest)
				startContainer(t, client, ctx, containerID)
				defer removeContainer(t, client, ctx, containerID)
				defer stopContainer(t, client, ctx, containerID)
			})
		}
	}
}

// todo (maksiman): add coverage for rego enforcer
func Test_RunContainer_WithPolicy_And_MountConstraints_NotAllowed(t *testing.T) {
	requireFeatures(t, featureLCOW, featureLCOWIntegrity)
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type config struct {
		name          string
		sideEffect    configSideEffect
		opts          []securitypolicy.ContainerConfigOpt
		expectedError string
	}

	testSandboxMountOpts := []securitypolicy.ContainerConfigOpt{
		securitypolicy.WithMountConstraints(
			[]securitypolicy.MountConfig{
				{
					HostPath:      "sandbox:///sandbox/path",
					ContainerPath: "/container/path",
				},
			},
		),
	}
	for _, testConfig := range []config{
		{
			name: "InvalidSandboxMountSource",
			sideEffect: func(req *runtime.CreateContainerRequest) error {
				req.Config.Mounts = append(
					req.Config.Mounts, &runtime.Mount{
						HostPath:      "sandbox:///sandbox/invalid/path",
						ContainerPath: "/container/path",
						Propagation:   runtime.MountPropagation_PROPAGATION_BIDIRECTIONAL,
					},
				)
				return nil
			},
			opts:          testSandboxMountOpts,
			expectedError: "is not allowed by mount constraints",
		},
		{
			name: "InvalidSandboxMountDestination",
			sideEffect: func(req *runtime.CreateContainerRequest) error {
				req.Config.Mounts = append(
					req.Config.Mounts, &runtime.Mount{
						HostPath:      "sandbox:///sandbox/path",
						ContainerPath: "/container/path/invalid",
						Propagation:   runtime.MountPropagation_PROPAGATION_BIDIRECTIONAL,
					},
				)
				return nil
			},
			opts:          testSandboxMountOpts,
			expectedError: "is not allowed by mount constraints",
		},
		{
			name: "InvalidSandboxMountFlagRO",
			sideEffect: func(req *runtime.CreateContainerRequest) error {
				req.Config.Mounts = append(
					req.Config.Mounts, &runtime.Mount{
						HostPath:      "sandbox:///sandbox/path",
						ContainerPath: "/container/path",
						Propagation:   runtime.MountPropagation_PROPAGATION_BIDIRECTIONAL,
						Readonly:      true,
					},
				)
				return nil
			},
			opts:          testSandboxMountOpts,
			expectedError: "is not allowed by mount constraints",
		},
		{
			name: "InvalidSandboxMountFlagRW",
			sideEffect: func(req *runtime.CreateContainerRequest) error {
				req.Config.Mounts = append(
					req.Config.Mounts, &runtime.Mount{
						HostPath:      "sandbox:///sandbox/path",
						ContainerPath: "/container/path",
						Propagation:   runtime.MountPropagation_PROPAGATION_BIDIRECTIONAL,
					},
				)
				return nil
			},
			opts: []securitypolicy.ContainerConfigOpt{
				securitypolicy.WithMountConstraints(
					[]securitypolicy.MountConfig{
						{
							HostPath:      "sandbox:///sandbox/path",
							ContainerPath: "/container/path",
							Readonly:      true,
						},
					},
				)},
			expectedError: "is not allowed by mount constraints",
		},
		{
			name: "InvalidHostPathForRegex",
			sideEffect: func(req *runtime.CreateContainerRequest) error {
				req.Config.Mounts = append(
					req.Config.Mounts, &runtime.Mount{
						HostPath:      "sandbox:///sandbox/path/regex/no/match",
						ContainerPath: "/container/path",
						Propagation:   runtime.MountPropagation_PROPAGATION_BIDIRECTIONAL,
					},
				)
				return nil
			},
			opts: []securitypolicy.ContainerConfigOpt{
				securitypolicy.WithMountConstraints(
					[]securitypolicy.MountConfig{
						{
							HostPath:      "sandbox:///sandbox/path/R.+",
							ContainerPath: "/container/path",
						},
					},
				)},
			expectedError: "is not allowed by mount constraints",
		},
	} {
		t.Run(testConfig.name, func(t *testing.T) {
			alpinePolicy := alpineSecurityPolicy(t, "json", false, false, testConfig.opts...)
			sandboxRequest := sandboxRequestWithPolicy(t, alpinePolicy)
			sandboxRequest.Config.Annotations[annotations.SecurityPolicyEnforcer] = "standard"

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

			if err := testConfig.sideEffect(containerRequest); err != nil {
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
			if !strings.Contains(err.Error(), testConfig.expectedError) {
				t.Fatalf("expected %q in error message, got: %q", testConfig.expectedError, err)
			}
		})
	}
}

func Test_RunPrivilegedContainer_WithPolicy_And_AllowElevated_Set(t *testing.T) {
	requireFeatures(t, featureLCOWIntegrity)
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, pc := range policyTestMatrix {
		t.Run(t.Name()+fmt.Sprintf("_Enforcer_%s_Input_%s", pc.enforcer, pc.input), func(t *testing.T) {
			alpinePolicy := alpineSecurityPolicy(t, pc.input, false, false, securitypolicy.WithAllowElevated(true))
			sandboxRequest := sandboxRequestWithPolicy(t, alpinePolicy)
			sandboxRequest.Config.Linux = &runtime.LinuxPodSandboxConfig{
				SecurityContext: &runtime.LinuxSandboxSecurityContext{
					Privileged: true,
				},
			}
			sandboxRequest.Config.Annotations[annotations.SecurityPolicyEnforcer] = pc.enforcer

			podID := runPodSandbox(t, client, ctx, sandboxRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			contRequest := getCreateContainerRequest(
				podID,
				"alpine-privileged",
				imageLcowAlpine,
				validPolicyAlpineCommand,
				sandboxRequest.Config,
			)
			contRequest.Config.Linux = &runtime.LinuxContainerConfig{
				SecurityContext: &runtime.LinuxContainerSecurityContext{
					Privileged: true,
				},
			}
			containerID := createContainer(t, client, ctx, contRequest)
			defer removeContainer(t, client, ctx, containerID)
			startContainer(t, client, ctx, containerID)
			defer stopContainer(t, client, ctx, containerID)
		})
	}
}

// todo (maksiman): add coverage for rego enforcer
func Test_RunPrivilegedContainer_WithPolicy_And_AllowElevated_NotSet(t *testing.T) {
	requireFeatures(t, featureLCOWIntegrity)
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	alpinePolicy := alpineSecurityPolicy(t, "json", false, false)
	sandboxRequest := sandboxRequestWithPolicy(t, alpinePolicy)
	sandboxRequest.Config.Linux = &runtime.LinuxPodSandboxConfig{
		SecurityContext: &runtime.LinuxSandboxSecurityContext{
			Privileged: true,
		},
	}
	sandboxRequest.Config.Annotations[annotations.SecurityPolicyEnforcer] = "standard"

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	contRequest := getCreateContainerRequest(
		podID,
		"alpine-privileged",
		imageLcowAlpine,
		validPolicyAlpineCommand,
		sandboxRequest.Config,
	)
	contRequest.Config.Linux = &runtime.LinuxContainerConfig{
		SecurityContext: &runtime.LinuxContainerSecurityContext{
			Privileged: true,
		},
	}
	containerID := createContainer(t, client, ctx, contRequest)
	defer removeContainer(t, client, ctx, containerID)
	if _, err := client.StartContainer(
		ctx,
		&runtime.StartContainerRequest{ContainerId: containerID},
	); err == nil {
		t.Fatalf("expected to fail")
	} else {
		expectedErrStr := "privileged escalation unmatched by policy rule"
		if !strings.Contains(err.Error(), expectedErrStr) {
			t.Fatalf("expected different error: %s", err)
		}
	}
}

// todo (maksiman): add coverage for rego enforcer
func Test_RunContainer_WithPolicy_CannotSet_AllowAll_And_Containers(t *testing.T) {
	requireFeatures(t, featureLCOW, featureLCOWIntegrity)
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defaultContainers, err := helpers.PolicyContainersFromConfigs(helpers.DefaultContainerConfigs())
	if err != nil {
		t.Fatalf("failed to create policy for default containers: %s", err)
	}

	policy := securitypolicy.NewSecurityPolicy(true, defaultContainers)
	stringPolicy, err := policy.EncodeToString()
	if err != nil {
		t.Fatalf("failed to encode policy to base64 string: %s", err)
	}

	sandboxRequest := sandboxRequestWithPolicy(t, stringPolicy)
	_, err = client.RunPodSandbox(ctx, sandboxRequest)
	if err == nil {
		t.Fatal("expected to fail")
	}
	if !strings.Contains(err.Error(), securitypolicy.ErrInvalidOpenDoorPolicy.Error()) {
		t.Fatalf("expected error %s, got %s", securitypolicy.ErrInvalidOpenDoorPolicy, err)
	}
}

func Test_RunContainer_WithPolicy_And_SecurityPolicyEnv_Annotation(t *testing.T) {
	requireFeatures(t, featureLCOW, featureLCOWIntegrity)
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	openDoorPolicy, err := securitypolicy.NewOpenDoorPolicy().EncodeToString()
	if err != nil {
		t.Fatalf("failed to create open door policy string: %s", err)
	}

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The command prints environment variables to stdout, which we can capture
	// and validate later
	alpineCmd := []string{"ash", "-c", "env && sleep 1"}

	opts := []securitypolicy.ContainerConfigOpt{
		securitypolicy.WithCommand(alpineCmd),
		securitypolicy.WithAllowStdioAccess(true),
	}
	for _, config := range []struct {
		name   string
		policy string
	}{
		{
			name:   "OpenDoorPolicy",
			policy: openDoorPolicy,
		},
		{
			name:   "StandardPolicy",
			policy: alpineSecurityPolicy(t, "json", false, false, opts...),
		},
		{
			name:   "RegoPolicy",
			policy: alpineSecurityPolicy(t, "rego", false, false, opts...),
		},
	} {
		for _, setPolicyEnv := range []bool{true, false} {
			testName := fmt.Sprintf("%s_SecurityPolicyEnvSet_%v", config.name, setPolicyEnv)
			t.Run(testName, func(t *testing.T) {
				sandboxRequest := sandboxRequestWithPolicy(t, config.policy)

				podID := runPodSandbox(t, client, ctx, sandboxRequest)
				defer removePodSandbox(t, client, ctx, podID)
				defer stopPodSandbox(t, client, ctx, podID)

				containerRequest := getCreateContainerRequest(
					podID,
					"alpine-with-policy",
					imageLcowAlpine,
					alpineCmd,
					sandboxRequest.Config,
				)
				certValue := "dummy-cert-value"
				if setPolicyEnv {
					containerRequest.Config.Annotations = map[string]string{
						annotations.UVMSecurityPolicyEnv: "true",
						annotations.HostAMDCertificate:   certValue,
					}
				} else {
					containerRequest.Config.Annotations = map[string]string{
						annotations.UVMSecurityPolicyEnv: "false",
					}
				}

				// setup logfile to capture stdout
				logPath := filepath.Join(t.TempDir(), "log.txt")
				containerRequest.Config.LogPath = logPath

				containerID := createContainer(t, client, ctx, containerRequest)
				defer removeContainer(t, client, ctx, containerID)

				startContainer(t, client, ctx, containerID)
				requireContainerState(ctx, t, client, containerID, runtime.ContainerState_CONTAINER_RUNNING)

				// no need to stop the container since it'll exit by itself
				requireContainerState(ctx, t, client, containerID, runtime.ContainerState_CONTAINER_EXITED)

				content, err := os.ReadFile(logPath)
				if err != nil {
					t.Fatalf("error reading log file: %s", err)
				}
				targetEnvs := []string{
					fmt.Sprintf("UVM_SECURITY_POLICY=%s", config.policy),
					"UVM_REFERENCE_INFO=",
					fmt.Sprintf("UVM_HOST_AMD_CERTIFICATE=%s", certValue),
				}
				if setPolicyEnv {
					// make sure that the expected environment variable was set
					for _, env := range targetEnvs {
						if !strings.Contains(string(content), env) {
							t.Fatalf("missing init process environment variable: %s", env)
						}
					}
				} else {
					for _, env := range targetEnvs {
						if strings.Contains(string(content), env) {
							t.Fatalf("environment variable should not be set for init process: %s", env)
						}
					}
				}
			})
		}
	}
}

func Test_RunContainer_WithPolicy_And_SecurityPolicyEnv_Dropping(t *testing.T) {
	requireFeatures(t, featureLCOW, featureLCOWIntegrity)
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The command prints environment variables to stdout, which we can capture
	// and validate later
	alpineCmd := []string{"ash", "-c", "env && sleep 1"}

	for _, config := range []struct {
		name    string
		policy  string
		allowed bool
	}{
		{
			name:    "Dropped",
			policy:  alpineSecurityPolicy(t, "rego", true, false, securitypolicy.WithCommand(alpineCmd)),
			allowed: true,
		},
		{
			name:    "NotDropped",
			policy:  alpineSecurityPolicy(t, "rego", false, false, securitypolicy.WithCommand(alpineCmd)),
			allowed: false,
		},
	} {
		t.Run(config.name, func(t *testing.T) {
			sandboxRequest := sandboxRequestWithPolicy(t, config.policy)

			podID := runPodSandbox(t, client, ctx, sandboxRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			containerRequest := getCreateContainerRequest(
				podID,
				"alpine-with-policy",
				imageLcowAlpine,
				alpineCmd,
				sandboxRequest.Config,
			)

			// setup logfile to capture stdout
			logPath := filepath.Join(t.TempDir(), "log.txt")
			containerRequest.Config.LogPath = logPath

			badKV := &runtime.KeyValue{
				Key: "INVALID_ENV", Value: "this/should/cause/an/error/",
			}
			containerRequest.Config.Envs = append(containerRequest.Config.Envs, badKV)

			response, err := client.CreateContainer(ctx, containerRequest)
			if err != nil {
				t.Fatalf("error creating container: %v", err)
			}

			containerID := response.ContainerId
			defer removeContainer(t, client, ctx, containerID)

			_, err = client.StartContainer(
				ctx, &runtime.StartContainerRequest{
					ContainerId: containerID,
				},
			)

			if config.allowed {
				if err != nil {
					t.Fatalf("failed EnforceCreateContainer in sandbox: %s, with: %v", containerRequest.PodSandboxId, err)
				}
			} else {
				if err == nil {
					t.Fatalf("expected EnforceCreateContainer to be denied")
				}
				return
			}

			requireContainerState(ctx, t, client, containerID, runtime.ContainerState_CONTAINER_RUNNING)

			// no need to stop the container since it'll exit by itself
			requireContainerState(ctx, t, client, containerID, runtime.ContainerState_CONTAINER_EXITED)

			content, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatalf("error reading log file: %s", err)
			}

			badEnv := fmt.Sprintf("%s=%s", badKV.Key, badKV.Value)
			if strings.Contains(string(content), badEnv) {
				t.Fatalf("INVALID_ENV env var shouldn't be set for init process:\n%s\n", string(content))
			}
		})
	}
}

// The test covers positive test scenarios around scratch encryption:
// - policy allows unencrypted scratch and scratch is encrypted
// - policy allows unencrypted scratch and scratch is not encrypted
// - policy doesn't allow unencrypted scratch and scratch is encrypted
func Test_RunPodSandboxAllowed_WithPolicy_EncryptedScratchPolicy(t *testing.T) {
	requireFeatures(t, featureLCOWIntegrity, featureLCOWCrypt)
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, tc := range []struct {
		allowUnencrypted  bool
		encryptAnnotation bool
	}{
		{
			true,
			true,
		},
		{
			true,
			false,
		}, {
			false,
			true,
		},
	} {
		t.Run(fmt.Sprintf("AllowUnencrypted_%t_EncryptionEnabled_%t", tc.allowUnencrypted, tc.encryptAnnotation), func(t *testing.T) {
			policy := policyFromOpts(
				t,
				"rego",
				securitypolicy.WithExternalProcesses(defaultExternalProcesses),
				securitypolicy.WithAllowUnencryptedScratch(tc.allowUnencrypted),
				securitypolicy.WithAllowEnvVarDropping(true),
				securitypolicy.WithAllowCapabilityDropping(true),
			)
			sandboxRequest := sandboxRequestWithPolicy(t, policy)
			// sandboxRequestWithPolicy sets security policy annotation, so we
			// won't get a nil point deref here.
			sandboxRequest.Config.Annotations[annotations.EncryptedScratchDisk] = fmt.Sprintf("%t", tc.encryptAnnotation)
			podID := runPodSandbox(t, client, ctx, sandboxRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			if tc.encryptAnnotation {
				output := shimDiagExecOutput(ctx, t, podID, []string{"ls", "-l", "/dev/mapper"})
				if !strings.Contains(output, "dm-crypt-scsi-contr") {
					t.Log(output)
					t.Fatal("expected to find dm-crypt target")
				}
			}
		})
	}
}

// The test covers negative scenario when policy doesn't allow unencrypted scratch
// and scratch is not encrypted.
func Test_RunPodSandboxNotAllowed_WithPolicy_EncryptedScratchPolicy(t *testing.T) {
	requireFeatures(t, featureLCOWIntegrity, featureLCOWCrypt)
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	policy := policyFromOpts(
		t,
		"rego",
		securitypolicy.WithExternalProcesses(defaultExternalProcesses),
		securitypolicy.WithAllowUnencryptedScratch(false),
		securitypolicy.WithAllowEnvVarDropping(true),
		securitypolicy.WithAllowCapabilityDropping(true),
	)
	sandboxRequest := sandboxRequestWithPolicy(t, policy)
	sandboxRequest.Config.Annotations[annotations.EncryptedScratchDisk] = "false"

	// we didn't pass encrypt scratch annotation and policy should reject pod creation
	response, err := client.RunPodSandbox(ctx, sandboxRequest)
	if err == nil {
		_, err := client.StopPodSandbox(ctx, &runtime.StopPodSandboxRequest{PodSandboxId: response.PodSandboxId})
		if err != nil {
			t.Errorf("failed to stop sandbox: %s", err)
		}
		_, err = client.RemovePodSandbox(ctx, &runtime.RemovePodSandboxRequest{PodSandboxId: response.PodSandboxId})
		if err != nil {
			t.Errorf("failed to remove sandbox: %s", err)
		}
		t.Fatalf("expected to fail")
	}
	expectedError := "unencrypted scratch not allowed"
	if !strings.Contains(err.Error(), expectedError) {
		t.Fatalf("expected '%s' error, got '%s'", expectedError, err)
	}
}

func Test_RunContainer_WithPolicy_And_Binary_Logger_Without_Stdio(t *testing.T) {
	requireFeatures(t, featureLCOWIntegrity)

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	binaryPath := requireBinary(t, "sample-logging-driver.exe")

	logPath := "binary:///" + binaryPath

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	for _, tc := range []struct {
		stdioAllowed   bool
		expectedOutput string
	}{
		{
			true,
			"hello\nworld\n",
		},
		{
			false,
			"",
		},
	} {
		t.Run(fmt.Sprintf("StdioAllowed_%v", tc.stdioAllowed), func(t *testing.T) {
			cmd := []string{"ash", "-c", "echo hello; sleep 1; echo world"}
			policy := alpineSecurityPolicy(
				t,
				"rego",
				true,
				false,
				securitypolicy.WithAllowStdioAccess(tc.stdioAllowed),
				securitypolicy.WithCommand(cmd),
			)
			podReq := sandboxRequestWithPolicy(t, policy)
			podID := runPodSandbox(t, client, ctx, podReq)
			defer removePodSandbox(t, client, ctx, podID)

			logFileName := fmt.Sprintf(`%s\stdout.txt`, t.TempDir())
			conReq := getCreateContainerRequest(
				podID,
				fmt.Sprintf("alpine-stdio-allowed-%v", tc.stdioAllowed),
				imageLcowAlpine,
				cmd,
				podReq.Config,
			)
			conReq.Config.LogPath = logPath + fmt.Sprintf("?%s", logFileName)

			containerID := createContainer(t, client, ctx, conReq)
			defer removeContainer(t, client, ctx, containerID)

			startContainer(t, client, ctx, containerID)
			defer stopContainer(t, client, ctx, containerID)

			requireContainerState(ctx, t, client, containerID, runtime.ContainerState_CONTAINER_RUNNING)
			requireContainerState(ctx, t, client, containerID, runtime.ContainerState_CONTAINER_EXITED)

			content, err := os.ReadFile(logFileName)
			if err != nil {
				t.Fatalf("failed to read log file: %s", err)
			}
			if tc.expectedOutput != string(content) {
				t.Fatalf("expected output %q, got %q", tc.expectedOutput, string(content))
			}
		})
	}
}

func Test_ExecInContainer_WithPolicy(t *testing.T) {
	requireFeatures(t, featureLCOWIntegrity)
	pullRequiredLCOWImages(t, []string{imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, tc := range []struct {
		execProcessConfig  securitypolicy.ExecProcessConfig
		execProcessRequest []string
		shouldFail         bool
	}{
		{
			execProcessConfig: securitypolicy.ExecProcessConfig{
				Command: []string{"ls"},
			},
			execProcessRequest: []string{"ls"},
			shouldFail:         false,
		},
		{
			execProcessConfig: securitypolicy.ExecProcessConfig{
				Command: []string{"ls"},
			},
			execProcessRequest: []string{"ls", "-l"},
			shouldFail:         true,
		},
	} {
		t.Run(fmt.Sprintf("ExecInContainer_ShouldFail_%t", tc.shouldFail), func(t *testing.T) {
			cmd := []string{"ash", "-c", "while true; do sleep 1; done"}
			policy := alpineSecurityPolicy(
				t,
				"rego",
				true,
				false,
				securitypolicy.WithExecProcesses([]securitypolicy.ExecProcessConfig{tc.execProcessConfig}),
				securitypolicy.WithCommand(cmd),
			)
			sandboxRequest := sandboxRequestWithPolicy(t, policy)
			podID := runPodSandbox(t, client, ctx, sandboxRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			conReq := getCreateContainerRequest(
				podID,
				fmt.Sprintf("alpine-exec-not-allowed-%t", tc.shouldFail),
				imageLcowAlpine,
				cmd,
				sandboxRequest.Config,
			)

			containerID := createContainer(t, client, ctx, conReq)
			defer removeContainer(t, client, ctx, containerID)

			startContainer(t, client, ctx, containerID)
			defer stopContainer(t, client, ctx, containerID)

			requireContainerState(ctx, t, client, containerID, runtime.ContainerState_CONTAINER_RUNNING)

			execReq := &runtime.ExecSyncRequest{
				ContainerId: containerID,
				Cmd:         tc.execProcessRequest,
				Timeout:     20,
			}
			_, err := client.ExecSync(ctx, execReq)
			if err == nil {
				if tc.shouldFail {
					t.Fatal("exec should've been denied by policy")
				}
			} else {
				if !tc.shouldFail {
					t.Fatalf("unexpected exec failure: %s", err)
				}
				if !strings.Contains(err.Error(), "invalid command") {
					t.Fatalf("expected 'invalid command' error, got '%s' instead", err)
				}
			}
		})
	}
}

func Test_ExecInContainer_WithPolicy_Privileged(t *testing.T) {
	requireFeatures(t, featureLCOWIntegrity)
	pullRequiredLCOWImages(t, []string{imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, tc := range []struct {
		execProcessConfig  securitypolicy.ExecProcessConfig
		execProcessRequest []string
		shouldFail         bool
	}{
		{
			execProcessConfig: securitypolicy.ExecProcessConfig{
				Command: []string{"ls"},
			},
			execProcessRequest: []string{"ls"},
			shouldFail:         false,
		},
		{
			execProcessConfig: securitypolicy.ExecProcessConfig{
				Command: []string{"ls"},
			},
			execProcessRequest: []string{"ls", "-l"},
			shouldFail:         true,
		},
	} {
		t.Run(fmt.Sprintf("ExecInContainer_ShouldFail_%t", tc.shouldFail), func(t *testing.T) {
			cmd := []string{"ash", "-c", "while true; do sleep 1; done"}
			policy := alpineSecurityPolicy(
				t,
				"rego",
				true,
				false,
				securitypolicy.WithExecProcesses([]securitypolicy.ExecProcessConfig{tc.execProcessConfig}),
				securitypolicy.WithCommand(cmd),
				securitypolicy.WithAllowElevated(true),
			)
			sandboxRequest := sandboxRequestWithPolicy(t, policy)
			sandboxRequest.Config.Linux = &runtime.LinuxPodSandboxConfig{
				SecurityContext: &runtime.LinuxSandboxSecurityContext{
					Privileged: true,
				},
			}
			podID := runPodSandbox(t, client, ctx, sandboxRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			conReq := getCreateContainerRequest(
				podID,
				fmt.Sprintf("alpine-exec-not-allowed-%t", tc.shouldFail),
				imageLcowAlpine,
				cmd,
				sandboxRequest.Config,
			)
			conReq.Config.Linux = &runtime.LinuxContainerConfig{
				SecurityContext: &runtime.LinuxContainerSecurityContext{
					Privileged: true,
				},
			}

			containerID := createContainer(t, client, ctx, conReq)
			defer removeContainer(t, client, ctx, containerID)

			startContainer(t, client, ctx, containerID)
			defer stopContainer(t, client, ctx, containerID)

			requireContainerState(ctx, t, client, containerID, runtime.ContainerState_CONTAINER_RUNNING)

			execReq := &runtime.ExecSyncRequest{
				ContainerId: containerID,
				Cmd:         tc.execProcessRequest,
				Timeout:     20,
			}
			_, err := client.ExecSync(ctx, execReq)
			if err == nil {
				if tc.shouldFail {
					t.Fatal("exec should've been denied by policy")
				}
			} else {
				if !tc.shouldFail {
					t.Fatalf("unexpected exec failure: %s", err)
				}
				if !strings.Contains(err.Error(), "invalid command") {
					t.Fatalf("expected 'invalid command' error, got '%s' instead", err)
				}
			}
		})
	}
}

func Test_ExecInUVM_WithPolicy(t *testing.T) {
	requireFeatures(t, featureLCOWIntegrity)

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, tc := range []struct {
		execInUVMConfig  securitypolicy.ExternalProcessConfig
		execInUVMRequest []string
		shouldFail       bool
	}{
		{
			execInUVMConfig: securitypolicy.ExternalProcessConfig{
				Command:          []string{"ls"},
				WorkingDir:       "/",
				AllowStdioAccess: true,
			},
			execInUVMRequest: []string{"ls"},
			shouldFail:       false,
		},
		{
			execInUVMConfig: securitypolicy.ExternalProcessConfig{
				Command:          []string{"ls"},
				WorkingDir:       "/",
				AllowStdioAccess: true,
			},
			execInUVMRequest: []string{"ls", "-l"},
			shouldFail:       true,
		},
	} {
		t.Run(fmt.Sprintf("ShouldFail_%t", tc.shouldFail), func(t *testing.T) {
			policy := policyFromOpts(t, "rego",
				securitypolicy.WithExternalProcesses([]securitypolicy.ExternalProcessConfig{tc.execInUVMConfig}),
				securitypolicy.WithAllowRuntimeLogging(true),
				securitypolicy.WithAllowUnencryptedScratch(true),
			)
			sandboxRequest := sandboxRequestWithPolicy(t, policy)
			podID := runPodSandbox(t, client, ctx, sandboxRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			_, err := shimDiagExecOutputWithErr(ctx, t, podID, tc.execInUVMRequest)
			if err != nil {
				if !tc.shouldFail {
					t.Fatalf("external process exec should succeed, got error instead: %s", err)
				}
				if !strings.Contains(err.Error(), "invalid command") {
					t.Fatalf("expected invalid command error, got %s", err)
				}
			} else {
				if tc.shouldFail {
					t.Fatal("external process exec should have failed")
				}
			}
		})
	}
}

func Test_RunPodSandbox_Concurrently(t *testing.T) {
	requireFeatures(t, featureLCOWIntegrity)

	for i := 0; i < 20; i++ {
		t.Run(fmt.Sprintf("ParallelPodRun_%d", i+1), func(t *testing.T) {
			t.Parallel()
			client := newTestRuntimeClient(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			policy := policyFromOpts(
				t,
				"rego",
				securitypolicy.WithAllowUnencryptedScratch(true),
			)
			runpRequest := &runtime.RunPodSandboxRequest{
				Config: &runtime.PodSandboxConfig{
					Metadata: &runtime.PodSandboxMetadata{
						Name:      fmt.Sprintf("%s_%d", t.Name(), i),
						Namespace: testNamespace,
					},
					Annotations: map[string]string{
						annotations.NoSecurityHardware:     strconv.FormatBool(!*flagSevSnp),
						annotations.SecurityPolicy:         policy,
						annotations.SecurityPolicyEnforcer: "rego",
						annotations.EncryptedScratchDisk:   strconv.FormatBool(*flagSevSnp),
					},
				},
				RuntimeHandler: lcowRuntimeHandler,
			}

			podID := runPodSandbox(t, client, ctx, runpRequest)
			defer func() {
				removePodSandbox(t, client, ctx, podID)
			}()
			defer func() {
				stopPodSandbox(t, client, ctx, podID)
			}()
			time.Sleep(5 * time.Second)
		})
	}
}

func Test_RunContainer_WithPolicy_And_NoNewPrivileges(t *testing.T) {
	requireFeatures(t, featureLCOW, featureLCOWIntegrity)
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	policy := alpineSecurityPolicy(t, "rego", false, false, securitypolicy.WithAllowPrivilegeEscalation(false))

	for _, config := range []struct {
		name                     string
		allowPrivilegeEscalation string
		allowed                  bool
	}{
		{
			name:                     "AllowPrivilegeEscalation_False_Allow",
			allowPrivilegeEscalation: "false",
			allowed:                  true,
		},
		{
			name:                     "AllowPrivilegeEscalation_True_Deny",
			allowPrivilegeEscalation: "true",
			allowed:                  false,
		},
	} {
		t.Run(config.name, func(t *testing.T) {
			sandboxRequest := sandboxRequestWithPolicy(t, policy)

			podID := runPodSandbox(t, client, ctx, sandboxRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			containerRequest := getCreateContainerRequest(
				podID,
				"alpine-with-policy-and-no-new-privs",
				imageLcowAlpine,
				validPolicyAlpineCommand,
				sandboxRequest.Config,
			)
			containerRequest.Config.Annotations = map[string]string{
				"io.microsoft.cri.lcow.container.allowprivilegeescalation": config.allowPrivilegeEscalation,
			}

			response, err := client.CreateContainer(ctx, containerRequest)
			if err != nil {
				t.Fatalf("error creating container: %v", err)
			}

			containerID := response.ContainerId
			defer removeContainer(t, client, ctx, containerID)

			_, err = client.StartContainer(
				ctx, &runtime.StartContainerRequest{
					ContainerId: containerID,
				},
			)

			if err == nil {
				defer stopContainer(t, client, ctx, containerID)
				if !config.allowed {
					t.Errorf("expected container to fail to start")
				}
			} else {
				if config.allowed {
					t.Errorf("expected container to start successfully: %s", err)
				}
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

func Test_RunContainer_WithPolicy_And_RunAs(t *testing.T) {
	requireFeatures(t, featureLCOW, featureLCOWIntegrity)
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowCustomUser})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := []string{"sh", "-c", "echo 'Hello'"}
	policy := securityPolicyFromImageWithOpts(t, imageLcowCustomUser, "rego", false, false, securitypolicy.WithCommand(cmd), securitypolicy.WithUser(userConfig(1000, 1000)))

	for _, config := range []struct {
		name    string
		uid     string
		gid     string
		allowed bool
	}{
		{
			name:    "UID_Match_GID_Match_Allow",
			uid:     "1000",
			gid:     "1000",
			allowed: true,
		},
		{
			name:    "UID_Match_GID_NoMatch_Deny",
			uid:     "1000",
			gid:     "0",
			allowed: false,
		},
		{
			name:    "UID_NoMatch_GID_Match_Deny",
			uid:     "0",
			gid:     "1000",
			allowed: false,
		},
		{
			name:    "UID_NoMatch_GID_NoMatch_Deny",
			uid:     "0",
			gid:     "0",
			allowed: false,
		},
	} {
		t.Run(config.name, func(t *testing.T) {
			sandboxRequest := sandboxRequestWithPolicy(t, policy)

			podID := runPodSandbox(t, client, ctx, sandboxRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			containerRequest := getCreateContainerRequest(
				podID,
				"alpine-with-policy-and-custom-user",
				imageLcowCustomUser,
				cmd,
				sandboxRequest.Config,
			)
			containerRequest.Config.Annotations = map[string]string{
				"io.microsoft.cri.lcow.container.runasuser":  config.uid,
				"io.microsoft.cri.lcow.container.runasgroup": config.gid,
			}

			response, err := client.CreateContainer(ctx, containerRequest)
			if err != nil {
				t.Fatalf("error creating container: %v", err)
			}

			containerID := response.ContainerId
			defer removeContainer(t, client, ctx, containerID)

			_, err = client.StartContainer(
				ctx, &runtime.StartContainerRequest{
					ContainerId: containerID,
				},
			)

			if err == nil {
				defer stopContainer(t, client, ctx, containerID)
				if !config.allowed {
					t.Errorf("expected container to fail to start")
				}
			} else {
				if config.allowed {
					t.Errorf("expected container to start successfully: %s", err)
				}
			}
		})
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

func Test_RunContainer_WithPolicy_And_Capabilities(t *testing.T) {
	requireFeatures(t, featureLCOW, featureLCOWIntegrity)
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowCustomUser})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	noDropCapsPolicy := alpineSecurityPolicy(t, "rego", false, false, securitypolicy.WithCapabilities(capabilitiesConfig()))
	dropCapsPolicy := alpineSecurityPolicy(t, "rego", false, true, securitypolicy.WithCapabilities(capabilitiesConfig()))

	for _, config := range []struct {
		name    string
		policy  string
		add     string
		drop    string
		allowed bool
	}{
		{
			name:    "NoDropping_NoAdd_NoDrop_Allow",
			policy:  noDropCapsPolicy,
			add:     "",
			drop:    "",
			allowed: true,
		},
		{
			name:    "NoDropping_Add_NoDrop_Deny",
			policy:  noDropCapsPolicy,
			add:     "CAP_SYS_ADMIN",
			drop:    "",
			allowed: false,
		},
		{
			name:    "NoDropping_NoAdd_Drop_Deny",
			policy:  noDropCapsPolicy,
			add:     "",
			drop:    "CAP_CHOWN,CAP_NET_BIND_SERVICE",
			allowed: false,
		},
		{
			name:    "Dropping_Add_NoDrop_Allow",
			policy:  dropCapsPolicy,
			add:     "CAP_SYS_ADMIN,CAP_NET_BROADCAST",
			drop:    "",
			allowed: true,
		},
		{
			name:    "Dropping_NoAdd_Drop_Deny",
			policy:  dropCapsPolicy,
			add:     "",
			drop:    "CAP_CHOWN",
			allowed: false,
		},
	} {
		t.Run(config.name, func(t *testing.T) {
			sandboxRequest := sandboxRequestWithPolicy(t, config.policy)

			podID := runPodSandbox(t, client, ctx, sandboxRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			containerRequest := getCreateContainerRequest(
				podID,
				"alpine-with-policy-and-capabilities",
				imageLcowAlpine,
				validPolicyAlpineCommand,
				sandboxRequest.Config,
			)
			containerRequest.Config.Annotations = map[string]string{
				"io.microsoft.cri.lcow.container.capabilities.add":  config.add,
				"io.microsoft.cri.lcow.container.capabilities.drop": config.drop,
			}

			response, err := client.CreateContainer(ctx, containerRequest)
			if err != nil {
				t.Fatalf("error creating container: %v", err)
			}

			containerID := response.ContainerId
			defer removeContainer(t, client, ctx, containerID)

			_, err = client.StartContainer(
				ctx, &runtime.StartContainerRequest{
					ContainerId: containerID,
				},
			)

			if err == nil {
				defer stopContainer(t, client, ctx, containerID)
				if !config.allowed {
					t.Errorf("expected container to fail to start")
				}
			} else {
				if config.allowed {
					t.Errorf("expected container to start successfully: %s", err)
				}
			}
		})
	}
}

//go:embed seccomp_valid.json
var validSeccomp []byte

//go:embed seccomp_invalid.json
var invalidSeccomp []byte

func Test_RunContainer_WithPolicy_And_Seccomp(t *testing.T) {
	requireFeatures(t, featureLCOW, featureLCOWIntegrity)
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	seccompPolicy := alpineSecurityPolicy(t, "rego", false, false, securitypolicy.WithSeccompProfilePath("seccomp_valid.json"))
	noSeccompPolicy := alpineSecurityPolicy(t, "rego", false, false)

	validSeccompBase64 := base64.StdEncoding.EncodeToString(validSeccomp)
	invalidSeccompBase64 := base64.StdEncoding.EncodeToString(invalidSeccomp)

	for _, config := range []struct {
		name          string
		policy        string
		seccompBase64 string
		allowed       bool
	}{
		{
			name:          "Seccomp_Valid_Allow",
			policy:        seccompPolicy,
			seccompBase64: validSeccompBase64,
			allowed:       true,
		},
		{
			name:          "Seccomp_Invalid_Deny",
			policy:        seccompPolicy,
			seccompBase64: invalidSeccompBase64,
			allowed:       false,
		},
		{
			name:          "NoSeccomp_Nil_Allow",
			policy:        noSeccompPolicy,
			seccompBase64: "",
			allowed:       true,
		},
		{
			name:          "NoSeccomp_Valid_Deny",
			policy:        noSeccompPolicy,
			seccompBase64: validSeccompBase64,
			allowed:       false,
		},
	} {
		t.Run(config.name, func(t *testing.T) {
			sandboxRequest := sandboxRequestWithPolicy(t, config.policy)

			podID := runPodSandbox(t, client, ctx, sandboxRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			containerRequest := getCreateContainerRequest(
				podID,
				"alpine-with-policy-and-seccomp",
				imageLcowAlpine,
				validPolicyAlpineCommand,
				sandboxRequest.Config,
			)

			if len(config.seccompBase64) > 0 {
				containerRequest.Config.Annotations = map[string]string{
					"io.microsoft.cri.lcow.container.seccompprofile": config.seccompBase64,
				}
			}

			response, err := client.CreateContainer(ctx, containerRequest)
			if err != nil {
				t.Fatalf("error creating container: %v", err)
			}

			containerID := response.ContainerId
			defer removeContainer(t, client, ctx, containerID)

			_, err = client.StartContainer(
				ctx, &runtime.StartContainerRequest{
					ContainerId: containerID,
				},
			)

			if err == nil {
				defer stopContainer(t, client, ctx, containerID)
				if !config.allowed {
					t.Errorf("expected container to fail to start")
				}
			} else {
				if config.allowed && !strings.Contains(err.Error(), "seccomp: config provided but seccomp not supported") {
					t.Errorf("expected container to start successfully: %s", err)
				}
			}
		})
	}
}

//go:embed policy-v0.1.0.rego
var oldPolicy []byte

func Test_RunPrivilegedContainer_WithPolicy_BackwardsCompatible(t *testing.T) {
	requireFeatures(t, featureLCOWIntegrity)
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	alpinePolicy := base64.StdEncoding.EncodeToString(oldPolicy)
	sandboxRequest := sandboxRequestWithPolicy(t, alpinePolicy)
	sandboxRequest.Config.Linux = &runtime.LinuxPodSandboxConfig{
		SecurityContext: &runtime.LinuxSandboxSecurityContext{
			Privileged: true,
		},
	}
	sandboxRequest.Config.Annotations[annotations.SecurityPolicyEnforcer] = "rego"

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	contRequest := getCreateContainerRequest(
		podID,
		"alpine-privileged",
		imageLcowAlpine,
		validPolicyAlpineCommand,
		sandboxRequest.Config,
	)
	contRequest.Config.Linux = &runtime.LinuxContainerConfig{
		SecurityContext: &runtime.LinuxContainerSecurityContext{
			Privileged: true,
		},
	}
	containerID := createContainer(t, client, ctx, contRequest)
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)
}

func Test_RunContainer_WithPolicy_BackwardsCompatible(t *testing.T) {
	requireFeatures(t, featureLCOWIntegrity)
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	alpinePolicy := base64.StdEncoding.EncodeToString(oldPolicy)
	sandboxRequest := sandboxRequestWithPolicy(t, alpinePolicy)
	sandboxRequest.Config.Annotations[annotations.SecurityPolicyEnforcer] = "rego"

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	contRequest := getCreateContainerRequest(
		podID,
		"alpine-privileged",
		imageLcowAlpine,
		validPolicyAlpineCommand,
		sandboxRequest.Config,
	)
	containerID := createContainer(t, client, ctx, contRequest)
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)
}
