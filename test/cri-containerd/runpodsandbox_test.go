//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/internal/cpugroup"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/lcow"
	"github.com/Microsoft/hcsshim/internal/processorinfo"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/Microsoft/hcsshim/test/internal/require"
	testuvm "github.com/Microsoft/hcsshim/test/internal/uvm"
	"github.com/containerd/containerd/log"
	"golang.org/x/sys/windows"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func runPodSandboxTest(t *testing.T, request *runtime.RunPodSandboxRequest) {
	t.Helper()
	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	podID := runPodSandbox(t, client, ctx, request)
	stopPodSandbox(t, client, ctx, podID)
	removePodSandbox(t, client, ctx, podID)
}

func Test_RunPodSandbox_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(t, wcowProcessRuntimeHandler)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(t, wcowHypervisorRuntimeHandler)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(t, lcowRuntimeHandler)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_Events_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	podctx, podcancel := context.WithCancel(context.Background())
	defer podcancel()

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(t, lcowRuntimeHandler)

	topicNames, filters := getTargetRunTopics()
	targetNamespace := "k8s.io"

	eventService := newTestEventService(t)
	stream, errs := eventService.Subscribe(ctx, filters...)

	podID := runPodSandbox(t, client, podctx, request)
	stopPodSandbox(t, client, podctx, podID)
	removePodSandbox(t, client, podctx, podID)

	for _, topic := range topicNames {
		select {
		case env := <-stream:
			if topic != env.Topic {
				t.Fatalf("event topic %v does not match expected topic %v", env.Topic, topic)
			}
			if targetNamespace != env.Namespace {
				t.Fatalf("event namespace %v does not match expected namespace %v", env.Namespace, targetNamespace)
			}
			t.Logf("event topic seen: %v", env.Topic)

			id, _, err := convertEvent(env.Event)
			if err != nil {
				t.Fatalf("topic %v event: %v", env.Topic, err)
			}
			if id != podID {
				t.Fatalf("event topic %v belongs to pod %v, not targeted pod %v", env.Topic, id, podID)
			}
		case err := <-errs:
			t.Fatalf("event subscription err %v", err)
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				t.Fatalf("event %v deadline exceeded", topic)
			}
		}
	}
}

func Test_RunPodSandbox_VirtualMemory_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowHypervisorRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.AllowOvercommit: "true",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_VirtualMemory_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.AllowOvercommit: "true",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_VirtualMemory_DeferredCommit_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowHypervisorRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.AllowOvercommit:      "true",
			annotations.EnableDeferredCommit: "true",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_VirtualMemory_DeferredCommit_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.AllowOvercommit:      "true",
			annotations.EnableDeferredCommit: "true",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_PhysicalMemory_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowHypervisorRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.AllowOvercommit: "false",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_FullyPhysicallyBacked_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowHypervisorRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.FullyPhysicallyBacked: "true",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_VSMBNoDirectMap_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowHypervisorRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.VSMBNoDirectMap: "true",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_PhysicalMemory_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.AllowOvercommit: "false",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_FullyPhysicallyBacked_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.FullyPhysicallyBacked: "true",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_MemorySize_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowProcessRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.ContainerMemorySizeInMB: "128",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_MemorySize_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowHypervisorRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.MemorySizeInMB: "768", // 128 is too small for WCOW. It is really slow boot.
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_MemorySize_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.MemorySizeInMB: "200",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_MMIO_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	if osversion.Build() < osversion.V20H1 {
		t.Skip("Requires build +20H1")
	}
	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowProcessRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.MemoryLowMMIOGapInMB:   "100",
			annotations.MemoryHighMMIOBaseInMB: "100",
			annotations.MemoryHighMMIOGapInMB:  "100",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_MMIO_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	if osversion.Build() < osversion.V20H1 {
		t.Skip("Requires build +20H1")
	}
	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowHypervisorRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.MemoryLowMMIOGapInMB:   "100",
			annotations.MemoryHighMMIOBaseInMB: "100",
			annotations.MemoryHighMMIOGapInMB:  "100",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_MMIO_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	if osversion.Build() < osversion.V20H1 {
		t.Skip("Requires build +20H1")
	}
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.MemoryLowMMIOGapInMB:   "100",
			annotations.MemoryHighMMIOBaseInMB: "100",
			annotations.MemoryHighMMIOGapInMB:  "100",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPUCount_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowProcessRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.ContainerProcessorCount: "1",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPUCount_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowHypervisorRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.ProcessorCount: "1",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPUCount_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.ProcessorCount: "1",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPULimit_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowProcessRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.ContainerProcessorLimit: "9000",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPULimit_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowHypervisorRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.ProcessorLimit: "90000",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPULimit_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.ProcessorLimit: "90000",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPUWeight_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowProcessRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.ContainerProcessorWeight: "500",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPUWeight_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowHypervisorRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.ContainerProcessorWeight: "500",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPUWeight_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.ProcessorWeight: "500",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_StorageQoSBandwithMax_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowProcessRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.ContainerStorageQoSBandwidthMaximum: fmt.Sprintf("%d", 1024*1024), // 1MB/s
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_StorageQoSBandwithMax_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowHypervisorRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.StorageQoSBandwidthMaximum: fmt.Sprintf("%d", 1024*1024), // 1MB/s
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_StorageQoSBandwithMax_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.StorageQoSBandwidthMaximum: fmt.Sprintf("%d", 1024*1024), // 1MB/s
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_StorageQoSIopsMax_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowProcessRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.ContainerStorageQoSIopsMaximum: "300",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_StorageQoSIopsMax_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowHypervisorRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.StorageQoSIopsMaximum: "300",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_StorageQoSIopsMax_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.StorageQoSIopsMaximum: "300",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_InitrdBoot_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.PreferredRootFSType: "initrd",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_RootfsVhdBoot_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.PreferredRootFSType: "vhd",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_VPCIEnabled_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.VPCIEnabled: "true",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_UEFIBoot_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.KernelDirectBoot: "false",
		}),
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_DnsConfig_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(t, wcowProcessRuntimeHandler)
	request.Config.DnsConfig = &runtime.DNSConfig{
		Searches: []string{"8.8.8.8", "8.8.4.4"},
	}
	runPodSandboxTest(t, request)
	// TODO: JTERRY75 - This is just a boot test at present. We need to create a
	// container, exec the ipconfig and parse the results to verify that the
	// searches are set.
}

