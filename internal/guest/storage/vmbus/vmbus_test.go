//go:build linux
// +build linux

package vmbus

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func Test_WaitForVMBusDevicePath_Success(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	vmBusGUID := "1111-2222-3333-4444"
	pciDir := "pci1234:00"

	storageWaitForFileMatchingPattern = func(ctx context.Context, pattern string) (string, error) {
		vmBusDirPath, targetPattern := filepath.Split(pattern)
		if targetPattern == "pci*" {
			return filepath.Join(vmBusDirPath, pciDir), nil
		}
		return "", nil
	}

	vmBusGUIDPattern := filepath.Join(vmBusGUID, "pci*")
	expectedResult := filepath.Join("/sys/bus/vmbus/devices", vmBusGUID, pciDir)
	result, err := WaitForDevicePath(ctx, vmBusGUIDPattern)
	if err != nil {
		t.Fatalf("expected to succeed, instead got: %v", err)
	}
	if result != expectedResult {
		t.Fatalf("result %s does not match expected result %s", result, expectedResult)
	}
}
