//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"testing"

	cri "github.com/kevpar/cri/pkg/api/v1"
)

func newTestPluginClient(tb testing.TB) cri.CRIPluginServiceClient {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	conn, err := createGRPCConn(ctx)
	if err != nil {
		tb.Fatalf("failed to dial runtime client: %v", err)
	}
	return cri.NewCRIPluginServiceClient(conn)
}

func resetContainer(tb testing.TB, client cri.CRIPluginServiceClient, ctx context.Context, containerID string) {
	tb.Helper()
	_, err := client.ResetContainer(ctx, &cri.ResetContainerRequest{
		ContainerId: containerID,
	})
	if err != nil {
		tb.Fatalf("failed ResetContainer request for container: %s, with: %v", containerID, err)
	}
}

func resetPodSandbox(tb testing.TB, client cri.CRIPluginServiceClient, ctx context.Context, podID string) {
	tb.Helper()
	_, err := client.ResetPodSandbox(ctx, &cri.ResetPodSandboxRequest{
		PodSandboxId: podID,
	})
	if err != nil {
		tb.Fatalf("failed ResetPodSandbox request for container: %s, with: %v", podID, err)
	}
}
