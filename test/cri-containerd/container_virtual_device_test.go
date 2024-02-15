//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/osversion"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

const containerDeviceUtilPath = "C:\\device-util.exe"
const gpuWin32InstanceIDPrefix = "PCI#VEN_10DE"

// makeGPUExecCommand constructs the container command to check for the
// existence of a nvidia GPU device and returns the command in an
// ExecSyncRequest
func makeGPUExecCommand(os string, containerID string) *runtime.ExecSyncRequest {
	cmd := []string{"ls", "/dev/nvidia0"}
	if os == "windows" {
		cmd = []string{containerDeviceUtilPath, "obj-dir"}
	}

	return &runtime.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         cmd,
		Timeout:     20,
	}
}

// verifyGPUIsPresent is a helper function that runs a command in the container
// to verify the existence of a GPU and fails the running test is none are found
func isGPUPresentWCOW(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, containerID string) bool {
	t.Helper()
	execReq := makeGPUExecCommand("windows", containerID)
	response := execSync(t, client, ctx, execReq)
	if len(response.Stderr) != 0 {
		t.Fatalf("expected to see no error, instead saw %s", string(response.Stderr))
	}
	out := string(response.Stdout)
	devices := strings.Split(out, ",")
	if len(devices) == 0 {
		t.Fatal("expected to see devices on container, none found")
	}
	for _, d := range devices {
		if strings.HasPrefix(d, gpuWin32InstanceIDPrefix) {
			return true
		}
	}
	return false
}

// findTestNvidiaGPUDevice returns the first nvidia pcip device on the host
func findTestNvidiaGPUDevice() (string, error) {
	out, err := exec.Command(
		"powershell",
		`(Get-PnpDevice -presentOnly | where-object {$_.InstanceID -Match 'PCIP\\VEN_10DE.*'})[0].InstanceId`,
	).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// findTestNvidiaGPULocationPath returns the location path of the first pci nvidia device on the host
func findTestNvidiaGPULocationPath() (string, error) {
	out, err := exec.Command(
		"powershell",
		`((Get-PnpDevice -presentOnly | where-object {$_.InstanceID -Match 'PCI\\VEN_10DE.*'})[0] | Get-PnpDeviceProperty DEVPKEY_Device_LocationPaths).Data[0]`,
	).Output()
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(string(out)), nil
}

// findTestVirtualDeviceID returns the instance ID of the first generic pcip device on the host
//
//nolint:unused // may be used in future tests
func findTestVirtualDeviceID() (string, error) {
	out, err := exec.Command(
		"powershell",
		`(Get-PnpDevice -presentOnly | where-object {$_.InstanceID -Match 'PCIP.*'})[0].InstanceId`,
	).Output()
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(string(out)), nil
}

func getGPUContainerRequestWCOW(t *testing.T, podID string, podConfig *runtime.PodSandboxConfig, device *runtime.Device) *runtime.CreateContainerRequest {
	t.Helper()
	return &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
			Devices: []*runtime.Device{
				device,
			},
			Mounts: []*runtime.Mount{
				{
					HostPath:      testDeviceUtilFilePath,
					ContainerPath: containerDeviceUtilPath,
				},
			},
			Annotations: map[string]string{
				annotations.VirtualMachineKernelDrivers: testDriversPath,
			},
		},
		PodSandboxId:  podID,
		SandboxConfig: podConfig,
	}
}

func runContainerLifetime(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, containerID string) {
	t.Helper()
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	stopContainer(t, client, ctx, containerID)
}

func Test_RunContainer_VirtualDevice_LocationPath_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess, featureGPU)

	testDeviceLocationPath, err := findTestNvidiaGPULocationPath()
	if err != nil {
		t.Fatalf("skipping test, failed to retrieve assignable device on host with: %v", err)
	}
	if testDeviceLocationPath == "" {
		t.Fatal("skipping test, host has no assignable devices")
	}

	pullRequiredImages(t, []string{imageWindowsNanoserver})
	client := newTestRuntimeClient(t)

	podctx := context.Background()
	sandboxRequest := getRunPodSandboxRequest(t, wcowProcessRuntimeHandler)

	podID := runPodSandbox(t, client, podctx, sandboxRequest)
	defer removePodSandbox(t, client, podctx, podID)
	defer stopPodSandbox(t, client, podctx, podID)

	device := &runtime.Device{
		HostPath: "vpci-location-path://" + testDeviceLocationPath,
	}

	containerRequest := getGPUContainerRequestWCOW(t, podID, sandboxRequest.Config, device)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	containerID := createContainer(t, client, ctx, containerRequest)
	runContainerLifetime(t, client, ctx, containerID)

	if !isGPUPresentWCOW(t, client, ctx, containerID) {
		t.Fatalf("expected to see a GPU device on container %s, none present", containerID)
	}
}

