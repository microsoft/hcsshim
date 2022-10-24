//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	"github.com/Microsoft/hcsshim/internal/tools/securitypolicy/helpers"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

var validPolicyAlpineCommand = []string{"ash", "-c", "echo 'Hello'"}

type configSideEffect func(*runtime.CreateContainerRequest) error

func securityPolicyFromContainers(policyType string, containers []securitypolicy.ContainerConfig) (string, error) {
	pc, err := helpers.PolicyContainersFromConfigs(containers)
	if err != nil {
		return "", err
	}
	policyString, err := securitypolicy.MarshalPolicy(policyType, false, pc,
		[]securitypolicy.ExternalProcessConfig{},
		[]securitypolicy.FragmentConfig{},
		true,
		true,
		true)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString([]byte(policyString)), nil
}

func sandboxSecurityPolicy(t *testing.T, policyType string) string {
	t.Helper()
	defaultContainers := helpers.DefaultContainerConfigs()
	policyString, err := securityPolicyFromContainers(policyType, defaultContainers)
	if err != nil {
		t.Fatalf("failed to create security policy string: %s", err)
	}
	return policyString
}

func alpineSecurityPolicy(t *testing.T, policyType string, opts ...securitypolicy.ContainerConfigOpt) string {
	t.Helper()
	defaultContainers := helpers.DefaultContainerConfigs()

	alpineContainer := securitypolicy.ContainerConfig{
		ImageName: imageLcowAlpine,
		Command:   validPolicyAlpineCommand,
	}

	for _, o := range opts {
		if err := o(&alpineContainer); err != nil {
			t.Fatalf("failed to apply config opt: %s", err)
		}
	}

	containers := append(defaultContainers, alpineContainer)
	policyString, err := securityPolicyFromContainers(policyType, containers)
	if err != nil {
		t.Fatalf("failed to create security policy string: %s", err)
	}
	return policyString
}

func sandboxRequestWithPolicy(t *testing.T, policy string) *runtime.RunPodSandboxRequest {
	t.Helper()
	return getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		WithSandboxAnnotations(
			map[string]string{
				annotations.NoSecurityHardware:  "true",
				annotations.SecurityPolicy:      policy,
				annotations.VPMemNoMultiMapping: "true",
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
			sandboxPolicy := sandboxSecurityPolicy(t, pc.input)
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
			alpinePolicy := alpineSecurityPolicy(t, pc.input)
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
				alpinePolicy := alpineSecurityPolicy(t, pc.input, testConfig.opts...)
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
			alpinePolicy := alpineSecurityPolicy(t, "json")
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
						HostPath:      "sandbox://sandbox/path",
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
							HostPath:      "sandbox://sandbox/path",
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
						HostPath:      "sandbox://sandbox/path",
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
							HostPath:      "sandbox://sandbox/path",
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
						HostPath:      "sandbox://sandbox/path/regexp",
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
							HostPath:      "sandbox://sandbox/path/r.+",
							ContainerPath: "/container/path",
						},
					},
				)},
		},
	} {
		for _, pc := range policyTestMatrix {
			t.Run(testConfig.name+fmt.Sprintf("_Enforcer_%s_Input_%s", pc.enforcer, pc.input), func(t *testing.T) {
				alpinePolicy := alpineSecurityPolicy(t, pc.input, testConfig.opts...)
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
					HostPath:      "sandbox://sandbox/path",
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
						HostPath:      "sandbox://sandbox/invalid/path",
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
						HostPath:      "sandbox://sandbox/path",
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
						HostPath:      "sandbox://sandbox/path",
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
						HostPath:      "sandbox://sandbox/path",
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
							HostPath:      "sandbox://sandbox/path",
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
						HostPath:      "sandbox://sandbox/path/regex/no/match",
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
							HostPath:      "sandbox://sandbox/path/R.+",
							ContainerPath: "/container/path",
						},
					},
				)},
			expectedError: "is not allowed by mount constraints",
		},
	} {
		t.Run(testConfig.name, func(t *testing.T) {
			alpinePolicy := alpineSecurityPolicy(t, "json", testConfig.opts...)
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
			alpinePolicy := alpineSecurityPolicy(t, pc.input, securitypolicy.WithAllowElevated(true))
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

	alpinePolicy := alpineSecurityPolicy(t, "json")
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
		expectedStr1 := "Destination:/sys"
		expectedStr2 := "is not allowed by mount constraints"
		if !strings.Contains(err.Error(), expectedStr1) || !strings.Contains(err.Error(), expectedStr2) {
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
			policy: alpineSecurityPolicy(t, "json", securitypolicy.WithCommand(alpineCmd)),
		},
		{
			name:   "RegoPolicy",
			policy: alpineSecurityPolicy(t, "rego", securitypolicy.WithCommand(alpineCmd)),
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
				if setPolicyEnv {
					containerRequest.Config.Annotations = map[string]string{
						annotations.UVMSecurityPolicyEnv: "true",
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
				policyEnv := fmt.Sprintf("UVM_SECURITY_POLICY=%s", config.policy)
				measurementEnv := "UVM_REFERENCE_INFO="
				if setPolicyEnv {
					// make sure that the expected environment variable was set
					if !strings.Contains(string(content), policyEnv) || !strings.Contains(string(content), measurementEnv) {
						t.Fatalf("UVM_SECURITY_POLICY and UVM_REFERENCE_INFO env vars should be set for init"+
							" process:\n%s\n", string(content))
					}
				} else {
					if strings.Contains(string(content), policyEnv) || strings.Contains(string(content), measurementEnv) {
						t.Fatalf("UVM_SECURITY_POLICY and UVM_REFERENCE_INFO env vars shouldn't be set for init"+
							" process:\n%s\n",
							string(content))
					}
				}
			})
		}
	}
}
