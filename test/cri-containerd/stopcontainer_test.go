//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"testing"
	"time"

	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func Test_StopContainer_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sandboxRequest := getRunPodSandboxRequest(t, lcowRuntimeHandler)

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	request := &runtime.CreateContainerRequest{
		PodSandboxId: podID,
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
		},
		SandboxConfig: sandboxRequest.Config,
	}

	containerID := createContainer(t, client, ctx, request)
	defer removeContainer(t, client, ctx, containerID)

	startContainer(t, client, ctx, containerID)
	stopContainer(t, client, ctx, containerID)
}

func Test_StopContainer_WithTimeout_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sandboxRequest := getRunPodSandboxRequest(t, lcowRuntimeHandler)

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	request := &runtime.CreateContainerRequest{
		PodSandboxId: podID,
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
		},
		SandboxConfig: sandboxRequest.Config,
	}

	containerID := createContainer(t, client, ctx, request)
	defer removeContainer(t, client, ctx, containerID)

	startContainer(t, client, ctx, containerID)
	stopContainerWithTimeout(t, client, ctx, containerID, 10)
}

func Test_StopContainer_WithExec_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sandboxRequest := getRunPodSandboxRequest(t, lcowRuntimeHandler)

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	request := &runtime.CreateContainerRequest{
		PodSandboxId: podID,
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
		},
		SandboxConfig: sandboxRequest.Config,
	}

	containerID := createContainer(t, client, ctx, request)
	defer removeContainer(t, client, ctx, containerID)

	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	execRequest(t, client, ctx, &runtime.ExecRequest{
		ContainerId: containerID,
		Cmd: []string{
			"top",
		},
		Stdout: true,
	})
}

func Test_StopContainer_ReusePod_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{alpineAspNet, alpineAspnetUpgrade})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sandboxRequest := getRunPodSandboxRequest(t, lcowRuntimeHandler)

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	request := &runtime.CreateContainerRequest{
		PodSandboxId: podID,
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: alpineAspNet,
			},
			Command: []string{
				"top",
			},
		},
		SandboxConfig: sandboxRequest.Config,
	}

	containerID := createContainer(t, client, ctx, request)
	runContainerLifetime(t, client, ctx, containerID)

	request.Config.Image.Image = alpineAspnetUpgrade
	containerID = createContainer(t, client, ctx, request)
	runContainerLifetime(t, client, ctx, containerID)
}

func Test_Gracefultermination_WCOW_Process_Nanoserver(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)
	pullRequiredImages(t, []string{gracefulTerminationNanoserver})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sandboxRequest := getRunPodSandboxRequest(t, wcowProcessRuntimeHandler)

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	request := &runtime.CreateContainerRequest{
		PodSandboxId: podID,
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{},
			Image: &runtime.ImageSpec{
				Image: gracefulTerminationNanoserver,
			},
		},
		SandboxConfig: sandboxRequest.Config,
	}

	containerID := createContainer(t, client, ctx, request)
	defer removeContainer(t, client, ctx, containerID)

	startContainer(t, client, ctx, containerID)

	// stop container with timeout of 60 seconds
	stopContainerWithTimeout(t, client, ctx, containerID, 60)

	finalResponse, err := client.ContainerStatus(ctx, &runtime.ContainerStatusRequest{ContainerId: containerID})
	if err != nil {
		t.Fatalf("failed to get container status after container stop: %v", err)
	}

	// get container's start and end time. These times are in nanoseconds
	ctrStartedAt := finalResponse.GetStatus().GetStartedAt()
	ctrFinishedAt := finalResponse.GetStatus().GetFinishedAt()

	// ensure that the container has stopped only after approx 60 seconds
	timeDiff := ctrFinishedAt - ctrStartedAt

	// ensure that time difference is > 60 seconds
	if timeDiff < 60*int64(time.Second) {
		t.Fatalf("Container did not shutdown gracefully \n")
	}
}

