// +build functional

package cri_containerd

import (
	"context"
	"testing"

	"github.com/Microsoft/hcsshim/internal/oci"
)

func Test_KernelOptions_To_LCOW_Sandbox(t *testing.T) {
	requireFeatures(t, featureLCOW)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := newTestRuntimeClient(t)

	// use ubuntu to make sure that multiple container layers will be mapped properly
	pullRequiredLcowImages(t, []string{imageLcowK8sPause, ubuntu1804})

	annotations := map[string]string{
		oci.AnnotationFullyPhysicallyBacked: "true",
		oci.AnnotationMemorySizeInMB:        "2048",
		oci.AnnotationKernelBootOptions:     "hugepagesz=2M hugepages=10",
	}
	podReq := getRunPodSandboxRequest(t, lcowRuntimeHandler, annotations)
	podID := runPodSandbox(t, client, ctx, podReq)
	defer removePodSandbox(t, client, ctx, podID)

	cmd := []string{"bash", "-c", "while true; do echo 'Hello, World!'; sleep 1; done"}
	contReq := getCreateContainerRequest(podID, "ubuntu_latest", ubuntu1804, cmd, podReq.Config)
	containerID := createContainer(t, client, ctx, contReq)
	defer removeContainer(t, client, ctx, containerID)

	startContainer(t, client, ctx, containerID)

	// stop container
	stopContainer(t, client, ctx, containerID)

}
