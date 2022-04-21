//go:build windows
// +build windows

package devices

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/pkg/errors"
)

// AddDevice is the api exposed to hcsoci to handle assigning a device on a UVM
//
// `idType` refers to the specified device's type, supported types here are `VPCIDeviceIDType`
// and `VPCIDeviceIDTypeLegacy`.
//
// `deviceID` refers to the specified device's identifier. This must refer to a device instance id
// for hyper-v isolated device assignment.
//
// `deviceUtilPath` refers to the path in the UVM of the device-util tool used for finding the given
// device's location path(s).
//
// Returns the allocated vpci device in `vpci` to be tracked for release by the caller. On failure in
// this function, `vpci` is released and nil is returned for that value.
//
// Returns a slice of strings representing the resulting location path(s) for the specified device.
func AddDevice(ctx context.Context, vm *uvm.UtilityVM, idType, deviceID string, index uint16, deviceUtilPath string) (vpci *uvm.VPCIDevice, locationPaths []string, err error) {
	defer func() {
		if err != nil && vpci != nil {
			// best effort clean up allocated resource on failure
			if releaseErr := vpci.Release(ctx); releaseErr != nil {
				log.G(ctx).WithError(releaseErr).Error("failed to release container resource")
			}
			vpci = nil
		}
	}()
	if idType == uvm.VPCIDeviceIDType || idType == uvm.VPCIDeviceIDTypeLegacy {
		vpci, err = vm.AssignDevice(ctx, deviceID, index, "")
		if err != nil {
			return vpci, nil, errors.Wrapf(err, "failed to assign device %s of type %s to pod %s", deviceID, idType, vm.ID())
		}
		vmBusInstanceID := vm.GetAssignedDeviceVMBUSInstanceID(vpci.VMBusGUID)
		log.G(ctx).WithField("vmbus id", vmBusInstanceID).Debug("vmbus instance ID")

		locationPaths, err = getChildrenDeviceLocationPaths(ctx, vm, vmBusInstanceID, deviceUtilPath)
		return vpci, locationPaths, err
	}

	return vpci, nil, fmt.Errorf("device type %s for device %s is not supported in windows", idType, deviceID)
}

// getChildrenDeviceLocationPaths queries the UVM with the device-util tool with the formatted
// parent bus device for the children devices' location paths from the uvm's view.
// Returns a slice of strings representing the resulting children location paths
func getChildrenDeviceLocationPaths(ctx context.Context, vm *uvm.UtilityVM, vmBusInstanceID string, deviceUtilPath string) ([]string, error) {
	p, l, err := cmd.CreateNamedPipeListener()
	if err != nil {
		return nil, err
	}
	defer l.Close()

	var pipeResults []string
	errChan := make(chan error)

	go readCsPipeOutput(l, errChan, &pipeResults)

	args := createDeviceUtilChildrenCommand(deviceUtilPath, vmBusInstanceID)
	cmdReq := &cmd.CmdProcessRequest{
		Args:   args,
		Stdout: p,
	}
	exitCode, err := cmd.ExecInUvm(ctx, vm, cmdReq)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find devices with exit code %d", exitCode)
	}

	// wait to finish parsing stdout results
	select {
	case err := <-errChan:
		if err != nil {
			return nil, err
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return pipeResults, nil
}

// createDeviceUtilChildrenCommand constructs a device-util command to query the UVM for
// device information
//
// `deviceUtilPath` is the UVM path to device-util
//
// `vmBusInstanceID` is a string of the vmbus instance ID already assigned to the UVM
//
// Returns a slice of strings that represent the location paths in the UVM of the
// target devices
func createDeviceUtilChildrenCommand(deviceUtilPath string, vmBusInstanceID string) []string {
	parentIDsFlag := fmt.Sprintf("--parentID=%s", vmBusInstanceID)
	args := []string{deviceUtilPath, "children", parentIDsFlag, "--property=location"}
	return args
}
