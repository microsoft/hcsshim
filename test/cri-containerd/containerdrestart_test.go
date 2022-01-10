//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/pkg/annotations"
	criAnnotations "github.com/kevpar/cri/pkg/annotations"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// CRI will terminate any running containers when it is restarted.
// Run a container, restart containerd, validate the container is terminated.
func Test_ContainerdRestart_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW, featureTerminateOnRestart)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sandboxRequest := getRunPodSandboxRequest(t, lcowRuntimeHandler)

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	request := &runtime.CreateContainerRequest{
		PodSandboxId: podID,
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageLcowAlpine,
			},
			Command: []string{
				"top",
			},
		},
		SandboxConfig: sandboxRequest.Config,
	}

	containerID := createContainer(t, client, ctx, request)
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	t.Log("Restart containerd")
	stopContainerd(t)
	startContainerd(t)
	client = newTestRuntimeClient(t)

	containerStatus, err := client.ContainerStatus(ctx, &runtime.ContainerStatusRequest{ContainerId: containerID})
	if err != nil {
		t.Fatal(err)
	}
	if containerStatus.Status.State != runtime.ContainerState_CONTAINER_EXITED {
		t.Errorf("Container was not terminated on containerd restart. Status is %d", containerStatus.Status.State)
	}
	podStatus, err := client.PodSandboxStatus(ctx, &runtime.PodSandboxStatusRequest{PodSandboxId: podID})
	if err != nil {
		t.Fatal(err)
	}
	if podStatus.Status.State != runtime.PodSandboxState_SANDBOX_NOTREADY {
		t.Errorf("Pod was not terminated on containerd restart. Status is %d", podStatus.Status.State)
	}
}

