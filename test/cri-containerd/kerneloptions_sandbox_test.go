// +build functional

package cri_containerd

import (
	"context"
	"strings"
	"testing"

	"github.com/Microsoft/hcsshim/internal/oci"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func Test_KernelOptions_To_LCOW_Sandbox(t *testing.T) {
	requireFeatures(t, featureLCOW)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := newTestRuntimeClient(t)

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
	defer stopContainer(t, client, ctx, containerID)

	// check /proc/meminfo, HugePages_Total should be set to 10
	execResponse := execSync(t, client, ctx, &runtime.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         []string{"grep", "-i", "huge", "/proc/meminfo"},
	})

	if !strings.Contains(string(execResponse.Stdout), "HugePages_Total:      10") {
		t.Fatalf("Expected number of hugepages to be 10. Got output instead: %s", string(execResponse.Stdout))
	}
}
