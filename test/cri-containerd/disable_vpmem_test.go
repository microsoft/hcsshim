//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/pkg/annotations"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// Use unique names for pods & containers so that if we run this test multiple times in parallel we don't
// get failures due to same pod/container names.
func uniqueRef() string {
	t := time.Now()
	var b [3]byte
	// Ignore read failures, just decreases uniqueness
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%d-%s", t.UnixNano(), base64.URLEncoding.EncodeToString(b[:]))
}

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
	podID := ""
	containerIDs := make([]string, nContainers)

	defer cleanupPod(t, client, ctx, &podID)
	for i := 0; i < nContainers; i++ {
		defer cleanupContainer(t, client, ctx, &containerIDs[i])
	}

	sandboxRequest := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.VPMemCount: "0",
		}),
	)
	// override pod name
	sandboxRequest.Config.Metadata.Name = fmt.Sprintf("%s-pod-%s", t.Name(), uniqueRef())

	response, err := client.RunPodSandbox(ctx, sandboxRequest)
	if err != nil {
		t.Fatalf("failed RunPodSandbox request with: %v", err)
	}
	podID = response.PodSandboxId

	var wg sync.WaitGroup
	wg.Add(nContainers)
	for idx := 0; idx < nContainers; idx++ {
		go func(i int) {
			defer wg.Done()
			request := &runtime.CreateContainerRequest{
				PodSandboxId: podID,
				Config: &runtime.ContainerConfig{
					Metadata: &runtime.ContainerMetadata{
						Name: fmt.Sprintf("%s-container", uniqueRef()),
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
