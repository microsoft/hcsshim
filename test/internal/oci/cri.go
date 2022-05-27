package oci

import (
	imagespec "github.com/opencontainers/image-spec/specs-go/v1"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

//
// not technically OCI, but making a CRI package seems overkill
//

func LinuxWorkloadRuntimeConfig(name string, cmd, args []string, wd string) *runtime.ContainerConfig {
	return &runtime.ContainerConfig{
		Metadata: &runtime.ContainerMetadata{
			Name: name,
		},
		Command:    cmd,
		Args:       args,
		WorkingDir: wd,
	}
}

func LinuxWorkloadImageConfig() *imagespec.ImageConfig {
	return LinuxImageConfig([]string{""}, []string{"/bin/sh"}, "/")
}

func LinuxSandboxRuntimeConfig(name string) *runtime.PodSandboxConfig {
	return &runtime.PodSandboxConfig{
		Metadata: &runtime.PodSandboxMetadata{
			Name:      name,
			Namespace: "default",
		},
		Hostname: "",
		Windows:  &runtime.WindowsPodSandboxConfig{},
	}
}

// based off of:
// containerd\pkg\cri\server\sandbox_run_windows.go
// containerd\pkg\cri\server\container_create.go
// containerd\pkg\cri\server\container_create_windows.go

func LinuxSandboxImageConfig(pause bool) *imagespec.ImageConfig {
	entry := []string{"/bin/sh", "-c", TailNullArgs}
	if pause {
		entry = []string{"/pause"}
	}

	return LinuxImageConfig(entry, []string{}, "/")
}
