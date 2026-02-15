//go:build windows

package builder

import (
	"strconv"
	"strings"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/pkg/errors"
)

// vPCIDevice represents a vpci device.
type vPCIDevice struct {
	// vmbusGUID is the instance ID for this device when it is exposed via VMBus
	vmbusGUID string
	// deviceInstanceID is the instance ID of the device on the host
	deviceInstanceID string
	// virtualFunctionIndex is the function index for the pci device to assign
	virtualFunctionIndex uint16
	// refCount stores the number of references to this device in the UVM
	refCount uint32
}

// DeviceOptions configures device settings for the Utility VM.
type DeviceOptions interface {
	// AddVPCIDevice adds a PCI device to the Utility VM.
	// If the device is already added, we return an error.
	AddVPCIDevice(device hcsschema.VirtualPciFunction, numaAffinity bool) error
	// AddSCSIController adds a SCSI controller to the Utility VM with the specified ID.
	AddSCSIController(id string)
	// AddSCSIDisk adds a SCSI disk to the Utility VM under the specified controller and LUN.
	AddSCSIDisk(controller string, lun string, disk hcsschema.Attachment) error
	// AddVPMemController adds a VPMem controller to the Utility VM with the specified maximum devices and size.
	AddVPMemController(maximumDevices uint32, maximumSizeBytes uint64)
	// AddVPMemDevice adds a VPMem device to the Utility VM under the VPMem controller.
	AddVPMemDevice(id string, device hcsschema.VirtualPMemDevice) error
	// AddVSMBShare adds a VSMB share to the Utility VM.
	AddVSMBShare(share hcsschema.VirtualSmbShare) error
	// SetSerialConsole sets up a serial console for `port`. Output will be relayed to the listener specified
	// by `listenerPath`. For HCS `listenerPath` this is expected to be a path to a named pipe.
	SetSerialConsole(port uint32, listenerPath string) error
	// EnableGraphicsConsole enables a graphics console for the Utility VM.
	EnableGraphicsConsole()
}

var _ DeviceOptions = (*UtilityVM)(nil)

func (uvmb *UtilityVM) AddVPCIDevice(device hcsschema.VirtualPciFunction, numaAffinity bool) error {
	_, ok := uvmb.assignedDevices[device]
	if ok {
		return errors.Wrapf(errAlreadySet, "device %v already assigned to utility VM", device)
	}

	vmbusGUID, err := guid.NewV4()
	if err != nil {
		return errors.Wrap(err, "failed to generate VMBus GUID for device")
	}

	var propagateAffinity *bool
	if numaAffinity {
		propagateAffinity = &numaAffinity
	}

	if uvmb.doc.VirtualMachine.Devices.VirtualPci == nil {
		uvmb.doc.VirtualMachine.Devices.VirtualPci = make(map[string]hcsschema.VirtualPciDevice)
	}

	uvmb.doc.VirtualMachine.Devices.VirtualPci[vmbusGUID.String()] = hcsschema.VirtualPciDevice{
		Functions: []hcsschema.VirtualPciFunction{
			device,
		},
		PropagateNumaAffinity: propagateAffinity,
	}

	uvmb.assignedDevices[device] = &vPCIDevice{
		vmbusGUID:            vmbusGUID.String(),
		deviceInstanceID:     device.DeviceInstancePath,
		virtualFunctionIndex: device.VirtualFunction,
		refCount:             1,
	}

	return nil
}

func (uvmb *UtilityVM) SetSerialConsole(port uint32, listenerPath string) error {
	if !strings.HasPrefix(listenerPath, `\\.\pipe\`) {
		return errors.New("listener for serial console is not a named pipe")
	}

	uvmb.doc.VirtualMachine.Devices.ComPorts = map[string]hcsschema.ComPort{
		strconv.Itoa(int(port)): { // "0" would be COM1
			NamedPipe: listenerPath,
		},
	}
	return nil
}

func (uvmb *UtilityVM) EnableGraphicsConsole() {
	uvmb.doc.VirtualMachine.Devices.Keyboard = &hcsschema.Keyboard{}
	uvmb.doc.VirtualMachine.Devices.EnhancedModeVideo = &hcsschema.EnhancedModeVideo{}
	uvmb.doc.VirtualMachine.Devices.VideoMonitor = &hcsschema.VideoMonitor{}
}
