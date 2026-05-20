//go:build windows && functional
// +build windows,functional

// V2 LCOW-specific CRI tests. These mirror the Test_V2Sandbox_* pattern
// established in the azcri test suite: each test gates on featureLCOWV2 and
// targets lcowV2RuntimeHandler directly (no v1 fallback), because the
// scenarios under test exercise the V2 runtime path.
//
// To run these tests:
//  1. The CI containerd config must register `runhcs-lcow-v2` →
//     `containerd-shim-lcow-v2.exe` (runtime_type io.containerd.lcow.v2).
//  2. The test binary must be invoked with -feature=LCOWV2.

package cri_containerd

import (
	"context"
	"testing"

	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// Test_V2_LCOW_PodLifecycle exercises the basic pod-sandbox lifecycle through
// the V2 shim: RunPodSandbox → StopPodSandbox → RemovePodSandbox. This is the
// minimum end-to-end smoke test that proves containerd → shim handshake works
// on the V2 path.
func Test_V2_LCOW_PodLifecycle(t *testing.T) {
	requireFeatures(t, featureLCOWV2)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	podReq := getRunPodSandboxRequest(t, lcowV2RuntimeHandler)
	podID := runPodSandbox(t, client, ctx, podReq)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)
}

// Test_V2_LCOW_ContainerLifecycle exercises a full container lifecycle inside
// a V2 sandbox: RunPodSandbox → CreateContainer → StartContainer →
// StopContainer → RemoveContainer → StopPodSandbox → RemovePodSandbox.
func Test_V2_LCOW_ContainerLifecycle(t *testing.T) {
	requireFeatures(t, featureLCOWV2)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	podReq := getRunPodSandboxRequest(t, lcowV2RuntimeHandler)
	podID := runPodSandbox(t, client, ctx, podReq)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	cReq := getCreateContainerRequest(podID, "alpine", imageLcowAlpine,
		[]string{"echo", "hello"}, podReq.Config)
	containerID := createContainer(t, client, ctx, cReq)
	defer removeContainer(t, client, ctx, containerID)

	startContainer(t, client, ctx, containerID)
	stopContainer(t, client, ctx, containerID)
}

// Test_V2_LCOW_ContainerExec runs a workload container and verifies that
// ExecSync into it succeeds with the expected exit code. Validates the GCS
// exec path through the V2 controller.
func Test_V2_LCOW_ContainerExec(t *testing.T) {
	requireFeatures(t, featureLCOWV2)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	podReq := getRunPodSandboxRequest(t, lcowV2RuntimeHandler)
	podID := runPodSandbox(t, client, ctx, podReq)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	cReq := getCreateContainerRequest(podID, "alpine", imageLcowAlpine,
		[]string{"top"}, podReq.Config)
	containerID := createContainer(t, client, ctx, cReq)
	defer removeContainer(t, client, ctx, containerID)

	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	execResp, err := client.ExecSync(ctx, &runtime.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         []string{"echo", "hello"},
		Timeout:     20,
	})
	if err != nil {
		t.Fatalf("ExecSync failed: %v", err)
	}
	if execResp.ExitCode != 0 {
		t.Fatalf("ExecSync returned exit code %d, stderr: %s", execResp.ExitCode, string(execResp.Stderr))
	}
}
