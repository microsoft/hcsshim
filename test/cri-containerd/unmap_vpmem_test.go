//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"testing"
)

const (
	ubuntu2004LargeLayers = "cplatpublic.azurecr.io/ubuntu2004largelayers:latest"
	ubuntu2204LargeLayers = "cplatpublic.azurecr.io/ubuntu2204largelayers:latest"
)

// The container images used in this test have 4 layers with large size over
// 512MiB. We start the containers in order: ubuntu-22.04 and then ubuntu-20.04.
// ubuntu-22.04 container layers span multiple VPMem devices, but importantly
// one of the layers is allocated at VPMem device 2 offset 0. The ubuntu-20.04
// container layers are also large and a few are allocated at VPMem device 2
// and non-zero offset. When ubuntu-22.04 container is stopped and removed we
// end-up in the edge case situation where there's no mapped VHD on VPMem 2 and
// offset 0. With old behavior, where we removed VPMem device, the resource
// cleanup for container ubuntu-20.04 would fail, but the layer would still be
// "present" from hcsshim's perspective and we'd run into overlay fs mount
// failure when re-creating container ubuntu-20.04 again.
// With the behavior now changed not to remove the VPMem device, but rather
// leave it intact and recycle, the bug no longer surfaces.
func Test_Force_LayerUnmap_Not_In_Order(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, ubuntu2004LargeLayers, ubuntu2204LargeLayers})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sandboxRequest := getRunPodSandboxRequest(t, lcowRuntimeHandler)

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	var containerIDs []string
	requestTemplate := getCreateContainerRequest(
		podID,
		"",
		"",
		[]string{"bash", "-c", "while true; do echo 'hello'; sleep 1; done"},
		sandboxRequest.Config,
	)

	createAndStartContainers := func() {
		for _, config := range []struct {
			containerName string
			imageName     string
			command       []string
		}{
			{
				containerName: "ubuntu-22.04-v1",
				imageName:     ubuntu2204LargeLayers,
			},
			{
				containerName: "ubuntu-20.04-v1",
				imageName:     ubuntu2004LargeLayers,
			},
		} {
			requestTemplate.Config.Metadata.Name = config.containerName
			requestTemplate.Config.Image.Image = config.imageName

			containerID := createContainer(t, client, ctx, requestTemplate)
			startContainer(t, client, ctx, containerID)
			containerIDs = append(containerIDs, containerID)
		}
	}

	cleanup := func(ids []string) {
		for _, id := range ids {
			stopContainer(t, client, ctx, id)
			removeContainer(t, client, ctx, id)
		}
	}

	// The first round of container create and starts will go as usual. An error
	// will happen when releasing resources, but it's not bubbled up to the caller.
	createAndStartContainers()
	cleanup(containerIDs)

	// Now we would be in a broken state, where the layer has been unmounted
	// inside the guest, but not on the host and mounting container overlay fs
	// will fail.
	createAndStartContainers()
}