func Test_RunPodSandbox_DnsConfig_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(t, wcowHypervisorRuntimeHandler)
	request.Config.DnsConfig = &runtime.DNSConfig{
		Searches: []string{"8.8.8.8", "8.8.4.4"},
	}
	runPodSandboxTest(t, request)
	// TODO: JTERRY75 - This is just a boot test at present. We need to create a
	// container, exec the ipconfig and parse the results to verify that the
	// searches are set.
}

func Test_RunPodSandbox_DnsConfig_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(t, lcowRuntimeHandler)
	request.Config.DnsConfig = &runtime.DNSConfig{
		Searches: []string{"8.8.8.8", "8.8.4.4"},
	}
	runPodSandboxTest(t, request)
	// TODO: JTERRY75 - This is just a boot test at present. We need to create a
	// container, cat /etc/resolv.conf and parse the results to verify that the
	// searches are set.
}

func Test_RunPodSandbox_PortMappings_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(t, wcowProcessRuntimeHandler)
	request.Config.PortMappings = []*runtime.PortMapping{
		{
			Protocol:      runtime.Protocol_TCP,
			ContainerPort: 80,
			HostPort:      8080,
		},
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_PortMappings_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(t, wcowHypervisorRuntimeHandler)
	request.Config.PortMappings = []*runtime.PortMapping{
		{
			Protocol:      runtime.Protocol_TCP,
			ContainerPort: 80,
			HostPort:      8080,
		},
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_PortMappings_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(t, lcowRuntimeHandler)
	request.Config.PortMappings = []*runtime.PortMapping{
		{
			Protocol:      runtime.Protocol_TCP,
			ContainerPort: 80,
			HostPort:      8080,
		},
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CustomizableScratchDefaultSize_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	annots := map[string]string{
		annotations.AllowOvercommit: "true",
	}

	output, errorMsg, exitCode := createSandboxContainerAndExecForCustomScratch(t, annots)

	if exitCode != 0 {
		t.Fatalf("Exec into container failed with: %v and exit code: %d, Test_RunPodSandbox_CustomizableScratchDefaultSize_LCOW", errorMsg, exitCode)
	}

	// Format of output for df is below
	// Filesystem           1K-blocks      Used Available Use% Mounted on
	// overlay               20642524        36  19577528   0% /
	// tmpfs                    65536         0     65536   0% /dev
	scanner := bufio.NewScanner(strings.NewReader(output))
	found := false
	var cols []string
	for scanner.Scan() {
		outputLine := scanner.Text()
		if cols = strings.Fields(outputLine); cols[0] == "overlay" && cols[5] == "/" {
			found = true
			t.Log(outputLine)
			break
		}
	}

	if !found {
		t.Fatalf("could not find the correct output line for overlay mount on / n: error: %v, exitcode: %d", errorMsg, exitCode)
	}

	// df command shows size in KB, 20642524 is 20GB
	actualMountSize, _ := strconv.ParseInt(cols[1], 10, 64)
	expectedMountSize := int64(20642524)
	toleranceInKB := int64(10240)
	if actualMountSize < (expectedMountSize-toleranceInKB) || actualMountSize > (expectedMountSize+toleranceInKB) {
		t.Fatalf("Size of the overlay filesystem mounted at / is not within 10MB of 20642524 (20GB). It is %s", cols[1])
	}
}

func Test_RunPodSandbox_CustomizableScratchCustomSize_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	annots := map[string]string{
		annotations.AllowOvercommit: "true",
		"containerd.io/snapshot/io.microsoft.container.storage.rootfs.size-gb": "200",
	}

	output, errorMsg, exitCode := createSandboxContainerAndExecForCustomScratch(t, annots)

	if exitCode != 0 {
		t.Fatalf("Exec into container failed with: %v and exit code: %d, Test_RunPodSandbox_CustomizableScratchDefaultSize_LCOW", errorMsg, exitCode)
	}

	// Format of output for df is below
	// Filesystem           1K-blocks      Used Available Use% Mounted on
	// overlay               20642524        36  19577528   0% /
	// tmpfs                    65536         0     65536   0% /dev
	scanner := bufio.NewScanner(strings.NewReader(output))
	found := false
	var cols []string
	for scanner.Scan() {
		outputLine := scanner.Text()
		if cols = strings.Fields(outputLine); cols[0] == "overlay" && cols[5] == "/" {
			found = true
			t.Log(outputLine)
			break
		}
	}

	if !found {
		t.Log(output)
		t.Fatalf("could not find the correct output line for overlay mount on / n: error: %v, exitcode: %d", errorMsg, exitCode)
	}

	// df command shows size in KB, 206425432 is 200GB
	actualMountSize, _ := strconv.ParseInt(cols[1], 10, 64)
	expectedMountSize := int64(206425432)
	toleranceInKB := int64(10240)
	if actualMountSize < (expectedMountSize-toleranceInKB) || actualMountSize > (expectedMountSize+toleranceInKB) {
		t.Log(output)
		t.Fatalf("Size of the overlay filesystem mounted at / is not within 10MB of 206425432 (200GB). It is %s", cols[1])
	}
}

func Test_RunPodSandbox_Mount_SandboxDir_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	annots := map[string]string{
		annotations.AllowOvercommit: "true",
	}

	mounts := []*runtime.Mount{
		{
			HostPath:      "sandbox:///boot",
			ContainerPath: "/containerUvmDir",
		},
	}
	cmd := []string{
		"mount",
	}

	output, errorMsg, exitCode := createSandboxContainerAndExec(t, annots, mounts, cmd)

	if exitCode != 0 {
		t.Fatalf("Exec into container failed with: %v and exit code: %d, %s", errorMsg, exitCode, t.Name())
	}

	t.Log(output)

	//TODO: Parse the output of the exec command to make sure the uvm mount was successful
}

