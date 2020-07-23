// +build functional

package cri_containerd

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/hcsshim/osversion"
	testutilities "github.com/Microsoft/hcsshim/test/functional/utilities"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func runCreateContainerTest(t *testing.T, runtimeHandler string, request *runtime.CreateContainerRequest) {
	sandboxRequest := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name() + "-Sandbox",
				Uid:       "0",
				Namespace: testNamespace,
			},
		},
		RuntimeHandler: runtimeHandler,
	}
	runCreateContainerTestWithSandbox(t, sandboxRequest, request)
}

func runCreateContainerTestWithSandbox(t *testing.T, sandboxRequest *runtime.RunPodSandboxRequest, request *runtime.CreateContainerRequest) {
	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	request.PodSandboxId = podID
	request.SandboxConfig = sandboxRequest.Config

	containerID := createContainer(t, client, ctx, request)
	defer removeContainer(t, client, ctx, containerID)

	startContainer(t, client, ctx, containerID)
	stopContainer(t, client, ctx, containerID)
}

func Test_CreateContainer_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			// Hold this command open until killed (pause for Windows)
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
		},
	}
	runCreateContainerTest(t, wcowProcessRuntimeHandler, request)
}

func Test_CreateContainer_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			// Hold this command open until killed (pause for Windows)
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
		},
	}
	runCreateContainerTest(t, wcowHypervisorRuntimeHandler, request)
}

func Test_CreateContainer_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	request := &runtime.CreateContainerRequest{
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
		},
	}
	runCreateContainerTest(t, lcowRuntimeHandler, request)
}

func Test_CreateContainer_WCOW_Process_Tty(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			// Tty will hold this open until killed.
			Command: []string{
				"cmd",
			},
			Tty: true,
		},
	}
	runCreateContainerTest(t, wcowProcessRuntimeHandler, request)
}

func Test_CreateContainer_WCOW_Hypervisor_Tty(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			// Tty will hold this open until killed.
			Command: []string{
				"cmd",
			},
			Tty: true,
		},
	}
	runCreateContainerTest(t, wcowHypervisorRuntimeHandler, request)
}

func Test_CreateContainer_LCOW_Tty(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageLcowAlpine,
			},
			// Tty will hold this open until killed.
			Command: []string{
				"sh",
			},
			Tty: true,
		},
	}
	runCreateContainerTest(t, lcowRuntimeHandler, request)
}

func Test_CreateContainer_LCOW_Privileged(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	sandboxRequest := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name() + "-Sandbox",
				Uid:       "0",
				Namespace: testNamespace,
			},
			Linux: &runtime.LinuxPodSandboxConfig{
				SecurityContext: &runtime.LinuxSandboxSecurityContext{
					Privileged: true,
				},
			},
		},
		RuntimeHandler: lcowRuntimeHandler,
	}

	request := &runtime.CreateContainerRequest{
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
			Linux: &runtime.LinuxContainerConfig{
				SecurityContext: &runtime.LinuxContainerSecurityContext{
					Privileged: true,
				},
			},
		},
	}
	runCreateContainerTestWithSandbox(t, sandboxRequest, request)
}

func Test_CreateContainer_MemorySize_Config_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			// Hold this command open until killed (pause for Windows)
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
			Windows: &runtime.WindowsContainerConfig{
				Resources: &runtime.WindowsContainerResources{
					MemoryLimitInBytes: 768 * 1024 * 1024, // 768MB
				},
			},
		},
	}
	runCreateContainerTest(t, wcowProcessRuntimeHandler, request)
}

func Test_CreateContainer_MemorySize_Annotation_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			// Hold this command open until killed (pause for Windows)
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
			Annotations: map[string]string{
				"io.microsoft.container.memory.sizeinmb": fmt.Sprintf("%d", 768*1024*1024), // 768MB
			},
		},
	}
	runCreateContainerTest(t, wcowProcessRuntimeHandler, request)
}

func Test_CreateContainer_MemorySize_Config_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			// Hold this command open until killed (pause for Windows)
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
			Windows: &runtime.WindowsContainerConfig{
				Resources: &runtime.WindowsContainerResources{
					MemoryLimitInBytes: 768 * 1024 * 1024, // 768MB
				},
			},
		},
	}
	runCreateContainerTest(t, wcowHypervisorRuntimeHandler, request)
}

