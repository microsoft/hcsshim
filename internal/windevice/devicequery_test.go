//go:build windows

package windevice

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Microsoft/go-winio/vhd"
)

func TestDeviceInterfaceInstancesWithPnpUtil(t *testing.T) {
	ctx := context.Background()
	initialInterfacesList, err := getDeviceInterfaceInstancesByClass(ctx, &devClassDiskGUID, false)
	if err != nil {
		t.Fatalf("failed to get initial disk interfaces: %v", err)
	}
	t.Logf("initial interface list: %+v\n", initialInterfacesList)

	// make a fake VHD and attach it.
	tempDir := t.TempDir()
	vhdxPath := filepath.Join(tempDir, "test.vhdx")
	if err := vhd.CreateVhdx(vhdxPath, 1, 1); err != nil {
		t.Fatalf("failed to create vhd: %s", err)
	}
	if err := vhd.AttachVhd(vhdxPath); err != nil {
		t.Fatalf("failed to attach vhd: %s", err)
	}
	t.Cleanup(func() {
		if err := vhd.DetachVhd(vhdxPath); err != nil {
			t.Logf("failed to detach VHD during cleanup: %s\n", err)
		}
	})

	interfaceListAfterVHDAttach, err := getDeviceInterfaceInstancesByClass(ctx, &devClassDiskGUID, false)
	if err != nil {
		t.Fatalf("failed to get initial disk interfaces: %v", err)
	}
	t.Logf("interface list after attaching VHD: %+v\n", interfaceListAfterVHDAttach)

	if len(initialInterfacesList) != (len(interfaceListAfterVHDAttach) - 1) {
		t.Fatalf("expected to find exactly 1 new interface in the returned interfaces list")
	}

	if err := vhd.DetachVhd(vhdxPath); err != nil {
		t.Fatalf("failed to detach VHD: %s\n", err)
	}

	// Looks like a small time gap is required before we query for device interfaces again to get the updated list.
	time.Sleep(2 * time.Second)

	finalInterfaceList, err := getDeviceInterfaceInstancesByClass(ctx, &devClassDiskGUID, false)
	if err != nil {
		t.Fatalf("failed to get initial disk interfaces: %v", err)
	}
	t.Logf("interface list after detaching VHD: %+v\n", finalInterfaceList)

	if len(initialInterfacesList) != len(finalInterfaceList) {
		t.Fatalf("expected interface lists to have same length")
	}
}