func Test_RunPodSandbox_Mount_SandboxDir_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	client := newTestRuntimeClient(t)
	ctx := context.Background()

	sbRequest := getRunPodSandboxRequest(t, wcowHypervisorRuntimeHandler)
	podID := runPodSandbox(t, client, ctx, sbRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	command := []string{
		"cmd",
		"/c",
		"ping",
		"-t",
		"127.0.0.1",
	}

	mounts := []*runtime.Mount{
		{
			HostPath:      "sandbox:///test",
			ContainerPath: "C:\\test",
		},
	}
	// Create 2 containers with sandbox mounts and verify both can write and see the others files
	container1Name := t.Name() + "-Container-" + "1"
	container1Id := createContainerInSandbox(t, client, ctx, podID, container1Name, imageWindowsNanoserver, command, nil, mounts, sbRequest.Config)
	defer removeContainer(t, client, ctx, container1Id)

	startContainer(t, client, ctx, container1Id)
	defer stopContainer(t, client, ctx, container1Id)

	execEcho := []string{
		"cmd",
		"/c",
		"echo",
		`"test"`,
		">",
		"C:\\test\\test.txt",
	}
	_, errorMsg, exitCode := execContainer(t, client, ctx, container1Id, execEcho)
	if exitCode != 0 {
		t.Fatalf("Exec into container failed with: %v and exit code: %d, %s", errorMsg, exitCode, container1Id)
	}

	container2Name := t.Name() + "-Container-" + "2"
	container2Id := createContainerInSandbox(t, client, ctx, podID, container2Name, imageWindowsNanoserver, command, nil, mounts, sbRequest.Config)
	defer removeContainer(t, client, ctx, container2Id)

	startContainer(t, client, ctx, container2Id)
	defer stopContainer(t, client, ctx, container2Id)

	// Test that we can see the file made in the first container in the second one.
	execDir := []string{
		"cmd",
		"/c",
		"dir",
		"C:\\test\\test.txt",
	}
	_, errorMsg, exitCode = execContainer(t, client, ctx, container2Id, execDir)
	if exitCode != 0 {
		t.Fatalf("Exec into container failed with: %v and exit code: %d, %s", errorMsg, exitCode, container2Id)
	}
}

func Test_RunPodSandbox_Mount_SandboxDir_NoShare_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	client := newTestRuntimeClient(t)
	ctx := context.Background()

	sbRequest := getRunPodSandboxRequest(t, wcowHypervisorRuntimeHandler)
	podID := runPodSandbox(t, client, ctx, sbRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	command := []string{
		"cmd",
		"/c",
		"ping",
		"-t",
		"127.0.0.1",
	}

	mounts := []*runtime.Mount{
		{
			HostPath:      "sandbox:///test",
			ContainerPath: "C:\\test",
		},
	}
	// This test case is making sure that the sandbox mount doesn't show up in another container if not
	// explicitly asked for. Make first container with the mount and another shortly after without.
	container1Name := t.Name() + "-Container-" + "1"
	container1Id := createContainerInSandbox(t, client, ctx, podID, container1Name, imageWindowsNanoserver, command, nil, mounts, sbRequest.Config)
	defer removeContainer(t, client, ctx, container1Id)

	startContainer(t, client, ctx, container1Id)
	defer stopContainer(t, client, ctx, container1Id)

	container2Name := t.Name() + "-Container-" + "2"
	container2Id := createContainerInSandbox(t, client, ctx, podID, container2Name, imageWindowsNanoserver, command, nil, nil, sbRequest.Config)
	defer removeContainer(t, client, ctx, container2Id)

	startContainer(t, client, ctx, container2Id)
	defer stopContainer(t, client, ctx, container2Id)

	// Test that we can't see the file made in the first container in the second one.
	execDir := []string{
		"cmd",
		"/c",
		"dir",
		"C:\\test\\",
	}
	output, _, exitCode := execContainer(t, client, ctx, container2Id, execDir)
	if exitCode == 0 {
		t.Fatalf("Found directory in second container when not expected: %s", output)
	}
}

