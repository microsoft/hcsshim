//go:build windows

package vmutils

const (
	// GpuDeviceIDType is the assigned device ID type for GPU devices.
	GpuDeviceIDType = "gpu"
	// VPCIDeviceIDTypeLegacy is the legacy assigned device ID type for vPCI devices.
	VPCIDeviceIDTypeLegacy = "vpci"
	// VPCIDeviceIDType is the assigned device ID type for vPCI instance IDs.
	VPCIDeviceIDType = "vpci-instance-id"
)

// IsValidDeviceType returns true if the device type is valid i.e. supported by the runtime.
func IsValidDeviceType(deviceType string) bool {
	return (deviceType == VPCIDeviceIDType) ||
		(deviceType == VPCIDeviceIDTypeLegacy) ||
		(deviceType == GpuDeviceIDType)
}
