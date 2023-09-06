//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"strings"
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

	t.Log("Restarting containerd")
	stopContainerd(t)
	startContainerd(t)
	client = newTestRuntimeClient(t)
	waitForCRI(ctx, t, client, 15*time.Second)

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
	requireFeatures(t, featureCRIPlugin)
	requireAnyFeature(t, featureWCOWProcess, featureWCOWHypervisor, featureLCOW)

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
			Runtime: wcowProcessRuntimeHandler,
			Image:   imageWindowsNanoserver,
			Command: []string{
				"cmd",
				"/c",
				"ping -t 127.0.0.1",
			},
		},
	}

	for _, tt := range tests {
		// Test both implicit and explicit restart
		// Implicit restart uses a container annotation to cause pod run and container start
		// to automatically restart exited pods and containers.
		// Explicit requires an intervening call to the restart pod or container CRI extension
		// command between stoping the pod or container, and then calling run or start again.
		for _, explicit := range []bool{false, true} {
			suffix := "_Implicit"
			if explicit {
				suffix = "_Explicit"
			}

			t.Run(tt.Name+suffix, func(t *testing.T) {
				requireFeatures(t, tt.Feature)

				switch tt.Feature {
				case featureLCOW:
					pullRequiredLCOWImages(t, append([]string{imageLcowK8sPause}, tt.Image))
				case featureWCOWHypervisor, featureWCOWProcess:
					pullRequiredImages(t, []string{tt.Image})
				}

				opts := tt.SandboxOpts
				if !explicit {
					opts = append(tt.SandboxOpts,
						WithSandboxAnnotations(map[string]string{
							criAnnotations.EnableReset: "true",
						}))
				}
				sandboxRequest := getRunPodSandboxRequest(t, tt.Runtime, opts...)
				podID := runPodSandbox(t, client, ctx, sandboxRequest)
				defer removePodSandbox(t, client, ctx, podID)
				defer stopPodSandbox(t, client, ctx, podID)

				request := getCreateContainerRequest(podID, t.Name()+"-Container", tt.Image, tt.Command, sandboxRequest.Config)
				request.Config.Annotations = map[string]string{}

				if !explicit {
					request.Config.Annotations[criAnnotations.EnableReset] = "true"
				}

				containerID := createContainer(t, client, ctx, request)
				startContainer(t, client, ctx, containerID)
				defer removeContainer(t, client, ctx, containerID)
				defer stopContainer(t, client, ctx, containerID)

				/*******************************************************************
				* restart container
				*******************************************************************/
				stopContainer(t, client, ctx, containerID)
				assertContainerState(t, client, ctx, containerID, runtime.ContainerState_CONTAINER_EXITED)

				if explicit {
					resetContainer(t, pluginClient, ctx, containerID)
					assertContainerState(t, client, ctx, containerID, runtime.ContainerState_CONTAINER_CREATED)
				}

				startContainer(t, client, ctx, containerID)
				assertContainerState(t, client, ctx, containerID, runtime.ContainerState_CONTAINER_RUNNING)

				/*******************************************************************
				* restart pod
				*******************************************************************/
				stopContainer(t, client, ctx, containerID)
				assertContainerState(t, client, ctx, containerID, runtime.ContainerState_CONTAINER_EXITED)

				stopPodSandbox(t, client, ctx, podID)
				assertPodSandboxState(t, client, ctx, podID, runtime.PodSandboxState_SANDBOX_NOTREADY)

				if explicit {
					resetPodSandbox(t, pluginClient, ctx, podID)
				} else {
					newPodID := runPodSandbox(t, client, ctx, sandboxRequest)
					if newPodID != podID {
						defer removePodSandbox(t, client, ctx, newPodID)
						defer stopPodSandbox(t, client, ctx, newPodID)
						t.Fatalf("pod restarted with different id (%q) from original (%q)", newPodID, podID)
					}
				}

				assertPodSandboxState(t, client, ctx, podID, runtime.PodSandboxState_SANDBOX_READY)
				assertContainerState(t, client, ctx, containerID, runtime.ContainerState_CONTAINER_CREATED)

				startContainer(t, client, ctx, containerID)
				assertContainerState(t, client, ctx, containerID, runtime.ContainerState_CONTAINER_RUNNING)
			})
		}
	}
}