func Test_RunPodSandbox_CPUGroup(t *testing.T) {
	require.Build(t, osversion.V21H1)
	ctx := context.Background()
	presentID := "FA22A12C-36B3-486D-A3E9-BC526C2B450B"

	processorTopology, err := processorinfo.HostProcessorInfo(ctx)
	if err != nil {
		t.Fatalf("failed to get host processor information: %s", err)
	}
	lpIndices := make([]uint32, processorTopology.LogicalProcessorCount)
	for i, p := range processorTopology.LogicalProcessors {
		lpIndices[i] = p.LpIndex
	}

	if err := cpugroup.Create(ctx, presentID, lpIndices); err != nil {
		t.Fatalf("failed to create test cpugroup with: %v", err)
	}

	defer func() {
		err := cpugroup.Delete(ctx, presentID)
		if err != nil && err != cpugroup.ErrHVStatusInvalidCPUGroupState {
			t.Fatalf("failed to clean up test cpugroup with: %v", err)
		}
	}()

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
		requireFeatures(t, test.requiredFeatures...)
		if test.runtimeHandler == lcowRuntimeHandler {
			pullRequiredLCOWImages(t, []string{test.sandboxImage})
		} else {
			pullRequiredImages(t, []string{test.sandboxImage})
		}

		request := &runtime.RunPodSandboxRequest{
			Config: &runtime.PodSandboxConfig{
				Metadata: &runtime.PodSandboxMetadata{
					Name:      t.Name(),
					Uid:       "0",
					Namespace: testNamespace,
				},
				Annotations: map[string]string{
					annotations.CPUGroupID: presentID,
				},
			},
			RuntimeHandler: test.runtimeHandler,
		}
		runPodSandboxTest(t, request)
	}
}

func createExt4VHD(ctx context.Context, t *testing.T, path string) {
	t.Helper()
	// UVM related functions called below produce a lot debug logs. Set the logger
	// output to Discard if verbose flag is not set. This way we can still capture
	// these logs in a wpr session.
	if !testing.Verbose() {
		origLogOut := log.L.Logger.Out
		log.L.Logger.SetOutput(io.Discard)
		defer log.L.Logger.SetOutput(origLogOut)
	}
	uvm := testuvm.CreateAndStartLCOW(ctx, t, t.Name()+"-createExt4VHD")
	defer uvm.Close()

	if err := lcow.CreateScratch(ctx, uvm, path, 2, ""); err != nil {
		t.Fatal(err)
	}
}

func Test_RunPodSandbox_MultipleContainersSameVhd_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	annots := map[string]string{
		annotations.AllowOvercommit: "true",
	}

	// Create a temporary ext4 VHD to mount into the container.
	vhdHostDir := t.TempDir()
	vhdHostPath := filepath.Join(vhdHostDir, "temp.vhdx")
	createExt4VHD(ctx, t, vhdHostPath)

	vhdContainerPath := "/containerDir"

	mounts := []*runtime.Mount{
		{
			HostPath:      "vhd://" + vhdHostPath,
			ContainerPath: vhdContainerPath,
		},
	}

	sbRequest := getRunPodSandboxRequest(t, lcowRuntimeHandler, WithSandboxAnnotations(annots))

	podID := runPodSandbox(t, client, ctx, sbRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	execCommand := []string{
		"ls",
		vhdContainerPath,
	}

	command := []string{
		"top",
	}

	// create 2 containers with vhd mounts and verify both can mount vhd
	for i := 1; i < 3; i++ {
		containerName := t.Name() + "-Container-" + strconv.Itoa(i)
		containerID := createContainerInSandbox(t, client, ctx, podID, containerName, imageLcowAlpine, command, annots, mounts, sbRequest.Config)
		defer removeContainer(t, client, ctx, containerID)

		startContainer(t, client, ctx, containerID)
		defer stopContainer(t, client, ctx, containerID)

		_, errorMsg, exitCode := execContainer(t, client, ctx, containerID, execCommand)

		// For container 1 and 2 we should find the mounts
		if exitCode != 0 {
			t.Fatalf("Exec into container failed with: %v and exit code: %d, %s", errorMsg, exitCode, containerID)
		}
	}

	// For the 3rd container don't add any mounts
	// this makes sure you can have containers that share vhd mounts and
	// at the same time containers in a pod that don't have any mounts
	mounts = []*runtime.Mount{}
	containerName := t.Name() + "-Container-3"
	containerID := createContainerInSandbox(t, client, ctx, podID, containerName, imageLcowAlpine, command, annots, mounts, sbRequest.Config)
	defer removeContainer(t, client, ctx, containerID)

	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	output, errorMsg, exitCode := execContainer(t, client, ctx, containerID, execCommand)

	// 3rd container should not have the mount and ls should fail
	if exitCode == 0 {
		t.Fatalf("Exec into container succeeded but we expected it to fail: %v and exit code: %s, %s", errorMsg, output, containerID)
	}
}

