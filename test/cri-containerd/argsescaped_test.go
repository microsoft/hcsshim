//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"strings"
	"testing"

	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func Test_ArgsEscaped_Exec(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)
	pullRequiredImages(t, []string{imageWindowsNanoserver17763, imageWindowsArgsEscaped})

	// ArgsEscaped refers to a non-standard OCI image spec field
	// that indicates that the command line for Windows Containers
	// should be used from args[0] without escaping. This behavior
	// comes into play with images that use a shell-form ENTRYPOINT
	// or CMD in their Dockerfile. The behavior that this is testing
	// is that execs work properly with these images. Hcsshim prefers
	// the commandline field on the OCI runtime spec and will ignore
	// Args if this is filled in, which ArgsEscaped does. In Containerd/cri
	// plugin the containers runtime spec is used as a base for the execs
	// spec as well, so if commandline isn't cleared out then we'll end up
	// launching the init process again instead of what the user requested.
	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sbRequest := getRunPodSandboxRequest(
		t,
		wcowHypervisor17763RuntimeHandler,
	)
	podID := runPodSandbox(t, client, ctx, sbRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	cRequest := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsArgsEscaped,
			},
		},
		PodSandboxId:  podID,
		SandboxConfig: sbRequest.Config,
	}

	containerID := createContainer(t, client, ctx, cRequest)
	defer removeContainer(t, client, ctx, containerID)

	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	// Run a simple exec that is different that what the entrypoint+cmd in the image is which is
	// "cmd /c ping -t 127.0.0.1"
	echoText := "hello world"
	execCommand := []string{
		"cmd",
		"/c",
		"echo",
		echoText,
	}
	execRequest := &runtime.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         execCommand,
		Timeout:     10,
	}

	r := execSync(t, client, ctx, execRequest)
	if r.ExitCode != 0 {
		t.Fatalf("failed with exit code %d: %s", r.ExitCode, string(r.Stderr))
	}

	if !strings.Contains(string(r.Stdout), echoText) {
		t.Fatalf("expected stdout to contain %s", echoText)
	}
}