// test preserving state after restarting pod
func Test_Container_CRI_Restart_State(t *testing.T) {
	testFile := "t.txt"
	wcowTestFile := `C:\Users\ContainerUser\t.txt`

	requireFeatures(t, featureCRIPlugin)
	requireAnyFeature(t, featureWCOWProcess, featureWCOWHypervisor, featureLCOW)

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
			SetStateCommand: []string{"cmd", "/c", "echo - >> " + wcowTestFile},
			GetStateCommand: []string{"cmd", "/c", "type", wcowTestFile},
			ExpectedResult:  "- \r\n",
		},
		{
			Name:            "WCOW_Process",
			Feature:         featureWCOWProcess,
			Runtime:         wcowProcessRuntimeHandler,
			Image:           imageWindowsNanoserver,
			Command:         []string{"cmd", "/c", "ping -t 127.0.0.1"},
			SetStateCommand: []string{"cmd", "/c", "echo - >> " + wcowTestFile},
			GetStateCommand: []string{"cmd", "/c", "type", wcowTestFile},
			ExpectedResult:  "- \r\n",
		},
	}

	for _, tt := range tests {
		for _, restart := range []bool{false, true} {
			suffix := "_Restart"
			if !restart {
				suffix = "_No" + suffix
			}

			t.Run(tt.Name+suffix, func(t *testing.T) {
				requireFeatures(t, tt.Feature)
				if restart {
					requireFeatures(t, featureTerminateOnRestart)
				}

				switch tt.Feature {
				case featureLCOW:
					pullRequiredLCOWImages(t, append([]string{imageLcowK8sPause}, tt.Image))
				case featureWCOWHypervisor, featureWCOWProcess:
					pullRequiredImages(t, []string{tt.Image})
				}

				sandboxRequest := getRunPodSandboxRequest(t, tt.Runtime,
					append(tt.SandboxOpts,
						WithSandboxAnnotations(map[string]string{
							criAnnotations.EnableReset: "true",
						}))...)

				podID := runPodSandbox(t, client, ctx, sandboxRequest)
				defer removePodSandbox(t, client, ctx, podID)
				defer stopPodSandbox(t, client, ctx, podID)

				request := getCreateContainerRequest(podID, t.Name()+"-Container", tt.Image, tt.Command, sandboxRequest.Config)
				request.Config.Annotations = map[string]string{
					criAnnotations.EnableReset: "true",
				}

				containerID := createContainer(t, client, ctx, request)
				startContainer(t, client, ctx, containerID)
				defer removeContainer(t, client, ctx, containerID)
				defer stopContainer(t, client, ctx, containerID)

				startExecRequest := &runtime.ExecSyncRequest{
					ContainerId: containerID,
					Cmd:         tt.SetStateCommand,
					Timeout:     1,
				}
				req := execSync(t, client, ctx, startExecRequest)
				if req.ExitCode != 0 {
					t.Fatalf("exec %v failed with exit code %d: %s", startExecRequest.Cmd, req.ExitCode, string(req.Stderr))
				}
				t.Logf("exec: %s", tt.SetStateCommand)

				// check the write worked
				startExecRequest = &runtime.ExecSyncRequest{
					ContainerId: containerID,
					Cmd:         tt.GetStateCommand,
					Timeout:     1,
				}

				req = execSync(t, client, ctx, startExecRequest)
				if req.ExitCode != 0 {
					t.Fatalf("exec %v failed with exit code %d: %s %s", startExecRequest.Cmd, req.ExitCode, string(req.Stdout), string(req.Stderr))
				}

				if string(req.Stdout) != tt.ExpectedResult {
					t.Fatalf("did not properly set container state; expected %q, got: %q", tt.ExpectedResult, string(req.Stdout))
				}

				/*******************************************************************
				* restart pod
				*******************************************************************/
				stopContainer(t, client, ctx, containerID)
				stopPodSandbox(t, client, ctx, podID)

				if restart {
					// allow for any garbage collection and clean up to happen
					time.Sleep(time.Second * 1)
					stopContainerd(t)
					startContainerd(t)
					client = newTestRuntimeClient(t)
					waitForCRI(ctx, t, client, 15*time.Second)
				}

				newPodID := runPodSandbox(t, client, ctx, sandboxRequest)
				if newPodID != podID {
					defer removePodSandbox(t, client, ctx, newPodID)
					defer stopPodSandbox(t, client, ctx, newPodID)
					t.Fatalf("pod restarted with different id (%q) from original (%q)", newPodID, podID)
				}

				startContainer(t, client, ctx, containerID)
				req = execSync(t, client, ctx, startExecRequest)
				if req.ExitCode != 0 {
					t.Fatalf("exec %v failed with exit code %d: %s %s", startExecRequest.Cmd, req.ExitCode, string(req.Stdout), string(req.Stderr))
				}

				if string(req.Stdout) != tt.ExpectedResult {
					t.Fatalf("expected %q, got: %q", tt.ExpectedResult, string(req.Stdout))
				}
			})
		}
	}
}

func waitForCRI(ctx context.Context, tb testing.TB, client runtime.RuntimeServiceClient, timeout time.Duration) {
	tb.Helper()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ch := make(chan error)
	defer close(ch)

	go func() {
		sleep := timeout / 10
		for {
			select {
			case <-ctx.Done():
				// context timed out or was cancelled
				return
			default:
			}

			_, err := client.Version(ctx, &runtime.VersionRequest{})
			if err == nil || !strings.Contains(err.Error(), "server is not initialized yet") {
				ch <- err
				return
			}
			tb.Logf("CRI is not yet initialized, sleeping for %s", sleep.String())
			time.Sleep(sleep)
		}
	}()

	select {
	case err := <-ctx.Done():
		tb.Fatalf("could not wait for CRI plugin to initialize: %v", err)
	case err := <-ch:
		if err != nil {
			tb.Fatalf("error while checking CRI plugin to initialization status: %v", err)
		}
	}
}
