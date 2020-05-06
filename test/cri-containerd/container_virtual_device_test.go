// +build functional

package cri_containerd

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/osversion"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// makeGPUExecCommand constructs the container command to check for the
// existence of a nvidia GPU device and returns the command in an
// ExecSyncRequest
func makeGPUExecCommand(containerID string) *runtime.ExecSyncRequest {
	cmd := []string{"ls", "/dev/nvidia0"}
	return &runtime.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         cmd,
		Timeout:     20,
	}
}

// verifyGPUIsPresent is a helper function that runs a command in the container
// to verify the existence of a GPU and fails the running test is none are found
func verifyGPUIsPresent(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, containerID string) {
	execReq := makeGPUExecCommand(containerID)
	response := execSync(t, client, ctx, execReq)
	if len(response.Stderr) != 0 {
		t.Fatalf("expected to see no error, instead saw %s", string(response.Stderr))
	}
	if len(response.Stdout) == 0 {
		t.Fatal("expected to see GPU device on container, not present")
	}
}

// verifyGPUIsNotPresent is a helper function that runs a command in the container
// to verify that there are no GPUs present in the container and fails the running test
// if any are found
func verifyGPUIsNotPresent(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, containerID string) {
	execReq := makeGPUExecCommand(containerID)
	response := execSync(t, client, ctx, execReq)
	if len(response.Stderr) == 0 {
		t.Fatal("expected to see an error as file /dev/nvidia0 should not exist, instead saw none")
	} else if len(response.Stdout) != 0 {
		t.Fatal("expected to not see GPU device on container, but some are present")
	}
}

// findTestDevices returns the first nvidia pcip device on the host
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

func getGPUPodRequestLCOW(t *testing.T) *runtime.RunPodSandboxRequest {
	return &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.virtualmachine.lcow.kerneldirectboot":                  "false",
				"io.microsoft.virtualmachine.computetopology.memory.allowovercommit": "false",
				"io.microsoft.virtualmachine.lcow.preferredrootfstype":               "initrd",
				"io.microsoft.virtualmachine.devices.virtualpmem.maximumcount":       "0",
				"io.microsoft.virtualmachine.lcow.vpcienabled":                       "true",
				// we believe this is a sufficiently large high MMIO space amount for this test.
				// if a given gpu device needs more, this test will fail to create the container
				// and may hang.
				"io.microsoft.virtualmachine.computetopology.memory.highmmiogapinmb": "64000",
				"io.microsoft.virtualmachine.lcow.bootfilesrootpath":                 testGPUBootFiles,
			},
		},
		RuntimeHandler: lcowRuntimeHandler,
	}
}

func getGPUContainerRequest(t *testing.T, podID string, podConfig *runtime.PodSandboxConfig, device *runtime.Device) *runtime.CreateContainerRequest {
	return &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageLcowAlpine,
			},
			Command: []string{
				"top",
			},
			Devices: []*runtime.Device{
				device,
			},
			Linux: &runtime.LinuxContainerConfig{},
			Annotations: map[string]string{
				"io.microsoft.container.gpu.capabilities": "utility",
			},
		},
		PodSandboxId:  podID,
		SandboxConfig: podConfig,
	}
}

func Test_RunContainer_VirtualDevice_GPU_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW, featureGPU)

	if osversion.Get().Build < 19566 {
		t.Skip("Requires build +19566")
	}

	testDeviceInstanceID, err := findTestNvidiaGPUDevice()
	if err != nil {
		t.Skipf("skipping test, failed to find assignable nvidia gpu on host with: %v", err)
	}
	if testDeviceInstanceID == "" {
		t.Skipf("skipping test, host has no assignable nvidia gpu devices")
	}

	pullRequiredLcowImages(t, []string{imageLcowK8sPause, imageLcowAlpine})
	client := newTestRuntimeClient(t)

	podctx := context.Background()
	sandboxRequest := getGPUPodRequestLCOW(t)

	podID := runPodSandbox(t, client, podctx, sandboxRequest)
	defer removePodSandbox(t, client, podctx, podID)
	defer stopPodSandbox(t, client, podctx, podID)

	device := &runtime.Device{
		HostPath: "gpu://" + testDeviceInstanceID,
	}

	containerRequest := getGPUContainerRequest(t, podID, sandboxRequest.Config, device)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	containerID := createContainer(t, client, ctx, containerRequest)
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	verifyGPUIsPresent(t, client, ctx, containerID)
}