func Test_RunContainer_VirtualDevice_ClassGUID_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess, featureGPU)

	// instance ID is only used here to ensure there are devices present on the host
	instanceID, err := findTestNvidiaGPUDevice()
	if err != nil {
		t.Fatalf("skipping test, failed to retrieve assignable device on host with: %v", err)
	}
	if instanceID == "" {
		t.Fatal("skipping test, host has no assignable devices")
	}

	// use fixed GPU class guid
	testDeviceClassGUID := "5B45201D-F2F2-4F3B-85BB-30FF1F953599"

	pullRequiredImages(t, []string{imageWindowsNanoserver})
	client := newTestRuntimeClient(t)

	podctx := context.Background()
	sandboxRequest := getRunPodSandboxRequest(t, wcowProcessRuntimeHandler)

	podID := runPodSandbox(t, client, podctx, sandboxRequest)
	defer removePodSandbox(t, client, podctx, podID)
	defer stopPodSandbox(t, client, podctx, podID)

	device := &runtime.Device{
		HostPath: "class://" + testDeviceClassGUID,
	}

	containerRequest := getGPUContainerRequestWCOW(t, podID, sandboxRequest.Config, device)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	containerID := createContainer(t, client, ctx, containerRequest)
	runContainerLifetime(t, client, ctx, containerID)

	if !isGPUPresentWCOW(t, client, ctx, containerID) {
		t.Fatalf("expected to see a GPU device on container %s, none present", containerID)
	}
}

func Test_RunContainer_VirtualDevice_GPU_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor, featureGPU)

	if osversion.Build() < osversion.V20H1 {
		t.Skip("Requires build +20H1")
	}

	testDeviceInstanceID, err := findTestNvidiaGPUDevice()
	if err != nil {
		t.Fatalf("skipping test, failed to retrieve assignable device on host with: %v", err)
	}
	if testDeviceInstanceID == "" {
		t.Fatal("skipping test, host has no assignable devices")
	}

	pullRequiredImages(t, []string{imageWindowsNanoserver})
	client := newTestRuntimeClient(t)

	podctx := context.Background()
	sandboxRequest := getRunPodSandboxRequest(
		t,
		wcowHypervisorRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.FullyPhysicallyBacked: "true",
		}),
	)

	podID := runPodSandbox(t, client, podctx, sandboxRequest)
	defer removePodSandbox(t, client, podctx, podID)
	defer stopPodSandbox(t, client, podctx, podID)

	device := &runtime.Device{
		HostPath: "vpci://" + testDeviceInstanceID,
	}
	containerRequest := getGPUContainerRequestWCOW(t, podID, sandboxRequest.Config, device)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	containerID := createContainer(t, client, ctx, containerRequest)
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	if !isGPUPresentWCOW(t, client, ctx, containerID) {
		t.Fatalf("expected to see a GPU device on container %s, none present", containerID)
	}
}

func Test_RunContainer_VirtualDevice_GPU_and_NoGPU_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor, featureGPU)

	if osversion.Build() < osversion.V20H1 {
		t.Skip("Requires build +20H1")
	}

	testDeviceInstanceID, err := findTestNvidiaGPUDevice()
	if err != nil {
		t.Fatalf("skipping test, failed to retrieve assignable device on host with: %v", err)
	}
	if testDeviceInstanceID == "" {
		t.Fatal("skipping test, host has no assignable devices")
	}

	pullRequiredImages(t, []string{imageWindowsNanoserver})
	client := newTestRuntimeClient(t)

	podctx := context.Background()
	sandboxRequest := getRunPodSandboxRequest(
		t,
		wcowHypervisorRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.FullyPhysicallyBacked: "true",
		}),
	)

	podID := runPodSandbox(t, client, podctx, sandboxRequest)
	defer removePodSandbox(t, client, podctx, podID)
	defer stopPodSandbox(t, client, podctx, podID)

	device := &runtime.Device{
		HostPath: "vpci://" + testDeviceInstanceID,
	}

	containerRequest := getGPUContainerRequestWCOW(t, podID, sandboxRequest.Config, device)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// create container with a GPU present
	gpuContainerID := createContainer(t, client, ctx, containerRequest)
	defer removeContainer(t, client, ctx, gpuContainerID)
	startContainer(t, client, ctx, gpuContainerID)
	defer stopContainer(t, client, ctx, gpuContainerID)

	if !isGPUPresentWCOW(t, client, ctx, gpuContainerID) {
		t.Fatalf("expected to see a GPU device on container %s, none present", gpuContainerID)
	}

	// create container without a GPU
	noGPUName := t.Name() + "-No-GPU-Container"
	containerRequest.Config.Metadata.Name = noGPUName
	containerRequest.Config.Devices = []*runtime.Device{}
	noGPUContainerID := createContainer(t, client, ctx, containerRequest)
	defer removeContainer(t, client, ctx, noGPUContainerID)
	startContainer(t, client, ctx, noGPUContainerID)
	defer stopContainer(t, client, ctx, noGPUContainerID)

	// verify that we can't access the GPU in the No-GPU-Container
	if isGPUPresentWCOW(t, client, ctx, noGPUContainerID) {
		t.Fatalf("expected to see NO GPU device in container %s", noGPUContainerID)
	}
}

