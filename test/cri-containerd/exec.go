// +build functional

package cri_containerd

import (
	"context"
	"testing"

	runtime "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

func execSync(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, request *runtime.ExecSyncRequest) *runtime.ExecSyncResponse {
	response, err := client.ExecSync(ctx, request)
	if err != nil {
		t.Fatalf("failed ExecSync request with: %v", err)
	}
	return response
}

func execRequest(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, request *runtime.ExecRequest) string {
	response, err := client.Exec(ctx, request)
	if err != nil {
		t.Fatalf("failed Exec request with: %v", err)
	}
	return response.Url
}