func Test_RunContainer_VirtualDevice_GPU_Multiple_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW, featureGPU)

	if osversion.Get().Build < 19566 {
		t.Skip("Requires build +19566")
	}

	numContainers := 2
	testDeviceInstanceID, err := findTestNvidiaGPUDevice()
	if err != nil {
		t.Skipf("skipping test, failed to find assignable nvidia gpu on host with: %v", err)
	}
	if testDeviceInstanceID == "" {
		t.Skipf("skipping test, host has no assignable nvidia gpu devices")
	}

	pullRequiredLcowImages(t, []string{imageLcowK8sPause, imageLcowAlpine})
	client := newTestRuntimeClient(t)

	podctx := context.Background()
	sandboxRequest := getGPUPodRequestLCOW(t)

	podID := runPodSandbox(t, client, podctx, sandboxRequest)
	defer removePodSandbox(t, client, podctx, podID)
	defer stopPodSandbox(t, client, podctx, podID)

	device := &runtime.Device{
		HostPath: "gpu://" + testDeviceInstanceID,
	}

	containerRequest := getGPUContainerRequest(t, podID, sandboxRequest.Config, device)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for i := 0; i < numContainers; i++ {

		name := t.Name() + "-Container-" + fmt.Sprintf("%d", i)
		containerRequest.Config.Metadata.Name = name

		containerID := createContainer(t, client, ctx, containerRequest)
		defer removeContainer(t, client, ctx, containerID)
		startContainer(t, client, ctx, containerID)
		defer stopContainer(t, client, ctx, containerID)

		verifyGPUIsPresent(t, client, ctx, containerID)
	}
}

func Test_RunContainer_VirtualDevice_GPU_and_NoGPU_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW, featureGPU)

	if osversion.Get().Build < 19566 {
		t.Skip("Requires build +19566")
	}

	testDeviceInstanceID, err := findTestNvidiaGPUDevice()
	if err != nil {
		t.Skipf("skipping test, failed to find assignable nvidia gpu on host with: %v", err)
	}
	if testDeviceInstanceID == "" {
		t.Skipf("skipping test, host has no assignable nvidia gpu devices")
	}

	pullRequiredLcowImages(t, []string{imageLcowK8sPause, imageLcowAlpine})
	client := newTestRuntimeClient(t)

	podctx := context.Background()
	sandboxRequest := getGPUPodRequestLCOW(t)

	podID := runPodSandbox(t, client, podctx, sandboxRequest)
	defer removePodSandbox(t, client, podctx, podID)
	defer stopPodSandbox(t, client, podctx, podID)

	device := &runtime.Device{
		HostPath: "gpu://" + testDeviceInstanceID,
	}

	containerGPURequest := getGPUContainerRequest(t, podID, sandboxRequest.Config, device)

	containerNoGPURequest := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: "No-GPU-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageLcowAlpine,
			},
			Command: []string{
				"top",
			},
			Linux: &runtime.LinuxContainerConfig{},
		},
		PodSandboxId:  podID,
		SandboxConfig: sandboxRequest.Config,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// create container with a GPU present
	gpuContainerID := createContainer(t, client, ctx, containerGPURequest)
	defer removeContainer(t, client, ctx, gpuContainerID)
	startContainer(t, client, ctx, gpuContainerID)
	defer stopContainer(t, client, ctx, gpuContainerID)

	// verify that we can access the GPU in the GPU-Container
	verifyGPUIsPresent(t, client, ctx, gpuContainerID)

	// create container without a GPU
	noGPUContainerID := createContainer(t, client, ctx, containerNoGPURequest)
	defer removeContainer(t, client, ctx, noGPUContainerID)
	startContainer(t, client, ctx, noGPUContainerID)
	defer stopContainer(t, client, ctx, noGPUContainerID)

	// verify that we can't access the GPU in the No-GPU-Container
	verifyGPUIsNotPresent(t, client, ctx, noGPUContainerID)

}

func Test_RunContainer_VirtualDevice_GPU_Multiple_Removal_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW, featureGPU)

	if osversion.Get().Build < 19566 {
		t.Skip("Requires build +19566")
	}

	testDeviceInstanceID, err := findTestNvidiaGPUDevice()
	if err != nil {
		t.Skipf("skipping test, failed to find assignable nvidia gpu on host with: %v", err)
	}
	if testDeviceInstanceID == "" {
		t.Skipf("skipping test, host has no assignable nvidia gpu devices")
	}

	pullRequiredLcowImages(t, []string{imageLcowK8sPause, imageLcowAlpine})
	client := newTestRuntimeClient(t)

	podctx := context.Background()
	sandboxRequest := getGPUPodRequestLCOW(t)

	podID := runPodSandbox(t, client, podctx, sandboxRequest)
	defer removePodSandbox(t, client, podctx, podID)
	defer stopPodSandbox(t, client, podctx, podID)

	device := &runtime.Device{
		HostPath: "gpu://" + testDeviceInstanceID,
	}

	containerRequest := getGPUContainerRequest(t, podID, sandboxRequest.Config, device)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// create and start up first container with GPU
	containerRequest.Config.Metadata.Name = t.Name() + "-Container-1"
	containerOneID := createContainer(t, client, ctx, containerRequest)
	defer removeContainer(t, client, ctx, containerOneID)
	startContainer(t, client, ctx, containerOneID)
	defer stopContainer(t, client, ctx, containerOneID)

	// run full lifetime of second container with GPU
	containerRequest.Config.Metadata.Name = t.Name() + "-Container-2"
	containerTwoID := createContainer(t, client, ctx, containerRequest)
	runContainerLifetime(t, client, ctx, containerTwoID)

	// verify after removing second container that we can still see
	// the GPU on the first container
	verifyGPUIsPresent(t, client, ctx, containerOneID)
}
