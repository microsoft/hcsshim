//go:build windows
// +build windows

package devices

import (
	"context"
	"fmt"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/resources"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/internal/uvm/scsi"
)

// InstallDriver mounts a share from the host into the UVM, installs any kernel drivers in the share,
// and configures the environment for library files and/or binaries in the share.
//
// InstallDriver mounts a specified kernel driver, then installs it in the UVM.
//
// `share` is a directory path on the host that contains files for standard driver installation.
// For windows this means files for pnp installation (.inf, .cat, .sys, .cert files).
// For linux this means a vhd file that contains the drivers under /lib/modules/`uname -r` for use
// with depmod and modprobe. For GPU, this vhd may also contain library files (under /usr/lib) and
// binaries (under /usr/bin or /usr/sbin) to be used in conjunction with the modules.
//
// Returns a ResourceCloser for the added mount. On failure, the mounted share will be released,
// the returned ResourceCloser will be nil, and an error will be returned.
func InstallDrivers(ctx context.Context, vm *uvm.UtilityVM, share string, gpuDriver bool) (closer resources.ResourceCloser, err error) {
	defer func() {
		if err != nil && closer != nil {
			// best effort clean up allocated resource on failure
			if releaseErr := closer.Release(ctx); releaseErr != nil {
				log.G(ctx).WithError(releaseErr).Error("failed to release container resource")
			}
			closer = nil
		}
	}()
	if vm.OS() == "windows" {
		options := vm.DefaultVSMBOptions(true)
		closer, err = vm.AddVSMB(ctx, share, options)
		if err != nil {
			return closer, fmt.Errorf("failed to add VSMB share to utility VM for path %+v: %s", share, err)
		}
		uvmPath, err := vm.GetVSMBUvmPath(ctx, share, true)
		if err != nil {
			return closer, err
		}
		// attempt to install even if the driver has already been installed before so we
		// can guarantee the device is ready for use afterwards
		return closer, execPnPInstallDriver(ctx, vm, uvmPath)
	}

	// first mount driver as scsi in standard mount location
	mount, err := vm.SCSIManager.AddVirtualDisk(
		ctx,
		share,
		true,
		vm.ID(),
		&scsi.MountConfig{},
	)
	if err != nil {
		return closer, fmt.Errorf("failed to add SCSI disk to utility VM for path %+v: %s", share, err)
	}
	closer = mount
	uvmPathForShare := mount.GuestPath()

	// construct path that the drivers will be remounted as read/write in the UVM

	// 914aadc8-f700-4365-8016-ddad0a9d406d. Random GUID chosen for namespace.
	ns := guid.GUID{Data1: 0x914aadc8, Data2: 0xf700, Data3: 0x4365, Data4: [8]byte{0x80, 0x16, 0xdd, 0xad, 0x0a, 0x9d, 0x40, 0x6d}}
	driverGUID, err := guid.NewV5(ns, []byte(share))
	if err != nil {
		return closer, fmt.Errorf("failed to create a guid path for driver %+v: %s", share, err)
	}
	uvmReadWritePath := fmt.Sprintf(guestpath.LCOWGlobalDriverPrefixFmt, driverGUID.String())
	if gpuDriver {
		// if installing gpu drivers in lcow, use the nvidia mount path instead
		uvmReadWritePath = guestpath.LCOWNvidiaMountPath
	}

	// install drivers using gcs tool `install-drivers`
	return closer, execGCSInstallDriver(ctx, vm, uvmPathForShare, uvmReadWritePath)
}

func execGCSInstallDriver(ctx context.Context, vm *uvm.UtilityVM, driverDir string, driverReadWriteDir string) error {
	p, l, err := cmd.CreateNamedPipeListener()
	if err != nil {
		return err
	}
	defer l.Close()

	var stderrOutput string
	errChan := make(chan error)

	go readAllPipeOutput(l, errChan, &stderrOutput)

	args := []string{
		"/bin/install-drivers",
		driverReadWriteDir,
		driverDir,
	}
	req := &cmd.CmdProcessRequest{
		Args:   args,
		Stderr: p,
	}

	// A call to `ExecInUvm` may fail in the following ways:
	// - The process runs and exits with a non-zero exit code. In this case we need to wait on the output
	//   from stderr so we can log it for debugging.
	// - There's an error trying to run the process. No need to wait for stderr logs.
	// - There's an error copying IO. No need to wait for stderr logs.
	//
	// Since we cannot distinguish between the cases above, we should always wait to read the stderr output.
	exitCode, execErr := cmd.ExecInUvm(ctx, vm, req)

	// wait to finish parsing stdout results
	select {
	case err := <-errChan:
		if err != nil && err != noExecOutputErr {
			return fmt.Errorf("failed to get stderr output from command %s: %v", driverDir, err)
		}
	case <-ctx.Done():
		return fmt.Errorf("timed out waiting for the console output from installing driver %s: %v", driverDir, ctx.Err())
	}

	if execErr != nil {
		return fmt.Errorf("%v: failed to install driver %s in uvm with exit code %d: %v", execErr, driverDir, exitCode, stderrOutput)
	}
	return nil
}