func Test_RunContainer_VirtualDevice_GPU_Multiple_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor, featureGPU)

	if osversion.Build() < osversion.V20H1 {
		t.Skip("Requires build +20H1")
	}

	numContainers := 2
	testDeviceInstanceID, err := findTestNvidiaGPUDevice()
	if err != nil {
		t.Fatalf("skipping test, failed to retrieve assignable device on host with: %v", err)
	}
	if testDeviceInstanceID == "" {
		t.Fatal("skipping test, host has no assignable devices")
	}

	pullRequiredImages(t, []string{imageWindowsNanoserver})
	client := newTestRuntimeClient(t)

	podctx := context.Background()
	sandboxRequest := getRunPodSandboxRequest(
		t,
		wcowHypervisorRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.FullyPhysicallyBacked: "true",
		}),
	)

	podID := runPodSandbox(t, client, podctx, sandboxRequest)
	defer removePodSandbox(t, client, podctx, podID)
	defer stopPodSandbox(t, client, podctx, podID)

	device := &runtime.Device{
		HostPath: "vpci://" + testDeviceInstanceID,
	}

	containerRequest := getGPUContainerRequestWCOW(t, podID, sandboxRequest.Config, device)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	for i := 0; i < numContainers; i++ {
		name := t.Name() + "-GPU-Container" + fmt.Sprintf("%d", i)
		containerRequest.Config.Metadata.Name = name

		containerID := createContainer(t, client, ctx, containerRequest)
		defer removeContainer(t, client, ctx, containerID)
		startContainer(t, client, ctx, containerID)
		defer stopContainer(t, client, ctx, containerID)

		if !isGPUPresentWCOW(t, client, ctx, containerID) {
			t.Fatalf("expected to see a GPU device on container %s, none present", containerID)
		}
	}
}

func Test_RunContainer_VirtualDevice_GPU_Multiple_Removal_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor, featureGPU)

	if osversion.Build() < osversion.V20H1 {
		t.Skip("Requires build +20H1")
	}

	testDeviceInstanceID, err := findTestNvidiaGPUDevice()
	if err != nil {
		t.Fatalf("skipping test, failed to retrieve assignable device on host with: %v", err)
	}
	if testDeviceInstanceID == "" {
		t.Fatal("skipping test, host has no assignable devices")
	}

	pullRequiredImages(t, []string{imageWindowsNanoserver})
	client := newTestRuntimeClient(t)

	podctx := context.Background()
	sandboxRequest := getRunPodSandboxRequest(
		t,
		wcowHypervisorRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.FullyPhysicallyBacked: "true",
		}),
	)

	podID := runPodSandbox(t, client, podctx, sandboxRequest)
	defer removePodSandbox(t, client, podctx, podID)
	defer stopPodSandbox(t, client, podctx, podID)

	device := &runtime.Device{
		HostPath: "vpci://" + testDeviceInstanceID,
	}

	containerRequest := getGPUContainerRequestWCOW(t, podID, sandboxRequest.Config, device)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// create container with a GPU present
	gpuContainerIDOne := createContainer(t, client, ctx, containerRequest)
	defer removeContainer(t, client, ctx, gpuContainerIDOne)
	startContainer(t, client, ctx, gpuContainerIDOne)
	defer stopContainer(t, client, ctx, gpuContainerIDOne)

	// run full lifetime of second container with GPU
	containerRequest.Config.Metadata.Name = t.Name() + "-GPU-Container-2"
	gpuContainerIDTwo := createContainer(t, client, ctx, containerRequest)
	runContainerLifetime(t, client, ctx, gpuContainerIDTwo)

	// verify after removing second container that we can still see
	// the GPU on the first container
	if !isGPUPresentWCOW(t, client, ctx, gpuContainerIDOne) {
		t.Fatalf("expected to see a GPU device on container %s, none present", gpuContainerIDOne)
	}
}
