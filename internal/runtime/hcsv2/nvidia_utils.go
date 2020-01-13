// +build linux

package hcsv2

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Microsoft/opengcs/internal/storage/pci"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

// path that the shim mounts the nvidia gpu vhd to in the uvm
// this MUST match the path mapped to in the shim
const lcowNvidiaMountPath = "/run/nvidia"

// annotation to find the gpu capabilities on the container spec
// must match the hcsshim annotation string for gpu capabilities
const annotationContainerGPUCapabilities = "io.microsoft.container.gpu.capabilities"
const nvidiaDebugFilePath = "/nvidia-container.log"

// TODO katiewasnothere: prestart hooks will be depracated, this needs to be moved to a createRuntime hook
// described here: https://github.com/opencontainers/runtime-spec/blob/39c287c415bf86fb5b7506528d471db5405f8ca8/config.md#posix-platform-hooks
// addNvidiaDevicePreHook builds the arguments for nvidia-container-cli and creates the prestart hook
func addNvidiaDevicePreHook(ctx context.Context, spec *oci.Spec) error {
	nvidiaToolBinary := "nvidiaPrestartHook"
	nvidiaToolPath, err := exec.LookPath(nvidiaToolBinary)
	if err != nil {
		return errors.Wrapf(err, "failed to find %s for container GPU support", nvidiaToolBinary)
	}

	debugOption := fmt.Sprintf("--debug=%s", nvidiaDebugFilePath)

	// TODO katiewasnothere: right now both host and container ldconfig do not work as expected for nvidia-container-cli
	// ldconfig needs to be run in the container to setup the correct symlinks to the library files nvidia-container-cli
	// maps into the container
	args := []string{nvidiaToolPath, debugOption, "--load-kmods", "--no-pivot", "configure", "--ldconfig=@/sbin/ldconfig"}
	if capabilities, ok := spec.Annotations[annotationContainerGPUCapabilities]; ok {
		caps := strings.Split(capabilities, ",")
		for _, c := range caps {
			args = append(args, fmt.Sprintf("--%s", c))
		}
	}

	for _, d := range spec.Windows.Devices {
		switch d.IDType {
		case "gpu":
			busLocation, err := pci.FindDeviceBusLocationFromVMBusGUID(ctx, d.ID)
			if err != nil {
				return errors.Wrapf(err, "failed to find nvidia gpu bus location")
			}
			args = append(args, fmt.Sprintf("--device=%s", busLocation))
		}
	}

	args = append(args, "--no-cgroups", "--pid=%v", spec.Root.Path)

	if spec.Hooks == nil {
		spec.Hooks = &oci.Hooks{}
	}

	nvidiaHook := oci.Hook{
		Path: nvidiaToolPath,
		Args: args,
		Env:  updateEnvWithNvidiaVariables(),
	}

	spec.Hooks.Prestart = append(spec.Hooks.Prestart, nvidiaHook)
	return nil
}

// updateEnvWithNvidiaVariables creates an env with the nvidia gpu vhd in PATH and insecure mode set
func updateEnvWithNvidiaVariables() []string {
	pathPrefix := "PATH="
	nvidiaBin := fmt.Sprintf("%s/bin", lcowNvidiaMountPath)
	env := os.Environ()
	for i, v := range env {
		if strings.HasPrefix(v, pathPrefix) {
			newPath := fmt.Sprintf("%s:%s", v, nvidiaBin)
			env[i] = newPath
		}
	}
	// NVC_INSECURE_MODE allows us to run nvidia-container-cli without seccomp
	// we don't currently use seccomp in the uvm, so avoid using it here for now as well
	env = append(env, "NVC_INSECURE_MODE=1")
	return env
}
