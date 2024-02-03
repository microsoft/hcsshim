//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"errors"
	"fmt"

	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"

	iannotations "github.com/Microsoft/hcsshim/internal/annotations"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/Microsoft/hcsshim/pkg/annotations"

	"github.com/Microsoft/hcsshim/test/pkg/definitions/cpugroup"
	"github.com/Microsoft/hcsshim/test/pkg/definitions/hcs"
	"github.com/Microsoft/hcsshim/test/pkg/definitions/processorinfo"

	"github.com/Microsoft/hcsshim/test/pkg/require"
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
	requireAnyFeature(t, featureWCOWHypervisor)
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
		if err != nil && !errors.Is(err, cpugroup.ErrHVStatusInvalidCPUGroupState) {
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
	}

	for _, test := range tests {
		requireFeatures(t, test.requiredFeatures...)
		pullRequiredImages(t, []string{test.sandboxImage})

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

func Test_RunPodSandbox_AdditionalRegValues_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	client := newTestRuntimeClient(t)
	ctx := context.Background()

	regHive := "System"
	regKey := `Software\Microsoft\hcsshim`
	regName := "TestKeyValueName"
	regValue := "test key value value"
	annot := fmt.Sprintf(
		`[ {"Key": {"Hive": %q, "Name": %q}, "Name": %q, "Type": "String", "StringValue":  %q} ]`,
		regHive, regKey, regName, regValue)
	t.Logf("registry annotation: %s", annot)
	sbRequest := getRunPodSandboxRequest(t, wcowHypervisorRuntimeHandler, WithSandboxAnnotations(
		map[string]string{
			iannotations.AdditionalRegistryValues: annot,
		},
	))
	podID := runPodSandbox(t, client, ctx, sbRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	// nanoserver doesn't come with a reg.exe, so share it (and its mui file) into the uVM

	// just in case, for some wild reason, host has a system root other than `C:\Windows`
	sys32 := os.Getenv("SystemRoot")
	if sys32 == "" {
		sys32 = `C:\Windows`
	}
	sys32 = filepath.Join(sys32, "System32")
	for _, f := range []string{`reg.exe`, `en-US\reg.exe.mui`} {
		shareInPod(ctx, t, podID, filepath.Join(sys32, f), filepath.Join(`C:\Windows\System32`, f), true)
	}

	out := shimDiagExecOutput(ctx, t, podID,
		[]string{
			"cmd.exe", "/c",
			fmt.Sprintf(`C:\Windows\System32\reg.exe query HKEY_LOCAL_MACHINE\%s\%s /v %s /t REG_SZ`, regHive, regKey, regName),
		},
	)

	if !strings.Contains(out, regValue) {
		t.Fatalf("registry %q value does not contain %q:\n%s", regHive+`\`+regName, regValue, out)
	}
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
