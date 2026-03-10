//go:build windows

package vpci

import (
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
		// we have a vf index
		return filepath.Dir(rawDevicePath), uint16(index)
	}
	// otherwise, just use default index and full device ID given
	return rawDevicePath, 0
}
