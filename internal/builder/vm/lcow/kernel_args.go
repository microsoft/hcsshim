//go:build windows

package lcow

import (
	"context"
	"fmt"
	"strings"

	runhcsoptions "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	iannotations "github.com/Microsoft/hcsshim/internal/annotations"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"
	shimannotations "github.com/Microsoft/hcsshim/pkg/annotations"

	"github.com/sirupsen/logrus"
)

// buildKernelArgs constructs the kernel command line from the parsed config structs.
func buildKernelArgs(
	ctx context.Context,
	opts *runhcsoptions.Options,
	annotations map[string]string,
	processorCount uint32,
	kernelDirect bool,
	isVPMem bool,
	hasConsole bool,
	rootFsFile string,
) (string, error) {

	log.G(ctx).WithField("rootFsFile", rootFsFile).Debug("buildKernelArgs: starting kernel arguments construction")

	// Parse intermediate values from annotations that are only used for kernel args
	vpciEnabled := oci.ParseAnnotationsBool(ctx, annotations, shimannotations.VPCIEnabled, false)
	disableTimeSyncService := oci.ParseAnnotationsBool(ctx, annotations, shimannotations.DisableLCOWTimeSyncService, false)
	writableOverlayDirs := oci.ParseAnnotationsBool(ctx, annotations, iannotations.WritableOverlayDirs, false)
	processDumpLocation := oci.ParseAnnotationsString(annotations, shimannotations.ContainerProcessDumpLocation, "")

	// Build kernel arguments in logical sections for better readability.
	var args []string

	// 1. Root filesystem configuration.
	rootfsArgs, err := buildRootfsArgs(ctx, annotations, rootFsFile, kernelDirect, isVPMem)
	if err != nil {
		return "", err
	}
	if rootfsArgs != "" {
		args = append(args, rootfsArgs)
	}

	// 2. Vsock transport configuration
	// Explicitly disable virtio_vsock_init to ensure we use hv_sock transport.
	// For kernels built without virtio-vsock this is a no-op.
	args = append(args, "initcall_blacklist=virtio_vsock_init")

	// 3. Console and debugging configuration
	args = append(args, buildConsoleArgs(hasConsole)...)

	if !hasConsole {
		// Terminate the VM if there is a kernel panic.
		args = append(args, "panic=-1", "quiet")
	}

	// 4. User-provided kernel boot options from annotations
	kernelBootOptions := oci.ParseAnnotationsString(annotations, shimannotations.KernelBootOptions, "")
	if kernelBootOptions != "" {
		args = append(args, kernelBootOptions)
	}

	// 5. PCI configuration
	if !vpciEnabled {
		args = append(args, "pci=off")
	}

	// 6. CPU configuration
	args = append(args, fmt.Sprintf("nr_cpus=%d", processorCount))

	// 7. Miscellaneous kernel parameters
	// brd.rd_nr=0 disables ramdisk, pmtmr=0 disables ACPI PM timer
	args = append(args, "brd.rd_nr=0", "pmtmr=0")

	// 8. Init arguments (passed after "--" separator)
	initArgs := buildInitArgs(ctx, opts, writableOverlayDirs, disableTimeSyncService, processDumpLocation, rootFsFile, hasConsole)
	args = append(args, "--", initArgs)

	result := strings.Join(args, " ")
	log.G(ctx).WithField("kernelArgs", result).Debug("buildKernelArgs completed successfully")
	return result, nil
}

