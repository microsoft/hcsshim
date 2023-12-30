//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"fmt"
	"testing"

	"github.com/Microsoft/hcsshim/internal/memory"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/Microsoft/hcsshim/test/pkg/require"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

const processorWeightMax = 10000

func calculateJobCPUWeight(processorWeight uint32) uint32 {
	if processorWeight == 0 {
		return 0
	}
	return 1 + uint32((8*processorWeight)/processorWeightMax)
}

//nolint:unused // may be used in future tests
func calculateJobCPURate(hostProcs uint32, processorCount uint32) uint32 {
	rate := (processorCount * 10000) / hostProcs
	if rate == 0 {
		return 1
	}
	return rate
}

func Test_Container_UpdateResources_CPUShare(t *testing.T) {
	requireAnyFeature(t, featureWCOWProcess, featureWCOWHypervisor)
	require.Build(t, osversion.V20H2)

	type config struct {
		name             string
		requiredFeatures []string
		runtimeHandler   string
		sandboxImage     string
		containerImage   string
		cmd              []string
	}
	tests := []config{
		{
			name:             "WCOW_Process",
			requiredFeatures: []string{featureWCOWProcess, featureCRIUpdateContainer},
			runtimeHandler:   wcowProcessRuntimeHandler,
			sandboxImage:     imageWindowsNanoserver,
			containerImage:   imageWindowsNanoserver,
			cmd:              []string{"cmd", "/c", "ping", "-t", "127.0.0.1"},
		},
		{
			name:             "WCOW_Hypervisor",
			requiredFeatures: []string{featureWCOWHypervisor, featureCRIUpdateContainer},
			runtimeHandler:   wcowHypervisorRuntimeHandler,
			sandboxImage:     imageWindowsNanoserver,
			containerImage:   imageWindowsNanoserver,
			cmd:              []string{"cmd", "/c", "ping", "-t", "127.0.0.1"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireFeatures(t, test.requiredFeatures...)

			if test.runtimeHandler == wcowHypervisorRuntimeHandler {
				pullRequiredImages(t, []string{test.sandboxImage})
			}

			podRequest := getRunPodSandboxRequest(t, test.runtimeHandler)

			client := newTestRuntimeClient(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			podID := runPodSandbox(t, client, ctx, podRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			containerRequest := &runtime.CreateContainerRequest{
				Config: &runtime.ContainerConfig{
					Metadata: &runtime.ContainerMetadata{
						Name: t.Name() + "-Container",
					},
					Image: &runtime.ImageSpec{
						Image: test.containerImage,
					},
					Command: test.cmd,
				},
				PodSandboxId:  podID,
				SandboxConfig: podRequest.Config,
			}

			containerID := createContainer(t, client, ctx, containerRequest)
			defer removeContainer(t, client, ctx, containerID)

			startContainer(t, client, ctx, containerID)
			defer stopContainer(t, client, ctx, containerID)

			// make request to increase cpu shares
			updateReq := &runtime.UpdateContainerResourcesRequest{
				ContainerId: containerID,
			}

			var expected uint32 = 5000
			updateReq.Windows = &runtime.WindowsContainerResources{
				CpuShares: int64(expected),
			}

			if _, err := client.UpdateContainerResources(ctx, updateReq); err != nil {
				t.Fatalf("updating container resources for %s with %v", containerID, err)
			}

			targetShimName := "k8s.io-" + podID
			jobExpectedValue := calculateJobCPUWeight(expected)
			checkWCOWResourceLimit(t, ctx, test.runtimeHandler, targetShimName, containerID, "cpu-weight", uint64(jobExpectedValue))
		})
	}
}

func Test_Container_UpdateResources_CPUShare_NotRunning(t *testing.T) {
	requireFeatures(t, featureCRIUpdateContainer)
	requireAnyFeature(t, featureWCOWProcess, featureWCOWHypervisor)

	type config struct {
		name             string
		requiredFeatures []string
		runtimeHandler   string
		sandboxImage     string
		containerImage   string
		cmd              []string
	}
	tests := []config{
		{
			name:             "WCOW_Process",
			requiredFeatures: []string{featureWCOWProcess},
			runtimeHandler:   wcowProcessRuntimeHandler,
			sandboxImage:     imageWindowsNanoserver,
			containerImage:   imageWindowsNanoserver,
			cmd:              []string{"cmd", "/c", "ping", "-t", "127.0.0.1"},
		},
		{
			name:             "WCOW_Hypervisor",
			requiredFeatures: []string{featureWCOWHypervisor},
			runtimeHandler:   wcowHypervisorRuntimeHandler,
			sandboxImage:     imageWindowsNanoserver,
			containerImage:   imageWindowsNanoserver,
			cmd:              []string{"cmd", "/c", "ping", "-t", "127.0.0.1"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireFeatures(t, test.requiredFeatures...)
			if test.runtimeHandler == wcowHypervisorRuntimeHandler {
				pullRequiredImages(t, []string{test.sandboxImage})
			}

			podRequest := getRunPodSandboxRequest(t, test.runtimeHandler)

			client := newTestRuntimeClient(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			podID := runPodSandbox(t, client, ctx, podRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			containerRequest := &runtime.CreateContainerRequest{
				Config: &runtime.ContainerConfig{
					Metadata: &runtime.ContainerMetadata{
						Name: t.Name() + "-Container",
					},
					Image: &runtime.ImageSpec{
						Image: test.containerImage,
					},
					Command: test.cmd,
				},
				PodSandboxId:  podID,
				SandboxConfig: podRequest.Config,
			}

			containerID := createContainer(t, client, ctx, containerRequest)
			defer removeContainer(t, client, ctx, containerID)

			// make request to increase cpu shares == cpu weight
			updateReq := &runtime.UpdateContainerResourcesRequest{
				ContainerId: containerID,
			}

			var expected uint32 = 5000
			updateReq.Windows = &runtime.WindowsContainerResources{
				CpuShares: int64(expected),
			}

			if _, err := client.UpdateContainerResources(ctx, updateReq); err != nil {
				t.Fatalf("updating container resources for %s with %v", containerID, err)
			}

			startContainer(t, client, ctx, containerID)
			defer stopContainer(t, client, ctx, containerID)

			targetShimName := "k8s.io-" + podID
			jobExpectedValue := calculateJobCPUWeight(expected)
			checkWCOWResourceLimit(t, ctx, test.runtimeHandler, targetShimName, containerID, "cpu-weight", uint64(jobExpectedValue))
		})
	}
}

func Test_Container_UpdateResources_Memory(t *testing.T) {
	requireFeatures(t, featureCRIUpdateContainer)
	requireAnyFeature(t, featureWCOWProcess, featureWCOWHypervisor)

	type config struct {
		name             string
		requiredFeatures []string
		runtimeHandler   string
		sandboxImage     string
		containerImage   string
		cmd              []string
	}
	tests := []config{
		{
			name:             "WCOW_Process",
			requiredFeatures: []string{featureWCOWProcess},
			runtimeHandler:   wcowProcessRuntimeHandler,
			sandboxImage:     imageWindowsNanoserver,
			containerImage:   imageWindowsNanoserver,
			cmd:              []string{"cmd", "/c", "ping", "-t", "127.0.0.1"},
		},
		{
			name:             "WCOW_Hypervisor",
			requiredFeatures: []string{featureWCOWHypervisor},
			runtimeHandler:   wcowHypervisorRuntimeHandler,
			sandboxImage:     imageWindowsNanoserver,
			containerImage:   imageWindowsNanoserver,
			cmd:              []string{"cmd", "/c", "ping", "-t", "127.0.0.1"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireFeatures(t, test.requiredFeatures...)
			if test.runtimeHandler == wcowHypervisorRuntimeHandler {
				pullRequiredImages(t, []string{test.sandboxImage})
			}

			podRequest := getRunPodSandboxRequest(t, test.runtimeHandler)

			client := newTestRuntimeClient(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			podID := runPodSandbox(t, client, ctx, podRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			var startingMemorySize int64 = 768 * memory.MiB
			containerRequest := &runtime.CreateContainerRequest{
				Config: &runtime.ContainerConfig{
					Metadata: &runtime.ContainerMetadata{
						Name: t.Name() + "-Container",
					},
					Image: &runtime.ImageSpec{
						Image: test.containerImage,
					},
					Command: test.cmd,
					Annotations: map[string]string{
						annotations.ContainerMemorySizeInMB: fmt.Sprintf("%d", startingMemorySize), // 768MB
					},
				},
				PodSandboxId:  podID,
				SandboxConfig: podRequest.Config,
			}

			containerID := createContainer(t, client, ctx, containerRequest)
			defer removeContainer(t, client, ctx, containerID)

			startContainer(t, client, ctx, containerID)
			defer stopContainer(t, client, ctx, containerID)

			// make request for cpu shares
			updateReq := &runtime.UpdateContainerResourcesRequest{
				ContainerId: containerID,
			}

			newMemorySize := startingMemorySize / 2
			updateReq.Windows = &runtime.WindowsContainerResources{
				MemoryLimitInBytes: newMemorySize,
			}

			if _, err := client.UpdateContainerResources(ctx, updateReq); err != nil {
				t.Fatalf("updating container resources for %s with %v", containerID, err)
			}
			targetShimName := "k8s.io-" + podID
			checkWCOWResourceLimit(t, ctx, test.runtimeHandler, targetShimName, containerID, "memory-limit", uint64(newMemorySize))
		})
	}
}
