// +build functional

package cri_containerd

import (
	"context"
	"testing"

	runtime "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

func Test_SandboxStats_Single_LCOW(t *testing.T) {
	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Namespace: testNamespace,
			},
		},
		RuntimeHandler: lcowRuntimeHandler,
	}

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

func Test_SandboxStats_List_ContainerID_LCOW(t *testing.T) {
	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Namespace: testNamespace,
			},
		},
		RuntimeHandler: lcowRuntimeHandler,
	}

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

func Test_SandboxStats_List_PodID_LCOW(t *testing.T) {
	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Namespace: testNamespace,
			},
		},
		RuntimeHandler: lcowRuntimeHandler,
	}

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
