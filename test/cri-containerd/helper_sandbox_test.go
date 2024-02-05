//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"testing"

	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

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