func Test_CreateContainer_MemorySize_Annotation_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			// Hold this command open until killed (pause for Windows)
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
			Annotations: map[string]string{
				"io.microsoft.container.memory.sizeinmb": fmt.Sprintf("%d", 768*1024*1024), // 768MB
			},
		},
	}
	runCreateContainerTest(t, wcowHypervisorRuntimeHandler, request)
}

func Test_CreateContainer_MemorySize_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	request := &runtime.CreateContainerRequest{
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
			Linux: &runtime.LinuxContainerConfig{
				Resources: &runtime.LinuxContainerResources{
					MemoryLimitInBytes: 768 * 1024 * 1024, // 768MB
				},
			},
		},
	}
	runCreateContainerTest(t, lcowRuntimeHandler, request)
}

func Test_CreateContainer_CPUCount_Config_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			// Hold this command open until killed (pause for Windows)
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
			Windows: &runtime.WindowsContainerConfig{
				Resources: &runtime.WindowsContainerResources{
					CpuCount: 1,
				},
			},
		},
	}
	runCreateContainerTest(t, wcowProcessRuntimeHandler, request)
}

func Test_CreateContainer_CPUCount_Annotation_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			// Hold this command open until killed (pause for Windows)
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
			Annotations: map[string]string{
				"io.microsoft.container.processor.count": "1",
			},
		},
	}
	runCreateContainerTest(t, wcowProcessRuntimeHandler, request)
}

func Test_CreateContainer_CPUCount_Config_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			// Hold this command open until killed (pause for Windows)
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
			Windows: &runtime.WindowsContainerConfig{
				Resources: &runtime.WindowsContainerResources{
					CpuCount: 1,
				},
			},
		},
	}
	runCreateContainerTest(t, wcowHypervisorRuntimeHandler, request)
}

func Test_CreateContainer_CPUCount_Annotation_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			// Hold this command open until killed (pause for Windows)
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
			Annotations: map[string]string{
				"io.microsoft.container.processor.count": "1",
			},
		},
	}
	runCreateContainerTest(t, wcowHypervisorRuntimeHandler, request)
}

func Test_CreateContainer_CPUCount_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	request := &runtime.CreateContainerRequest{
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
			Linux: &runtime.LinuxContainerConfig{
				Resources: &runtime.LinuxContainerResources{
					CpusetCpus: "0",
				},
			},
		},
	}
	runCreateContainerTest(t, lcowRuntimeHandler, request)
}

func Test_CreateContainer_CPULimit_Config_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			// Hold this command open until killed (pause for Windows)
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
			Windows: &runtime.WindowsContainerConfig{
				Resources: &runtime.WindowsContainerResources{
					CpuMaximum: 9000,
				},
			},
		},
	}
	runCreateContainerTest(t, wcowProcessRuntimeHandler, request)
}

func Test_CreateContainer_CPULimit_Annotation_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			// Hold this command open until killed (pause for Windows)
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
			Annotations: map[string]string{
				"io.microsoft.container.processor.limit": "9000",
			},
		},
	}
	runCreateContainerTest(t, wcowProcessRuntimeHandler, request)
}

func Test_CreateContainer_CPULimit_Config_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			// Hold this command open until killed (pause for Windows)
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
			Windows: &runtime.WindowsContainerConfig{
				Resources: &runtime.WindowsContainerResources{
					CpuMaximum: 9000,
				},
			},
		},
	}
	runCreateContainerTest(t, wcowHypervisorRuntimeHandler, request)
}

func Test_CreateContainer_CPULimit_Annotation_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			// Hold this command open until killed (pause for Windows)
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
			Annotations: map[string]string{
				"io.microsoft.container.processor.limit": "9000",
			},
		},
	}
	runCreateContainerTest(t, wcowHypervisorRuntimeHandler, request)
}

func Test_CreateContainer_CPUQuota_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	request := &runtime.CreateContainerRequest{
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
			Linux: &runtime.LinuxContainerConfig{
				Resources: &runtime.LinuxContainerResources{
					CpuQuota:  1000000,
					CpuPeriod: 500000,
				},
			},
		},
	}
	runCreateContainerTest(t, lcowRuntimeHandler, request)
}

