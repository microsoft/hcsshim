//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"testing"

	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// CRI will terminate any running containers when it is restarted.
// Run a container, restart containerd, validate the container is terminated.
func Test_ContainerdRestart_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW, featureTerminateOnRestart)

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

	t.Log("Restart containerd")
	stopContainerd(t)
	startContainerd(t)
	client = newTestRuntimeClient(t)

	containerStatus, err := client.ContainerStatus(ctx, &runtime.ContainerStatusRequest{ContainerId: containerID})
	if err != nil {
		t.Fatal(err)
	}
	if containerStatus.Status.State != runtime.ContainerState_CONTAINER_EXITED {
		t.Errorf("Container was not terminated on containerd restart. Status is %d", containerStatus.Status.State)
	}
	podStatus, err := client.PodSandboxStatus(ctx, &runtime.PodSandboxStatusRequest{PodSandboxId: podID})
	if err != nil {
		t.Fatal(err)
	}
	if podStatus.Status.State != runtime.PodSandboxState_SANDBOX_NOTREADY {
		t.Errorf("Pod was not terminated on containerd restart. Status is %d", podStatus.Status.State)
	}
}
