//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func Test_Container_File_Share_Writable_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	client := newTestRuntimeClient(t)
	ctx := context.Background()

	sbRequest := getRunPodSandboxRequest(t, wcowHypervisorRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.DisableWritableFileShares: "true",
		}),
	)
	podID := runPodSandbox(t, client, ctx, sbRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	var (
		tempDir       = t.TempDir()
		containerPath = "C:\\test"
		testFile      = "t.txt"
		testContent   = "hello world"
	)

	if err := os.WriteFile(
		filepath.Join(tempDir, testFile),
		[]byte(testContent),
		0644); err != nil {
		t.Fatalf("could not create test file: %v", err)
	}

	cRequest := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name(),
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			Command: []string{
				"cmd",
				"/c",
				"ping -t 127.0.0.1",
			},
			Mounts: []*runtime.Mount{
				{
					HostPath:      tempDir,
					ContainerPath: containerPath,
					Readonly:      false,
				},
			},
		},
		PodSandboxId:  podID,
		SandboxConfig: sbRequest.Config,
	}
	cID := createContainer(t, client, ctx, cRequest)
	defer removeContainer(t, client, ctx, cID)

	// container should fail because of writable mount
	_, err := client.StartContainer(ctx, &runtime.StartContainerRequest{ContainerId: cID})
	if err == nil {
		stopContainer(t, client, ctx, cID)
	}
	// error is serialized over gRPC then embedded into "rpc error: code = %s desc = %s"
	//  so error.Is() wont work
	if err == nil || !strings.Contains(err.Error(), fmt.Errorf("adding writable shares is denied: %w", hcs.ErrOperationDenied).Error()) {
		t.Fatalf("StartContainer did not fail with writable fileshare: error is %v", err)
	}

	// set it to read only
	cRequest.Config.Metadata.Name = t.Name() + "_2"
	cRequest.Config.Mounts[0].Readonly = true

	cID = createContainer(t, client, ctx, cRequest)
	defer removeContainer(t, client, ctx, cID)
	startContainer(t, client, ctx, cID)
	defer stopContainer(t, client, ctx, cID)

	testExec := []string{
		"cmd",
		"/c",
		"type",
		filepath.Join(containerPath, testFile),
	}
	output, errMsg, exitCode := execContainer(t, client, ctx, cID, testExec)
	if exitCode != 0 {
		t.Fatalf("could not find mounted file: %s %s", errMsg, output)
	}
	if output != testContent {
		t.Fatalf("did not correctly read file; got %q, expected %q", output, testContent)
	}
}

func Test_Container_File_Share_Writable_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx := context.Background()

	sbRequest := getRunPodSandboxRequest(t, lcowRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.DisableWritableFileShares: "true",
		}),
	)
	podID := runPodSandbox(t, client, ctx, sbRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	var (
		tempDir       = t.TempDir()
		containerPath = "/mnt/test"
		testFile      = "t.txt"
		testContent   = "hello world"
	)

	if err := os.WriteFile(
		filepath.Join(tempDir, testFile),
		[]byte(testContent),
		0644,
	); err != nil {
		t.Fatalf("could not create test file: %v", err)
	}

	cRequest := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name(),
			},
			Image: &runtime.ImageSpec{
				Image: imageLcowAlpine,
			},
			Command: []string{
				"ash",
				"-c",
				"tail -f /dev/null",
			},
			Mounts: []*runtime.Mount{
				{
					HostPath:      tempDir,
					ContainerPath: containerPath,
					Readonly:      false,
				},
			},
		},
		PodSandboxId:  podID,
		SandboxConfig: sbRequest.Config,
	}
	cID := createContainer(t, client, ctx, cRequest)
	defer removeContainer(t, client, ctx, cID)

	// container should fail because of writable mount
	_, err := client.StartContainer(ctx, &runtime.StartContainerRequest{ContainerId: cID})
	if err == nil {
		stopContainer(t, client, ctx, cID)
	}
	// error is serialized over gRPC then embedded into "rpc error: code = %s desc = %s"
	//  so error.Is() wont work
	if err == nil || !strings.Contains(err.Error(), fmt.Errorf("adding writable shares is denied: %w", hcs.ErrOperationDenied).Error()) {
		t.Fatalf("StartContainer did not fail with writable fileshare: error is %v", err)
	}

	// set it to read only
	cRequest.Config.Metadata.Name = t.Name() + "_2"
	cRequest.Config.Mounts[0].Readonly = true

	cID = createContainer(t, client, ctx, cRequest)
	defer removeContainer(t, client, ctx, cID)
	startContainer(t, client, ctx, cID)
	defer stopContainer(t, client, ctx, cID)

	testExec := []string{
		"ash",
		"-c",
		// filepath.Join replaces `/` with `\`, so Join path manually
		"cat " + containerPath + "/" + testFile,
	}
	output, errMsg, exitCode := execContainer(t, client, ctx, cID, testExec)
	if exitCode != 0 {
		t.Fatalf("could not find mounted file: %s %s", errMsg, output)
	}
	if output != testContent {
		t.Fatalf("did not correctly read file; got %q, expected %q", output, testContent)
	}
}
