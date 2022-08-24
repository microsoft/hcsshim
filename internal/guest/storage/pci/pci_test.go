//go:build linux
// +build linux

package pci

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func Test_WaitForPCIDeviceFromVMBusGUID_Success(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	vmBusGUID := "1111-2222-3333-4444"
	pciDir := "pci1234:00"
	busLocation := "1234:00:00.0"

	vmbusWaitForDevicePath = func(ctx context.Context, vmbusGUIDPattern string) (string, error) {
		vmBusPath := filepath.Join("/sys/bus/vmbus/devices", vmbusGUIDPattern)
		vmBusDirPath, targetPattern := filepath.Split(vmBusPath)
		if targetPattern == "pci*" {
			return filepath.Join(vmBusDirPath, pciDir), nil
		}
		return "", nil
	}

	storageWaitForFileMatchingPattern = func(ctx context.Context, pattern string) (string, error) {
		vmBusPciDirPath, targetPattern := filepath.Split(pattern)
		if targetPattern == "1234:00*" {
			return filepath.Join(vmBusPciDirPath, busLocation), nil
		}
		return "", nil
	}

	resultBusLocation, err := FindDeviceBusLocationFromVMBusGUID(ctx, vmBusGUID)
	if err != nil {
		t.Fatalf("expected to succeed, instead got: %v", err)
	}
	if resultBusLocation != busLocation {
		t.Fatalf("result %s does not match expected result %s", resultBusLocation, busLocation)
	}
}
