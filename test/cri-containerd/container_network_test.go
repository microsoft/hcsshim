//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"strings"
	"testing"

	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func Test_Container_Network_Hostname(t *testing.T) {
	requireAnyFeature(t, featureWCOWProcess, featureWCOWHypervisor)

	type config struct {
		name             string
		requiredFeatures []string
		runtimeHandler   string
		sandboxImage     string
		containerImage   string
		cmd              []string
	}
	tests := []config{
		{
			name:             "WCOW_Process",
			requiredFeatures: []string{featureWCOWProcess},
			runtimeHandler:   wcowProcessRuntimeHandler,
			sandboxImage:     imageWindowsNanoserver,
			containerImage:   imageWindowsNanoserver,
			cmd:              []string{"cmd", "/c", "ping", "-t", "127.0.0.1"},
		},
		{
			name:             "WCOW_Hypervisor",
			requiredFeatures: []string{featureWCOWHypervisor},
			runtimeHandler:   wcowHypervisorRuntimeHandler,
			sandboxImage:     imageWindowsNanoserver,
			containerImage:   imageWindowsNanoserver,
			cmd:              []string{"cmd", "/c", "ping", "-t", "127.0.0.1"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireFeatures(t, test.requiredFeatures...)

			pullRequiredImages(t, []string{test.sandboxImage, test.containerImage})

			sandboxRequest := getRunPodSandboxRequest(t, test.runtimeHandler)
			sandboxRequest.Config.Hostname = "TestHost"

			client := newTestRuntimeClient(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			podID := runPodSandbox(t, client, ctx, sandboxRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			containerRequest := &runtime.CreateContainerRequest{
				PodSandboxId: podID,
				Config: &runtime.ContainerConfig{
					Metadata: &runtime.ContainerMetadata{
						Name: t.Name() + "-Container",
					},
					Image: &runtime.ImageSpec{
						Image: test.containerImage,
					},
					Command: test.cmd,
				},
				SandboxConfig: sandboxRequest.Config,
			}

			containerID := createContainer(t, client, ctx, containerRequest)
			defer removeContainer(t, client, ctx, containerID)

			startContainer(t, client, ctx, containerID)
			defer stopContainer(t, client, ctx, containerID)

			execResponse := execSync(t, client, ctx, &runtime.ExecSyncRequest{
				ContainerId: containerID,
				Cmd:         []string{"hostname"},
			})
			stdout := strings.Trim(string(execResponse.Stdout), " \r\n")
			if stdout != sandboxRequest.Config.Hostname {
				t.Fatalf("expected hostname: '%s', got '%s'", sandboxRequest.Config.Hostname, stdout)
			}
		})
	}
}