func Test_RunPodSandbox_MultipleContainersSameVhd_RShared_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sbRequest := getRunPodSandboxRequest(t, lcowRuntimeHandler)
	sbRequest.Config.Linux = &runtime.LinuxPodSandboxConfig{
		SecurityContext: &runtime.LinuxSandboxSecurityContext{
			Privileged: true,
		},
	}

	podID := runPodSandbox(t, client, ctx, sbRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	// Create a temporary ext4 VHD to mount into the container.
	vhdHostDir := t.TempDir()
	vhdHostPath := filepath.Join(vhdHostDir, "temp.vhdx")
	createExt4VHD(ctx, t, vhdHostPath)

	vhdContainerPath := "/containerDir"
	cRequest := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{},
			Image: &runtime.ImageSpec{
				Image: imageLcowAlpine,
			},
			// Hold this command open until killed
			Command: []string{
				"top",
			},
			Linux: &runtime.LinuxContainerConfig{
				SecurityContext: &runtime.LinuxContainerSecurityContext{
					Privileged: true,
				},
			},
			Mounts: []*runtime.Mount{
				{
					HostPath:      "vhd://" + vhdHostPath,
					ContainerPath: vhdContainerPath,
					// set 'rshared' propagation
					Propagation: runtime.MountPropagation_PROPAGATION_BIDIRECTIONAL,
				},
			},
		},
		PodSandboxId:  podID,
		SandboxConfig: sbRequest.Config,
	}

	containerName := t.Name() + "-Container-0"
	cRequest.Config.Metadata.Name = containerName
	containerID0 := createContainer(t, client, ctx, cRequest)
	defer removeContainer(t, client, ctx, containerID0)
	startContainer(t, client, ctx, containerID0)
	defer stopContainer(t, client, ctx, containerID0)

	containerName1 := t.Name() + "-Container-1"
	cRequest.Config.Metadata.Name = containerName1
	containerID1 := createContainer(t, client, ctx, cRequest)
	defer removeContainer(t, client, ctx, containerID1)
	startContainer(t, client, ctx, containerID1)
	defer stopContainer(t, client, ctx, containerID1)

	// create a test directory that will be the new mountpoint's source
	createTestDirCmd := []string{
		"mkdir",
		"/tmp/testdir",
	}
	_, errorMsg, exitCode := execContainer(t, client, ctx, containerID0, createTestDirCmd)
	if exitCode != 0 {
		t.Fatalf("Exec into container failed with: %v and exit code: %d, %s", errorMsg, exitCode, containerID0)
	}

	// create a file in the test directory
	createTestDirContentCmd := []string{
		"touch",
		"/tmp/testdir/test.txt",
	}
	_, errorMsg, exitCode = execContainer(t, client, ctx, containerID0, createTestDirContentCmd)
	if exitCode != 0 {
		t.Fatalf("Exec into container failed with: %v and exit code: %d, %s", errorMsg, exitCode, containerID0)
	}

	// create a test directory in the vhd that will be the new mountpoint's destination
	createTestDirVhdCmd := []string{
		"mkdir",
		fmt.Sprintf("%s/testdir", vhdContainerPath),
	}
	_, errorMsg, exitCode = execContainer(t, client, ctx, containerID0, createTestDirVhdCmd)
	if exitCode != 0 {
		t.Fatalf("Exec into container failed with: %v and exit code: %d, %s", errorMsg, exitCode, containerID0)
	}

	// perform rshared mount of test directory into the vhd
	mountTestDirToVhdCmd := []string{
		"mount",
		"-o",
		"rshared",
		"/tmp/testdir",
		fmt.Sprintf("%s/testdir", vhdContainerPath),
	}
	_, errorMsg, exitCode = execContainer(t, client, ctx, containerID0, mountTestDirToVhdCmd)
	if exitCode != 0 {
		t.Fatalf("Exec into container failed with: %v and exit code: %d, %s", errorMsg, exitCode, containerID0)
	}

	// try to list the test file in the second container to verify it was propagated correctly
	verifyTestMountCommand := []string{
		"ls",
		fmt.Sprintf("%s/testdir/test.txt", vhdContainerPath),
	}
	_, errorMsg, exitCode = execContainer(t, client, ctx, containerID1, verifyTestMountCommand)
	if exitCode != 0 {
		t.Fatalf("Exec into container failed with: %v and exit code: %d, %s", errorMsg, exitCode, containerID1)
	}
}

