package uvm

import (
	"context"
	"errors"
	"fmt"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/guestrequest"
	"github.com/Microsoft/hcsshim/internal/requesttype"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
)

const (
	GPUDeviceIDType         = "gpu"
	VPCILocationPathIDType  = "vpci-location-path"
	VPCIClassGUIDTypeLegacy = "class"
	VPCIClassGUIDType       = "vpci-class-guid"
)

// VPCIDevice represents a vpci device. Holds its guid and a handle to the uvm it
// belongs to.
type VPCIDevice struct {
	// vm is the handle to the UVM that this device belongs to
	vm *UtilityVM
	// VMBusGUID is the instance ID for this device when it is exposed via VMBus
	VMBusGUID string
	// deviceInstanceID is the instance ID of the device on the host
	deviceInstanceID string
	// refCount stores the number of references to this device in the UVM
	refCount uint32
}

// Release frees the resources of the corresponding vpci device
func (vpci *VPCIDevice) Release(ctx context.Context) error {
	if err := vpci.vm.removeDevice(ctx, vpci.deviceInstanceID); err != nil {
		return fmt.Errorf("failed to remove VPCI device: %s", err)
	}
	return nil
}

// AssignDevice assigns a vpci device to the uvm
// if the device already exists, the stored VPCIDevice's ref count is increased
// and the VPCIDevice is returned.
// Otherwise, a new request is made to assign the target device indicated by the deviceID
// onto the UVM. A new VPCIDevice entry is made on the UVM and the VPCIDevice is returned
// to the caller
func (uvm *UtilityVM) AssignDevice(ctx context.Context, deviceID string) (*VPCIDevice, error) {
	if uvm.operatingSystem == "windows" {
		return nil, errors.New("assigned devices is not currently supported on wcow")
	}

	guid, err := guid.NewV4()
	if err != nil {
		return nil, err
	}
	vmBusGUID := guid.String()

	uvm.m.Lock()
	defer uvm.m.Unlock()

	existingVPCIDevice := uvm.vpciDevices[deviceID]
	if existingVPCIDevice != nil {
		existingVPCIDevice.refCount++
		return existingVPCIDevice, nil
	}

	targetDevice := hcsschema.VirtualPciDevice{
		Functions: []hcsschema.VirtualPciFunction{
			{
				DeviceInstancePath: deviceID,
			},
		},
	}

	request := &hcsschema.ModifySettingRequest{
		ResourcePath: fmt.Sprintf(virtualPciResourceFormat, vmBusGUID),
		RequestType:  requesttype.Add,
		Settings:     targetDevice}

	// WCOW (when supported) does not require a guest request as part of the
	// device assignment
	if uvm.operatingSystem != "windows" {
		// for LCOW, we need to make sure that specific paths relating to the
		// device exist so they are ready to be used by later
		// work in openGCS
		request.GuestRequest = guestrequest.GuestRequest{
			ResourceType: guestrequest.ResourceTypeVPCIDevice,
			RequestType:  requesttype.Add,
			Settings: guestrequest.LCOWMappedVPCIDevice{
				VMBusGUID: vmBusGUID,
			},
		}
	}

	if err := uvm.modify(ctx, request); err != nil {
		return nil, err
	}
	result := &VPCIDevice{
		vm:               uvm,
		VMBusGUID:        vmBusGUID,
		deviceInstanceID: deviceID,
		refCount:         1,
	}
	uvm.vpciDevices[deviceID] = result
	return result, nil
}

// removeDevice removes a vpci device from a uvm when there are
// no more references to a given VPCIDevice. Otherwise, decrements
// the reference count of the stored VPCIDevice and returns nil.
func (uvm *UtilityVM) removeDevice(ctx context.Context, deviceInstanceID string) error {
	uvm.m.Lock()
	defer uvm.m.Unlock()

	vpci := uvm.vpciDevices[deviceInstanceID]
	if vpci == nil {
		return fmt.Errorf("no device with ID %s is present on the uvm %s", deviceInstanceID, uvm.ID())
	}

	vpci.refCount--
	if vpci.refCount == 0 {
		delete(uvm.vpciDevices, deviceInstanceID)
		return uvm.modify(ctx, &hcsschema.ModifySettingRequest{
			ResourcePath: fmt.Sprintf(virtualPciResourceFormat, vpci.VMBusGUID),
			RequestType:  requesttype.Remove,
		})
	}
	return nil
}
