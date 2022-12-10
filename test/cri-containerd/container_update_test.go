//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/Microsoft/hcsshim/internal/memory"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/Microsoft/hcsshim/test/internal/require"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

const processorWeightMax = 10_000

func calculateJobCPUWeight(processorWeight uint32) uint32 {
	if processorWeight == 0 {
		return 0
	}
	return 1 + uint32((8*processorWeight)/processorWeightMax)
}

//nolint:deadcode,unused // may be used in future tests
func calculateJobCPURate(hostProcs uint32, processorCount uint32) uint32 {
	rate := (processorCount * 10000) / hostProcs
	if rate == 0 {
		return 1
	}
	return rate
}

func Test_Container_UpdateResources_CPUShare(t *testing.T) {
	require.Build(t, osversion.V20H2)
	type config struct {
		name             string
		requiredFeatures []string
		runtimeHandler   string
		sandboxImage     string
		containerImage   string
		cmd              []string
		useAnnotation    bool
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
		{
			name:             "LCOW",
			requiredFeatures: []string{featureLCOW, featureCRIUpdateContainer},
			runtimeHandler:   lcowRuntimeHandler,
			sandboxImage:     imageLcowK8sPause,
			containerImage:   imageLcowAlpine,
			cmd:              []string{"top"},
		},
	}

	// add copies of existing test cases, but enable using annotations to update resources
	for i, l := 0, len(tests); i < l; i++ {
		tt := tests[i]
		tt.requiredFeatures = append(tt.requiredFeatures, featureCRIPlugin)
		tt.useAnnotation = true
		tt.name += "_Annotation"
		tests = append(tests, tt)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireFeatures(t, test.requiredFeatures...)

			if test.runtimeHandler == lcowRuntimeHandler {
				pullRequiredLCOWImages(t, []string{test.sandboxImage, test.containerImage})
			} else if test.runtimeHandler == wcowHypervisorRuntimeHandler {
				pullRequiredImages(t, []string{test.sandboxImage, test.containerImage})
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
			annot := annotations.ContainerProcessorWeight
			updateReq := &runtime.UpdateContainerResourcesRequest{
				ContainerId: containerID,
				Annotations: make(map[string]string),
			}

			expected := uint32(5_000)
			expectedStr := strconv.FormatUint(uint64(expected), 10)
			if test.useAnnotation {
				updateReq.Annotations[annot] = expectedStr
			}
			if test.runtimeHandler == lcowRuntimeHandler {
				updateReq.Linux = &runtime.LinuxContainerResources{}
				if !test.useAnnotation {
					updateReq.Linux.CpuShares = int64(expected)
				}
			} else {
				updateReq.Windows = &runtime.WindowsContainerResources{}
				if !test.useAnnotation {
					updateReq.Windows.CpuShares = int64(expected)
				}
			}

			updateContainer(t, client, ctx, updateReq)

			if test.runtimeHandler == lcowRuntimeHandler {
				checkLCOWResourceLimit(t, ctx, client, containerID, "/sys/fs/cgroup/cpu/cpu.shares", uint64(expected))
			} else {
				targetShimName := "k8s.io-" + podID
				jobExpectedValue := calculateJobCPUWeight(expected)
				checkWCOWResourceLimit(t, ctx, test.runtimeHandler, targetShimName, containerID, "cpu-weight", uint64(jobExpectedValue))
			}

			spec := getContainerOCISpec(t, client, ctx, containerID)
			if test.useAnnotation {
				checkAnnotation(t, spec, annot, expectedStr)
			} else {
				var l uint64
				if test.runtimeHandler == lcowRuntimeHandler {
					if x := getOCILinuxResources(t, spec).CPU; x != nil && x.Shares != nil {
						l = *x.Shares
					}
				} else {
					if x := getOCIWindowsResources(t, spec).CPU; x != nil && x.Shares != nil {
						l = uint64(*x.Shares)
					}
				}
				if l != uint64(expected) {
					t.Fatalf("got cpu shares %d, expected %d", l, expected)
				}
			}
		})
	}
}

