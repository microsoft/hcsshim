// +build windows

package devices

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/resources"
	"github.com/Microsoft/hcsshim/internal/uvm"
)

// InstallWindowsDriver mounts a specified kernel driver using vsmb, then installs it in the UVM.
//
// `driver` is a directory path on the host that contains driver files for standard installation.
//
// Returns a ResourceCloser for the added vsmb share. On failure, the vsmb share will be released,
// the returned ResourceCloser will be nil, and an error will be returned.
func InstallWindowsDriver(ctx context.Context, vm *uvm.UtilityVM, driver string) (closer resources.ResourceCloser, err error) {
	defer func() {
		if err != nil && closer != nil {
			// best effort clean up allocated resource on failure
			if releaseErr := closer.Release(ctx); releaseErr != nil {
				log.G(ctx).WithError(releaseErr).Error("failed to release container resource")
			}
			closer = nil
		}
	}()
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
