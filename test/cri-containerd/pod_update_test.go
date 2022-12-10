//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/internal/cpugroup"
	"github.com/Microsoft/hcsshim/internal/memory"
	"github.com/Microsoft/hcsshim/internal/processorinfo"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func Test_Pod_UpdateResources_Memory(t *testing.T) {
	type config struct {
		name             string
		requiredFeatures []string
		runtimeHandler   string
		sandboxImage     string
		useAnnotation    bool
	}
	tests := []config{
		{
			name:             "WCOW_Hypervisor",
			requiredFeatures: []string{featureWCOWHypervisor},
			runtimeHandler:   wcowHypervisorRuntimeHandler,
			sandboxImage:     imageWindowsNanoserver,
		},
		{
			name:             "LCOW",
			requiredFeatures: []string{featureLCOW},
			runtimeHandler:   lcowRuntimeHandler,
			sandboxImage:     imageLcowK8sPause,
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
				pullRequiredLCOWImages(t, []string{test.sandboxImage})
			} else {
				pullRequiredImages(t, []string{test.sandboxImage})
			}

			annot := annotations.MemorySizeInMB
			startingMemorySizeMiB := int64(768)
			var startingMemorySize int64 = startingMemorySizeMiB * memory.MiB
			podRequest := getRunPodSandboxRequest(
				t,
				test.runtimeHandler,
				WithSandboxAnnotations(map[string]string{
					annot: fmt.Sprintf("%d", startingMemorySizeMiB),
				}),
			)

			client := newTestRuntimeClient(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			podID := runPodSandbox(t, client, ctx, podRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			// make request for shrinking memory size
			newMemorySize := startingMemorySize / 2
			newMemorySizeStr := strconv.FormatUint(uint64(startingMemorySizeMiB/2), 10) // in MiB
			updateReq := &runtime.UpdateContainerResourcesRequest{
				ContainerId: podID,
				Annotations: make(map[string]string),
			}

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
			// todo: verify VM memory limits

			spec := getPodSandboxOCISpec(t, client, ctx, podID)
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
						l = uint64(*x.Limit)
					}
				}
				if l != uint64(newMemorySize) {
					t.Fatalf("got memory limit %d, expected %d", l, newMemorySize)
				}
			}
		})
	}
}

func Test_Pod_UpdateResources_Memory_PA(t *testing.T) {
	type config struct {
		name             string
		requiredFeatures []string
		runtimeHandler   string
		sandboxImage     string
		useAnnotation    bool
	}
	tests := []config{
		{
			name:             "WCOW_Hypervisor",
			requiredFeatures: []string{featureWCOWHypervisor},
			runtimeHandler:   wcowHypervisorRuntimeHandler,
			sandboxImage:     imageWindowsNanoserver,
		},
		{
			name:             "LCOW",
			requiredFeatures: []string{featureLCOW},
			runtimeHandler:   lcowRuntimeHandler,
			sandboxImage:     imageLcowK8sPause,
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
				pullRequiredLCOWImages(t, []string{test.sandboxImage})
			} else {
				pullRequiredImages(t, []string{test.sandboxImage})
			}

			annot := annotations.MemorySizeInMB
			startingMemorySizeMiB := int64(200)
			var startingMemorySize int64 = startingMemorySizeMiB * memory.MiB
			podRequest := getRunPodSandboxRequest(
				t,
				test.runtimeHandler,
				WithSandboxAnnotations(map[string]string{
					annotations.FullyPhysicallyBacked: "true",
					annotations.MemorySizeInMB:        fmt.Sprintf("%d", startingMemorySizeMiB),
				}),
			)

			client := newTestRuntimeClient(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			podID := runPodSandbox(t, client, ctx, podRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			// make request for shrinking memory size
			newMemorySize := startingMemorySize / 2
			newMemorySizeStr := strconv.FormatUint(uint64(startingMemorySizeMiB/2), 10) // in MiB
			updateReq := &runtime.UpdateContainerResourcesRequest{
				ContainerId: podID,
				Annotations: make(map[string]string),
			}

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
			// todo: verify VM memory limits

			spec := getPodSandboxOCISpec(t, client, ctx, podID)
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
						l = uint64(*x.Limit)
					}
				}
				if l != uint64(newMemorySize) {
					t.Fatalf("got memory limit %d, expected %d", l, newMemorySize)
				}
			}
		})
	}
}

