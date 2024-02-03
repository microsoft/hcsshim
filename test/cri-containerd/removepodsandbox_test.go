//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

type TestConfig struct {
	name             string
	containerName    string
	requiredFeatures []string
	runtimeHandler   string
	sandboxImage     string
	containerImage   string
	cmd              []string
}

// Utility function to test removal of a sandbox with no containers and no previous call to stop the pod
func runPodSandboxTestWithoutPodStop(t *testing.T, request *runtime.RunPodSandboxRequest) {
	t.Helper()
	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	podID := runPodSandbox(t, client, ctx, request)
	removePodSandbox(t, client, ctx, podID)

	_, err := client.PodSandboxStatus(ctx, &runtime.PodSandboxStatusRequest{
		PodSandboxId: podID,
	})

	status, ok := status.FromError(err)
	if !ok || status.Code() != codes.NotFound {
		t.Fatalf("PodSandboxStatus did not return expected errorStatus: %s", err)
	}
}

// Utility function to start sandbox with one container and make sure that sandbox is removed in the end
func runPodWithContainer(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, tc *TestConfig) (string, string) {
	t.Helper()
	request := getRunPodSandboxRequest(t, tc.runtimeHandler)
	podID := runPodSandbox(t, client, ctx, request)
	defer removePodSandbox(t, client, ctx, podID)

	containerID := createContainerInSandbox(t, client, ctx, podID, tc.containerName, tc.containerImage, tc.cmd, nil, nil, request.Config)

	startContainer(t, client, ctx, containerID)

	return podID, containerID
}

// Utility function to test removal of a sandbox with one container and no previous call to stop the pod or container
func runContainerInSandboxTest(t *testing.T, tc *TestConfig) {
	t.Helper()
	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	podID, containerID := runPodWithContainer(t, client, ctx, tc)

	_, csErr := client.ContainerStatus(ctx, &runtime.ContainerStatusRequest{
		ContainerId: containerID,
	})

	csStatus, csOk := status.FromError(csErr)
	if !csOk || csStatus.Code() != codes.NotFound {
		t.Fatalf("ContainerStatus did not return expected errorStatus: %s", csErr)
	}

	_, psErr := client.PodSandboxStatus(ctx, &runtime.PodSandboxStatusRequest{
		PodSandboxId: podID,
	})

	psStatus, psOk := status.FromError(psErr)
	if !psOk || psStatus.Code() != codes.NotFound {
		t.Fatalf("PodSandboxStatus did not return expected errorStatus: %s", psErr)
	}
}

func Test_RunPodSandbox_Without_Sandbox_Stop(t *testing.T) {
	requireAnyFeature(t, featureWCOWProcess, featureWCOWHypervisor)

	tests := []TestConfig{
		{
			name:             "WCOW_Process",
			requiredFeatures: []string{featureWCOWProcess},
			runtimeHandler:   wcowProcessRuntimeHandler,
			sandboxImage:     imageWindowsNanoserver,
		},
		{
			name:             "WCOW_Hypervisor",
			requiredFeatures: []string{featureWCOWHypervisor},
			runtimeHandler:   wcowHypervisorRuntimeHandler,
			sandboxImage:     imageWindowsNanoserver,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireFeatures(t, test.requiredFeatures...)
			pullRequiredImages(t, []string{test.sandboxImage})

			request := getRunPodSandboxRequest(t, test.runtimeHandler)
			runPodSandboxTestWithoutPodStop(t, request)
		})
	}
}

func Test_RunContainer_Without_Sandbox_Stop(t *testing.T) {
	requireAnyFeature(t, featureWCOWProcess, featureWCOWHypervisor)

	tests := []TestConfig{
		{
			name:             "WCOW_Process",
			containerName:    t.Name() + "-Container-WCOW_Process",
			requiredFeatures: []string{featureWCOWProcess},
			runtimeHandler:   wcowProcessRuntimeHandler,
			sandboxImage:     imageWindowsNanoserver,
			containerImage:   imageWindowsNanoserver,
			cmd:              []string{"ping", "-t", "127.0.0.1"},
		},
		{
			name:             "WCOW_Hypervisor",
			containerName:    t.Name() + "-Container-WCOW_Hypervisor",
			requiredFeatures: []string{featureWCOWHypervisor},
			runtimeHandler:   wcowHypervisorRuntimeHandler,
			sandboxImage:     imageWindowsNanoserver,
			containerImage:   imageWindowsNanoserver,
			cmd:              []string{"ping", "-t", "127.0.0.1"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireFeatures(t, test.requiredFeatures...)

			pullRequiredImages(t, []string{test.sandboxImage, test.containerImage})
			runContainerInSandboxTest(t, &test)
		})
	}
}
