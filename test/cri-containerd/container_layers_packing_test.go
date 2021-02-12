// +build functional

package cri_containerd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Microsoft/hcsshim/internal/shimdiag"
	"github.com/Microsoft/hcsshim/osversion"
	testutilities "github.com/Microsoft/hcsshim/test/functional/utilities"
)

const (
	ubuntu1804          = "ubuntu:18.04"
	ubuntu70ExtraLayers = "cplatpublic.azurecr.io/ubuntu70extra:18.04"
	alpine70ExtraLayers = "cplatpublic.azurecr.io/alpine70extra:latest"
)

func filterStrings(input []string, include string) []string {
	var result []string
	for _, str := range input {
		if strings.Contains(str, include) {
			result = append(result, str)
		}
	}
	return result
}

func shimDiagExec(ctx context.Context, t *testing.T, podID string, cmd []string) string {
	shimName := fmt.Sprintf("k8s.io-%s", podID)
	shim, err := shimdiag.GetShim(shimName)
	if err != nil {
		t.Fatalf("failed to find shim %v: %v", shimName, err)
	}
	shimClient := shimdiag.NewShimDiagClient(shim)

	bufOut := &bytes.Buffer{}
	bw := bufio.NewWriter(bufOut)
	bufErr := &bytes.Buffer{}
	bwErr := bufio.NewWriter(bufErr)

	exitCode, err := execInHost(ctx, shimClient, cmd, nil, bw, bwErr)
	if err != nil {
		t.Fatalf("failed to exec request in the host with: %v and %v", err, bufErr.String())
	}
	if exitCode != 0 {
		t.Fatalf("exec request in host failed with exit code %v: %v", exitCode, bufErr.String())
	}

	return strings.TrimSpace(bufOut.String())
}

func validateTargets(ctx context.Context, t *testing.T, deviceNumber int, podID string, expected int) {
	dmDiag := shimDiagExec(ctx, t, podID, []string{"ls", "-l", "/dev/mapper"})
	dmPattern := fmt.Sprintf("dm-linear-pmem%d", deviceNumber)
	dmLines := filterStrings(strings.Split(dmDiag, "\n"), dmPattern)

	lrDiag := shimDiagExec(ctx, t, podID, []string{"ls", "-l", "/run/layers"})
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
	testutilities.RequiresBuild(t, osversion.V19H1)

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	requireFeatures(t, featureLCOW)

	// use ubuntu to make sure that multiple container layers will be mapped properly
	pullRequiredLcowImages(t, []string{imageLcowK8sPause, ubuntu1804})

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
			annotations := map[string]string{
				"io.microsoft.virtualmachine.lcow.preferredrootfstype": scenario.rootfsType,
			}
			podReq := getRunPodSandboxRequest(t, lcowRuntimeHandler, annotations)
			podID := runPodSandbox(t, client, ctx, podReq)
			defer removePodSandbox(t, client, ctx, podID)

			cmd := []string{"bash", "-c", "while true; do echo 'Hello, World!'; sleep 1; done"}
			contReq := getCreateContainerRequest(podID, "ubuntu_latest", ubuntu1804, cmd, podReq.Config)
			containerID := createContainer(t, client, ctx, contReq)
			defer removeContainer(t, client, ctx, containerID)

			startContainer(t, client, ctx, containerID)

			// check initial targets
			// NOTE: as of 02/03/2021, ubuntu:18.04 image has 3 image layers and k8s pause container has 1 layer
			validateTargets(ctx, t, scenario.deviceNumber, podID, 4)

			// stop container
			stopContainer(t, client, ctx, containerID)
			// only pause container layer should be mounted at this point
			validateTargets(ctx, t, scenario.deviceNumber, podID, 1)
		})
	}
}

func Test_Many_Container_Layers_Supported_On_VPMem(t *testing.T) {
	testutilities.RequiresBuild(t, osversion.V19H1)

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause, alpine70ExtraLayers, ubuntu70ExtraLayers})

	podReq := getRunPodSandboxRequest(t, lcowRuntimeHandler, nil)
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
	testutilities.RequiresBuild(t, osversion.V19H1)

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	requireFeatures(t, featureLCOW)

	pullRequiredLcowImages(t, []string{imageLcowK8sPause, alpine70ExtraLayers})

	annotations := map[string]string{
		"io.microsoft.virtualmachine.lcow.vpmem.nomultimapping": "true",
	}
	podReq := getRunPodSandboxRequest(t, lcowRuntimeHandler, annotations)
	podID := runPodSandbox(t, client, ctx, podReq)
	defer removePodSandbox(t, client, ctx, podID)

	cmd := []string{"bash", "-c", "while true; do echo 'Hello, World!'; sleep 1; done"}
	contReq := getCreateContainerRequest(podID, "ubuntu", ubuntu70ExtraLayers, cmd, podReq.Config)
	containerID := createContainer(t, client, ctx, contReq)
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	dmDiag := shimDiagExec(ctx, t, podID, []string{"ls", "-l", "/dev/mapper"})
	filtered := filterStrings(strings.Split(dmDiag, "\n"), "dm-linear")
	if len(filtered) > 0 {
		t.Fatalf("no linear devices should've been created.\n%s", dmDiag)
	}
}
