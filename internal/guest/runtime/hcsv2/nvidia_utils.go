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
	"github.com/sirupsen/logrus"

	"github.com/Microsoft/hcsshim/cmd/gcstools/generichook"
	"github.com/Microsoft/hcsshim/internal/guest/storage/pci"
	"github.com/Microsoft/hcsshim/internal/hooks"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/pkg/annotations"
)

const nvidiaDebugFilePath = "nvidia-container.log"
const nvidiaToolBinary = "nvidia-container-cli"

var nvidiaCapabilities = map[string]struct{}{
	"all":      {},
	"compat32": {},
	"compute":  {},
	"display":  {},
	"graphics": {},
	"ngx":      {},
	"utility":  {},
	"video":    {},
}

func nvidiaCapabilityArgs(capabilities string) ([]string, error) {
	caps := strings.Split(capabilities, ",")
	args := make([]string, 0, len(caps))
	for _, capability := range caps {
		if _, ok := nvidiaCapabilities[capability]; !ok {
			return nil, fmt.Errorf("unsupported NVIDIA GPU capability %q", capability)
		}
		args = append(args, "--"+capability)
	}
	return args, nil
}

// nvidiaConfigureArgs builds the fixed nvidia-container-cli configure arguments and
// appends the validated GPU capabilities. The fixed --ldconfig must never be
// overridable by the untrusted capabilities annotation.
func nvidiaConfigureArgs(genericHookPath, debugOption string, spec *oci.Spec) ([]string, error) {
	args := []string{
		genericHookPath,
		nvidiaToolBinary,
		debugOption,
		"--no-pivot",
		"configure",
		"--ldconfig=@/sbin/ldconfig",
	}
	if capabilities, ok := spec.Annotations[annotations.ContainerGPUCapabilities]; ok {
		capabilityArgs, err := nvidiaCapabilityArgs(capabilities)
		if err != nil {
			return nil, fmt.Errorf("invalid %s annotation: %w", annotations.ContainerGPUCapabilities, err)
		}
		args = append(args, capabilityArgs...)
	}
	return args, nil
}

// addNvidiaDeviceHook builds the arguments for nvidia-container-cli and creates the createRuntime [OCI hooks].
//
// [OCI hooks]: https://github.com/opencontainers/runtime-spec/blob/39c287c415bf86fb5b7506528d471db5405f8ca8/config.md#posix-platform-hooks
func addNvidiaDeviceHook(ctx context.Context, spec *oci.Spec, ociBundlePath string) error {
	genericHookBinary := "generichook"
	genericHookPath, err := exec.LookPath(genericHookBinary)
	if err != nil {
		return errors.Wrapf(err, "failed to find %s for container device support", genericHookBinary)
	}

	toolDebugPath := filepath.Join(ociBundlePath, nvidiaDebugFilePath)
	debugOption := fmt.Sprintf("--debug=%s", toolDebugPath)
	args, err := nvidiaConfigureArgs(genericHookPath, debugOption, spec)
	if err != nil {
		return err
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
	args = append(args, "--pid={{pid}}", spec.Root.Path)

	// setup environment variables for the hook to run in
	hookLogDebugFileEnvOpt := fmt.Sprintf("%s=%s", generichook.LogDebugFileEnvKey, toolDebugPath)
	hookEnv := append(updateEnvWithNvidiaVariables(), hookLogDebugFileEnvOpt)

	nvidiaHook := hooks.NewOCIHook(genericHookPath, args, hookEnv)
	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		log.G(ctx).WithField("hook", log.Format(ctx, nvidiaHook)).Debug("adding nvidia device runtime hook")
	}
	return hooks.AddOCIHook(spec, hooks.CreateRuntime, nvidiaHook)
}

// updateEnvWithNvidiaVariables creates an env with the nvidia specific variables set
func updateEnvWithNvidiaVariables() []string {
	// NVC_INSECURE_MODE allows us to run nvidia-container-cli without seccomp
	// we don't currently use seccomp in the uvm, so avoid using it here for now as well
	return append(os.Environ(), "NVC_INSECURE_MODE=1")
}