func Test_Pod_UpdateResources_CPUShares(t *testing.T) {
	type config struct {
		name             string
		requiredFeatures []string
		runtimeHandler   string
		sandboxImage     string
		useAnnotation    bool
	}
	tests := []config{
		{
			name:             "WCOW_Hypervisor",
			requiredFeatures: []string{featureWCOWHypervisor},
			runtimeHandler:   wcowHypervisorRuntimeHandler,
			sandboxImage:     imageWindowsNanoserver,
		},
		{
			name:             "LCOW",
			requiredFeatures: []string{featureLCOW},
			runtimeHandler:   lcowRuntimeHandler,
			sandboxImage:     imageLcowK8sPause,
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
				pullRequiredLCOWImages(t, []string{test.sandboxImage})
			} else {
				pullRequiredImages(t, []string{test.sandboxImage})
			}
			podRequest := getRunPodSandboxRequest(t, test.runtimeHandler)

			client := newTestRuntimeClient(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			podID := runPodSandbox(t, client, ctx, podRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			annot := annotations.ProcessorWeight
			shares := int64(2_000)
			sharesStr := strconv.FormatInt(shares, 10)
			updateReq := &runtime.UpdateContainerResourcesRequest{
				ContainerId: podID,
				Annotations: make(map[string]string),
			}

			if test.useAnnotation {
				updateReq.Annotations[annot] = sharesStr
			}
			if test.runtimeHandler == lcowRuntimeHandler {
				updateReq.Linux = &runtime.LinuxContainerResources{}
				if !test.useAnnotation {
					updateReq.Linux.CpuShares = shares
				}
			} else {
				updateReq.Windows = &runtime.WindowsContainerResources{}
				if !test.useAnnotation {
					updateReq.Windows.CpuShares = shares
				}
			}

			updateContainer(t, client, ctx, updateReq)

			spec := getPodSandboxOCISpec(t, client, ctx, podID)
			if test.useAnnotation {
				checkAnnotation(t, spec, annot, sharesStr)
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
				if l != uint64(shares) {
					t.Fatalf("got cpu shares %d, expected %d", l, shares)
				}
			}
		})
	}
}