func Test_RunPodSandbox_MultipleContainersSameVhd_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)
	// Prior to 19H1, we aren't able to easily create a formatted VHD, as
	// HcsFormatWritableLayerVhd requires the VHD to be mounted prior the call.
	if osversion.Build() < osversion.V19H1 {
		t.Skip("Requires at least 19H1")
	}

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	annots := map[string]string{
		annotations.AllowOvercommit: "true",
	}

	vhdHostDir := t.TempDir()
	vhdHostPath := filepath.Join(vhdHostDir, "temp.vhdx")

	if err := hcs.CreateNTFSVHD(ctx, vhdHostPath, 10); err != nil {
		t.Fatalf("failed to create NTFS VHD: %s", err)
	}

	vhdContainerPath := "C:\\containerDir"

	mounts := []*runtime.Mount{
		{
			HostPath:      "vhd://" + vhdHostPath,
			ContainerPath: vhdContainerPath,
		},
	}

	sbRequest := getRunPodSandboxRequest(t, wcowHypervisorRuntimeHandler, WithSandboxAnnotations(annots))

	podID := runPodSandbox(t, client, ctx, sbRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	execCommand := []string{
		"cmd",
		"/c",
		"dir",
		vhdContainerPath,
	}

	command := []string{
		"ping",
		"-t",
		"127.0.0.1",
	}

	// create 2 containers with vhd mounts and verify both can mount vhd
	for i := 1; i < 3; i++ {
		containerName := t.Name() + "-Container-" + strconv.Itoa(i)
		containerID := createContainerInSandbox(t, client,
			ctx, podID, containerName, imageWindowsNanoserver,
			command, annots, mounts, sbRequest.Config)
		defer removeContainer(t, client, ctx, containerID)

		startContainer(t, client, ctx, containerID)
		defer stopContainer(t, client, ctx, containerID)

		_, errorMsg, exitCode := execContainer(t, client, ctx, containerID, execCommand)

		// The dir command will return File Not Found error if the directory is empty.
		// Don't fail the test if that happens. It is expected behaviour in this case.
		if exitCode != 0 && !strings.Contains(errorMsg, "File Not Found") {
			t.Fatalf("Exec into container failed with: %v and exit code: %d, %s", errorMsg, exitCode, containerID)
		}
	}

	// For the 3rd container don't add any mounts
	// this makes sure you can have containers that share vhd mounts and
	// at the same time containers in a pod that don't have any mounts
	mounts = []*runtime.Mount{}
	containerName := t.Name() + "-Container-3"
	containerID := createContainerInSandbox(t, client,
		ctx, podID, containerName, imageWindowsNanoserver,
		command, annots, mounts, sbRequest.Config)
	defer removeContainer(t, client, ctx, containerID)

	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	output, errorMsg, exitCode := execContainer(t, client, ctx, containerID, execCommand)

	// 3rd container should not have the mount and ls should fail
	if exitCode != 0 && !strings.Contains(errorMsg, "File Not Found") {
		t.Fatalf("Exec into container failed: %v and exit code: %s, %s", errorMsg, output, containerID)
	}
}

func Test_RunPodSandbox_ProcessDump_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpineCoreDump})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sbRequest := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.ContainerProcessDumpLocation: "/coredumps/core",
		}),
	)

	podID := runPodSandbox(t, client, ctx, sbRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	mounts := []*runtime.Mount{
		{
			HostPath:      "sandbox:///coredump",
			ContainerPath: "/coredumps",
		},
	}

	annots := map[string]string{
		annotations.RLimitCore: "18446744073709551615;18446744073709551615",
	}

	// Setup container 1 that uses an image that stackoverflows shortly after starting.
	// This should generate a core dump file in the sandbox mount location
	c1Request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container1",
			},
			Image: &runtime.ImageSpec{
				Image: imageLcowAlpineCoreDump,
			},
			Command: []string{
				"./stackoverflow",
			},
			Annotations: annots,
			Mounts:      mounts,
		},
		PodSandboxId:  podID,
		SandboxConfig: sbRequest.Config,
	}

	container1ID := createContainer(t, client, ctx, c1Request)
	defer removeContainer(t, client, ctx, container1ID)

	startContainer(t, client, ctx, container1ID)
	defer stopContainer(t, client, ctx, container1ID)

	// Then setup a secondary container that will mount the same sandbox mount and
	// just verify that the core dump file is present.
	c2Request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container2",
			},
			Image: &runtime.ImageSpec{
				Image: imageLcowAlpineCoreDump,
			},
			// Hold this command open until killed
			Command: []string{
				"top",
			},
			Mounts: mounts,
		},
		PodSandboxId:  podID,
		SandboxConfig: sbRequest.Config,
	}

	container2ID := createContainer(t, client, ctx, c2Request)
	defer removeContainer(t, client, ctx, container2ID)

	startContainer(t, client, ctx, container2ID)
	defer stopContainer(t, client, ctx, container2ID)

	checkForDumpFile := func() error {
		// Check if the core dump file is present
		execCommand := []string{
			"ls",
			"/coredumps/core",
		}
		execRequest := &runtime.ExecSyncRequest{
			ContainerId: container2ID,
			Cmd:         execCommand,
			Timeout:     20,
		}

		r := execSync(t, client, ctx, execRequest)
		if r.ExitCode != 0 {
			return fmt.Errorf("failed with exit code %d running `ls`: %s", r.ExitCode, string(r.Stderr))
		}
		return nil
	}

	var (
		done    bool
		timeout = time.After(time.Second * 10)
	)
	for !done {
		// Keep checking for a core dump until timeout.
		select {
		case <-timeout:
			t.Fatal("failed to find core dump within timeout")
		default:
			if err := checkForDumpFile(); err == nil {
				done = true
			} else {
				time.Sleep(time.Millisecond * 500)
			}
		}
	}
}

