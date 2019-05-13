// +build functional,failing_tests

package cri_containerd

import (
	"context"
	"strings"
	"testing"

	runtime "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

//TODO:
// Create POD, Container interfaces and make
// Runp, create, exec, start, stop as methods instead of funcs
func execContainer(ctx context.Context, t *testing.T, client runtime.RuntimeServiceClient, request *runtime.ExecSyncRequest) *runtime.ExecSyncResponse {
	response, err := client.ExecSync(ctx, request)
	if err != nil {
		t.Fatalf("failed ExecSync in container: %s, with: %v", request.ContainerId, err)
	}
	return response
}

func runexecContainerTestWithSandbox(t *testing.T, sandboxRequest *runtime.RunPodSandboxRequest, request *runtime.CreateContainerRequest, execReq *runtime.ExecSyncRequest) *runtime.ExecSyncResponse {
	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer func() {
		stopAndRemovePodSandbox(t, client, ctx, podID)
	}()
	request.PodSandboxId = podID
	request.SandboxConfig = sandboxRequest.Config
	containerID := createContainer(t, client, ctx, request)
	defer func() {
		stopAndRemoveContainer(t, client, ctx, containerID)
	}()
	_, err := client.StartContainer(ctx, &runtime.StartContainerRequest{
		ContainerId: containerID,
	})
	if err != nil {
		t.Fatalf("failed StartContainer request for container: %s, with: %v", containerID, err)
	}

	execReq.ContainerId = containerID
	return execContainer(ctx, t, client, execReq)
}

func Test_ExecContainer_RunAs_LCOW(t *testing.T) {
	pullRequiredLcowImages(t, []string{imageLcowK8sPause, imageLcowCosmos})

	//run podsandbox request
	sandboxRequest := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name() + "-Sandbox2",
				Uid:       "0",
				Namespace: testNamespace,
			},
			Linux: &runtime.LinuxPodSandboxConfig{
				SecurityContext: &runtime.LinuxSandboxSecurityContext{
					Privileged: true,
				},
			},
		},
		RuntimeHandler: lcowRuntimeHandler,
	}

	//create container request
	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container2",
			},
			Image: &runtime.ImageSpec{
				Image: imageLcowCosmos,
			},
			// Hold this command open until killed
			Command: []string{
				"sleep",
				"100000",
			},
			Linux: &runtime.LinuxContainerConfig{
				SecurityContext: &runtime.LinuxContainerSecurityContext{
					RunAsUser: &runtime.Int64Value{
						Value: 9000, // cosmosarno user
					},
				},
			},
		},
	}

	//exec request
	execRequest := &runtime.ExecSyncRequest{
		ContainerId: "",
		// this is just saying 'give me the UID of the process with pid = 1; ignore headers'
		Cmd: []string{
			"ps",
			"-o",
			"uid",
			"-p",
			"1",
			"--no-headers",
		},
		Timeout: 20,
	}

	r := runexecContainerTestWithSandbox(t, sandboxRequest, request, execRequest)
	output := strings.TrimSpace(string(r.Stdout))
	errorMsg := string(r.Stderr)
	exitCode := int(r.ExitCode)

	t.Logf("exec request exited with code: %d", exitCode)
	t.Logf("exec request output: %v", output)

	if exitCode != 0 {
		t.Fatalf("Test %v exited with exit code: %d, Test_CreateContainer_RunAs_LCOW", errorMsg, exitCode)
	}

	if output != "9000" {
		t.Fatalf("failed to start container with runas option: error: %v, exitcode: %d", errorMsg, exitCode)
	}
}
