// +build functional

package cri_containerd

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	runtime "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
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
	pullRequiredImages(t, []string{imageWindowsRS5Nanoserver})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
		},
		RuntimeHandler: wcowProcessRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_WCOW_Hypervisor(t *testing.T) {
	pullRequiredImages(t, []string{imageWindowsRS5Nanoserver})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
		},
		RuntimeHandler: wcowHypervisorRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_LCOW(t *testing.T) {
	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
		},
		RuntimeHandler: lcowRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_VirtualMemory_WCOW_Hypervisor(t *testing.T) {
	pullRequiredImages(t, []string{imageWindowsRS5Nanoserver})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.virtualmachine.computetopology.memory.allowovercommit": "true",
			},
		},
		RuntimeHandler: wcowHypervisorRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_VirtualMemory_LCOW(t *testing.T) {
	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.virtualmachine.computetopology.memory.allowovercommit": "true",
			},
		},
		RuntimeHandler: lcowRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_VirtualMemory_DeferredCommit_WCOW_Hypervisor(t *testing.T) {
	pullRequiredImages(t, []string{imageWindowsRS5Nanoserver})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.virtualmachine.computetopology.memory.allowovercommit":      "true",
				"io.microsoft.virtualmachine.computetopology.memory.enabledeferredcommit": "true",
			},
		},
		RuntimeHandler: wcowHypervisorRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_VirtualMemory_DeferredCommit_LCOW(t *testing.T) {
	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.virtualmachine.computetopology.memory.allowovercommit":      "true",
				"io.microsoft.virtualmachine.computetopology.memory.enabledeferredcommit": "true",
			},
		},
		RuntimeHandler: lcowRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_PhysicalMemory_WCOW_Hypervisor(t *testing.T) {
	pullRequiredImages(t, []string{imageWindowsRS5Nanoserver})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.virtualmachine.computetopology.memory.allowovercommit": "false",
			},
		},
		RuntimeHandler: wcowHypervisorRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_PhysicalMemory_LCOW(t *testing.T) {
	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.virtualmachine.computetopology.memory.allowovercommit": "false",
			},
		},
		RuntimeHandler: lcowRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_MemorySize_WCOW_Process(t *testing.T) {
	pullRequiredImages(t, []string{imageWindowsRS5Nanoserver})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.container.memory.sizeinmb": "128",
			},
		},
		RuntimeHandler: wcowProcessRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_MemorySize_WCOW_Hypervisor(t *testing.T) {
	pullRequiredImages(t, []string{imageWindowsRS5Nanoserver})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.virtualmachine.computetopology.memory.sizeinmb": "768", // 128 is too small for WCOW. It is really slow boot.
			},
		},
		RuntimeHandler: wcowHypervisorRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_MemorySize_LCOW(t *testing.T) {
	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.virtualmachine.computetopology.memory.sizeinmb": "200",
			},
		},
		RuntimeHandler: lcowRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPUCount_WCOW_Process(t *testing.T) {
	pullRequiredImages(t, []string{imageWindowsRS5Nanoserver})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.container.processor.count": "1",
			},
		},
		RuntimeHandler: wcowProcessRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPUCount_WCOW_Hypervisor(t *testing.T) {
	pullRequiredImages(t, []string{imageWindowsRS5Nanoserver})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.virtualmachine.computetopology.processor.count": "1",
			},
		},
		RuntimeHandler: wcowHypervisorRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPUCount_LCOW(t *testing.T) {
	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.virtualmachine.computetopology.processor.count": "1",
			},
		},
		RuntimeHandler: lcowRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPULimit_WCOW_Process(t *testing.T) {
	pullRequiredImages(t, []string{imageWindowsRS5Nanoserver})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.container.processor.limit": "9000",
			},
		},
		RuntimeHandler: wcowProcessRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPULimit_WCOW_Hypervisor(t *testing.T) {
	pullRequiredImages(t, []string{imageWindowsRS5Nanoserver})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.virtualmachine.computetopology.processor.limit": "90000",
			},
		},
		RuntimeHandler: wcowHypervisorRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPULimit_LCOW(t *testing.T) {
	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.virtualmachine.computetopology.processor.limit": "90000",
			},
		},
		RuntimeHandler: lcowRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPUWeight_WCOW_Process(t *testing.T) {
	pullRequiredImages(t, []string{imageWindowsRS5Nanoserver})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.container.processor.weight": "500",
			},
		},
		RuntimeHandler: wcowProcessRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPUWeight_WCOW_Hypervisor(t *testing.T) {
	pullRequiredImages(t, []string{imageWindowsRS5Nanoserver})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.virtualmachine.computetopology.processor.weight": "500",
			},
		},
		RuntimeHandler: wcowHypervisorRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CPUWeight_LCOW(t *testing.T) {
	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.virtualmachine.computetopology.processor.weight": "500",
			},
		},
		RuntimeHandler: lcowRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_StorageQoSBandwithMax_WCOW_Process(t *testing.T) {
	pullRequiredImages(t, []string{imageWindowsRS5Nanoserver})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.container.storage.qos.bandwidthmaximum": fmt.Sprintf("%d", 1024*1024), // 1MB/s
			},
		},
		RuntimeHandler: wcowProcessRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_StorageQoSBandwithMax_WCOW_Hypervisor(t *testing.T) {
	pullRequiredImages(t, []string{imageWindowsRS5Nanoserver})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.virtualmachine.storageqos.bandwidthmaximum": fmt.Sprintf("%d", 1024*1024), // 1MB/s
			},
		},
		RuntimeHandler: wcowHypervisorRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_StorageQoSBandwithMax_LCOW(t *testing.T) {
	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.virtualmachine.storageqos.bandwidthmaximum": fmt.Sprintf("%d", 1024*1024), // 1MB/s
			},
		},
		RuntimeHandler: lcowRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_StorageQoSIopsMax_WCOW_Process(t *testing.T) {
	pullRequiredImages(t, []string{imageWindowsRS5Nanoserver})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.container.storage.qos.iopsmaximum": "300",
			},
		},
		RuntimeHandler: wcowProcessRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_StorageQoSIopsMax_WCOW_Hypervisor(t *testing.T) {
	pullRequiredImages(t, []string{imageWindowsRS5Nanoserver})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.virtualmachine.storageqos.iopsmaximum": "300",
			},
		},
		RuntimeHandler: wcowHypervisorRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_StorageQoSIopsMax_LCOW(t *testing.T) {
	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.virtualmachine.storageqos.iopsmaximum": "300",
			},
		},
		RuntimeHandler: lcowRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_InitrdBoot_LCOW(t *testing.T) {
	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.virtualmachine.lcow.preferredrootfstype": "initrd",
			},
		},
		RuntimeHandler: lcowRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_RootfsVhdBoot_LCOW(t *testing.T) {
	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.virtualmachine.lcow.preferredrootfstype": "vhd",
			},
		},
		RuntimeHandler: lcowRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_DnsConfig_WCOW_Process(t *testing.T) {
	pullRequiredImages(t, []string{imageWindowsRS5Nanoserver})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			DnsConfig: &runtime.DNSConfig{
				Searches: []string{"8.8.8.8", "8.8.4.4"},
			},
		},
		RuntimeHandler: wcowProcessRuntimeHandler,
	}
	runPodSandboxTest(t, request)
	// TODO: JTERRY75 - This is just a boot test at present. We need to create a
	// container, exec the ipconfig and parse the results to verify that the
	// searches are set.
}

