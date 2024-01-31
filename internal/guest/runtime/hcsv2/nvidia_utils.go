//go:build linux
// +build linux

package hcsv2

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"

	"github.com/Microsoft/hcsshim/cmd/gcstools/generichook"
	"github.com/Microsoft/hcsshim/internal/guest/storage/pci"
	"github.com/Microsoft/hcsshim/internal/hooks"
	"github.com/Microsoft/hcsshim/pkg/annotations"
)

const nvidiaDebugFilePath = "nvidia-container.log"
const nvidiaToolBinary = "nvidia-container-cli"

// described here: https://github.com/opencontainers/runtime-spec/blob/39c287c415bf86fb5b7506528d471db5405f8ca8/config.md#posix-platform-hooks
// addNvidiaDeviceHook builds the arguments for nvidia-container-cli and creates the prestart hook
func addNvidiaDeviceHook(ctx context.Context, spec *oci.Spec, ociBundlePath string) error {
	genericHookBinary := "generichook"
	genericHookPath, err := exec.LookPath(genericHookBinary)
	if err != nil {
		return errors.Wrapf(err, "failed to find %s for container device support", genericHookBinary)
	}

	toolDebugPath := filepath.Join(ociBundlePath, nvidiaDebugFilePath)
	debugOption := fmt.Sprintf("--debug=%s", toolDebugPath)
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

	// setup environment variables for the hook to run in
	hookLogDebugFileEnvOpt := fmt.Sprintf("%s=%s", generichook.LogDebugFileEnvKey, toolDebugPath)
	hookEnv := append(updateEnvWithNvidiaVariables(), hookLogDebugFileEnvOpt)

	nvidiaHook := hooks.NewOCIHook(genericHookPath, args, hookEnv)
	return hooks.AddOCIHook(spec, hooks.CreateRuntime, nvidiaHook)
}

// updateEnvWithNvidiaVariables creates an env with the nvidia specific variables set
func updateEnvWithNvidiaVariables() []string {
	// NVC_INSECURE_MODE allows us to run nvidia-container-cli without seccomp
	// we don't currently use seccomp in the uvm, so avoid using it here for now as well
	return append(os.Environ(), "NVC_INSECURE_MODE=1")
}
