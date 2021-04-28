// +build linux

package pci

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Microsoft/hcsshim/internal/guest/storage"
	"github.com/Microsoft/hcsshim/internal/guest/storage/vmbus"
)

var storageWaitForFileMatchingPattern = storage.WaitForFileMatchingPattern
var vmbusWaitForDevicePath = vmbus.WaitForDevicePath

// WaitForPCIDeviceFromVMBusGUID waits for bus location path of the device to be present
func WaitForPCIDeviceFromVMBusGUID(ctx context.Context, vmBusGUID string) error {
	_, err := FindDeviceBusLocationFromVMBusGUID(ctx, vmBusGUID)
	return err
}

// FindDeviceBusLocationFromVMBusGUID finds device bus location by
// reading /sys/bus/vmbus/devices/<vmBusGUID>/... for pci specific directories
func FindDeviceBusLocationFromVMBusGUID(ctx context.Context, vmBusGUID string) (string, error) {
	pciDir, err := findVMBusPCIDir(ctx, vmBusGUID)
	if err != nil {
		return "", err
	}

	pciDeviceLocation, err := findVMBusPCIDevice(ctx, pciDir)
	if err != nil {
		return "", err
	}
	return pciDeviceLocation, nil
}

// findVMBusPCIDir waits for the pci bus directory matching pattern
// /sys/bus/vmbus/devices/<vmBusGUID>/pci* to exist and returns
// the full resulting path or an error
func findVMBusPCIDir(ctx context.Context, vmBusGUID string) (string, error) {
	vmBusPCIPathPattern := filepath.Join(vmBusGUID, "pci*")
	return vmbusWaitForDevicePath(ctx, vmBusPCIPathPattern)
}

// findVMBusPCIDevice waits for the pci bus location directory under the path
// returned from findVMBusPCIDir to exist and returns the pci bus location or an error
func findVMBusPCIDevice(ctx context.Context, pciDirFullPath string) (string, error) {
	// trim /sys/bus/vmbus/devices/<vmBusGUID>/pciXXXX:XX to XXXX:XX
	_, pciDirName := filepath.Split(pciDirFullPath)
	busPrefix := strings.TrimPrefix(pciDirName, "pci")

	// under /sys/bus/vmbus/devices/<vmBusGUID>/pciXXXX:XX/ look for directory matching XXXX:XX* pattern
	busPathPattern := filepath.Join(pciDirFullPath, fmt.Sprintf("%s*", busPrefix))
	busFileFullPath, err := storageWaitForFileMatchingPattern(ctx, busPathPattern)
	if err != nil {
		return "", err
	}

	// return the resulting XXXX:XX:YY.Y pci bus location
	_, busFile := filepath.Split(busFileFullPath)
	return busFile, nil
}
