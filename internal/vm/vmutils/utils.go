//go:build windows

package vmutils

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/Microsoft/hcsshim/internal/log"
)

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

// ParseUVMReferenceInfo reads the UVM reference info file, and base64 encodes the content if it exists.
func ParseUVMReferenceInfo(ctx context.Context, referenceRoot, referenceName string) (string, error) {
	if referenceName == "" {
		return "", nil
	}

	fullFilePath := filepath.Join(referenceRoot, referenceName)
	content, err := os.ReadFile(fullFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			log.G(ctx).WithField("filePath", fullFilePath).Debug("UVM reference info file not found")
			return "", nil
		}
		return "", fmt.Errorf("failed to read UVM reference info file: %w", err)
	}

	return base64.StdEncoding.EncodeToString(content), nil
}
