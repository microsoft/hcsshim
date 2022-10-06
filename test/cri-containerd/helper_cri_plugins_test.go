//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"testing"

	cri "github.com/kevpar/cri/pkg/api/v1"
)

func newTestPluginClient(t *testing.T) cri.CRIPluginServiceClient {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	conn, err := createGRPCConn(ctx)
	if err != nil {
		t.Fatalf("failed to dial runtime client: %v", err)
	}
	return cri.NewCRIPluginServiceClient(conn)
}

func resetContainer(t *testing.T, client cri.CRIPluginServiceClient, ctx context.Context, containerID string) {
	t.Helper()
	_, err := client.ResetContainer(ctx, &cri.ResetContainerRequest{
		ContainerId: containerID,
	})
	if err != nil {
		t.Fatalf("failed ResetContainer request for container: %s, with: %v", containerID, err)
	}
}

func resetPodSandbox(t *testing.T, client cri.CRIPluginServiceClient, ctx context.Context, podID string) {
	t.Helper()
	_, err := client.ResetPodSandbox(ctx, &cri.ResetPodSandboxRequest{
		PodSandboxId: podID,
	})
	if err != nil {
		t.Fatalf("failed ResetPodSandbox request for container: %s, with: %v", podID, err)
	}
}