func Test_Gracefultermination_WCOW_Process_Servercore(t *testing.T) {
	requireFeatures(t, featureWCOWProcess)
	pullRequiredImages(t, []string{gracefulTerminationServercore})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sandboxRequest := getRunPodSandboxRequest(t, wcowProcessRuntimeHandler)

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	request := &runtime.CreateContainerRequest{
		PodSandboxId: podID,
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{},
			Image: &runtime.ImageSpec{
				Image: gracefulTerminationServercore,
			},
		},
		SandboxConfig: sandboxRequest.Config,
	}

	containerID := createContainer(t, client, ctx, request)
	defer removeContainer(t, client, ctx, containerID)

	startContainer(t, client, ctx, containerID)

	// stop container with timeout of 60 seconds
	stopContainerWithTimeout(t, client, ctx, containerID, 60)

	finalResponse, err := client.ContainerStatus(ctx, &runtime.ContainerStatusRequest{ContainerId: containerID})
	if err != nil {
		t.Fatalf("failed to get container status after container stop: %v", err)
	}

	// get container's start and end time. These times are in nanoseconds
	ctrStartedAt := finalResponse.GetStatus().GetStartedAt()
	ctrFinishedAt := finalResponse.GetStatus().GetFinishedAt()

	// ensure that the container has stopped only after approx 60 seconds
	timeDiff := ctrFinishedAt - ctrStartedAt

	// ensure that time difference is > 60 seconds
	if timeDiff < 60*int64(time.Second) {
		t.Fatalf("Container did not shutdown gracefully \n")
	}
}

func Test_Gracefultermination_WCOW_Hypervisor_Nanoserver(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)
	pullRequiredImages(t, []string{gracefulTerminationNanoserver})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sandboxRequest := getRunPodSandboxRequest(t, wcowHypervisorRuntimeHandler)

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	request := &runtime.CreateContainerRequest{
		PodSandboxId: podID,
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{},
			Image: &runtime.ImageSpec{
				Image: gracefulTerminationNanoserver,
			},
		},
		SandboxConfig: sandboxRequest.Config,
	}

	containerID := createContainer(t, client, ctx, request)
	defer removeContainer(t, client, ctx, containerID)

	startContainer(t, client, ctx, containerID)

	// stop container with timeout of 60 seconds
	stopContainerWithTimeout(t, client, ctx, containerID, 60)

	finalResponse, err := client.ContainerStatus(ctx, &runtime.ContainerStatusRequest{ContainerId: containerID})
	if err != nil {
		t.Fatalf("failed to get container status after container stop: %v", err)
	}

	// get container's start and end time. These times are in nanoseconds
	ctrStartedAt := finalResponse.GetStatus().GetStartedAt()
	ctrFinishedAt := finalResponse.GetStatus().GetFinishedAt()

	// ensure that the container has stopped only after approx 60 seconds
	timeDiff := ctrFinishedAt - ctrStartedAt

	// ensure that time difference is > 60 seconds
	if timeDiff < 60*int64(time.Second) {
		t.Fatalf("Container did not shutdown gracefully \n")
	}
}

func Test_Gracefultermination_WCOW_Hypervisor_Servercore(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)
	pullRequiredImages(t, []string{gracefulTerminationServercore})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sandboxRequest := getRunPodSandboxRequest(t, wcowHypervisorRuntimeHandler)

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	request := &runtime.CreateContainerRequest{
		PodSandboxId: podID,
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{},
			Image: &runtime.ImageSpec{
				Image: gracefulTerminationServercore,
			},
		},
		SandboxConfig: sandboxRequest.Config,
	}

	containerID := createContainer(t, client, ctx, request)
	defer removeContainer(t, client, ctx, containerID)

	startContainer(t, client, ctx, containerID)

	// stop container with timeout of 60 seconds
	stopContainerWithTimeout(t, client, ctx, containerID, 60)

	finalResponse, err := client.ContainerStatus(ctx, &runtime.ContainerStatusRequest{ContainerId: containerID})
	if err != nil {
		t.Fatalf("failed to get container status after container stop: %v", err)
	}

	// get container's start and end time. These times are in nanoseconds
	ctrStartedAt := finalResponse.GetStatus().GetStartedAt()
	ctrFinishedAt := finalResponse.GetStatus().GetFinishedAt()

	// ensure that the container has stopped only after approx 60 seconds
	timeDiff := ctrFinishedAt - ctrStartedAt

	// ensure that time difference is > 60 seconds
	if timeDiff < 60*int64(time.Second) {
		t.Fatalf("Container did not shutdown gracefully \n")
	}
}