// test restarting containers and pods
func Test_Container_CRI_Restart(t *testing.T) {
	pullRequiredImages(t, []string{imageWindowsNanoserver})
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	pluginClient := newTestPluginClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tests := []struct {
		Name        string
		Feature     string
		Runtime     string
		SandboxOpts []SandboxConfigOpt
		Image       string
		Command     []string
	}{
		{
			Name:    "LCOW",
			Feature: featureLCOW,
			Runtime: lcowRuntimeHandler,
			SandboxOpts: []SandboxConfigOpt{WithSandboxAnnotations(map[string]string{
				annotations.DisableLCOWTimeSyncService: "true",
			})},
			Image: imageLcowAlpine,
			Command: []string{
				"ash",
				"-c",
				"tail -f /dev/null",
			},
		},
		{
			Name:    "WCOW_Hypervisor",
			Feature: featureWCOWHypervisor,
			Runtime: wcowHypervisorRuntimeHandler,
			Image:   imageWindowsNanoserver,
			Command: []string{
				"cmd",
				"/c",
				"ping -t 127.0.0.1",
			},
		},
		{
			Name:    "WCOW_Process",
			Feature: featureWCOWProcess,
			Runtime: wcowHypervisorRuntimeHandler,
			Image:   imageWindowsNanoserver,
			Command: []string{
				"cmd",
				"/c",
				"ping -t 127.0.0.1",
			},
		},
	}

	for _, tt := range tests {
		for _, explicit := range []bool{false, true} {
			suffix := "_Implicit"
			if explicit {
				suffix = "_Explicit"
			}

			t.Run(tt.Name+suffix, func(subtest *testing.T) {
				requireFeatures(subtest, tt.Feature)

				opts := tt.SandboxOpts
				if !explicit {
					opts = append(tt.SandboxOpts,
						WithSandboxAnnotations(map[string]string{
							criAnnotations.EnableReset: "true",
						}))
				}
				sandboxRequest := getRunPodSandboxRequest(subtest, tt.Runtime, opts...)
				podID := runPodSandbox(subtest, client, ctx, sandboxRequest)
				defer removePodSandbox(subtest, client, ctx, podID)
				defer stopPodSandbox(subtest, client, ctx, podID)

				request := &runtime.CreateContainerRequest{
					PodSandboxId: podID,
					Config: &runtime.ContainerConfig{
						Metadata: &runtime.ContainerMetadata{
							Name: subtest.Name() + "-Container",
						},
						Image: &runtime.ImageSpec{
							Image: tt.Image,
						},
						Command:     tt.Command,
						Annotations: map[string]string{},
					},
					SandboxConfig: sandboxRequest.Config,
				}

				if !explicit {
					request.Config.Annotations[criAnnotations.EnableReset] = "true"
				}

				containerID := createContainer(subtest, client, ctx, request)
				startContainer(subtest, client, ctx, containerID)
				defer removeContainer(subtest, client, ctx, containerID)
				defer stopContainer(subtest, client, ctx, containerID)

				/*******************************************************************
				* restart container
				*******************************************************************/
				stopContainer(subtest, client, ctx, containerID)
				state := getContainerStatus(subtest, client, ctx, containerID)
				if state != runtime.ContainerState_CONTAINER_EXITED {
					subtest.Fatalf("failed to initally stop container, state is %v", state)
				}

				if explicit {
					resetContainer(t, pluginClient, ctx, containerID)
					state = getContainerStatus(subtest, client, ctx, containerID)
					if state != runtime.ContainerState_CONTAINER_CREATED {
						subtest.Fatalf("failed to reset container, state is %v", state)
					}
				}

				startContainer(subtest, client, ctx, containerID)
				state = getContainerStatus(subtest, client, ctx, containerID)
				if state != runtime.ContainerState_CONTAINER_RUNNING {
					subtest.Fatalf("failed to restart container, state is %v", state)
				}

				/*******************************************************************
				* restart pod
				*******************************************************************/
				stopContainer(subtest, client, ctx, containerID)
				state = getContainerStatus(subtest, client, ctx, containerID)
				if state != runtime.ContainerState_CONTAINER_EXITED {
					subtest.Fatalf("failed to stop container, state is %v", state)
				}

				stopPodSandbox(subtest, client, ctx, podID)
				podState := getPodSandboxStatus(subtest, client, ctx, podID).State
				if podState != runtime.PodSandboxState_SANDBOX_NOTREADY {
					subtest.Fatalf("failed to stop pod sandbox, state is %v", podState)
				}

				if explicit {
					resetPodSandbox(t, pluginClient, ctx, podID)
				} else {
					newPodID := runPodSandbox(subtest, client, ctx, sandboxRequest)
					if newPodID != podID {
						defer removePodSandbox(subtest, client, ctx, newPodID)
						defer stopPodSandbox(subtest, client, ctx, newPodID)
						subtest.Fatalf("pod restarted with different id (%q) from original (%q)", newPodID, podID)
					}
				}

				podState = getPodSandboxStatus(subtest, client, ctx, podID).State
				if podState != runtime.PodSandboxState_SANDBOX_READY {
					subtest.Fatalf("failed to restart pod sandbox, state is %v", podState)
				}

				state = getContainerStatus(subtest, client, ctx, containerID)
				if state != runtime.ContainerState_CONTAINER_CREATED {
					subtest.Fatalf("failed to reset container, state is %v", state)
				}

				startContainer(subtest, client, ctx, containerID)
				state = getContainerStatus(subtest, client, ctx, containerID)
				if state != runtime.ContainerState_CONTAINER_RUNNING {
					subtest.Fatalf("failed to restart container, state is %v", state)
				}
			})
		}
	}
}

