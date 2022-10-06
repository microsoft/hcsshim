//go:build linux
// +build linux

package hcsv2

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"

	"github.com/Microsoft/hcsshim/cmd/gcstools/generichook"
	"github.com/Microsoft/hcsshim/internal/guest/storage/pci"
	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/Microsoft/hcsshim/internal/hooks"
	"github.com/Microsoft/hcsshim/pkg/annotations"
)

const nvidiaDebugFilePath = "/nvidia-container.log"

const nvidiaToolBinary = "nvidia-container-cli"

// described here: https://github.com/opencontainers/runtime-spec/blob/39c287c415bf86fb5b7506528d471db5405f8ca8/config.md#posix-platform-hooks
// addNvidiaDeviceHook builds the arguments for nvidia-container-cli and creates the prestart hook
func addNvidiaDeviceHook(ctx context.Context, spec *oci.Spec) error {
	genericHookBinary := "generichook"
	genericHookPath, err := exec.LookPath(genericHookBinary)
	if err != nil {
		return errors.Wrapf(err, "failed to find %s for container device support", genericHookBinary)
	}

	debugOption := fmt.Sprintf("--debug=%s", nvidiaDebugFilePath)

	// TODO katiewasnothere: right now both host and container ldconfig do not work as expected for nvidia-container-cli
	// ldconfig needs to be run in the container to setup the correct symlinks to the library files nvidia-container-cli
	// maps into the container
	args := []string{
		genericHookPath,
		nvidiaToolBinary,
		debugOption,
		"--load-kmods",
		"--no-pivot",
		"configure",
		"--ldconfig=@/sbin/ldconfig",
	}
	if capabilities, ok := spec.Annotations[annotations.ContainerGPUCapabilities]; ok {
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

	// add template for pid argument to be injected later by the generic hook binary
	args = append(args, "--no-cgroups", "--pid={{pid}}", spec.Root.Path)

	hookLogDebugFileEnvOpt := fmt.Sprintf("%s=%s", generichook.LogDebugFileEnvKey, nvidiaDebugFilePath)
	hookEnv := append(updateEnvWithNvidiaVariables(), hookLogDebugFileEnvOpt)
	nvidiaHook := hooks.NewOCIHook(genericHookPath, args, hookEnv)
	return hooks.AddOCIHook(spec, hooks.CreateRuntime, nvidiaHook)
}

// Helper function to find the usr/lib path for the installed nvidia library files.
// This function assumes that the drivers have been installed using
// gcstool's `install-drivers` binary.
func getNvidiaDriversUsrLibPath() string {
	return fmt.Sprintf("%s/content/usr/lib", guestpath.LCOWNvidiaMountPath)
}

// Helper function to find the usr/bin path for the installed nvidia tools.
// This function assumes that the drivers have been installed using
// gcstool's `install-drivers` binary.
func getNvidiaDriverUsrBinPath() string {
	return fmt.Sprintf("%s/content/usr/bin", guestpath.LCOWNvidiaMountPath)
}

// updateEnvWithNvidiaVariables creates an env with the nvidia gpu vhd in PATH and insecure mode set
func updateEnvWithNvidiaVariables() []string {
	env := updatePathEnv(getNvidiaDriverUsrBinPath())
	// NVC_INSECURE_MODE allows us to run nvidia-container-cli without seccomp
	// we don't currently use seccomp in the uvm, so avoid using it here for now as well
	env = append(env, "NVC_INSECURE_MODE=1")
	return env
}

// updatePathEnv adds specified `dirs` to PATH variable and returns the result environment variables.
func updatePathEnv(dirs ...string) []string {
	pathPrefix := "PATH="
	additionalDirs := strings.Join(dirs, ":")
	env := os.Environ()
	for i, v := range env {
		if strings.HasPrefix(v, pathPrefix) {
			newPath := fmt.Sprintf("%s:%s", v, additionalDirs)
			env[i] = newPath
			return env
		}
	}
	return append(env, fmt.Sprintf("PATH=%s", additionalDirs))
}
