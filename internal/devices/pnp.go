//go:build windows
// +build windows

package devices

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/internal/winapi"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	uvmPnpExePath                  = "C:\\Windows\\System32\\pnputil.exe"
	pnputilNoMoreItemsErrorMessage = `driver not ranked higher than existing driver in UVM.
										if drivers were not previously present in the UVM, this
										is an expected race and can be ignored.`
)

var noExecOutputErr = errors.New("failed to get any pipe output")

// createPnPInstallDriverCommand creates a pnputil command to add and install drivers
// present in `driverUVMPath` and all subdirectories.
func createPnPInstallDriverCommand(driverUVMPath string) []string {
	dirFormatted := fmt.Sprintf("%s/*.inf", driverUVMPath)
	args := []string{
		"cmd",
		"/c",
		uvmPnpExePath,
		"/add-driver",
		dirFormatted,
		"/subdirs",
		"/install",
	}
	return args
}

// execPnPInstallDriver makes the calls to exec in the uvm the pnp command
// that installs a driver previously mounted into the uvm.
func execPnPInstallDriver(ctx context.Context, vm *uvm.UtilityVM, driverDir string) error {
	args := createPnPInstallDriverCommand(driverDir)
	cmdReq := &cmd.CmdProcessRequest{
		Args: args,
	}
	exitCode, err := cmd.ExecInUvm(ctx, vm, cmdReq)
	if err != nil && exitCode != winapi.ERROR_NO_MORE_ITEMS {
		return errors.Wrapf(err, "failed to install driver %s in uvm with exit code %d", driverDir, exitCode)
	} else if exitCode == winapi.ERROR_NO_MORE_ITEMS {
		// As mentioned in `pnputilNoMoreItemsErrorMessage`, this exit code comes from pnputil
		// but is not necessarily an error
		log.G(ctx).WithFields(logrus.Fields{
			logfields.UVMID: vm.ID(),
			"driver":        driverDir,
			"error":         pnputilNoMoreItemsErrorMessage,
		}).Warn("expected version of driver may not have been installed")
	}

	log.G(ctx).WithField("added drivers", driverDir).Debug("installed drivers")
	return nil
}

// readCsPipeOutput is a helper function that connects to a listener and reads
// the connection's comma separated output until done. resulting comma separated
// values are returned in the `result` param. The `errChan` param is used to
// propagate an errors to the calling function.
func readCsPipeOutput(l net.Listener, errChan chan<- error, result *[]string) {
	defer close(errChan)
	c, err := l.Accept()
	if err != nil {
		errChan <- errors.Wrapf(err, "failed to accept named pipe")
		return
	}
	bytes, err := io.ReadAll(c)
	if err != nil {
		errChan <- err
		return
	}

	elementsAsString := strings.TrimSuffix(string(bytes), "\n")
	elements := strings.Split(elementsAsString, ",")
	*result = append(*result, elements...)

	if len(*result) == 0 {
		errChan <- noExecOutputErr
		return
	}

	errChan <- nil
}

// readAllPipeOutput is a helper function that connects to a listener and attempts to
// read the connection's entire output. Resulting output is returned as a string
// in the `result` param. The `errChan` param is used to propagate an errors to
// the calling function.
func readAllPipeOutput(l net.Listener, errChan chan<- error, result *string) {
	defer close(errChan)
	c, err := l.Accept()
	if err != nil {
		errChan <- errors.Wrapf(err, "failed to accept named pipe")
		return
	}
	bytes, err := io.ReadAll(c)
	if err != nil {
		errChan <- err
		return
	}

	*result = string(bytes)

	if len(*result) == 0 {
		errChan <- noExecOutputErr
		return
	}

	errChan <- nil
}