func Test_RunPodSandbox_ProcessDump_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsProcessDump})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sbRequest := getRunPodSandboxRequest(
		t,
		wcowHypervisor19041RuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.ContainerProcessDumpLocation: "C:\\processdump",
		}),
	)

	podID := runPodSandbox(t, client, ctx, sbRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	mounts := []*runtime.Mount{
		{
			HostPath:      "sandbox:///processdump",
			ContainerPath: "C:\\processdump",
		},
	}

	// Setup container 1 that uses an image that throws a user exception shortly after starting.
	// This should generate a process dump file in the sandbox mount location
	c1Request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container1",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsProcessDump,
			},
			Command: []string{
				"C:\\app\\crashtest.exe",
				"ue",
			},
			Mounts: mounts,
		},
		PodSandboxId:  podID,
		SandboxConfig: sbRequest.Config,
	}

	container1ID := createContainer(t, client, ctx, c1Request)
	defer removeContainer(t, client, ctx, container1ID)

	startContainer(t, client, ctx, container1ID)
	defer stopContainer(t, client, ctx, container1ID)

	// Then setup a secondary container that will mount the same sandbox mount and
	// just verify that the process dump file is present.
	c2Request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container2",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsProcessDump,
			},
			// Hold this command open until killed
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
			Mounts: mounts,
		},
		PodSandboxId:  podID,
		SandboxConfig: sbRequest.Config,
	}

	container2ID := createContainer(t, client, ctx, c2Request)
	defer removeContainer(t, client, ctx, container2ID)

	startContainer(t, client, ctx, container2ID)
	defer stopContainer(t, client, ctx, container2ID)

	checkForDumpFile := func() error {
		// Check if the core dump file is present
		execCommand := []string{
			"cmd",
			"/c",
			"dir",
			"C:\\processdump",
		}
		execRequest := &runtime.ExecSyncRequest{
			ContainerId: container2ID,
			Cmd:         execCommand,
			Timeout:     20,
		}

		r := execSync(t, client, ctx, execRequest)
		if r.ExitCode != 0 {
			return fmt.Errorf("failed with exit code %d running `dir`: %s", r.ExitCode, string(r.Stderr))
		}

		if !strings.Contains(string(r.Stdout), ".dmp") {
			return fmt.Errorf("expected dmp file to be present in the directory, got: %s", string(r.Stdout))
		}
		return nil
	}

	var (
		done    bool
		timeout = time.After(time.Second * 15)
	)
	for !done {
		// Keep checking for a dump file until timeout.
		select {
		case <-timeout:
			t.Fatal("failed to find dump file before timeout")
		default:
			if err := checkForDumpFile(); err == nil {
				done = true
			} else {
				time.Sleep(time.Second * 1)
			}
		}
	}
}

func Test_RunPodSandbox_Timezone_Inherit_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsTimezone})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sbRequest := getRunPodSandboxRequest(
		t,
		wcowHypervisor17763RuntimeHandler,
	)

	podID := runPodSandbox(t, client, ctx, sbRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	cRequest := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsTimezone,
			},
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
		},
		PodSandboxId:  podID,
		SandboxConfig: sbRequest.Config,
	}

	containerID := createContainer(t, client, ctx, cRequest)
	defer removeContainer(t, client, ctx, containerID)

	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	// Run the binary in the image that simply prints the standard name of the time zone
	execCommand := []string{
		"C:\\go\\src\\timezone\\timezone.exe",
	}
	execRequest := &runtime.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         execCommand,
		Timeout:     20,
	}

	var tz windows.Timezoneinformation
	_, err := windows.GetTimeZoneInformation(&tz)
	if err != nil {
		t.Fatal(err)
	}
	tzStd := windows.UTF16ToString(tz.StandardName[:])

	r := execSync(t, client, ctx, execRequest)
	if r.ExitCode != 0 {
		t.Fatalf("failed with exit code %d: %s", r.ExitCode, string(r.Stderr))
	}

	if string(r.Stdout) != tzStd {
		t.Fatalf("expected %s for time zone, got: %s", tzStd, string(r.Stdout))
	}
}

func Test_RunPodSandbox_Timezone_NoInherit_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsTimezone})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sbRequest := getRunPodSandboxRequest(
		t,
		wcowHypervisor17763RuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.NoInheritHostTimezone: "true",
		}),
	)

	podID := runPodSandbox(t, client, ctx, sbRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	cRequest := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsTimezone,
			},
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
		},
		PodSandboxId:  podID,
		SandboxConfig: sbRequest.Config,
	}

	containerID := createContainer(t, client, ctx, cRequest)
	defer removeContainer(t, client, ctx, containerID)

	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	// Run the binary in the image that simply prints the standard name of the time zone
	execCommand := []string{
		"C:\\go\\src\\timezone\\timezone.exe",
	}
	execRequest := &runtime.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         execCommand,
		Timeout:     20,
	}

	r := execSync(t, client, ctx, execRequest)
	if r.ExitCode != 0 {
		t.Fatalf("failed with exit code %d: %s", r.ExitCode, string(r.Stderr))
	}

	if string(r.Stdout) != "Coordinated Universal Time" {
		t.Fatalf("expected 'Coordinated Universal Time' for time zone, got: %s", string(r.Stdout))
	}
}

func createSandboxContainerAndExecForCustomScratch(t *testing.T, annots map[string]string) (string, string, int) {
	t.Helper()
	cmd := []string{
		"df",
	}
	return createSandboxContainerAndExec(t, annots, nil, cmd)
}

func createContainerInSandbox(
	t *testing.T,
	client runtime.RuntimeServiceClient,
	ctx context.Context,
	podID, containerName, imageName string,
	command []string,
	annots map[string]string,
	mounts []*runtime.Mount,
	podConfig *runtime.PodSandboxConfig,
) string {
	t.Helper()
	cRequest := getCreateContainerRequest(podID, containerName, imageName, command, podConfig)
	cRequest.Config.Annotations = annots
	cRequest.Config.Mounts = mounts

	containerID := createContainer(t, client, ctx, cRequest)

	return containerID
}

