//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"strconv"
	"strings"
	"testing"

	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func runexecContainerTestWithSandbox(t *testing.T, sandboxRequest *runtime.RunPodSandboxRequest, request *runtime.CreateContainerRequest, execReq *runtime.ExecSyncRequest) *runtime.ExecSyncResponse {
	t.Helper()
	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	request.PodSandboxId = podID
	request.SandboxConfig = sandboxRequest.Config

	containerID := createContainer(t, client, ctx, request)
	defer removeContainer(t, client, ctx, containerID)

	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	execReq.ContainerId = containerID
	return execSync(t, client, ctx, execReq)
}