func Test_Container_UpdateResources_CPUShare_NotRunning(t *testing.T) {
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
		{
			name:             "LCOW",
			requiredFeatures: []string{featureLCOW, featureCRIUpdateContainer},
			runtimeHandler:   lcowRuntimeHandler,
			sandboxImage:     imageLcowK8sPause,
			containerImage:   imageLcowAlpine,
			cmd:              []string{"top"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireFeatures(t, test.requiredFeatures...)

			if test.runtimeHandler == lcowRuntimeHandler {
				pullRequiredLCOWImages(t, []string{test.sandboxImage, test.containerImage})
			} else if test.runtimeHandler == wcowHypervisorRuntimeHandler {
				pullRequiredImages(t, []string{test.sandboxImage, test.containerImage})
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

			var expected uint32 = 5_000
			if test.runtimeHandler == lcowRuntimeHandler {
				updateReq.Linux = &runtime.LinuxContainerResources{
					CpuShares: int64(expected),
				}
			} else {
				updateReq.Windows = &runtime.WindowsContainerResources{
					CpuShares: int64(expected),
				}
			}

			updateContainer(t, client, ctx, updateReq)

			startContainer(t, client, ctx, containerID)
			defer stopContainer(t, client, ctx, containerID)

			if test.runtimeHandler == lcowRuntimeHandler {
				checkLCOWResourceLimit(t, ctx, client, containerID, "/sys/fs/cgroup/cpu/cpu.shares", uint64(expected))
			} else {
				targetShimName := "k8s.io-" + podID
				jobExpectedValue := calculateJobCPUWeight(expected)
				checkWCOWResourceLimit(t, ctx, test.runtimeHandler, targetShimName, containerID, "cpu-weight", uint64(jobExpectedValue))
			}
		})
	}
}

func Test_Container_UpdateResources_Memory(t *testing.T) {
	type config struct {
		name             string
		requiredFeatures []string
		runtimeHandler   string
		sandboxImage     string
		containerImage   string
		cmd              []string
		useAnnotation    bool
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
		{
			name:             "LCOW",
			requiredFeatures: []string{featureLCOW, featureCRIUpdateContainer},
			runtimeHandler:   lcowRuntimeHandler,
			sandboxImage:     imageLcowK8sPause,
			containerImage:   imageLcowAlpine,
			cmd:              []string{"top"},
		},
	}

	// add copies of existing test cases, but enable using annotations to update resources
	for i, l := 0, len(tests); i < l; i++ {
		tt := tests[i]
		tt.requiredFeatures = append(tt.requiredFeatures, featureCRIPlugin)
		tt.useAnnotation = true
		tt.name += "_Annotation"
		tests = append(tests, tt)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireFeatures(t, test.requiredFeatures...)

			if test.runtimeHandler == lcowRuntimeHandler {
				pullRequiredLCOWImages(t, []string{test.sandboxImage, test.containerImage})
			} else if test.runtimeHandler == wcowHypervisorRuntimeHandler {
				pullRequiredImages(t, []string{test.sandboxImage, test.containerImage})
			}

			podRequest := getRunPodSandboxRequest(t, test.runtimeHandler)

			client := newTestRuntimeClient(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			podID := runPodSandbox(t, client, ctx, podRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			annot := annotations.ContainerMemorySizeInMB
			startingMemorySizeMiB := int64(768)
			startingMemorySize := int64(startingMemorySizeMiB * memory.MiB)
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
						annot: fmt.Sprintf("%d", startingMemorySizeMiB), // 768MB
					},
				},
				PodSandboxId:  podID,
				SandboxConfig: podRequest.Config,
			}

			containerID := createContainer(t, client, ctx, containerRequest)
			defer removeContainer(t, client, ctx, containerID)

			startContainer(t, client, ctx, containerID)
			defer stopContainer(t, client, ctx, containerID)

			// make request for memory limit
			updateReq := &runtime.UpdateContainerResourcesRequest{
				ContainerId: containerID,
				Annotations: make(map[string]string),
			}

			newMemorySize := startingMemorySize / 2
			newMemorySizeStr := strconv.FormatUint(uint64(startingMemorySizeMiB/2), 10) // in MiB
			if test.useAnnotation {
				updateReq.Annotations[annot] = newMemorySizeStr
			}
			if test.runtimeHandler == lcowRuntimeHandler {
				updateReq.Linux = &runtime.LinuxContainerResources{}
				if !test.useAnnotation {
					updateReq.Linux.MemoryLimitInBytes = newMemorySize
				}
			} else {
				updateReq.Windows = &runtime.WindowsContainerResources{}
				if !test.useAnnotation {
					updateReq.Windows.MemoryLimitInBytes = newMemorySize
				}
			}

			updateContainer(t, client, ctx, updateReq)

			if test.runtimeHandler == lcowRuntimeHandler {
				checkLCOWResourceLimit(t, ctx, client, containerID, "/sys/fs/cgroup/memory/memory.limit_in_bytes", uint64(newMemorySize))
			} else {
				targetShimName := "k8s.io-" + podID
				checkWCOWResourceLimit(t, ctx, test.runtimeHandler, targetShimName, containerID, "memory-limit", uint64(newMemorySize))
			}

			spec := getContainerOCISpec(t, client, ctx, containerID)
			if test.useAnnotation {
				checkAnnotation(t, spec, annot, newMemorySizeStr)
			} else {
				var l uint64
				if test.runtimeHandler == lcowRuntimeHandler {
					if x := getOCILinuxResources(t, spec).Memory; x != nil && x.Limit != nil {
						l = uint64(*x.Limit)
					}
				} else {
					if x := getOCIWindowsResources(t, spec).Memory; x != nil && x.Limit != nil {
						l = *x.Limit
					}
				}
				if l != uint64(newMemorySize) {
					t.Fatalf("got memory limit %d, expected %d", l, newMemorySize)
				}
			}
		})
	}
}

