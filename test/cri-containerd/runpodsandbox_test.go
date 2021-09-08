// +build functional

package cri_containerd

import (
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/internal/cpugroup"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/lcow"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/processorinfo"
	"github.com/Microsoft/hcsshim/osversion"
	testutilities "github.com/Microsoft/hcsshim/test/functional/utilities"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func runPodSandboxTest(t *testing.T, request *runtime.RunPodSandboxRequest) {
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

	request := getRunPodSandboxRequest(t, wcowProcessRuntimeHandler, nil)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(t, wcowHypervisorRuntimeHandler, nil)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(t, lcowRuntimeHandler, nil)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_Events_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	podctx, podcancel := context.WithCancel(context.Background())
	defer podcancel()

	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(t, lcowRuntimeHandler, nil)

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
		map[string]string{
			oci.AnnotationAllowOvercommit: "true",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_VirtualMemory_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		map[string]string{
			oci.AnnotationAllowOvercommit: "true",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_VirtualMemory_DeferredCommit_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowHypervisorRuntimeHandler,
		map[string]string{
			oci.AnnotationAllowOvercommit:      "true",
			oci.AnnotationEnableDeferredCommit: "true",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_VirtualMemory_DeferredCommit_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		map[string]string{
			oci.AnnotationAllowOvercommit:      "true",
			oci.AnnotationEnableDeferredCommit: "true",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_PhysicalMemory_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowHypervisorRuntimeHandler,
		map[string]string{
			oci.AnnotationAllowOvercommit: "false",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_FullyPhysicallyBacked_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowHypervisorRuntimeHandler,
		map[string]string{
			oci.AnnotationFullyPhysicallyBacked: "true",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_VSMBNoDirectMap_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowHypervisorRuntimeHandler,
		map[string]string{
			oci.AnnotationVSMBNoDirectMap: "true",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_PhysicalMemory_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		map[string]string{
			oci.AnnotationAllowOvercommit: "false",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_FullyPhysicallyBacked_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		map[string]string{
			oci.AnnotationFullyPhysicallyBacked: "true",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_MemorySize_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowProcessRuntimeHandler,
		map[string]string{
			oci.AnnotationContainerMemorySizeInMB: "128",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_MemorySize_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowHypervisorRuntimeHandler,
		map[string]string{
			oci.AnnotationMemorySizeInMB: "768", // 128 is too small for WCOW. It is really slow boot.
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_MemorySize_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		map[string]string{
			oci.AnnotationMemorySizeInMB: "200",
		},
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
		map[string]string{
			oci.AnnotationMemoryLowMMIOGapInMB:   "100",
			oci.AnnotationMemoryHighMMIOBaseInMB: "100",
			oci.AnnotationMemoryHighMMIOGapInMB:  "100",
		},
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
		map[string]string{
			oci.AnnotationMemoryLowMMIOGapInMB:   "100",
			oci.AnnotationMemoryHighMMIOBaseInMB: "100",
			oci.AnnotationMemoryHighMMIOGapInMB:  "100",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_MMIO_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	if osversion.Build() < osversion.V20H1 {
		t.Skip("Requires build +20H1")
	}
	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		map[string]string{
			oci.AnnotationMemoryLowMMIOGapInMB:   "100",
			oci.AnnotationMemoryHighMMIOBaseInMB: "100",
			oci.AnnotationMemoryHighMMIOGapInMB:  "100",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPUCount_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowProcessRuntimeHandler,
		map[string]string{
			oci.AnnotationContainerProcessorCount: "1",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPUCount_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowHypervisorRuntimeHandler,
		map[string]string{
			oci.AnnotationProcessorCount: "1",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPUCount_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		map[string]string{
			oci.AnnotationProcessorCount: "1",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPULimit_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowProcessRuntimeHandler,
		map[string]string{
			oci.AnnotationContainerProcessorLimit: "9000",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPULimit_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowHypervisorRuntimeHandler,
		map[string]string{
			oci.AnnotationProcessorLimit: "90000",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPULimit_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		map[string]string{
			oci.AnnotationProcessorLimit: "90000",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPUWeight_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowProcessRuntimeHandler,
		map[string]string{
			oci.AnnotationContainerProcessorWeight: "500",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPUWeight_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowHypervisorRuntimeHandler,
		map[string]string{
			oci.AnnotationContainerProcessorWeight: "500",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPUWeight_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		map[string]string{
			oci.AnnotationProcessorWeight: "500",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_StorageQoSBandwithMax_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowProcessRuntimeHandler,
		map[string]string{
			oci.AnnotationContainerStorageQoSBandwidthMaximum: fmt.Sprintf("%d", 1024*1024), // 1MB/s
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_StorageQoSBandwithMax_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowHypervisorRuntimeHandler,
		map[string]string{
			oci.AnnotationStorageQoSBandwidthMaximum: fmt.Sprintf("%d", 1024*1024), // 1MB/s
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_StorageQoSBandwithMax_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		map[string]string{
			oci.AnnotationStorageQoSBandwidthMaximum: fmt.Sprintf("%d", 1024*1024), // 1MB/s
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_StorageQoSIopsMax_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowProcessRuntimeHandler,
		map[string]string{
			oci.AnnotationContainerStorageQoSIopsMaximum: "300",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_StorageQoSIopsMax_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(
		t,
		wcowHypervisorRuntimeHandler,
		map[string]string{
			oci.AnnotationStorageQoSIopsMaximum: "300",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_StorageQoSIopsMax_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		map[string]string{
			oci.AnnotationStorageQoSIopsMaximum: "300",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_InitrdBoot_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		map[string]string{
			oci.AnnotationPreferredRootFSType: "initrd",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_RootfsVhdBoot_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		map[string]string{
			oci.AnnotationPreferredRootFSType: "vhd",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_VPCIEnabled_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		map[string]string{
			oci.AnnotationVPCIEnabled: "true",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_UEFIBoot_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		map[string]string{
			oci.AnnotationKernelDirectBoot: "false",
		},
	)
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_DnsConfig_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := getRunPodSandboxRequest(t, wcowProcessRuntimeHandler, nil)
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

	request := getRunPodSandboxRequest(t, wcowHypervisorRuntimeHandler, nil)
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

	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(t, lcowRuntimeHandler, nil)
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

	request := getRunPodSandboxRequest(t, wcowProcessRuntimeHandler, nil)
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

	request := getRunPodSandboxRequest(t, wcowHypervisorRuntimeHandler, nil)
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

	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := getRunPodSandboxRequest(t, lcowRuntimeHandler, nil)
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

	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	annotations := map[string]string{
		oci.AnnotationAllowOvercommit: "true",
	}

	output, errorMsg, exitCode := createSandboxContainerAndExecForCustomScratch(t, annotations)

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

	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	annotations := map[string]string{
		oci.AnnotationAllowOvercommit:                                          "true",
		"containerd.io/snapshot/io.microsoft.container.storage.rootfs.size-gb": "200",
	}

	output, errorMsg, exitCode := createSandboxContainerAndExecForCustomScratch(t, annotations)

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

	pullRequiredLcowImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	annotations := map[string]string{
		oci.AnnotationAllowOvercommit: "true",
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

	output, errorMsg, exitCode := createSandboxContainerAndExec(t, annotations, mounts, cmd)

	if exitCode != 0 {
		t.Fatalf("Exec into container failed with: %v and exit code: %d, %s", errorMsg, exitCode, t.Name())
	}

	t.Log(output)

	//TODO: Parse the output of the exec command to make sure the uvm mount was successful
}

func Test_RunPodSandbox_CPUGroup(t *testing.T) {
	testutilities.RequiresBuild(t, 20124)
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
			pullRequiredLcowImages(t, []string{test.sandboxImage})
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
					oci.AnnotationCPUGroupID: presentID,
				},
			},
			RuntimeHandler: test.runtimeHandler,
		}
		runPodSandboxTest(t, request)
	}
}

func createExt4VHD(ctx context.Context, t *testing.T, path string) {
	uvm := testutilities.CreateLCOWUVM(ctx, t, t.Name()+"-createExt4VHD")
	defer uvm.Close()

	if err := lcow.CreateScratch(ctx, uvm, path, 2, ""); err != nil {
		t.Fatal(err)
	}
}

func Test_RunPodSandbox_MultipleContainersSameVhd_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	annotations := map[string]string{
		oci.AnnotationAllowOvercommit: "true",
	}

	// Create a temporary ext4 VHD to mount into the container.
	vhdHostDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("failed to create temp directory: %s", err)
	}
	defer os.RemoveAll(vhdHostDir)
	vhdHostPath := filepath.Join(vhdHostDir, "temp.vhdx")
	createExt4VHD(ctx, t, vhdHostPath)

	vhdContainerPath := "/containerDir"

	mounts := []*runtime.Mount{
		{
			HostPath:      "vhd://" + vhdHostPath,
			ContainerPath: vhdContainerPath,
		},
	}

	sbRequest := getRunPodSandboxRequest(t, lcowRuntimeHandler, annotations)

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
		containerId := createContainerInSandbox(t, client, ctx, podID, containerName, imageLcowAlpine, command, annotations, mounts, sbRequest.Config)
		defer removeContainer(t, client, ctx, containerId)

		startContainer(t, client, ctx, containerId)
		defer stopContainer(t, client, ctx, containerId)

		_, errorMsg, exitCode := execContainer(t, client, ctx, containerId, execCommand)

		// For container 1 and 2 we should find the mounts
		if exitCode != 0 {
			t.Fatalf("Exec into container failed with: %v and exit code: %d, %s", errorMsg, exitCode, containerId)
		}
	}

	// For the 3rd container don't add any mounts
	// this makes sure you can have containers that share vhd mounts and
	// at the same time containers in a pod that don't have any mounts
	mounts = []*runtime.Mount{}
	containerName := t.Name() + "-Container-3"
	containerId := createContainerInSandbox(t, client, ctx, podID, containerName, imageLcowAlpine, command, annotations, mounts, sbRequest.Config)
	defer removeContainer(t, client, ctx, containerId)

	startContainer(t, client, ctx, containerId)
	defer stopContainer(t, client, ctx, containerId)

	output, errorMsg, exitCode := execContainer(t, client, ctx, containerId, execCommand)

	// 3rd container should not have the mount and ls should fail
	if exitCode == 0 {
		t.Fatalf("Exec into container succeeded but we expected it to fail: %v and exit code: %s, %s", errorMsg, output, containerId)
	}
}

func Test_RunPodSandbox_MultipleContainersSameVhd_RShared_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sbRequest := getRunPodSandboxRequest(t, lcowRuntimeHandler, nil)
	sbRequest.Config.Linux = &runtime.LinuxPodSandboxConfig{
		SecurityContext: &runtime.LinuxSandboxSecurityContext{
			Privileged: true,
		},
	}

	podID := runPodSandbox(t, client, ctx, sbRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	// Create a temporary ext4 VHD to mount into the container.
	vhdHostDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("failed to create temp directory: %s", err)
	}
	defer os.RemoveAll(vhdHostDir)
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
	containerId0 := createContainer(t, client, ctx, cRequest)
	defer removeContainer(t, client, ctx, containerId0)
	startContainer(t, client, ctx, containerId0)
	defer stopContainer(t, client, ctx, containerId0)

	containerName1 := t.Name() + "-Container-1"
	cRequest.Config.Metadata.Name = containerName1
	containerId1 := createContainer(t, client, ctx, cRequest)
	defer removeContainer(t, client, ctx, containerId1)
	startContainer(t, client, ctx, containerId1)
	defer stopContainer(t, client, ctx, containerId1)

	// create a test directory that will be the new mountpoint's source
	createTestDirCmd := []string{
		"mkdir",
		"/tmp/testdir",
	}
	_, errorMsg, exitCode := execContainer(t, client, ctx, containerId0, createTestDirCmd)
	if exitCode != 0 {
		t.Fatalf("Exec into container failed with: %v and exit code: %d, %s", errorMsg, exitCode, containerId0)
	}

	// create a file in the test directory
	createTestDirContentCmd := []string{
		"touch",
		"/tmp/testdir/test.txt",
	}
	_, errorMsg, exitCode = execContainer(t, client, ctx, containerId0, createTestDirContentCmd)
	if exitCode != 0 {
		t.Fatalf("Exec into container failed with: %v and exit code: %d, %s", errorMsg, exitCode, containerId0)
	}

	// create a test directory in the vhd that will be the new mountpoint's destination
	createTestDirVhdCmd := []string{
		"mkdir",
		fmt.Sprintf("%s/testdir", vhdContainerPath),
	}
	_, errorMsg, exitCode = execContainer(t, client, ctx, containerId0, createTestDirVhdCmd)
	if exitCode != 0 {
		t.Fatalf("Exec into container failed with: %v and exit code: %d, %s", errorMsg, exitCode, containerId0)
	}

	// perform rshared mount of test directory into the vhd
	mountTestDirToVhdCmd := []string{
		"mount",
		"-o",
		"rshared",
		"/tmp/testdir",
		fmt.Sprintf("%s/testdir", vhdContainerPath),
	}
	_, errorMsg, exitCode = execContainer(t, client, ctx, containerId0, mountTestDirToVhdCmd)
	if exitCode != 0 {
		t.Fatalf("Exec into container failed with: %v and exit code: %d, %s", errorMsg, exitCode, containerId0)
	}

	// try to list the test file in the second container to verify it was propagated correctly
	verifyTestMountCommand := []string{
		"ls",
		fmt.Sprintf("%s/testdir/test.txt", vhdContainerPath),
	}
	_, errorMsg, exitCode = execContainer(t, client, ctx, containerId1, verifyTestMountCommand)
	if exitCode != 0 {
		t.Fatalf("Exec into container failed with: %v and exit code: %d, %s", errorMsg, exitCode, containerId1)
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

	annotations := map[string]string{
		oci.AnnotationAllowOvercommit: "true",
	}

	vhdHostDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("failed to create temporary directory: %s", err)
	}
	defer os.RemoveAll(vhdHostDir)

	vhdHostPath := filepath.Join(vhdHostDir, "temp.vhdx")

	if err = hcs.CreateNTFSVHD(ctx, vhdHostPath, 10); err != nil {
		t.Fatalf("failed to create NTFS VHD: %s", err)
	}

	vhdContainerPath := "C:\\containerDir"

	mounts := []*runtime.Mount{
		{
			HostPath:      "vhd://" + vhdHostPath,
			ContainerPath: vhdContainerPath,
		},
	}

	sbRequest := getRunPodSandboxRequest(t, wcowHypervisorRuntimeHandler, annotations)

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
		containerId := createContainerInSandbox(t, client, ctx, podID, containerName, imageWindowsNanoserver, command, annotations, mounts, sbRequest.Config)
		defer removeContainer(t, client, ctx, containerId)

		startContainer(t, client, ctx, containerId)
		defer stopContainer(t, client, ctx, containerId)

		_, errorMsg, exitCode := execContainer(t, client, ctx, containerId, execCommand)

		// The dir command will return File Not Found error if the directory is empty.
		// Don't fail the test if that happens. It is expected behaviour in this case.
		if exitCode != 0 && !strings.Contains(errorMsg, "File Not Found") {
			t.Fatalf("Exec into container failed with: %v and exit code: %d, %s", errorMsg, exitCode, containerId)
		}
	}

	// For the 3rd container don't add any mounts
	// this makes sure you can have containers that share vhd mounts and
	// at the same time containers in a pod that don't have any mounts
	mounts = []*runtime.Mount{}
	containerName := t.Name() + "-Container-3"
	containerId := createContainerInSandbox(t, client, ctx, podID, containerName, imageWindowsNanoserver, command, annotations, mounts, sbRequest.Config)
	defer removeContainer(t, client, ctx, containerId)

	startContainer(t, client, ctx, containerId)
	defer stopContainer(t, client, ctx, containerId)

	output, errorMsg, exitCode := execContainer(t, client, ctx, containerId, execCommand)

	// 3rd container should not have the mount and ls should fail
	if exitCode != 0 && !strings.Contains(errorMsg, "File Not Found") {
		t.Fatalf("Exec into container failed: %v and exit code: %s, %s", errorMsg, output, containerId)
	}
}

func createSandboxContainerAndExecForCustomScratch(t *testing.T, annotations map[string]string) (string, string, int) {
	cmd := []string{
		"df",
	}
	return createSandboxContainerAndExec(t, annotations, nil, cmd)
}

func createContainerInSandbox(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, podId, containerName, imageName string, command []string,
	annotations map[string]string, mounts []*runtime.Mount, podConfig *runtime.PodSandboxConfig) string {

	cRequest := getCreateContainerRequest(podId, containerName, imageName, command, podConfig)
	cRequest.Config.Annotations = annotations
	cRequest.Config.Mounts = mounts

	containerID := createContainer(t, client, ctx, cRequest)

	return containerID
}

func execContainer(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, containerId string, command []string) (string, string, int) {
	execRequest := &runtime.ExecSyncRequest{
		ContainerId: containerId,
		Cmd:         command,
		Timeout:     20,
	}

	r := execSync(t, client, ctx, execRequest)
	output := strings.TrimSpace(string(r.Stdout))
	errorMsg := string(r.Stderr)
	exitCode := int(r.ExitCode)

	return output, errorMsg, exitCode
}

func createSandboxContainerAndExec(t *testing.T, annotations map[string]string, mounts []*runtime.Mount, execCommand []string) (output string, errorMsg string, exitCode int) {
	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sbRequest := getRunPodSandboxRequest(t, lcowRuntimeHandler, annotations)

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
			Annotations: annotations,
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

	pullRequiredLcowImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	annotations := map[string]string{
		oci.AnnotationFullyPhysicallyBacked: "true",
		oci.AnnotationMemorySizeInMB:        "2048",
		oci.AnnotationKernelBootOptions:     "hugepagesz=2M hugepages=10",
	}

	hugePagesCmd := []string{"grep", "-i", "HugePages_Total", "/proc/meminfo"}
	output, errorMsg, exitCode := createSandboxContainerAndExec(t, annotations, nil, hugePagesCmd)

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
