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

// This test runs a container with an image that waits for sigterm and then
// prints for loop counter down from 60 till the container is stopped with
// a timeout of 15 seconds. This is done to mimic graceful termination
// behavior and to ensure that the containers are killed only after 15 second
// timeout specified via the stop container command.
func Test_GracefulTermination(t *testing.T) {
	for name, tc := range map[string]struct {
		features       []string
		runtimeHandler string
		image          string
	}{
		"WCOWProcessNanoserver": {
			features:       []string{featureWCOWProcess},
			runtimeHandler: wcowProcessRuntimeHandler,
			image:          gracefulTerminationNanoserver,
		},
		"WCOWProcessServercore": {
			features:       []string{featureWCOWProcess},
			runtimeHandler: wcowProcessRuntimeHandler,
			image:          gracefulTerminationServercore,
		},
		"WCOWHypervisorNanoserver": {
			features:       []string{featureWCOWHypervisor},
			runtimeHandler: wcowHypervisorRuntimeHandler,
			image:          gracefulTerminationNanoserver,
		},
		"WCOWHypervisorServercore": {
			features:       []string{featureWCOWHypervisor},
			runtimeHandler: wcowHypervisorRuntimeHandler,
			image:          gracefulTerminationServercore,
		},
	} {
		t.Run(name, func(t *testing.T) {
			requireFeatures(t, tc.features...)
			pullRequiredImages(t, []string{tc.image})
			client := newTestRuntimeClient(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sandboxRequest := getRunPodSandboxRequest(t, tc.runtimeHandler)
			podID := runPodSandbox(t, client, ctx, sandboxRequest)
			defer removePodSandbox(t, client, ctx, podID)
			defer stopPodSandbox(t, client, ctx, podID)
			request := &runtime.CreateContainerRequest{
				PodSandboxId: podID,
				Config: &runtime.ContainerConfig{
					Metadata: &runtime.ContainerMetadata{},
					Image: &runtime.ImageSpec{
						Image: tc.image,
					},
				},
				SandboxConfig: sandboxRequest.Config,
			}
			containerID := createContainer(t, client, ctx, request)
			defer removeContainer(t, client, ctx, containerID)

			startContainer(t, client, ctx, containerID)
			// Wait few seconds for the container to be completely initialized
			time.Sleep(5 * time.Second)
			assertContainerState(t, client, ctx, containerID, runtime.ContainerState_CONTAINER_RUNNING)

			startTimeOfContainer := time.Now()
			// stop container with timeout of 15 seconds
			stopContainerWithTimeout(t, client, ctx, containerID, 15)
			assertContainerState(t, client, ctx, containerID, runtime.ContainerState_CONTAINER_EXITED)
			// get time elapsed before and after container stop command was issued
			elapsedTime := time.Since(startTimeOfContainer)
			// Ensure that the container has stopped after approx 15 seconds.
			// We are giving it a buffer of +/- 1 second
			if elapsedTime < 14*time.Second || elapsedTime > 16*time.Second {
				t.Fatalf("Container did not shutdown gracefully \n")
			}
		})
	}
}
