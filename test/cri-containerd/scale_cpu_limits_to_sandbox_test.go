//go:build functional
// +build functional

package cri_containerd

import (
	"context"
	"math"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/pkg/annotations"
	criruntime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

const imageWindowsMaxCPUWorkload = "cplatpublic.azurecr.io/golang-1.16.2-nanoserver-1809:max-cpu-workload"

func Test_Scale_CPU_Limits_To_Sandbox(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := newTestRuntimeClient(t)
	podReq := getRunPodSandboxRequest(t, wcowHypervisor17763RuntimeHandler)
	podID := runPodSandbox(t, client, ctx, podReq)
	defer removePodSandbox(t, client, ctx, podID)

	pullRequiredImages(t, []string{imageWindowsMaxCPUWorkload})

	cmd := []string{"cmd", "/c", `C:\load_cpu.exe`}
	contReq := getCreateContainerRequest(podID, "nanoserver-load-cpu", imageWindowsMaxCPUWorkload, cmd, podReq.Config)
	// set the limit to (roughly) 1 processor
	processorLimit := 10000 / runtime.NumCPU()
	contReq.Config.Annotations = map[string]string{
		annotations.ContainerProcessorLimit: strconv.Itoa(processorLimit),
	}

	contID := createContainer(t, client, ctx, contReq)
	defer removeContainer(t, client, ctx, contID)
	startContainer(t, client, ctx, contID)
	defer stopContainer(t, client, ctx, contID)

	statsRequest := &criruntime.ContainerStatsRequest{
		ContainerId: contID,
	}

	// baseline container stats request
	initialResponse, err := client.ContainerStats(ctx, statsRequest)
	if err != nil {
		t.Fatalf("error getting initial container stats: %s", err)
	}

	// give it 5 seconds for a better average, with just 1 second, the measurements
	// are consistently 25-30% higher than expected
	time.Sleep(5 * time.Second)

	// final container stats request
	finalResponse, err := client.ContainerStats(ctx, statsRequest)
	if err != nil {
		t.Fatalf("error getting container new container stats: %s", err)
	}

	// Estimate CPU usage by dividing total usage in nanoseconds by time passed in nanoseconds
	oldStats := initialResponse.GetStats().GetCpu()
	newStats := finalResponse.GetStats().GetCpu()
	deltaTime := newStats.GetTimestamp() - oldStats.GetTimestamp()
	deltaUsage := newStats.GetUsageCoreNanoSeconds().GetValue() - oldStats.GetUsageCoreNanoSeconds().GetValue()
	usagePercentage := float64(deltaUsage) / float64(deltaTime) * 100
	t.Logf("container CPU usage percentage: %f", usagePercentage)
	if math.Abs(usagePercentage-100) > 10 {
		t.Fatalf("expected CPU usage around 100 percent, got %f instead. Make sure that ScaleCpuLimitsToSandbox runtime option is set to true", usagePercentage)
	}
}
