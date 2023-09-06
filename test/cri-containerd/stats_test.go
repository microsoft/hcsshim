//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"strconv"
	"testing"

	"github.com/Microsoft/hcsshim/internal/memory"
	"github.com/Microsoft/hcsshim/pkg/annotations"

	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func runContainerAndQueryStats(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, request *runtime.CreateContainerRequest) {
	t.Helper()
	containerID := createContainer(t, client, ctx, request)
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	statsRequest := &runtime.ContainerStatsRequest{
		ContainerId: containerID,
	}

	stats, err := client.ContainerStats(ctx, statsRequest)
	if err != nil {
		t.Fatal(err)
	}

	stat := stats.Stats
	verifyStatsContent(t, stat)
}

func runContainerAndQueryListStats(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, request *runtime.CreateContainerRequest) {
	t.Helper()
	containerID := createContainer(t, client, ctx, request)
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	statsRequest := &runtime.ListContainerStatsRequest{
		Filter: &runtime.ContainerStatsFilter{
			Id: containerID,
		},
	}

	stats, err := client.ListContainerStats(ctx, statsRequest)
	if err != nil {
		t.Fatal(err)
	}

	if len(stats.Stats) != 1 {
		t.Fatalf("expected 1 stats result but got %d", len(stats.Stats))
	}
	stat := stats.Stats[0]
	verifyStatsContent(t, stat)
}

func verifyStatsContent(t *testing.T, stat *runtime.ContainerStats) {
	t.Helper()
	if stat == nil {
		t.Fatal("expected stat to be non nil")
	}
	if stat.Cpu.Timestamp == 0 {
		t.Fatalf("expected cpu stat timestamp != 0 but got %d", stat.Cpu.Timestamp)
	}
	if stat.Cpu.UsageCoreNanoSeconds.Value == 0 {
		t.Fatalf("expected cpu usage != 0 but got %d", stat.Cpu.UsageCoreNanoSeconds.Value)
	}
	if stat.Memory.Timestamp == 0 {
		t.Fatalf("expected memory stat timestamp != 0 but got %d", stat.Memory.Timestamp)
	}
	if stat.Memory.WorkingSetBytes.Value == 0 {
		t.Fatalf("expected memory usage != 0 but got %d", stat.Memory.WorkingSetBytes.Value)
	}
}

// Physically backed working set should be equal to the amount of memory we assigned
// to the UVM.
func verifyPhysicallyBackedWorkingSet(t *testing.T, num uint64, stat *runtime.ContainerStats) {
	t.Helper()
	if stat == nil {
		t.Fatal("expected stat to be non nil")
	}
	numInBytes := num * memory.MiB
	if stat.Memory.WorkingSetBytes.Value != numInBytes {
		t.Fatalf("expected working set size to be %d bytes but got: %d", numInBytes, stat.Memory.WorkingSetBytes.Value)
	}
}

func Test_SandboxStats_Single_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(t, lcowRuntimeHandler)

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	podID := runPodSandbox(t, client, ctx, request)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	statsRequest := &runtime.ContainerStatsRequest{
		ContainerId: podID,
	}

	stats, err := client.ContainerStats(ctx, statsRequest)
	if err != nil {
		t.Fatal(err)
	}

	stat := stats.Stats
	verifyStatsContent(t, stat)
}

func Test_SandboxStats_List_ContainerID_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(t, lcowRuntimeHandler)

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	podID := runPodSandbox(t, client, ctx, request)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	statsRequest := &runtime.ListContainerStatsRequest{
		Filter: &runtime.ContainerStatsFilter{
			Id: podID,
		},
	}

	stats, err := client.ListContainerStats(ctx, statsRequest)
	if err != nil {
		t.Fatal(err)
	}

	if len(stats.Stats) != 1 {
		t.Fatalf("expected 1 stats result but got %d", len(stats.Stats))
	}
	stat := stats.Stats[0]
	verifyStatsContent(t, stat)
}

func Test_SandboxStats_List_PodID_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(t, lcowRuntimeHandler)

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	podID := runPodSandbox(t, client, ctx, request)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	statsRequest := &runtime.ListContainerStatsRequest{
		Filter: &runtime.ContainerStatsFilter{
			PodSandboxId: podID,
		},
	}

	stats, err := client.ListContainerStats(ctx, statsRequest)
	if err != nil {
		t.Fatal(err)
	}

	if len(stats.Stats) != 1 {
		t.Fatalf("expected 1 stats result but got %d", len(stats.Stats))
	}
	stat := stats.Stats[0]
	verifyStatsContent(t, stat)
}

func Test_ContainerStats_ContainerID(t *testing.T) {
	requireAnyFeature(t, featureWCOWProcess, featureWCOWHypervisor, featureLCOW)

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
		{
			name:             "LCOW",
			requiredFeatures: []string{featureLCOW},
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
			} else {
				pullRequiredImages(t, []string{test.sandboxImage, test.containerImage})
			}

			podRequest := getRunPodSandboxRequest(t, test.runtimeHandler)

			client := newTestRuntimeClient(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			podID := runPodSandbox(t, client, ctx, podRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			request := &runtime.CreateContainerRequest{
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
			runContainerAndQueryStats(t, client, ctx, request)
		})
	}
}

func Test_ContainerStats_List_ContainerID(t *testing.T) {
	requireAnyFeature(t, featureWCOWProcess, featureWCOWHypervisor, featureLCOW)

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
		{
			name:             "LCOW",
			requiredFeatures: []string{featureLCOW},
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
			} else {
				pullRequiredImages(t, []string{test.sandboxImage, test.containerImage})
			}

			podRequest := getRunPodSandboxRequest(t, test.runtimeHandler)

			client := newTestRuntimeClient(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			podID := runPodSandbox(t, client, ctx, podRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			request := &runtime.CreateContainerRequest{
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
			runContainerAndQueryListStats(t, client, ctx, request)
		})
	}
}

func Test_SandboxStats_WorkingSet_PhysicallyBacked(t *testing.T) {
	requireAnyFeature(t, featureLCOW, featureWCOWHypervisor)

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

			// If we go too high we run the risk of getting a not enough memory
			// for this operation error. Seems like a nice sweetspot thats different than
			// the default that we can test. If we go less than 1GB WCOW is extremely
			// sluggish.
			var sizeInMB uint64 = 1536
			sizeInMBStr := strconv.FormatUint(sizeInMB, 10)
			podRequest := getRunPodSandboxRequest(
				t,
				test.runtimeHandler,
				WithSandboxAnnotations(map[string]string{
					annotations.AllowOvercommit:      "false",
					annotations.EnableDeferredCommit: "false",
					annotations.MemorySizeInMB:       sizeInMBStr,
				}),
			)

			client := newTestRuntimeClient(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			podID := runPodSandbox(t, client, ctx, podRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)

			statsRequest := &runtime.ContainerStatsRequest{
				ContainerId: podID,
			}

			stats, err := client.ContainerStats(ctx, statsRequest)
			if err != nil {
				t.Fatal(err)
			}

			stat := stats.Stats
			verifyPhysicallyBackedWorkingSet(t, sizeInMB, stat)
		})
	}
}
