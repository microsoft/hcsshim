//go:build functional

package cri_containerd

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func Test_RunContainer_GMSA_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess, featureGMSA)

	credSpec := gmsaSetup(t)
	pullRequiredImages(t, []string{imageWindowsNanoserver})
	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sandboxRequest := getRunPodSandboxRequest(t, wcowProcessRuntimeHandler)

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	request := &runtime.CreateContainerRequest{
		PodSandboxId: podID,
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
			Windows: &runtime.WindowsContainerConfig{
				SecurityContext: &runtime.WindowsContainerSecurityContext{
					CredentialSpec: credSpec,
				},
			},
		},
		SandboxConfig: sandboxRequest.Config,
	}

	containerID := createContainer(t, client, ctx, request)
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	// No klist and no powershell available
	cmd := []string{"cmd", "/c", "set", "USERDNSDOMAIN"}
	containerExecReq := &runtime.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         cmd,
		Timeout:     20,
	}
	r := execSync(t, client, ctx, containerExecReq)
	if r.ExitCode != 0 {
		t.Fatalf("failed with exit code %d running 'set USERDNSDOMAIN': %s", r.ExitCode, string(r.Stderr))
	}
	// Check for USERDNSDOMAIN environment variable. This acts as a way tell if a
	// user is joined to an Active Directory Domain and is successfully
	// authenticated as a domain identity.
	if !strings.Contains(string(r.Stdout), "USERDNSDOMAIN") {
		t.Fatalf("expected to see USERDNSDOMAIN entry")
	}
}

func Test_RunContainer_GMSA_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor, featureGMSA)

	credSpec := gmsaSetup(t)
	pullRequiredImages(t, []string{imageWindowsNanoserver})
	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sandboxRequest := getRunPodSandboxRequest(t, wcowHypervisorRuntimeHandler)

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	request := &runtime.CreateContainerRequest{
		PodSandboxId: podID,
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
			Windows: &runtime.WindowsContainerConfig{
				SecurityContext: &runtime.WindowsContainerSecurityContext{
					CredentialSpec: credSpec,
				},
			},
		},
		SandboxConfig: sandboxRequest.Config,
	}

	containerID := createContainer(t, client, ctx, request)
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	// No klist and no powershell available
	cmd := []string{"cmd", "/c", "set", "USERDNSDOMAIN"}
	containerExecReq := &runtime.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         cmd,
		Timeout:     20,
	}
	r := execSync(t, client, ctx, containerExecReq)
	if r.ExitCode != 0 {
		t.Fatalf("failed with exit code %d running 'set USERDNSDOMAIN': %s", r.ExitCode, string(r.Stderr))
	}
	// Check for USERDNSDOMAIN environment variable. This acts as a way tell if a
	// user is joined to an Active Directory Domain and is successfully
	// authenticated as a domain identity.
	if !strings.Contains(string(r.Stdout), "USERDNSDOMAIN") {
		t.Fatalf("expected to see USERDNSDOMAIN entry")
	}
}

func Test_RunContainer_GMSA_Disabled(t *testing.T) {
	requireFeatures(t, featureGMSA)

	credSpec := "totally real and definitely not a fake or arbitrary gMSA credential spec that is 1000%% properly formatted as JSON"
	pullRequiredImages(t, []string{imageWindowsNanoserver})
	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type run struct {
		Name    string
		Feature string
		Runtime string
	}

	runs := []run{
		{
			Name:    "WCOW_Hypervisor",
			Feature: featureWCOWHypervisor,
			Runtime: wcowHypervisorRuntimeHandler,
		},
		{
			Name:    "WCOW_Process",
			Feature: featureWCOWProcess,
			Runtime: wcowHypervisorRuntimeHandler,
		},
	}

	for _, r := range runs {
		t.Run(r.Name, func(subtest *testing.T) {
			requireFeatures(subtest, r.Feature)
			sandboxRequest := getRunPodSandboxRequest(subtest, r.Runtime)

			podID := runPodSandbox(subtest, client, ctx, sandboxRequest)
			defer removePodSandbox(subtest, client, ctx, podID)
			defer stopPodSandbox(subtest, client, ctx, podID)

			request := &runtime.CreateContainerRequest{
				PodSandboxId: podID,
				Config: &runtime.ContainerConfig{
					Metadata: &runtime.ContainerMetadata{
						Name: subtest.Name(),
					},
					Image: &runtime.ImageSpec{
						Image: imageWindowsNanoserver,
					},
					Command: []string{
						"cmd",
						"/c",
						"ping -t 127.0.0.1",
					},
					Annotations: map[string]string{
						annotations.WcowDisableGmsa: "true",
					},
					Windows: &runtime.WindowsContainerConfig{
						SecurityContext: &runtime.WindowsContainerSecurityContext{
							CredentialSpec: credSpec,
						},
					},
				},
				SandboxConfig: sandboxRequest.Config,
			}

			cID := createContainer(t, client, ctx, request)
			defer removeContainer(t, client, ctx, cID)

			// should fail because of gMSA creds
			_, err := client.StartContainer(ctx, &runtime.StartContainerRequest{ContainerId: cID})
			// error is serialized over gRPC then embedded into "rpc error: code = %s desc = %s"
			//  so error.Is() wont work
			if !strings.Contains(
				err.Error(),
				fmt.Errorf("gMSA credentials are disabled: %w", hcs.ErrOperationDenied).Error(),
			) {
				if err == nil {
					stopContainer(t, client, ctx, cID)
				}
				t.Fatalf("StartContainer did not fail with gMSA credentials: error is %q", err)
			}
		})
	}
}
