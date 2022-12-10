//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	criserver "github.com/containerd/containerd/pkg/cri/server"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/require"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func createContainer(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, request *runtime.CreateContainerRequest) string {
	t.Helper()
	response, err := client.CreateContainer(ctx, request)
	if err != nil {
		t.Fatalf("failed CreateContainer in sandbox: %s, with: %v", request.PodSandboxId, err)
	}
	return response.ContainerId
}

func startContainer(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, containerID string) {
	t.Helper()
	_, err := client.StartContainer(ctx, &runtime.StartContainerRequest{
		ContainerId: containerID,
	})
	if err != nil {
		t.Fatalf("failed StartContainer request for container: %s, with: %v", containerID, err)
	}
}

func stopContainer(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, containerID string) {
	t.Helper()
	stopContainerWithTimeout(t, client, ctx, containerID, 0)
}

func stopContainerWithTimeout(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, containerID string, timeout int64) {
	t.Helper()
	_, err := client.StopContainer(ctx, &runtime.StopContainerRequest{
		ContainerId: containerID,
		Timeout:     timeout,
	})
	if err != nil {
		t.Fatalf("failed StopContainer request for container: %s, with: %v", containerID, err)
	}
}

func removeContainer(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, containerID string) {
	t.Helper()
	_, err := client.RemoveContainer(ctx, &runtime.RemoveContainerRequest{
		ContainerId: containerID,
	})
	if err != nil {
		t.Fatalf("failed RemoveContainer request for container: %s, with: %v", containerID, err)
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
	t.Helper()
	return getContainerStatusFull(t, client, ctx, containerID).State
}

func assertContainerState(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, containerID string, state runtime.ContainerState) {
	t.Helper()
	if st := getContainerStatus(t, client, ctx, containerID); st != state {
		t.Fatalf("got container %q state %q; wanted %v", containerID, st.String(), state.String())
	}
}

func getContainerOCISpec(tb testing.TB, client runtime.RuntimeServiceClient, ctx context.Context, containerID string) *oci.Spec {
	tb.Helper()
	status, err := client.ContainerStatus(ctx, &runtime.ContainerStatusRequest{
		ContainerId: containerID,
		Verbose:     true,
	})
	if err != nil {
		tb.Fatalf("failed ContainerStatus for container %q: %v", containerID, err)
	}

	i := status.Info["info"]
	var info criserver.ContainerInfo
	if err := json.Unmarshal([]byte(i), &info); err != nil {
		tb.Fatalf("could not unmarshal container info %q: %v", i, err)
	}

	spec := info.RuntimeSpec
	if spec == nil {
		tb.Fatalf("container %q returned a nil spec", containerID)
	}
	return spec
}

func getContainerStatusFull(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, containerID string) *runtime.ContainerStatus {
	t.Helper()
	response, err := client.ContainerStatus(ctx, &runtime.ContainerStatusRequest{
		ContainerId: containerID,
	})
	if err != nil {
		t.Fatalf("failed ContainerStatus request for container: %s, with: %v", containerID, err)
	}
	return response.Status
}

// requireContainerState periodically checks the state of a container, returns
// an error if the expected container state isn't reached within 30 seconds.
func requireContainerState(
	ctx context.Context,
	t *testing.T,
	client runtime.RuntimeServiceClient,
	containerID string,
	expectedState runtime.ContainerState,
) {
	t.Helper()
	require.NoError(t, func() error {
		start := time.Now()
		var lastState runtime.ContainerState
		for {
			lastState = getContainerStatus(t, client, ctx, containerID)
			if lastState == expectedState {
				return nil
			}
			if time.Since(start) >= 30*time.Second {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		return fmt.Errorf(
			"expected state %q, last reported state %q",
			runtime.ContainerState_name[int32(expectedState)],
			runtime.ContainerState_name[int32(lastState)],
		)
	}())
}
