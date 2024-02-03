//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"testing"

	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func createContainer(tb testing.TB, client runtime.RuntimeServiceClient, ctx context.Context, request *runtime.CreateContainerRequest) string {
	tb.Helper()
	response, err := client.CreateContainer(ctx, request)
	if err != nil {
		tb.Fatalf("failed CreateContainer in sandbox: %s, with: %v", request.PodSandboxId, err)
	}
	return response.ContainerId
}

func startContainer(tb testing.TB, client runtime.RuntimeServiceClient, ctx context.Context, containerID string) {
	tb.Helper()
	_, err := client.StartContainer(ctx, &runtime.StartContainerRequest{
		ContainerId: containerID,
	})
	if err != nil {
		tb.Fatalf("failed StartContainer request for container: %s, with: %v", containerID, err)
	}
}

func stopContainer(tb testing.TB, client runtime.RuntimeServiceClient, ctx context.Context, containerID string) {
	tb.Helper()
	stopContainerWithTimeout(tb, client, ctx, containerID, 0)
}

func stopContainerWithTimeout(tb testing.TB, client runtime.RuntimeServiceClient, ctx context.Context, containerID string, timeout int64) {
	tb.Helper()
	_, err := client.StopContainer(ctx, &runtime.StopContainerRequest{
		ContainerId: containerID,
		Timeout:     timeout,
	})
	if err != nil {
		tb.Fatalf("failed StopContainer request for container: %s, with: %v", containerID, err)
	}
}

func removeContainer(tb testing.TB, client runtime.RuntimeServiceClient, ctx context.Context, containerID string) {
	tb.Helper()
	_, err := client.RemoveContainer(ctx, &runtime.RemoveContainerRequest{
		ContainerId: containerID,
	})
	if err != nil {
		tb.Fatalf("failed RemoveContainer request for container: %s, with: %v", containerID, err)
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

func getContainerStatus(tb testing.TB, client runtime.RuntimeServiceClient, ctx context.Context, containerID string) runtime.ContainerState {
	tb.Helper()
	return getContainerStatusFull(tb, client, ctx, containerID).State
}

func assertContainerState(tb testing.TB, client runtime.RuntimeServiceClient, ctx context.Context, containerID string, state runtime.ContainerState) {
	tb.Helper()
	if st := getContainerStatus(tb, client, ctx, containerID); st != state {
		tb.Fatalf("got container %q state %q; wanted %v", containerID, st.String(), state.String())
	}
}

func getContainerStatusFull(tb testing.TB, client runtime.RuntimeServiceClient, ctx context.Context, containerID string) *runtime.ContainerStatus {
	tb.Helper()
	response, err := client.ContainerStatus(ctx, &runtime.ContainerStatusRequest{
		ContainerId: containerID,
	})
	if err != nil {
		tb.Fatalf("failed ContainerStatus request for container: %s, with: %v", containerID, err)
	}
	return response.Status
}

func cleanupContainer(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, containerID *string) {
	t.Helper()
	if *containerID == "" {
		// Do nothing for empty containerID
		return
	}
	stopContainer(t, client, ctx, *containerID)
	removeContainer(t, client, ctx, *containerID)
}