// test preserving state after restarting pod
func Test_Container_CRI_Restart_State(t *testing.T) {
	testFile := "t.txt"

	pullRequiredImages(t, []string{imageWindowsNanoserver})
	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tests := []struct {
		Name            string
		Feature         string
		Runtime         string
		SandboxOpts     []SandboxConfigOpt
		Image           string
		Command         []string
		SetStateCommand []string
		GetStateCommand []string
		ExpectedResult  string
	}{
		{
			Name:    "LCOW",
			Feature: featureLCOW,
			Runtime: lcowRuntimeHandler,
			SandboxOpts: []SandboxConfigOpt{WithSandboxAnnotations(map[string]string{
				annotations.DisableLCOWTimeSyncService: "true",
			})},
			Image:           imageLcowAlpine,
			Command:         []string{"ash", "-c", "tail -f /dev/null"},
			SetStateCommand: []string{"ash", "-c", "echo - >> " + testFile},
			GetStateCommand: []string{"ash", "-c", "cat " + testFile},
			ExpectedResult:  "-\n",
		},
		{
			Name:            "WCOW_Hypervisor",
			Feature:         featureWCOWHypervisor,
			Runtime:         wcowHypervisorRuntimeHandler,
			Image:           imageWindowsNanoserver,
			Command:         []string{"cmd", "/c", "ping -t 127.0.0.1"},
			SetStateCommand: []string{"cmd", "/c", "echo - >> " + testFile},
			GetStateCommand: []string{"cmd", "/c", "type", testFile},
			ExpectedResult:  "- \r\n",
		},
		{
			Name:            "WCOW_Process",
			Feature:         featureWCOWProcess,
			Runtime:         wcowHypervisorRuntimeHandler,
			Image:           imageWindowsNanoserver,
			Command:         []string{"cmd", "/c", "ping -t 127.0.0.1"},
			SetStateCommand: []string{"cmd", "/c", "echo - >> " + testFile},
			GetStateCommand: []string{"cmd", "/c", "type", testFile},
			ExpectedResult:  "- \r\n",
		},
	}

	for _, tt := range tests {
		for _, restart := range []bool{false, true} {
			suffix := "_Restart"
			if !restart {
				suffix = "_No" + suffix
			}

			t.Run(tt.Name+suffix, func(subtest *testing.T) {
				requireFeatures(subtest, tt.Feature)
				if restart {
					requireFeatures(subtest, featureTerminateOnRestart)
				}

				sandboxRequest := getRunPodSandboxRequest(subtest, tt.Runtime,
					append(tt.SandboxOpts,
						WithSandboxAnnotations(map[string]string{
							criAnnotations.EnableReset: "true",
						}))...)

				podID := runPodSandbox(subtest, client, ctx, sandboxRequest)
				defer removePodSandbox(subtest, client, ctx, podID)
				defer stopPodSandbox(subtest, client, ctx, podID)

				request := &runtime.CreateContainerRequest{
					PodSandboxId: podID,
					Config: &runtime.ContainerConfig{
						Metadata: &runtime.ContainerMetadata{
							Name: subtest.Name() + "-Container",
						},
						Image: &runtime.ImageSpec{
							Image: tt.Image,
						},
						Command: tt.Command,
						Annotations: map[string]string{
							criAnnotations.EnableReset: "true",
						},
					},
					SandboxConfig: sandboxRequest.Config,
				}

				containerID := createContainer(subtest, client, ctx, request)
				startContainer(subtest, client, ctx, containerID)
				defer removeContainer(subtest, client, ctx, containerID)
				defer func() {
					stopContainer(subtest, client, ctx, containerID)
				}()

				execRequest := &runtime.ExecSyncRequest{
					ContainerId: containerID,
					Cmd:         tt.SetStateCommand,
					Timeout:     1,
				}
				req := execSync(subtest, client, ctx, execRequest)
				if req.ExitCode != 0 {
					subtest.Fatalf("exec %v failed with exit code %d: %s", execRequest.Cmd, req.ExitCode, string(req.Stderr))
				}

				// check the write worked
				execRequest = &runtime.ExecSyncRequest{
					ContainerId: containerID,
					Cmd:         tt.GetStateCommand,
					Timeout:     1,
				}

				req = execSync(subtest, client, ctx, execRequest)
				if req.ExitCode != 0 {
					subtest.Fatalf("exec %v failed with exit code %d: %s %s", execRequest.Cmd, req.ExitCode, string(req.Stdout), string(req.Stderr))
				}

				if string(req.Stdout) != tt.ExpectedResult {
					subtest.Fatalf("did not properly set container state; expected %q, got: %q", tt.ExpectedResult, string(req.Stdout))
				}

				/*******************************************************************
				* restart pod
				*******************************************************************/
				stopContainer(subtest, client, ctx, containerID)
				stopPodSandbox(subtest, client, ctx, podID)

				if restart {
					// allow for any garbage collection and clean up to happen
					time.Sleep(time.Second * 1)
					stopContainerd(subtest)
					startContainerd(subtest)
				}

				newPodID := runPodSandbox(subtest, client, ctx, sandboxRequest)
				if newPodID != podID {
					defer removePodSandbox(subtest, client, ctx, newPodID)
					defer stopPodSandbox(subtest, client, ctx, newPodID)
					subtest.Fatalf("pod restarted with different id (%q) from original (%q)", newPodID, podID)
				}

				startContainer(subtest, client, ctx, containerID)

				execRequest = &runtime.ExecSyncRequest{
					ContainerId: containerID,
					Cmd:         tt.GetStateCommand,
					Timeout:     1,
				}

				req = execSync(subtest, client, ctx, execRequest)
				if req.ExitCode != 0 {
					subtest.Fatalf("exec %v failed with exit code %d: %s %s", execRequest.Cmd, req.ExitCode, string(req.Stdout), string(req.Stderr))
				}

				if string(req.Stdout) != tt.ExpectedResult {
					subtest.Fatalf("expected %q, got: %q", tt.ExpectedResult, string(req.Stdout))
				}
			})
		}
	}
}
