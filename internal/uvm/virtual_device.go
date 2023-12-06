//go:build windows

package uvm

import (
	"context"
	"fmt"

	"github.com/Microsoft/go-winio/pkg/guid"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

const (
	GPUDeviceIDType         = "gpu"
	VPCILocationPathIDType  = "vpci-location-path"
	VPCIClassGUIDTypeLegacy = "class"
	VPCIClassGUIDType       = "vpci-class-guid"
	VPCIDeviceIDTypeLegacy  = "vpci"
	VPCIDeviceIDType        = "vpci-instance-id"
)

// this is the well known channel type GUID defined by VMBUS for all assigned devices
const vmbusChannelTypeGUIDFormatted = "{44c4f61d-4444-4400-9d52-802e27ede19f}"
const assignedDeviceEnumerator = "VMBUS"

type VPCIDeviceKey struct {
	deviceInstanceID     string
	virtualFunctionIndex uint16
}

// VPCIDevice represents a vpci device. Holds its guid and a handle to the uvm it
// belongs to.
type VPCIDevice struct {
	// vm is the handle to the UVM that this device belongs to
	vm *UtilityVM
	// VMBusGUID is the instance ID for this device when it is exposed via VMBus
	VMBusGUID string
	// deviceInstanceID is the instance ID of the device on the host
	deviceInstanceID string
	// virtualFunctionIndex is the function index for the pci device to assign
	virtualFunctionIndex uint16
	// refCount stores the number of references to this device in the UVM
	refCount uint32
}

// GetAssignedDeviceVMBUSInstanceID returns the instance ID of the VMBUS channel device node created.
//
// When a device is assigned to a UVM via VPCI support in HCS, a new VMBUS channel device node is
// created in the UVM. The actual device that was assigned in is exposed as a child on this VMBUS
// channel device node.
//
// A device node's instance ID is an identifier that distinguishes that device from other devices
// on the system. The GUID of a VMBUS channel device node refers to that channel's unique
// identifier used internally by VMBUS and can be used to determine the VMBUS channel
// device node's instance ID.
//
// A VMBUS channel device node's instance ID is in the form:
// "VMBUS\vmbusChannelTypeGUIDFormatted\{vmBusChannelGUID}"
func (uvm *UtilityVM) GetAssignedDeviceVMBUSInstanceID(vmBusChannelGUID string) string {
	return fmt.Sprintf("%s\\%s\\{%s}", assignedDeviceEnumerator, vmbusChannelTypeGUIDFormatted, vmBusChannelGUID)
}

// Release frees the resources of the corresponding vpci device
func (vpci *VPCIDevice) Release(ctx context.Context) error {
	if err := vpci.vm.RemoveDevice(ctx, vpci.deviceInstanceID, vpci.virtualFunctionIndex); err != nil {
		return fmt.Errorf("failed to remove VPCI device: %w", err)
	}
	return nil
}

// AssignDevice assigns a vpci device to the uvm
// if the device already exists, the stored VPCIDevice's ref count is increased
// and the VPCIDevice is returned.
// Otherwise, a new request is made to assign the target device indicated by the deviceID
// onto the UVM. A new VPCIDevice entry is made on the UVM and the VPCIDevice is returned
// to the caller.
// Allow callers to specify the vmbus guid they want the device to show up with.
func (uvm *UtilityVM) AssignDevice(ctx context.Context, deviceID string, index uint16, vmBusGUID string) (*VPCIDevice, error) {
	if vmBusGUID == "" {
		guid, err := guid.NewV4()
		if err != nil {
			return nil, err
		}
		vmBusGUID = guid.String()
	}

	key := VPCIDeviceKey{
		deviceInstanceID:     deviceID,
		virtualFunctionIndex: index,
	}

	uvm.m.Lock()
	defer uvm.m.Unlock()

	existingVPCIDevice := uvm.vpciDevices[key]
	if existingVPCIDevice != nil {
		existingVPCIDevice.refCount++
		return existingVPCIDevice, nil
	}

	targetDevice := hcsschema.VirtualPciDevice{
		Functions: []hcsschema.VirtualPciFunction{
			{
				DeviceInstancePath: deviceID,
				VirtualFunction:    index,
			},
		},
	}

	request := &hcsschema.ModifySettingRequest{
		ResourcePath: fmt.Sprintf(resourcepaths.VirtualPCIResourceFormat, vmBusGUID),
		RequestType:  guestrequest.RequestTypeAdd,
		Settings:     targetDevice,
	}

	// WCOW (when supported) does not require a guest request as part of the
	// device assignment
	if uvm.operatingSystem != "windows" {
		// for LCOW, we need to make sure that specific paths relating to the
		// device exist so they are ready to be used by later
		// work in openGCS
		request.GuestRequest = guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeVPCIDevice,
			RequestType:  guestrequest.RequestTypeAdd,
			Settings: guestresource.LCOWMappedVPCIDevice{
				VMBusGUID: vmBusGUID,
			},
		}
	}

	if err := uvm.modify(ctx, request); err != nil {
		return nil, err
	}
	result := &VPCIDevice{
		vm:                   uvm,
		VMBusGUID:            vmBusGUID,
		deviceInstanceID:     deviceID,
		virtualFunctionIndex: index,
		refCount:             1,
	}
	uvm.vpciDevices[key] = result
	return result, nil
}

// RemoveDevice removes a vpci device from a uvm when there are
// no more references to a given VPCIDevice. Otherwise, decrements
// the reference count of the stored VPCIDevice and returns nil.
func (uvm *UtilityVM) RemoveDevice(ctx context.Context, deviceInstanceID string, index uint16) error {
	key := VPCIDeviceKey{
		deviceInstanceID:     deviceInstanceID,
		virtualFunctionIndex: index,
	}

	uvm.m.Lock()
	defer uvm.m.Unlock()

	vpci := uvm.vpciDevices[key]
	if vpci == nil {
		return fmt.Errorf("no device with ID %s and index %d is present on the uvm %s", deviceInstanceID, index, uvm.ID())
	}

	vpci.refCount--
	if vpci.refCount == 0 {
		delete(uvm.vpciDevices, key)
		return uvm.modify(ctx, &hcsschema.ModifySettingRequest{
			ResourcePath: fmt.Sprintf(resourcepaths.VirtualPCIResourceFormat, vpci.VMBusGUID),
			RequestType:  guestrequest.RequestTypeRemove,
		})
	}
	return nil
}