func Test_CreateContainer_CPUWeight_Config_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			// Hold this command open until killed (pause for Windows)
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
			Windows: &runtime.WindowsContainerConfig{
				Resources: &runtime.WindowsContainerResources{
					CpuShares: 500,
				},
			},
		},
	}
	runCreateContainerTest(t, wcowProcessRuntimeHandler, request)
}

func Test_CreateContainer_CPUWeight_Annotation_WCOW_Process(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			// Hold this command open until killed (pause for Windows)
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
			Annotations: map[string]string{
				"io.microsoft.container.processor.weight": "500",
			},
		},
	}
	runCreateContainerTest(t, wcowProcessRuntimeHandler, request)
}

func Test_CreateContainer_CPUWeight_Config_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			// Hold this command open until killed (pause for Windows)
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
			Windows: &runtime.WindowsContainerConfig{
				Resources: &runtime.WindowsContainerResources{
					CpuMaximum: 500,
				},
			},
		},
	}
	runCreateContainerTest(t, wcowHypervisorRuntimeHandler, request)
}

func Test_CreateContainer_CPUWeight_Annotation_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			// Hold this command open until killed (pause for Windows)
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
			Annotations: map[string]string{
				"io.microsoft.container.processor.limit": "500",
			},
		},
	}
	runCreateContainerTest(t, wcowHypervisorRuntimeHandler, request)
}

func Test_CreateContainer_CPUShares_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	request := &runtime.CreateContainerRequest{
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
			Linux: &runtime.LinuxContainerConfig{
				Resources: &runtime.LinuxContainerResources{
					CpuShares: 1024,
				},
			},
		},
	}
	runCreateContainerTest(t, lcowRuntimeHandler, request)
}

func Test_CreateContainer_Mount_File_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)
	testutilities.RequiresBuild(t, osversion.V19H1)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	tempFile, err := ioutil.TempFile("", "test")

	if err != nil {
		t.Fatalf("Failed to create temp file: %s", err)
	}

	tempFile.Close()

	defer func() {
		if err := os.Remove(tempFile.Name()); err != nil {
			t.Fatalf("Failed to remove temp file: %s", err)
		}
	}()

	containerFilePath := "/foo/test.txt"

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Mounts: []*runtime.Mount{
				{
					HostPath:      tempFile.Name(),
					ContainerPath: containerFilePath,
				},
			},
			Image: &runtime.ImageSpec{
				Image: imageLcowAlpine,
			},
			Command: []string{
				"top",
			},
			Linux: &runtime.LinuxContainerConfig{},
		},
	}
	runCreateContainerTest(t, lcowRuntimeHandler, request)
}

func Test_CreateContainer_Mount_ReadOnlyFile_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)
	testutilities.RequiresBuild(t, osversion.V19H1)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	tempFile, err := ioutil.TempFile("", "test")

	if err != nil {
		t.Fatalf("Failed to create temp file: %s", err)
	}

	tempFile.Close()

	defer func() {
		if err := os.Remove(tempFile.Name()); err != nil {
			t.Fatalf("Failed to remove temp file: %s", err)
		}
	}()

	containerFilePath := "/foo/test.txt"

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Mounts: []*runtime.Mount{
				{
					HostPath:      tempFile.Name(),
					ContainerPath: containerFilePath,
					Readonly:      true,
				},
			},
			Image: &runtime.ImageSpec{
				Image: imageLcowAlpine,
			},
			Command: []string{
				"top",
			},
			Linux: &runtime.LinuxContainerConfig{},
		},
	}
	runCreateContainerTest(t, lcowRuntimeHandler, request)
}

func Test_CreateContainer_Mount_Dir_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %s", err)
	}
	defer os.RemoveAll(tempDir)

	containerFilePath := "/foo"

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Mounts: []*runtime.Mount{
				{
					HostPath:      tempDir,
					ContainerPath: containerFilePath,
				},
			},
			Image: &runtime.ImageSpec{
				Image: imageLcowAlpine,
			},
			Command: []string{
				"top",
			},
			Linux: &runtime.LinuxContainerConfig{},
		},
	}
	runCreateContainerTest(t, lcowRuntimeHandler, request)
}

