// +build windows

package devices

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/resources"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/pkg/errors"
)

// InstallKernelDriver mounts a specified kernel driver, then installs it in the UVM.
//
// `driver` is a directory path on the host that contains driver files for standard installation.
// For windows this means files for pnp installation (.inf, .cat, .sys, .cert files).
// For linux this means a vhd file that contains the drivers under /lib/modules/`uname -r` for use
// with depmod and modprobe.
//
// Returns a ResourceCloser for the added mount. On failure, the mounted share will be released,
// the returned ResourceCloser will be nil, and an error will be returned.
func InstallKernelDriver(ctx context.Context, vm *uvm.UtilityVM, driver string) (closer resources.ResourceCloser, err error) {
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
		closer, err = vm.AddVSMB(ctx, driver, options)
		if err != nil {
			return closer, fmt.Errorf("failed to add VSMB share to utility VM for path %+v: %s", driver, err)
		}
		uvmPath, err := vm.GetVSMBUvmPath(ctx, driver, true)
		if err != nil {
			return closer, err
		}
		return closer, execPnPInstallDriver(ctx, vm, uvmPath)
	}
	uvmPathForShare := fmt.Sprintf(uvm.LCOWGlobalMountPrefix, vm.UVMMountCounter())
	scsiCloser, err := vm.AddSCSI(ctx, driver, uvmPathForShare, true, false, []string{}, uvm.VMAccessTypeIndividual)
	if err != nil {
		return closer, fmt.Errorf("failed to add SCSI disk to utility VM for path %+v: %s", driver, err)
	}
	return scsiCloser, execModprobeInstallDriver(ctx, vm, uvmPathForShare)
}

func execModprobeInstallDriver(ctx context.Context, vm *uvm.UtilityVM, driverDir string) error {
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
		driverDir,
	}
	req := &cmd.CmdProcessRequest{
		Args:   args,
		Stderr: p,
	}

	exitCode, execErr := cmd.ExecInUvm(ctx, vm, req)

	// wait to finish parsing stdout results
	select {
	case err := <-errChan:
		if err != nil {
			return errors.Wrap(err, execErr.Error())
		}
	case <-ctx.Done():
		return errors.Wrap(ctx.Err(), execErr.Error())
	}

	if execErr != nil && execErr != noExecOutputErr {
		return errors.Wrapf(execErr, "failed to install driver %s in uvm with exit code %d: %v", driverDir, exitCode, stderrOutput)
	}

	log.G(ctx).WithField("added drivers", driverDir).Debug("installed drivers")
	return nil
}
