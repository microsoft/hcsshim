//go:build functional
// +build functional

package cri_containerd

import (
	"context"
	"testing"
	"time"

	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func createContainer(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, request *runtime.CreateContainerRequest) string {
	response, err := client.CreateContainer(ctx, request)
	if err != nil {
		t.Fatalf("failed CreateContainer in sandbox: %s, with: %v", request.PodSandboxId, err)
	}
	return response.ContainerId
}

func startContainer(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, containerID string) {
	_, err := client.StartContainer(ctx, &runtime.StartContainerRequest{
		ContainerId: containerID,
	})
	if err != nil {
		t.Fatalf("failed StartContainer request for container: %s, with: %v", containerID, err)
	}
}

func stopContainer(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, containerID string) {
	stopContainerWithTimeout(t, client, ctx, containerID, 0)
}

func stopContainerWithTimeout(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, containerID string, timeout int64) {
	_, err := client.StopContainer(ctx, &runtime.StopContainerRequest{
		ContainerId: containerID,
		Timeout:     timeout,
	})
	if err != nil {
		t.Fatalf("failed StopContainer request for container: %s, with: %v", containerID, err)
	}
}

func removeContainer(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, containerID string) {
	_, err := client.RemoveContainer(ctx, &runtime.RemoveContainerRequest{
		ContainerId: containerID,
	})
	if err != nil {
		t.Fatalf("failed RemoveContainer request for container: %s, with: %v", containerID, err)
	}
}

func removeContainerWithRetry(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, containerID string, retry uint, sleep time.Duration) {
	for i := 1; i <= int(retry); i++ {
		_, err := client.RemoveContainer(ctx, &runtime.RemoveContainerRequest{
			ContainerId: containerID,
		})
		if err != nil {
			if i == int(retry) {
				t.Fatalf("failed RemoveContainer request for container: %s, with: %v", containerID, err)
			} else {
				t.Logf("failed RemoveContainer request %d of %d for container %q: %v", i, retry, containerID, err)
				t.Logf("sleeping for %v", sleep)
				time.Sleep(sleep)
			}
		} else {
			break
		}
	}
}

func getCreateContainerRequest(podID string, name string, image string, command []string, podConfig *runtime.PodSandboxConfig) *runtime.CreateContainerRequest {
	return &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: name,
			},
			Image: &runtime.ImageSpec{
				Image: image,
			},
			Command: command,
		},
		PodSandboxId:  podID,
		SandboxConfig: podConfig,
	}
}

func getContainerStatus(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, containerID string) runtime.ContainerState {
	return getContainerStatusFull(t, client, ctx, containerID).State
}

func getContainerStatusFull(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, containerID string) *runtime.ContainerStatus {
	response, err := client.ContainerStatus(ctx, &runtime.ContainerStatusRequest{
		ContainerId: containerID,
	})
	if err != nil {
		t.Fatalf("failed ContainerStatus request for container: %s, with: %v", containerID, err)
	}
	return response.Status
}