func Test_CreateContainer_Mount_ReadOnlyDir_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %s", err)
	}
	defer os.RemoveAll(tempDir)

	containerFilePath := "/foo"

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Mounts: []*runtime.Mount{
				{
					HostPath:      tempDir,
					ContainerPath: containerFilePath,
					Readonly:      true,
				},
			},
			Image: &runtime.ImageSpec{
				Image: imageLcowAlpine,
			},
			Command: []string{
				"top",
			},
			Linux: &runtime.LinuxContainerConfig{},
		},
	}
	runCreateContainerTest(t, lcowRuntimeHandler, request)
}

func Test_CreateContainer_Mount_File_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)
	pullRequiredImages(t, []string{imageWindowsNanoserver})

	tempFile, err := ioutil.TempFile("", "test")

	if err != nil {
		t.Fatalf("Failed to create temp file: %s", err)
	}

	tempFile.Close()

	defer func() {
		if err := os.Remove(tempFile.Name()); err != nil {
			t.Fatalf("Failed to remove temp file: %s", err)
		}
	}()

	containerFilePath := `C:\foo\test`

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Mounts: []*runtime.Mount{
				{
					HostPath:      tempFile.Name(),
					ContainerPath: containerFilePath,
				},
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			Command: []string{
				"ping",
				"-t",
				"127.0.0.1",
			},
		},
	}
	runCreateContainerTest(t, wcowHypervisorRuntimeHandler, request)
}

func Test_CreateContainer_Mount_ReadOnlyFile_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	tempFile, err := ioutil.TempFile("", "test")

	if err != nil {
		t.Fatalf("Failed to create temp file: %s", err)
	}

	tempFile.Close()

	defer func() {
		if err := os.Remove(tempFile.Name()); err != nil {
			t.Fatalf("Failed to remove temp file: %s", err)
		}
	}()

	containerFilePath := `C:\foo\test`

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Mounts: []*runtime.Mount{
				{
					HostPath:      tempFile.Name(),
					ContainerPath: containerFilePath,
					Readonly:      true,
				},
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			Command: []string{
				"ping",
				"-t",
				"127.0.0.1",
			},
		},
	}
	runCreateContainerTest(t, wcowHypervisorRuntimeHandler, request)
}

func Test_CreateContainer_Mount_Dir_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %s", err)
	}
	defer os.RemoveAll(tempDir)

	containerFilePath := "C:\\foo"

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Mounts: []*runtime.Mount{
				{
					HostPath:      tempDir,
					ContainerPath: containerFilePath,
				},
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			Command: []string{
				"ping",
				"-t",
				"127.0.0.1",
			},
		},
	}
	runCreateContainerTest(t, wcowHypervisorRuntimeHandler, request)
}

func Test_CreateContainer_Mount_ReadOnlyDir_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %s", err)
	}
	defer os.RemoveAll(tempDir)

	containerFilePath := "C:\\foo"

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Mounts: []*runtime.Mount{
				{
					HostPath:      tempDir,
					ContainerPath: containerFilePath,
					Readonly:      true,
				},
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			Command: []string{
				"ping",
				"-t",
				"127.0.0.1",
			},
		},
	}
	runCreateContainerTest(t, wcowHypervisorRuntimeHandler, request)
}

func Test_CreateContainer_Mount_EmptyDir_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %s", err)
	}
	defer os.RemoveAll(tempDir)
	path := filepath.Join(tempDir, "kubernetes.io~empty-dir", "volume1")
	if err := os.MkdirAll(path, 0); err != nil {
		t.Fatalf("Failed to create kubernetes.io~empty-dir volume path: %s", err)
	}

	containerFilePath := "C:\\foo"

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Mounts: []*runtime.Mount{
				{
					HostPath:      path,
					ContainerPath: containerFilePath,
				},
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			Command: []string{
				"ping",
				"-t",
				"127.0.0.1",
			},
		},
	}
	runCreateContainerTest(t, wcowHypervisorRuntimeHandler, request)
}

func Test_CreateContainer_Mount_NamedPipe_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	path := `\\.\pipe\testpipe`
	pipe, err := winio.ListenPipe(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := pipe.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Mounts: []*runtime.Mount{
				{
					HostPath:      path,
					ContainerPath: path,
				},
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			Command: []string{
				"ping",
				"-t",
				"127.0.0.1",
			},
		},
	}
	runCreateContainerTest(t, wcowHypervisorRuntimeHandler, request)
}
