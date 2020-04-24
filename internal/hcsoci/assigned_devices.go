// +build windows

package hcsoci

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"

	winio "github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	"github.com/Microsoft/hcsshim/internal/uvm"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

func specHasAssignedDevices(coi *createOptionsInternal) bool {
	if (coi.Spec.Windows != nil) && (coi.Spec.Windows.Devices != nil) &&
		(len(coi.Spec.Windows.Devices) > 0) {
		return true
	}
	return false
}

// handleAssignedDevicesWindows does all of the work to setup the hosting UVM and retrieve
// device information for adding assigned devices on a WCOW container definition.
//
// First, devices are assigned into the hosting UVM. Drivers are then added into the UVM and
// installed on the matching devices. This ordering allows us to guarantee that driver
// installation on a device in the UVM is completed before we attempt to create a container.
//
// Then we find the location paths of the target devices in the UVM and return the results
// as WindowsDevices.
func handleAssignedDevicesWindows(ctx context.Context, coi *createOptionsInternal, r *Resources) ([]specs.WindowsDevice, error) {
	vpciVMBusInstanceIDs := []string{}
	for _, d := range coi.Spec.Windows.Devices {
		if d.IDType == uvm.VPCIDeviceIDType || d.IDType == uvm.VPCIDeviceIDTypeLegacy {
			vpci, err := coi.HostingSystem.AssignDevice(ctx, d.ID)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to assign device %s of type %s to pod %s", d.ID, d.IDType, coi.HostingSystem.ID())
			}
			r.resources = append(r.resources, vpci)
			vmBusInstanceID := coi.HostingSystem.GetAssignedDeviceParentID(vpci.VMBusGUID)
			log.G(ctx).WithField("vmbus id", vmBusInstanceID).Info("vmbus instance ID")

			vpciVMBusInstanceIDs = append(vpciVMBusInstanceIDs, vmBusInstanceID)
		}
	}
	if len(vpciVMBusInstanceIDs) == 0 {
		return nil, fmt.Errorf("no assignable devices on the spec %v", coi.Spec.Windows.Devices)
	}
	if err := setupDriversWindows(ctx, coi, r); err != nil {
		return nil, err
	}
	deviceUtilPath, err := setupDeviceUtilTools(ctx, coi, r)
	if err != nil {
		return nil, err
	}
	return getBusAssignedChildrenDeviceLocationPaths(ctx, coi, vpciVMBusInstanceIDs, deviceUtilPath)
}

// getBusAssignedChildrenDeviceLocationPaths queries the UVM with the device-util tool with the formatted
// parent bus device for the children devices' location paths from the uvm's view
// Returns a slice of WindowsDevices created from the resulting children location paths
func getBusAssignedChildrenDeviceLocationPaths(ctx context.Context, coi *createOptionsInternal, vmBusInstanceIDs []string, deviceUtilPath string) ([]specs.WindowsDevice, error) {
	p, l, err := createNamedPipeListener()
	if err != nil {
		return nil, err
	}
	defer l.Close()

	var pipeResults []string
	errChan := make(chan error)
	defer close(errChan)

	go readCsPipeOutput(l, errChan, &pipeResults)

	args := createDeviceUtilCommand(deviceUtilPath, vmBusInstanceIDs)
	req := &shimdiag.ExecProcessRequest{
		Args:   args,
		Stdout: p,
	}
	exitCode, err := ExecInUvm(ctx, coi.HostingSystem, req)
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

	// convert stdout results into windows devices
	results := []specs.WindowsDevice{}
	for _, value := range pipeResults {
		specDev := specs.WindowsDevice{
			ID:     value,
			IDType: uvm.VPCILocationPathIDType,
		}
		results = append(results, specDev)
	}
	log.G(ctx).WithField("parsed devices", results).Info("found child assigned devices")
	return results, nil
}

func createNamedPipeListener() (string, net.Listener, error) {
	g, err := guid.NewV4()
	if err != nil {
		return "", nil, err
	}
	p := `\\.\pipe\` + g.String()
	l, err := winio.ListenPipe(p, nil)
	if err != nil {
		return "", nil, err
	}
	return p, l, nil
}

// readCsPipeOutput is a helper function that connects to a listener and reads
// the connection's comma separated outut until done. resulting comma separated
// values are returned in the `result` param. The `done` param is used by this
// func to indicate completion.
func readCsPipeOutput(l net.Listener, errChan chan<- error, result *[]string) {
	c, err := l.Accept()
	if err != nil {
		errChan <- errors.Wrapf(err, "failed to accept named pipe")
		return
	}
	r := bufio.NewReader(c)
	var readErr error
	var rawElem string
	for {
		rawElem, readErr = r.ReadString(',')
		if len(rawElem) != 0 {
			// remove the comma at the end of the line
			elem := string(rawElem[:len(rawElem)-1])
			*result = append(*result, elem)
		}
		if readErr != nil {
			break
		}
	}
	if readErr == io.EOF {
		errChan <- nil
		return
	}
	errChan <- readErr
}