func Test_RunPodSandbox_DnsConfig_WCOW_Hypervisor(t *testing.T) {
	pullRequiredImages(t, []string{imageWindowsRS5Nanoserver})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			DnsConfig: &runtime.DNSConfig{
				Searches: []string{"8.8.8.8", "8.8.4.4"},
			},
		},
		RuntimeHandler: wcowHypervisorRuntimeHandler,
	}
	runPodSandboxTest(t, request)
	// TODO: JTERRY75 - This is just a boot test at present. We need to create a
	// container, exec the ipconfig and parse the results to verify that the
	// searches are set.
}

func Test_RunPodSandbox_DnsConfig_LCOW(t *testing.T) {
	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			DnsConfig: &runtime.DNSConfig{
				Searches: []string{"8.8.8.8", "8.8.4.4"},
			},
		},
		RuntimeHandler: lcowRuntimeHandler,
	}
	runPodSandboxTest(t, request)
	// TODO: JTERRY75 - This is just a boot test at present. We need to create a
	// container, cat /etc/resolv.conf and parse the results to verify that the
	// searches are set.
}

func Test_RunPodSandbox_PortMappings_WCOW_Process(t *testing.T) {
	pullRequiredImages(t, []string{imageWindowsRS5Nanoserver})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			PortMappings: []*runtime.PortMapping{
				{
					Protocol:      runtime.Protocol_TCP,
					ContainerPort: 80,
					HostPort:      8080,
				},
			},
		},
		RuntimeHandler: wcowProcessRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_PortMappings_WCOW_Hypervisor(t *testing.T) {
	pullRequiredImages(t, []string{imageWindowsRS5Nanoserver})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			PortMappings: []*runtime.PortMapping{
				{
					Protocol:      runtime.Protocol_TCP,
					ContainerPort: 80,
					HostPort:      8080,
				},
			},
		},
		RuntimeHandler: wcowHypervisorRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_PortMappings_LCOW(t *testing.T) {
	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	request := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			PortMappings: []*runtime.PortMapping{
				{
					Protocol:      runtime.Protocol_TCP,
					ContainerPort: 80,
					HostPort:      8080,
				},
			},
		},
		RuntimeHandler: lcowRuntimeHandler,
	}
	runPodSandboxTest(t, request)
}

func Test_RunPodSandbox_CustomizableScratchDefaultSize_LCOW(t *testing.T) {
	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	annotations := map[string]string{
		"io.microsoft.virtualmachine.computetopology.memory.allowovercommit": "true",
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
	pullRequiredLcowImages(t, []string{imageLcowK8sPause})

	annotations := map[string]string{
		"io.microsoft.virtualmachine.computetopology.memory.allowovercommit":   "true",
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
	pullRequiredLcowImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	annotations := map[string]string{
		"io.microsoft.virtualmachine.computetopology.memory.allowovercommit": "true",
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

func createSandboxContainerAndExecForCustomScratch(t *testing.T, annotations map[string]string) (string, string, int) {
	cmd := []string{
		"df",
	}
	return createSandboxContainerAndExec(t, annotations, nil, cmd)
}

func createSandboxContainerAndExec(t *testing.T, annotations map[string]string, mounts []*runtime.Mount, execCommand []string) (output string, errorMsg string, exitCode int) {
	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sbRequest := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: annotations,
		},
		RuntimeHandler: lcowRuntimeHandler,
	}

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