func Test_Pod_UpdateResources_CPUGroup(t *testing.T) {
	t.Skip("Skipping for now")
	ctx := context.Background()

	processorTopology, err := processorinfo.HostProcessorInfo(ctx)
	if err != nil {
		t.Fatalf("failed to get host processor information: %s", err)
	}
	lpIndices := make([]uint32, processorTopology.LogicalProcessorCount)
	for i, p := range processorTopology.LogicalProcessors {
		lpIndices[i] = p.LpIndex
	}

	startCPUGroupID := "FA22A12C-36B3-486D-A3E9-BC526C2B450B"
	if err := cpugroup.Create(ctx, startCPUGroupID, lpIndices); err != nil {
		t.Fatalf("failed to create test cpugroup with: %v", err)
	}

	defer func() {
		err := cpugroup.Delete(ctx, startCPUGroupID)
		if err != nil && err != cpugroup.ErrHVStatusInvalidCPUGroupState {
			t.Fatalf("failed to clean up test cpugroup with: %v", err)
		}
	}()

	updateCPUGroupID := "FA22A12C-36B3-486D-A3E9-BC526C2B450C"
	if err := cpugroup.Create(ctx, updateCPUGroupID, lpIndices); err != nil {
		t.Fatalf("failed to create test cpugroup with: %v", err)
	}

	defer func() {
		err := cpugroup.Delete(ctx, updateCPUGroupID)
		if err != nil && err != cpugroup.ErrHVStatusInvalidCPUGroupState {
			t.Fatalf("failed to clean up test cpugroup with: %v", err)
		}
	}()

	//nolint:unused // false positive about config being unused
	type config struct {
		name             string
		requiredFeatures []string
		runtimeHandler   string
		sandboxImage     string
	}

	tests := []config{
		{
			name:             "WCOW_Hypervisor",
			requiredFeatures: []string{featureWCOWHypervisor},
			runtimeHandler:   wcowHypervisorRuntimeHandler,
			sandboxImage:     imageWindowsNanoserver,
		},
		{
			name:             "LCOW",
			requiredFeatures: []string{featureLCOW},
			runtimeHandler:   lcowRuntimeHandler,
			sandboxImage:     imageLcowK8sPause,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireFeatures(t, test.requiredFeatures...)
			if test.runtimeHandler == lcowRuntimeHandler {
				pullRequiredLCOWImages(t, []string{test.sandboxImage})
			} else {
				pullRequiredImages(t, []string{test.sandboxImage})
			}

			annot := annotations.CPUGroupID
			podRequest := getRunPodSandboxRequest(t, test.runtimeHandler,
				WithSandboxAnnotations(map[string]string{annot: startCPUGroupID}))
			client := newTestRuntimeClient(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			podID := runPodSandbox(t, client, ctx, podRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			updateReq := &runtime.UpdateContainerResourcesRequest{
				ContainerId: podID,
				Annotations: map[string]string{
					annot: updateCPUGroupID,
				},
			}

			if test.runtimeHandler == lcowRuntimeHandler {
				updateReq.Linux = &runtime.LinuxContainerResources{}
			} else {
				updateReq.Windows = &runtime.WindowsContainerResources{}
			}

			updateContainer(t, client, ctx, updateReq)

			spec := getPodSandboxOCISpec(t, client, ctx, podID)
			checkAnnotation(t, spec, annot, updateCPUGroupID)
		})
	}
}

func Test_Pod_UpdateResources_Restart(t *testing.T) {
	requireFeatures(t, featureCRIUpdateContainer, featureCRIPlugin)

	annot := annotations.ProcessorCount
	enableReset := "io.microsoft.cri.enablereset"
	fakeAnnotation := "io.microsoft.virtualmachine.computetopology.fake-annotation"

	tests := []struct {
		name           string
		features       []string
		runtimeHandler string
		podImage       string
		image          string
		cmd            []string
		checkCmd       []string
		useAnnotation  bool
	}{
		{
			name:           "WCOW_Hypervisor",
			features:       []string{featureWCOWHypervisor},
			runtimeHandler: wcowHypervisorRuntimeHandler,
			podImage:       imageWindowsNanoserver,
			image:          imageWindowsNanoserver,
			cmd:            []string{"cmd", "/c", "ping", "-t", "127.0.0.1"},
			checkCmd:       []string{"cmd", "/c", `echo %NUMBER_OF_PROCESSORS%`},
		},
		{
			name:           "LCOW",
			features:       []string{featureLCOW},
			runtimeHandler: lcowRuntimeHandler,
			podImage:       imageLcowK8sPause,
			image:          imageLcowAlpine,
			cmd:            []string{"top"},
			checkCmd:       []string{"ash", "-c", "nproc"},
		},
	}

	// add copies of existing test cases, but enable using annotations to update resources
	for i, l := 0, len(tests); i < l; i++ {
		tt := tests[i]
		tt.useAnnotation = true
		tt.name += "_Annotation"
		tests = append(tests, tt)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireFeatures(t, tt.features...)

			if tt.runtimeHandler == lcowRuntimeHandler {
				pullRequiredLCOWImages(t, []string{tt.podImage, tt.image})
			} else if tt.runtimeHandler == wcowHypervisorRuntimeHandler {
				pullRequiredImages(t, []string{tt.podImage, tt.image})
			}

			client := newTestRuntimeClient(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			count := int64(3)
			// cannot specify pod count at start without annotations, but currently annotations
			// and resource limits do not mix, so have the pod start with default values
			if !tt.useAnnotation {
				// the default
				count = 2
			}
			countStr := strconv.FormatInt(count, 10)
			podAnnotations := map[string]string{
				enableReset: "true",
			}
			// annotations and resource settings do no mix
			if tt.useAnnotation {
				podAnnotations[annot] = countStr
			}
			podRequest := getRunPodSandboxRequest(t, tt.runtimeHandler, WithSandboxAnnotations(podAnnotations))
			podID := runPodSandbox(t, client, ctx, podRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			cRequest := getCreateContainerRequest(podID, t.Name()+"-Container", tt.image, tt.cmd, podRequest.Config)
			cRequest.Config.Annotations = map[string]string{
				enableReset: "true",
			}

			cID := createContainer(t, client, ctx, cRequest)
			defer removeContainer(t, client, ctx, cID)
			startContainer(t, client, ctx, cID)
			defer stopContainer(t, client, ctx, cID)

			execRequest := &runtime.ExecSyncRequest{
				ContainerId: cID,
				Cmd:         tt.checkCmd,
				Timeout:     1,
			}
			if out := strings.TrimSpace(execSuccess(t, client, ctx, execRequest)); out != countStr {
				t.Errorf("exec %v: got %q, watned %q", tt.checkCmd, out, countStr)
			}

			newCount := int64(1)
			newCountStr := strconv.FormatInt(newCount, 10)
			// updating the pod on the windows side, so use WindowsContainerResources
			req := &runtime.UpdateContainerResourcesRequest{
				ContainerId: podID,
				Windows:     &runtime.WindowsContainerResources{},
				Annotations: map[string]string{
					fakeAnnotation: "this shouldn't persist",
				},
			}

			if tt.useAnnotation {
				req.Annotations[annot] = newCountStr
			} else {
				req.Windows.CpuCount = newCount
			}

			updateContainer(t, client, ctx, req)
			t.Logf("update request for pod with %+v and annotations: %+v", req.Windows, req.Annotations)

			spec := getPodSandboxOCISpec(t, client, ctx, podID)
			checkAnnotation(t, spec, fakeAnnotation, "")
			if tt.useAnnotation {
				checkAnnotation(t, spec, annot, newCountStr)
			} else {
				if v := *spec.Windows.Resources.CPU.Count; v != uint64(newCount) {
					t.Fatalf("got %d CPU cores, expected %d", v, newCount)
				}
			}

			// check persistance after update

			stopContainer(t, client, ctx, cID)
			stopPodSandbox(t, client, ctx, podID)

			// let GC run and things be cleaned up
			time.Sleep(500 * time.Millisecond)

			runPodSandbox(t, client, ctx, podRequest)
			startContainer(t, client, ctx, cID)

			if out := strings.TrimSpace(execSuccess(t, client, ctx, execRequest)); out != newCountStr {
				t.Errorf("exec %v: got %q, watned %q", tt.checkCmd, out, newCountStr)
			}

			// spec updates should persist
			spec = getPodSandboxOCISpec(t, client, ctx, podID)
			if tt.useAnnotation {
				checkAnnotation(t, spec, annot, newCountStr)
			} else {
				var l uint64
				if x := getOCIWindowsResources(t, spec).CPU; x != nil && x.Count != nil {
					l = *x.Count
				}
				if l != uint64(newCount) {
					t.Fatalf("got %d CPU cores, expected %d", l, newCount)
				}
			}
		})
	}
}