// buildRootfsArgs constructs kernel arguments for root filesystem configuration.
func buildRootfsArgs(
	ctx context.Context,
	annotations map[string]string,
	rootFsFile string,
	kernelDirect bool,
	isVPMem bool,
) (string, error) {
	log.G(ctx).WithFields(logrus.Fields{
		"rootFsFile":   rootFsFile,
		"kernelDirect": kernelDirect,
		"isVPMem":      isVPMem,
	}).Debug("buildRootfsArgs: starting rootfs args construction")

	isInitrd := rootFsFile == vmutils.InitrdFile
	isVHD := rootFsFile == vmutils.VhdFile

	// InitRd boot (applicable only for UEFI mode - kernel direct handles initrd via InitRdPath)
	if isInitrd && !kernelDirect {
		return "initrd=/" + rootFsFile, nil
	}

	// VHD boot
	if isVHD {
		// VPMem VHD(X) booting.
		if isVPMem {
			return "root=/dev/pmem0 ro rootwait init=/init", nil
		}

		// SCSI VHD booting with dm-verity.
		dmVerityMode := oci.ParseAnnotationsBool(ctx, annotations, shimannotations.DmVerityMode, false)
		if dmVerityMode {
			dmVerityCreateArgs := oci.ParseAnnotationsString(annotations, shimannotations.DmVerityCreateArgs, "")
			if len(dmVerityCreateArgs) == 0 {
				return "", fmt.Errorf("DmVerityCreateArgs should be set when DmVerityMode is true and not booting from a vmgs file")
			}
			return fmt.Sprintf("root=/dev/dm-0 dm-mod.create=%q init=/init", dmVerityCreateArgs), nil
		}

		return "root=/dev/sda ro rootwait init=/init", nil
	}

	return "", nil
}

// buildConsoleArgs constructs kernel arguments for console configuration.
func buildConsoleArgs(hasConsole bool) []string {
	var args []string

	// Serial console configuration
	if hasConsole {
		args = append(args, "8250_core.nr_uarts=1", "8250_core.skip_txen_test=1", "console=ttyS0,115200")
	} else {
		args = append(args, "8250_core.nr_uarts=0")
	}

	return args
}

// buildInitArgs constructs the init arguments (passed after "--" in kernel command line).
func buildInitArgs(
	ctx context.Context,
	opts *runhcsoptions.Options,
	writableOverlayDirs bool,
	disableTimeSyncService bool,
	processDumpLocation string,
	rootFsFile string,
	hasConsole bool,
) string {
	log.G(ctx).WithFields(logrus.Fields{
		"rootFsFile": rootFsFile,
		"hasConsole": hasConsole,
	}).Debug("buildInitArgs: starting init args construction")
	// Inject initial entropy over vsock during init launch
	entropyArgs := fmt.Sprintf("-e %d", vmutils.LinuxEntropyVsockPort)

	// Build GCS execution command
	gcsCmd := buildGCSCommand(opts, disableTimeSyncService, processDumpLocation)

	// Construct init arguments
	var initArgsList []string
	initArgsList = append(initArgsList, entropyArgs)

	// Handle writable overlay directories for VHD
	if writableOverlayDirs {
		switch rootFsFile {
		case vmutils.InitrdFile:
			log.G(ctx).Warn("ignoring `WritableOverlayDirs` option since rootfs is already writable")
		case vmutils.VhdFile:
			initArgsList = append(initArgsList, "-w")
		}
	}

	// Add GCS command execution
	if hasConsole {
		// Launch a shell on the console for debugging
		initArgsList = append(initArgsList, `sh -c"`+gcsCmd+`& exec sh"`)
	} else {
		initArgsList = append(initArgsList, gcsCmd)
	}

	result := strings.Join(initArgsList, " ")
	log.G(ctx).Debug("buildInitArgs completed successfully")
	return result
}

// buildGCSCommand constructs the GCS (Guest Compute Service) command line.
func buildGCSCommand(
	opts *runhcsoptions.Options,
	disableTimeSyncService bool,
	processDumpLocation string,
) string {
	// Start with vsockexec wrapper
	var cmdParts []string
	cmdParts = append(cmdParts, "/bin/vsockexec")

	// Add logging vsock port
	cmdParts = append(cmdParts, fmt.Sprintf("-e %d", vmutils.LinuxLogVsockPort))

	// Determine log level
	logLevel := "info"
	if opts != nil && opts.LogLevel != "" {
		logLevel = opts.LogLevel
	}

	// Build GCS base command
	gcsParts := []string{
		"/bin/gcs",
		"-v4",
		"-log-format json",
		"-loglevel " + logLevel,
	}

	// Add optional GCS flags
	if disableTimeSyncService {
		gcsParts = append(gcsParts, "-disable-time-sync")
	}

	if opts != nil && opts.ScrubLogs {
		gcsParts = append(gcsParts, "-scrub-logs")
	}

	if processDumpLocation != "" {
		gcsParts = append(gcsParts, "-core-dump-location", processDumpLocation)
	}

	// Combine vsockexec and GCS command
	cmdParts = append(cmdParts, strings.Join(gcsParts, " "))

	return strings.Join(cmdParts, " ")
}
