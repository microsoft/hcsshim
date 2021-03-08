// +build functional

package cri_containerd

import (
	"context"
	"strings"
	"testing"
	"time"

	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func getJobContainerPodRequestWCOW(t *testing.T) *runtime.RunPodSandboxRequest {
	return &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"microsoft.com/hostprocess-container": "true",
			},
		},
		RuntimeHandler: wcowProcessRuntimeHandler,
	}
}

func getJobContainerRequestWCOW(t *testing.T, podID string, podConfig *runtime.PodSandboxConfig) *runtime.CreateContainerRequest {
	return &runtime.CreateContainerRequest{
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

			Annotations: map[string]string{
				"microsoft.com/hostprocess":              "true",
				"microsoft.com/hostprocess-inherit-user": "true",
			},
		},
		PodSandboxId:  podID,
		SandboxConfig: podConfig,
	}
}

func Test_RunContainer_InheritUser_JobContainer_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWProcess, featureHostProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})
	client := newTestRuntimeClient(t)

	username := "nt authority\\system"
	podctx := context.Background()
	sandboxRequest := getJobContainerPodRequestWCOW(t)

	podID := runPodSandbox(t, client, podctx, sandboxRequest)
	defer removePodSandbox(t, client, podctx, podID)
	defer stopPodSandbox(t, client, podctx, podID)

	containerRequest := getJobContainerRequestWCOW(t, podID, sandboxRequest.Config)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	containerID := createContainer(t, client, ctx, containerRequest)
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	execResponse := execSync(t, client, ctx, &runtime.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         []string{"whoami"},
	})
	stdout := strings.Trim(string(execResponse.Stdout), " \r\n")
	if !strings.Contains(stdout, username) {
		t.Fatalf("expected user: '%s', got '%s'", username, stdout)
	}
}
