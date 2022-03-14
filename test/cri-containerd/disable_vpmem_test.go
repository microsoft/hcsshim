//go:build functional
// +build functional

package cri_containerd

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/Microsoft/hcsshim/pkg/annotations"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func Test_70LayerImagesWithNoVPmemForLayers(t *testing.T) {
	requireFeatures(t, featureLCOW)

	ubuntu70Image := "cplatpublic.azurecr.io/ubuntu70extra:18.04"
	alpine70Image := "cplatpublic.azurecr.io/alpine70extra:latest"
	testImages := []string{ubuntu70Image, alpine70Image}
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, ubuntu70Image, alpine70Image})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nContainers := 4
	containerIDs := make([]string, nContainers)

	for i := 0; i < nContainers; i++ {
		defer cleanupContainer(t, client, ctx, &containerIDs[i])
	}

	sandboxRequest := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			// 1 VPMEM device will be used for UVM VHD and all container layers
			// will be attached over SCSI.
			annotations.VPMemCount: "1",
			// Make sure the 1 VPMEM device isn't used for multimapping.
			annotations.VPMemNoMultiMapping: "true",
		}),
	)
	// override pod name
	sandboxRequest.Config.Metadata.Name = fmt.Sprintf("%s-pod", t.Name())

	response, err := client.RunPodSandbox(ctx, sandboxRequest)
	if err != nil {
		t.Fatalf("failed RunPodSandbox request with: %v", err)
	}
	podID := response.PodSandboxId

	var wg sync.WaitGroup
	wg.Add(nContainers)
	for idx := 0; idx < nContainers; idx++ {
		go func(i int) {
			request := &runtime.CreateContainerRequest{
				PodSandboxId: podID,
				Config: &runtime.ContainerConfig{
					Metadata: &runtime.ContainerMetadata{
						Name: fmt.Sprintf("%s-container-%d", t.Name(), i),
					},
					Image: &runtime.ImageSpec{
						Image: testImages[i%2],
					},
					Command: []string{
						"/bin/sh",
						"-c",
						"while true; do echo 'Hello, World!'; sleep 1; done",
					},
				},
				SandboxConfig: sandboxRequest.Config,
			}

			containerIDs[i] = createContainer(t, client, ctx, request)
			startContainer(t, client, ctx, containerIDs[i])
			wg.Done()
		}(idx)
	}
	wg.Wait()

	for i := 0; i < nContainers; i++ {
		containerExecReq := &runtime.ExecSyncRequest{
			ContainerId: containerIDs[i],
			Cmd:         []string{"ls"},
			Timeout:     20,
		}
		r := execSync(t, client, ctx, containerExecReq)
		if r.ExitCode != 0 {
			t.Fatalf("failed to exec inside container, exit code: %d", r.ExitCode)
		}
	}
}
