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

func execContainerLCOW(t *testing.T, uid int64, cmd []string) *runtime.ExecSyncResponse {
	t.Helper()
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowCosmos})

	// run podsandbox request
	sandboxRequest := getRunPodSandboxRequest(t, lcowRuntimeHandler)
	sandboxRequest.Config.Linux = &runtime.LinuxPodSandboxConfig{
		SecurityContext: &runtime.LinuxSandboxSecurityContext{
			Privileged: true,
		},
	}

	// create container request
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
					// Our tests rely on the init process for the workload
					// container being pid 1, but the CRI default is to use the
					// pod's pid namespace.
					NamespaceOptions: &runtime.NamespaceOption{
						Pid: runtime.NamespaceMode_CONTAINER,
					},
					RunAsUser: &runtime.Int64Value{
						Value: uid,
					},
				},
			},
		},
	}

	//exec request
	execRequest := &runtime.ExecSyncRequest{
		ContainerId: "",
		Cmd:         cmd,
		Timeout:     20,
	}

	return runexecContainerTestWithSandbox(t, sandboxRequest, request, execRequest)
}

func Test_ExecContainer_RunAs_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	// this is just saying 'give me the UID of the process with pid = 1; ignore headers'
	r := execContainerLCOW(t,
		9000, // cosmosarno user
		[]string{
			"ps",
			"-o",
			"uid",
			"-p",
			"1",
			"--no-headers",
		})

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

func Test_ExecContainer_LCOW_HasEntropy(t *testing.T) {
	requireFeatures(t, featureLCOW)

	r := execContainerLCOW(t, 9000, []string{"cat", "/proc/sys/kernel/random/entropy_avail"})
	if r.ExitCode != 0 {
		t.Fatalf("failed with exit code %d to cat entropy_avail: %s", r.ExitCode, r.Stderr)
	}
	output := strings.TrimSpace(string(r.Stdout))
	bits, err := strconv.ParseInt(output, 10, 0)
	if err != nil {
		t.Fatalf("could not parse entropy output %s: %s", output, err)
	}
	if bits < 256 {
		t.Fatalf("%d is fewer than 256 bits entropy", bits)
	}
	t.Logf("got %d bits entropy", bits)
}