func execContainer(
	t *testing.T,
	client runtime.RuntimeServiceClient,
	ctx context.Context,
	containerID string,
	command []string,
) (string, string, int) {
	t.Helper()
	execRequest := &runtime.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         command,
		Timeout:     20,
	}

	r := execSync(t, client, ctx, execRequest)
	output := strings.TrimSpace(string(r.Stdout))
	errorMsg := string(r.Stderr)
	exitCode := int(r.ExitCode)

	return output, errorMsg, exitCode
}

func createSandboxContainerAndExec(t *testing.T, annots map[string]string, mounts []*runtime.Mount, execCommand []string) (output string, errorMsg string, exitCode int) {
	t.Helper()
	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sbRequest := getRunPodSandboxRequest(t, lcowRuntimeHandler, WithSandboxAnnotations(annots))

	podID := runPodSandbox(t, client, ctx, sbRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	testMounts := []*runtime.Mount{}

	if mounts != nil {
		testMounts = mounts
	}

	cRequest := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageLcowAlpine,
			},
			// Hold this command open until killed
			Command: []string{
				"top",
			},
			Annotations: annots,
			Mounts:      testMounts,
		},
		PodSandboxId:  podID,
		SandboxConfig: sbRequest.Config,
	}

	containerID := createContainer(t, client, ctx, cRequest)
	defer removeContainer(t, client, ctx, containerID)

	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	//exec request
	execRequest := &runtime.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         execCommand,
		Timeout:     20,
	}

	r := execSync(t, client, ctx, execRequest)
	output = strings.TrimSpace(string(r.Stdout))
	errorMsg = string(r.Stderr)
	exitCode = int(r.ExitCode)

	return output, errorMsg, exitCode
}

func Test_RunPodSandbox_KernelOptions_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	annots := map[string]string{
		annotations.FullyPhysicallyBacked: "true",
		annotations.MemorySizeInMB:        "2048",
		annotations.KernelBootOptions:     "hugepagesz=2M hugepages=10",
	}

	hugePagesCmd := []string{"grep", "-i", "HugePages_Total", "/proc/meminfo"}
	output, errorMsg, exitCode := createSandboxContainerAndExec(t, annots, nil, hugePagesCmd)

	if exitCode != 0 {
		t.Fatalf("Exec into container failed with: %v and exit code: %d, %s", errorMsg, exitCode, t.Name())
	}

	splitOutput := strings.Split(output, ":")
	numOfHugePages, err := strconv.Atoi(strings.TrimSpace(splitOutput[1]))
	if err != nil {
		t.Fatalf("Error happened while extracting number of hugepages: %v from output : %s", err, output)
	}

	if numOfHugePages != 10 {
		t.Fatalf("Expected number of hugepages to be 10. Got output instead: %d", numOfHugePages)
	}
}

func Test_RunPodSandbox_TimeSyncService(t *testing.T) {
	requireFeatures(t, featureLCOW)

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler)

	podID := runPodSandbox(t, client, ctx, request)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	shimName := fmt.Sprintf("k8s.io-%s", podID)

	shim, err := shimdiag.GetShim(shimName)
	if err != nil {
		t.Fatalf("failed to find shim %s: %s", shimName, err)
	}

	psCmd := []string{"ps"}
	shimClient := shimdiag.NewShimDiagClient(shim)
	outBuf := bytes.Buffer{}
	outw := bufio.NewWriter(&outBuf)
	errBuf := bytes.Buffer{}
	errw := bufio.NewWriter(&errBuf)
	exitCode, err := execInHost(ctx, shimClient, psCmd, nil, outw, errw)
	if err != nil {
		t.Fatalf("failed to exec `%s` in the uvm with %s", psCmd[0], err)
	}
	if exitCode != 0 {
		t.Fatalf("exec `%s` in the uvm failed with exit code: %d, std error: %s", psCmd[0], exitCode, errBuf.String())
	}
	if !strings.Contains(outBuf.String(), "chronyd") {
		t.Logf("standard output of exec %s is: %s\n", psCmd[0], outBuf.String())
		t.Fatalf("chronyd is not running inside the uvm")
	}
}

func Test_RunPodSandbox_DisableTimeSyncService(t *testing.T) {
	requireFeatures(t, featureLCOW)

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		WithSandboxAnnotations(
			map[string]string{
				annotations.DisableLCOWTimeSyncService: "true",
			}),
	)

	podID := runPodSandbox(t, client, ctx, request)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	shimName := fmt.Sprintf("k8s.io-%s", podID)

	shim, err := shimdiag.GetShim(shimName)
	if err != nil {
		t.Fatalf("failed to find shim %s: %s", shimName, err)
	}

	psCmd := []string{"ps"}
	shimClient := shimdiag.NewShimDiagClient(shim)
	outBuf := bytes.Buffer{}
	outw := bufio.NewWriter(&outBuf)
	errBuf := bytes.Buffer{}
	errw := bufio.NewWriter(&errBuf)
	exitCode, err := execInHost(ctx, shimClient, psCmd, nil, outw, errw)
	if err != nil {
		t.Fatalf("failed to exec `%s` in the uvm with %s", psCmd[0], err)
	}
	if exitCode != 0 {
		t.Fatalf("exec `%s` in the uvm failed with exit code: %d, std error: %s", psCmd[0], exitCode, errBuf.String())
	}
	if strings.Contains(outBuf.String(), "chronyd") {
		t.Logf("standard output of exec %s is: %s\n", psCmd[0], outBuf.String())
		t.Fatalf("chronyd should not be running inside the uvm")
	}
}