// validate that annotation update is rolled back on failure
func Test_Container_UpdateResources_Annotation_Failure(t *testing.T) {
	requireFeatures(t, featureCRIUpdateContainer, featureCRIPlugin)

	// don't have an invalid value for Linux (container) resources similar to how Windows has
	tests := []struct {
		name           string
		features       []string
		runtimeHandler string
		podImage       string
		image          string
		cmd            []string
	}{
		{
			name:           "WCOW_Process",
			features:       []string{featureWCOWProcess},
			runtimeHandler: wcowProcessRuntimeHandler,
			podImage:       imageWindowsNanoserver,
			image:          imageWindowsNanoserver,
			cmd:            []string{"cmd", "/c", "ping", "-t", "127.0.0.1"},
		},
		{
			name:           "WCOW_Hypervisor",
			features:       []string{featureWCOWHypervisor},
			runtimeHandler: wcowHypervisorRuntimeHandler,
			podImage:       imageWindowsNanoserver,
			image:          imageWindowsNanoserver,
			cmd:            []string{"cmd", "/c", "ping", "-t", "127.0.0.1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireFeatures(t, tt.features...)

			if tt.runtimeHandler == lcowRuntimeHandler {
				pullRequiredLCOWImages(t, []string{tt.podImage, tt.image})
			} else if tt.runtimeHandler == wcowHypervisorRuntimeHandler {
				pullRequiredImages(t, []string{tt.podImage, tt.image})
			}

			podRequest := getRunPodSandboxRequest(t, tt.runtimeHandler)

			client := newTestRuntimeClient(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			podID := runPodSandbox(t, client, ctx, podRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			annot := annotations.ContainerProcessorWeight
			weight := uint32(5_000)
			weightStr := strconv.FormatUint(uint64(weight), 10)
			containerRequest := &runtime.CreateContainerRequest{
				Config: &runtime.ContainerConfig{
					Metadata: &runtime.ContainerMetadata{
						Name: t.Name() + "-Container",
					},
					Image: &runtime.ImageSpec{
						Image: tt.image,
					},
					Command: tt.cmd,
					Annotations: map[string]string{
						annot: weightStr,
					},
				},
				PodSandboxId:  podID,
				SandboxConfig: podRequest.Config,
			}

			containerID := createContainer(t, client, ctx, containerRequest)
			defer removeContainer(t, client, ctx, containerID)

			startContainer(t, client, ctx, containerID)
			defer stopContainer(t, client, ctx, containerID)

			updateReq := &runtime.UpdateContainerResourcesRequest{
				ContainerId: containerID,
				Windows:     &runtime.WindowsContainerResources{},
				Annotations: map[string]string{
					annot: "10001",
				},
			}

			if _, err := client.UpdateContainerResources(ctx, updateReq); err == nil {
				t.Fatalf("updating container resources for %s should have failed", podID)
			}

			targetShimName := "k8s.io-" + podID

			jobExpectedValue := calculateJobCPUWeight(weight)
			checkWCOWResourceLimit(t, ctx, tt.runtimeHandler, targetShimName, containerID, "cpu-weight", uint64(jobExpectedValue))

			spec := getContainerOCISpec(t, client, ctx, containerID)
			checkAnnotation(t, spec, annot, weightStr)
		})
	}

}
