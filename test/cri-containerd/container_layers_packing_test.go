//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Microsoft/hcsshim/osversion"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/Microsoft/hcsshim/test/internal/require"
)

const (
	ubuntu1804          = "ubuntu@sha256:07782849f2cff04e9bc29449c27d0fb2076e61e8bdb4475ec5dbc5386ed41a4f"
	ubuntu70ExtraLayers = "cplatpublic.azurecr.io/ubuntu70extra:18.04"
	alpine70ExtraLayers = "cplatpublic.azurecr.io/alpine70extra:latest"
)

func validateTargets(ctx context.Context, t *testing.T, deviceNumber int, podID string, expected int) {
	t.Helper()
	dmDiag := shimDiagExecOutput(ctx, t, podID, []string{"ls", "-l", "/dev/mapper"})
	dmPattern := fmt.Sprintf("dm-linear-pmem%d", deviceNumber)
	dmLines := filterStrings(strings.Split(dmDiag, "\n"), dmPattern)

	lrDiag := shimDiagExecOutput(ctx, t, podID, []string{"ls", "-l", "/run/layers"})
	lrPattern := fmt.Sprintf("p%d", deviceNumber)
	lrLines := filterStrings(strings.Split(lrDiag, "\n"), lrPattern)
	if len(lrLines) != len(dmLines) {
		t.Fatalf("number of layers and device-mapper targets mismatch:\n%s\n%s", dmDiag, lrDiag)
	}

	if len(dmLines) != expected {
		t.Fatalf("expected %d layers, got %d.\n%s\n%s", expected, len(dmLines), dmDiag, lrDiag)
	}
}

func Test_Container_Layer_Packing_On_VPMem(t *testing.T) {
	require.Build(t, osversion.V19H1)

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	requireFeatures(t, featureLCOW)

	// use ubuntu to make sure that multiple container layers will be mapped properly
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, ubuntu1804})

	type config struct {
		rootfsType   string
		deviceNumber int
	}

	for _, scenario := range []config{
		{
			rootfsType:   "initrd",
			deviceNumber: 0,
		},
		{
			rootfsType:   "vhd",
			deviceNumber: 1,
		},
	} {
		t.Run(fmt.Sprintf("PreferredRootFSType-%s", scenario.rootfsType), func(t *testing.T) {
			annots := map[string]string{
				annotations.PreferredRootFSType: scenario.rootfsType,
			}
			podReq := getRunPodSandboxRequest(t, lcowRuntimeHandler, WithSandboxAnnotations(annots))
			podID := runPodSandbox(t, client, ctx, podReq)
			defer removePodSandbox(t, client, ctx, podID)

			cmd := []string{"bash", "-c", "while true; do echo 'Hello, World!'; sleep 1; done"}
			contReq := getCreateContainerRequest(podID, "ubuntu_latest", ubuntu1804, cmd, podReq.Config)
			containerID := createContainer(t, client, ctx, contReq)
			defer removeContainer(t, client, ctx, containerID)

			startContainer(t, client, ctx, containerID)

			// check initial targets
			// NOTE: as of 08/12/2021, ubuntu:18.04 (digest: sha256:07782849f2cff04e9bc29449c27d0fb2076e61e8bdb4475ec5dbc5386ed41a4f)
			// image has 1 image layer and k8s pause container has 1 layer
			validateTargets(ctx, t, scenario.deviceNumber, podID, 2)

			// stop container
			stopContainer(t, client, ctx, containerID)
			// only pause container layer should be mounted at this point
			validateTargets(ctx, t, scenario.deviceNumber, podID, 1)
		})
	}
}

func Test_Many_Container_Layers_Supported_On_VPMem(t *testing.T) {
	require.Build(t, osversion.V19H1)

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, alpine70ExtraLayers, ubuntu70ExtraLayers})

	podReq := getRunPodSandboxRequest(t, lcowRuntimeHandler)
	podID := runPodSandbox(t, client, ctx, podReq)
	defer removePodSandbox(t, client, ctx, podID)

	cmd := []string{"bash", "-c", "while true; do echo 'Hello, World!'; sleep 1; done"}

	contReq1 := getCreateContainerRequest(podID, "ubuntu70extra", ubuntu70ExtraLayers, cmd, podReq.Config)
	containerID1 := createContainer(t, client, ctx, contReq1)
	defer removeContainer(t, client, ctx, containerID1)
	startContainer(t, client, ctx, containerID1)
	defer stopContainer(t, client, ctx, containerID1)

	cmd = []string{"ash", "-c", "while true; do echo 'Hello, World!'; sleep 1; done"}
	contReq2 := getCreateContainerRequest(podID, "alpine70extra", alpine70ExtraLayers, cmd, podReq.Config)
	containerID2 := createContainer(t, client, ctx, contReq2)
	defer removeContainer(t, client, ctx, containerID2)
	startContainer(t, client, ctx, containerID2)
	defer stopContainer(t, client, ctx, containerID2)
}

func Test_Annotation_Disable_Multi_Mapping(t *testing.T) {
	require.Build(t, osversion.V19H1)

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, alpine70ExtraLayers})

	annots := map[string]string{
		annotations.VPMemNoMultiMapping: "true",
	}
	podReq := getRunPodSandboxRequest(t, lcowRuntimeHandler, WithSandboxAnnotations(annots))
	podID := runPodSandbox(t, client, ctx, podReq)
	defer removePodSandbox(t, client, ctx, podID)

	cmd := []string{"bash", "-c", "while true; do echo 'Hello, World!'; sleep 1; done"}
	contReq := getCreateContainerRequest(podID, "ubuntu", ubuntu70ExtraLayers, cmd, podReq.Config)
	containerID := createContainer(t, client, ctx, contReq)
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	dmDiag := shimDiagExecOutput(ctx, t, podID, []string{"ls", "-l", "/dev/mapper"})
	filtered := filterStrings(strings.Split(dmDiag, "\n"), "dm-linear")
	if len(filtered) > 0 {
		t.Fatalf("no linear devices should've been created.\n%s", dmDiag)
	}
}
