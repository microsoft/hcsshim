// +build windows

package hcsoci

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/pkg/errors"
)

func createPnPInstallDriverCommand(driverUVMPath string) []string {
	dirFormatted := fmt.Sprintf("%s/*.inf", driverUVMPath)
	args := []string{
		"cmd",
		"/c",
		"C:\\Windows\\System32\\pnputil.exe",
		"/add-driver",
		dirFormatted,
		"/subdirs",
		"/install",
		"&",
	}
	return args
}

func createPnPInstallAllDriverArgs(driverUVMDirs []string) []string {
	var result []string
	for _, d := range driverUVMDirs {
		driverArgs := createPnPInstallDriverCommand(d)
		result = append(result, driverArgs...)
	}
	return result
}

// execPnPInstallAllDrivers makes the call to exec in the uvm the pnp command
// that installs all drivers previously mounted into the uvm. The pnp command
// must exist in the UVM image.
func execPnPInstallAllDrivers(ctx context.Context, vm *uvm.UtilityVM, driverDirs []string) error {
	args := createPnPInstallAllDriverArgs(driverDirs)
	req := &shimdiag.ExecProcessRequest{
		Args: args,
	}
	exitCode, err := ExecInUvm(ctx, vm, req)
	if err != nil {
		return errors.Wrapf(err, "failed to install drivers in uvm with exit code %d", exitCode)
	}
	log.G(ctx).WithField("added drivers", driverDirs).Debug("installed drivers")
	return nil
}
