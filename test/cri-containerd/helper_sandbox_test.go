//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"testing"

	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// isLCOWV2 reports whether the LCOWV2 feature flag is set on the current test
// invocation. Callers should prefer the higher-level helpers below
// (lcowRuntimeHandlerForTest, requireV1Only) so the V2 selection logic stays
// in one place.
func isLCOWV2() bool {
	return flagFeatures.IsSet(featureLCOWV2)
}

// lcowRuntimeHandlerForTest returns the LCOW runtime handler that the current
// test should target. When the LCOWV2 feature flag is set, it returns the V2
// shim handler (containerd-shim-lcow-v2.exe via runtime_type
// io.containerd.lcow.v2). Otherwise it returns the V1 handler
// (containerd-shim-runhcs-v1.exe via runtime_type io.containerd.runhcs.v1).
//
// Tests that exercise generic LCOW lifecycle and work on both shims should use
// this helper instead of hard-coding lcowRuntimeHandler, so the same suite can
// be run twice in CI: once for V1 (default) and once with -feature LCOWV2 for V2.
// Mirrors the pattern in the azcri repo.
func lcowRuntimeHandlerForTest(tb testing.TB) string {
	tb.Helper()
	if isLCOWV2() {
		return lcowV2RuntimeHandler
	}
	return lcowRuntimeHandler
}

// requireV1Only skips the test when the LCOWV2 feature flag is set.
// Use this for tests that depend on V1-only features such as VPMEM,
// VHD/initrd boot modes, or other UVM knobs not exposed in the v2 builder.
func requireV1Only(tb testing.TB) {
	tb.Helper()
	if isLCOWV2() {
		tb.Skip("test requires V1 shim features (VPMEM/VHD/initrd) not exposed in V2")
	}
}

type SandboxConfigOpt func(*runtime.PodSandboxConfig) error

func WithSandboxAnnotations(annotations map[string]string) SandboxConfigOpt {
	return func(config *runtime.PodSandboxConfig) error {
		if config.Annotations == nil {
			config.Annotations = make(map[string]string)
		}
		for k, v := range annotations {
			config.Annotations[k] = v
		}
		return nil
	}
}

func WithSandboxLabels(labels map[string]string) SandboxConfigOpt {
	return func(config *runtime.PodSandboxConfig) error {
		if config.Labels == nil {
			config.Labels = make(map[string]string)
		}

		for k, v := range labels {
			config.Labels[k] = v
		}
		return nil
	}
}

func runPodSandbox(tb testing.TB, client runtime.RuntimeServiceClient, ctx context.Context, request *runtime.RunPodSandboxRequest) string {
	tb.Helper()
	response, err := client.RunPodSandbox(ctx, request)
	if err != nil {
		tb.Fatalf("failed RunPodSandbox request with: %v", err)
	}
	return response.PodSandboxId
}

func stopPodSandbox(tb testing.TB, client runtime.RuntimeServiceClient, ctx context.Context, podID string) {
	tb.Helper()
	_, err := client.StopPodSandbox(ctx, &runtime.StopPodSandboxRequest{
		PodSandboxId: podID,
	})
	if err != nil {
		tb.Fatalf("failed StopPodSandbox for sandbox: %s, request with: %v", podID, err)
	}
}

func removePodSandbox(tb testing.TB, client runtime.RuntimeServiceClient, ctx context.Context, podID string) {
	tb.Helper()
	_, err := client.RemovePodSandbox(ctx, &runtime.RemovePodSandboxRequest{
		PodSandboxId: podID,
	})
	if err != nil {
		tb.Fatalf("failed RemovePodSandbox for sandbox: %s, request with: %v", podID, err)
	}
}

func getTestSandboxConfig(tb testing.TB, opts ...SandboxConfigOpt) *runtime.PodSandboxConfig {
	tb.Helper()
	c := &runtime.PodSandboxConfig{
		Metadata: &runtime.PodSandboxMetadata{
			Name:      tb.Name(),
			Namespace: testNamespace,
		},
	}

	if *flagVirtstack != "" {
		vmServicePath := testVMServiceBinary
		if *flagVMServiceBinary != "" {
			vmServicePath = *flagVMServiceBinary
		}
		opts = append(opts, WithSandboxAnnotations(map[string]string{
			"io.microsoft.virtualmachine.vmsource":          *flagVirtstack,
			"io.microsoft.virtualmachine.vmservice.address": testVMServiceAddress,
			"io.microsoft.virtualmachine.vmservice.path":    vmServicePath,
		}))
	}

	for _, o := range opts {
		if err := o(c); err != nil {
			tb.Helper()
			tb.Fatalf("failed to apply PodSandboxConfig option: %s", err)
		}
	}
	return c
}

func getRunPodSandboxRequest(tb testing.TB, runtimeHandler string, sandboxOpts ...SandboxConfigOpt) *runtime.RunPodSandboxRequest {
	tb.Helper()
	return &runtime.RunPodSandboxRequest{
		Config:         getTestSandboxConfig(tb, sandboxOpts...),
		RuntimeHandler: runtimeHandler,
	}
}

func cleanupPod(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, podID *string) {
	t.Helper()
	if *podID == "" {
		// Do nothing for empty podID
		return
	}
	stopPodSandbox(t, client, ctx, *podID)
	removePodSandbox(t, client, ctx, *podID)
}
