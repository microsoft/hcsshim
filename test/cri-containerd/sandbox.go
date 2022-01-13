//go:build functional
// +build functional

package cri_containerd

import (
	"context"
	"testing"

	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
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

func getTestSandboxConfig(t *testing.T, opts ...SandboxConfigOpt) *runtime.PodSandboxConfig {
	c := &runtime.PodSandboxConfig{
		Metadata: &runtime.PodSandboxMetadata{
			Name:      t.Name(),
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
			t.Fatalf("failed to apply PodSandboxConfig option: %s", err)
		}
	}
	return c
}

func getRunPodSandboxRequest(t *testing.T, runtimeHandler string, sandboxOpts ...SandboxConfigOpt) *runtime.RunPodSandboxRequest {
	return &runtime.RunPodSandboxRequest{
		Config:         getTestSandboxConfig(t, sandboxOpts...),
		RuntimeHandler: runtimeHandler,
	}
}
