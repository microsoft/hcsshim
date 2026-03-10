//go:build windows

package lcow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	runhcsoptions "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"
	"github.com/Microsoft/hcsshim/osversion"
	shimannotations "github.com/Microsoft/hcsshim/pkg/annotations"

	"github.com/sirupsen/logrus"
)

// resolveBootFilesPath resolves and validates the boot files root path.
func resolveBootFilesPath(ctx context.Context, opts *runhcsoptions.Options, annotations map[string]string) (string, error) {
	log.G(ctx).Debug("resolveBootFilesPath: starting boot files path resolution")

	// If the customer provides the boot files path then it is given preference over the default path.
	// Similarly, based on the existing behavior in old shim, the annotation provided boot files path
	// is given preference over those in runhcs options.
	bootFilesRootPath := oci.ParseAnnotationsString(annotations, shimannotations.BootFilesRootPath, opts.BootFilesRootPath)
	if bootFilesRootPath == "" {
		bootFilesRootPath = vmutils.DefaultLCOWOSBootFilesPath()
	}

	if p, err := filepath.Abs(bootFilesRootPath); err == nil {
		bootFilesRootPath = p
	} else {
		log.G(ctx).WithFields(logrus.Fields{
			logfields.Path:  bootFilesRootPath,
			logrus.ErrorKey: err,
		}).Warning("could not make boot files path absolute")
	}

	if _, err := os.Stat(bootFilesRootPath); err != nil {
		return "", fmt.Errorf("boot_files_root_path %q not found: %w", bootFilesRootPath, err)
	}

	log.G(ctx).WithField(logfields.Path, bootFilesRootPath).Debug("resolveBootFilesPath completed successfully")
	return bootFilesRootPath, nil
}

// parseBootOptions parses LCOW boot options from annotations and options.
// Returns the HCS Chipset config and the full rootfs path.
func parseBootOptions(ctx context.Context, opts *runhcsoptions.Options, annotations map[string]string) (*hcsschema.Chipset, string, error) {
	log.G(ctx).Debug("parseBootOptions: starting boot options parsing")

	// Resolve and validate boot files path.
	bootFilesPath, err := resolveBootFilesPath(ctx, opts, annotations)
	if err != nil {
		return nil, "", err
	}

	log.G(ctx).WithField(logfields.Path, bootFilesPath).Debug("using boot files path")

	chipset := &hcsschema.Chipset{}

	// Set the default rootfs to initrd.
	rootFsFile := vmutils.InitrdFile

	// Helper to check file existence in boot files path.
	fileExists := func(filename string) bool {
		_, err := os.Stat(filepath.Join(bootFilesPath, filename))
		return err == nil
	}

	// Reset the default values based on the presence of files in the boot files path.
	// We have a rootfs.vhd in the boot files path. Use it over an initrd.img
	if fileExists(vmutils.VhdFile) {
		rootFsFile = vmutils.VhdFile
		log.G(ctx).WithField(
			vmutils.VhdFile, filepath.Join(bootFilesPath, vmutils.VhdFile),
		).Debug("updated LCOW root filesystem to " + vmutils.VhdFile)
	}

	// KernelDirect supports uncompressed kernel if the kernel is present.
	// Default to uncompressed if on box. NOTE: If `kernel` is already
	// uncompressed and simply named 'kernel' it will still be used
	// uncompressed automatically.
	kernelDirectBootSupported := osversion.Build() >= 18286
	useKernelDirect := oci.ParseAnnotationsBool(ctx, annotations, shimannotations.KernelDirectBoot, kernelDirectBootSupported)

	log.G(ctx).WithFields(logrus.Fields{
		"kernelDirectSupported": kernelDirectBootSupported,
		"useKernelDirect":       useKernelDirect,
	}).Debug("determined boot mode")

	// If customer specifies kernel direct boot but the build does not support it, return an error.
	if useKernelDirect && !kernelDirectBootSupported {
		return nil, "", fmt.Errorf("KernelDirectBoot is not supported on builds older than 18286")
	}

	// Determine kernel file based on boot mode
	var kernelFileName string
	if useKernelDirect {
		// KernelDirect supports uncompressed kernel if present.
		if fileExists(vmutils.UncompressedKernelFile) {
			kernelFileName = vmutils.UncompressedKernelFile
			log.G(ctx).WithField(vmutils.UncompressedKernelFile, filepath.Join(bootFilesPath, vmutils.UncompressedKernelFile)).Debug("updated LCOW kernel file to " + vmutils.UncompressedKernelFile)
		} else if fileExists(vmutils.KernelFile) {
			kernelFileName = vmutils.KernelFile
		} else {
			return nil, "", fmt.Errorf("kernel file not found in boot files path for kernel direct boot")
		}
	} else {
		kernelFileName = vmutils.KernelFile
		if !fileExists(vmutils.KernelFile) {
			return nil, "", fmt.Errorf("kernel file %q not found in boot files path: %w", vmutils.KernelFile, os.ErrNotExist)
		}
	}

	log.G(ctx).WithField("kernelFile", kernelFileName).Debug("selected kernel file")

	// Parse preferred rootfs type annotation. This overrides the default set above based on file presence.
	if preferredRootfsType := oci.ParseAnnotationsString(annotations, shimannotations.PreferredRootFSType, ""); preferredRootfsType != "" {
		log.G(ctx).WithField("preferredRootFSType", preferredRootfsType).Debug("applying preferred rootfs type override")
		switch preferredRootfsType {
		case "initrd":
			rootFsFile = vmutils.InitrdFile
		case "vhd":
			rootFsFile = vmutils.VhdFile
		default:
			return nil, "", fmt.Errorf("invalid PreferredRootFSType: %s", preferredRootfsType)
		}
		if !fileExists(rootFsFile) {
			return nil, "", fmt.Errorf("%q not found in boot files path", rootFsFile)
		}
	}

	log.G(ctx).WithField("rootFsFile", rootFsFile).Debug("selected rootfs file")

	// Set up boot configuration based on boot mode
	if useKernelDirect {
		log.G(ctx).Debug("configuring kernel direct boot")
		chipset.LinuxKernelDirect = &hcsschema.LinuxKernelDirect{
			KernelFilePath: filepath.Join(bootFilesPath, kernelFileName),
			// KernelCmdLine will be populated later by buildKernelArgs
		}
		if rootFsFile == vmutils.InitrdFile {
			chipset.LinuxKernelDirect.InitRdPath = filepath.Join(bootFilesPath, rootFsFile)
			log.G(ctx).WithField("initrdPath", chipset.LinuxKernelDirect.InitRdPath).Debug("configured initrd for kernel direct boot")
		}
	} else {
		// UEFI boot
		log.G(ctx).Debug("configuring UEFI boot")
		chipset.Uefi = &hcsschema.Uefi{
			BootThis: &hcsschema.UefiBootEntry{
				DevicePath:    `\` + kernelFileName,
				DeviceType:    "VmbFs",
				VmbFsRootPath: bootFilesPath,
			},
		}
	}

	rootFsFullPath := filepath.Join(bootFilesPath, rootFsFile)
	log.G(ctx).WithFields(logrus.Fields{
		"rootFsFullPath":  rootFsFullPath,
		"kernelFilePath":  filepath.Join(bootFilesPath, kernelFileName),
		"useKernelDirect": useKernelDirect,
	}).Debug("parseBootOptions completed successfully")

	return chipset, rootFsFullPath, nil
}
