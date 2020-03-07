// +build functional

package cri_containerd

import (
	"context"
	"testing"

	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func runPodSandbox(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, request *runtime.RunPodSandboxRequest) string {
	response, err := client.RunPodSandbox(ctx, request)
	if err != nil {
		t.Fatalf("failed RunPodSandbox request with: %v", err)
	}
	return response.PodSandboxId
}

func stopPodSandbox(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, podID string) {
	_, err := client.StopPodSandbox(ctx, &runtime.StopPodSandboxRequest{
		PodSandboxId: podID,
	})
	if err != nil {
		t.Fatalf("failed StopPodSandbox for sandbox: %s, request with: %v", podID, err)
	}
}

func removePodSandbox(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, podID string) {
	_, err := client.RemovePodSandbox(ctx, &runtime.RemovePodSandboxRequest{
		PodSandboxId: podID,
	})
	if err != nil {
		t.Fatalf("failed RemovePodSandbox for sandbox: %s, request with: %v", podID, err)
	}
}
