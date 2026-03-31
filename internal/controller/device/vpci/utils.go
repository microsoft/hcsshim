//go:build windows

package vpci

import (
	"fmt"
	"path/filepath"
	"strconv"
)

const (
	// GpuDeviceIDType is the assigned device ID type for GPU devices.
	GpuDeviceIDType = "gpu"

	// DeviceIDTypeLegacy is the legacy assigned device ID type for vPCI devices.
	DeviceIDTypeLegacy = "vpci"
	// DeviceIDType is the assigned device ID type for vPCI instance IDs.
	DeviceIDType = "vpci-instance-id"
)

const (
	// vmBusChannelTypeGUIDFormatted is the well-known channel type GUID defined by
	// VMBus for all assigned devices.
	vmBusChannelTypeGUIDFormatted = "{44c4f61d-4444-4400-9d52-802e27ede19f}"

	// assignedDeviceEnumerator is the VMBus enumerator prefix used in device
	// instance IDs for assigned devices.
	assignedDeviceEnumerator = "VMBUS"
)

// IsValidDeviceType returns true if the device type is valid i.e. supported by the runtime.
func IsValidDeviceType(deviceType string) bool {
	return (deviceType == DeviceIDType) ||
		(deviceType == DeviceIDTypeLegacy) ||
		(deviceType == GpuDeviceIDType)
}

// GetDeviceInfoFromPath takes a device path and parses it into the PCI ID and
// virtual function index if one is specified.
func GetDeviceInfoFromPath(rawDevicePath string) (string, uint16) {
	indexString := filepath.Base(rawDevicePath)
	index, err := strconv.ParseUint(indexString, 10, 16)
	if err == nil {
		// We have a VF index.
		return filepath.Dir(rawDevicePath), uint16(index)
	}
	// Otherwise, just use default index and the full device ID as given.
	return rawDevicePath, 0
}

// GetAssignedDeviceVMBUSInstanceID returns the instance ID of the VMBus channel
// device node created when a device is assigned to a UVM via vPCI.
//
// When a device is assigned to a UVM via vPCI support in HCS, a new VMBus channel device node is
// created in the UVM. The actual device that was assigned in is exposed as a child on this VMBus
// channel device node.
//
// A device node's instance ID is an identifier that distinguishes that device from other devices
// on the system. The GUID of a VMBus channel device node refers to that channel's unique
// identifier used internally by VMBus and can be used to determine the VMBus channel
// device node's instance ID.
//
// A VMBus channel device node's instance ID is in the form:
//
//	"VMBUS\{channelTypeGUID}\{vmBusChannelGUID}"
func GetAssignedDeviceVMBUSInstanceID(vmBusChannelGUID string) string {
	return fmt.Sprintf("%s\\%s\\{%s}", assignedDeviceEnumerator, vmBusChannelTypeGUIDFormatted, vmBusChannelGUID)
}
