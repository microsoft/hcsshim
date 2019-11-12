// +build functional

package cri_containerd

import (
	"context"
	"testing"

	runtime "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

func Test_StopContainer_LCOW(t *testing.T) {
	pullRequiredLcowImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sandboxRequest := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name() + "-Sandbox",
				Namespace: testNamespace,
			},
		},
		RuntimeHandler: lcowRuntimeHandler,
	}

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
	pullRequiredLcowImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sandboxRequest := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name() + "-Sandbox",
				Namespace: testNamespace,
			},
		},
		RuntimeHandler: lcowRuntimeHandler,
	}

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
	pullRequiredLcowImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sandboxRequest := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name() + "-Sandbox",
				Namespace: testNamespace,
			},
		},
		RuntimeHandler: lcowRuntimeHandler,
	}

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
