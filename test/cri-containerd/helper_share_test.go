//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"fmt"
	"testing"

	"github.com/Microsoft/hcsshim/test/pkg/definitions/shimdiag"
)

func shareInPod(ctx context.Context, tb testing.TB, podID string, hostPath, uvmPath string, readOnly bool) {
	tb.Helper()
	shimName := fmt.Sprintf("k8s.io-%s", podID)
	shim, err := shimdiag.GetShim(shimName)
	if err != nil {
		tb.Fatalf("could not get shim for pod %q: %v", podID, err)
	}
	defer shim.Close()

	tb.Logf("sharing %q in shim %q as %q", hostPath, shimName, uvmPath)
	if err := shareInUVM(ctx, shimdiag.NewShimDiagClient(shim), hostPath, uvmPath, readOnly); err != nil {
		tb.Fatalf("could not share %q in pod %q: %v", hostPath, podID, err)
	}
}

func shareInUVM(ctx context.Context, client shimdiag.ShimDiagService, hostPath, uvmPath string, readOnly bool) error {
	req := &shimdiag.ShareRequest{
		HostPath: hostPath,
		UvmPath:  uvmPath,
		ReadOnly: readOnly,
	}
	_, err := client.DiagShare(ctx, req)
	if err != nil {
		return err
	}
	return nil
}
