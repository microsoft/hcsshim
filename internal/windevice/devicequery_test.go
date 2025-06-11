//go:build windows

package windevice

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Microsoft/go-winio/vhd"
	"golang.org/x/sys/windows"
)

func TestGetDeviceInterfaceInstances(t *testing.T) {
	ctx := context.Background()
	initialInterfacesList, err := getDeviceInterfaceInstancesByClass(ctx, &devClassDiskGUID, false)
	if err != nil {
		t.Fatalf("failed to get initial disk interfaces: %v", err)
	}
	t.Logf("initial interface list: %+v\n", initialInterfacesList)

	// make a fake VHD and attach it.
	vhdDir := t.TempDir()
	vhdxPath := filepath.Join(vhdDir, "test_scsi_device.vhdx")

	if err := vhd.CreateVhdx(vhdxPath, 1, 1); err != nil {
		t.Fatalf("failed to create vhd: %s", err)
	}

	diskHandle, err := vhd.OpenVirtualDisk(vhdxPath, vhd.VirtualDiskAccessNone, vhd.OpenVirtualDiskFlagNone)
	if err != nil {
		t.Fatalf("failed to open VHD handle: %s", err)
	}
	t.Cleanup(func() {
		if closeErr := windows.CloseHandle(windows.Handle(diskHandle)); closeErr != nil {
			t.Logf("Failed to close VHD handle: %s", closeErr)
		}
	})

	// AttachVirtualDiskParameters MUST use `Version : 1` for it work on WS2019. We
	// don't plan to use these newly added methods (that are being tested here) on
	// WS2019, but good to still run the test on WS2019 if we can easily do that.
	err = vhd.AttachVirtualDisk(diskHandle, vhd.AttachVirtualDiskFlagNone, &vhd.AttachVirtualDiskParameters{Version: 1})
	if err != nil {
		t.Fatalf("failed to attach VHD: %s", err)
	}
	t.Cleanup(func() {
		if detachErr := vhd.DetachVirtualDisk(diskHandle); detachErr != nil {
			t.Logf("failed to detach vhd: %s", detachErr)
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

	if detachErr := vhd.DetachVirtualDisk(diskHandle); detachErr != nil {
		t.Fatalf("failed to detach vhd: %s", detachErr)
	}

	// Looks like a small time gap is required before we query for device interfaces again to get the updated list.
	time.Sleep(1 * time.Second)

	finalInterfaceList, err := getDeviceInterfaceInstancesByClass(ctx, &devClassDiskGUID, false)
	if err != nil {
		t.Fatalf("failed to get initial disk interfaces: %v", err)
	}
	t.Logf("interface list after detaching VHD: %+v\n", finalInterfaceList)

	if len(initialInterfacesList) != len(finalInterfaceList) {
		t.Fatalf("expected interface lists to have same length")
	}
}
