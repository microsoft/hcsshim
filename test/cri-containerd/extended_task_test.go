//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"fmt"
	"testing"

	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/Microsoft/hcsshim/test/pkg/definitions/extendedtask"
	"github.com/Microsoft/hcsshim/test/pkg/definitions/shimdiag"
)

func getPodProcessorInfo(ctx context.Context, podID string) (*extendedtask.ComputeProcessorInfoResponse, error) {
	shimName := fmt.Sprintf("k8s.io-%s", podID)
	shim, err := shimdiag.GetShim(shimName)
	if err != nil {
		return nil, err
	}
	svc := extendedtask.NewExtendedTaskClient(shim)
	return svc.ComputeProcessorInfo(ctx, &extendedtask.ComputeProcessorInfoRequest{ID: podID})
}

func Test_ExtendedTask_ProcessorInfo(t *testing.T) {
	requireAnyFeature(t, featureWCOWProcess, featureWCOWHypervisor)

	type config struct {
		name             string
		requiredFeatures []string
		runtimeHandler   string
		sandboxImage     string
		expectedError    bool
	}
	tests := []config{
		{
			name:             "WCOW_Process returns error",
			requiredFeatures: []string{featureWCOWProcess},
			runtimeHandler:   wcowProcessRuntimeHandler,
			sandboxImage:     imageWindowsNanoserver,
			expectedError:    true,
		},
		{
			name:             "WCOW_Hypervisor return correct cpu count",
			requiredFeatures: []string{featureWCOWHypervisor},
			runtimeHandler:   wcowHypervisorRuntimeHandler,
			sandboxImage:     imageWindowsNanoserver,
			expectedError:    false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireFeatures(t, test.requiredFeatures...)
			pullRequiredImages(t, []string{test.sandboxImage})

			request := getRunPodSandboxRequest(
				t,
				test.runtimeHandler,
				WithSandboxAnnotations(map[string]string{
					annotations.ProcessorCount: "2",
				}),
			)

			client := newTestRuntimeClient(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			podID := runPodSandbox(t, client, ctx, request)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			resp, err := getPodProcessorInfo(ctx, podID)
			if test.expectedError {
				if err == nil {
					t.Fatalf("expected to get an error, instead got %v response", resp)
				}
			} else {
				if err != nil {
					t.Fatalf("failed to get pod processor info with %v", err)
				}
				if resp == nil {
					t.Fatalf("expected non-nil processor info response")
				}
				if resp.Count != 2 {
					t.Fatalf("expected to see 2 cpus on the pod, instead got %v", resp.Count)
				}
			}
		})
	}
}
