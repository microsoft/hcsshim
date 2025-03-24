//go:build windows

package windevice

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Microsoft/go-winio/vhd"
	"github.com/Microsoft/hcsshim/internal/fsformatter"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
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

const (
	diskSizeInGB           = 35
	defaultVHDxBlockSizeMB = 1
)

// startFsformatterDriver checks if fsformatter driver
// has already been loaded and starts the service.
// Returns syscall.ERROR_FILE_NOT_FOUND if driver
// is not loaded.
func startFsformatterDriver() error {
	m, err := mgr.Connect()
	if err != nil {
		log.Fatalf("Failed to connect to service manager: %v", err)
	}
	defer func() {
		_ = m.Disconnect()
	}()

	// Ensure fsformatter driver is loaded by querying for the service.
	serviceName := "kernelfsformatter"
	s, err := m.OpenService(serviceName)
	if err != nil {
		return syscall.ERROR_FILE_NOT_FOUND
	}
	defer s.Close()

	_, err = s.Query()
	if err != nil {
		return syscall.ERROR_FILE_NOT_FOUND
	}

	err = s.Start()
	if err != nil && !strings.Contains(err.Error(), "An instance of the service is already running") {
		return fmt.Errorf("Failed to start service: %w", err)
	}

	return nil
}

func TestFormatVHDXToReFS(t *testing.T) {
	// Ensure fsformatter service is loaded and started
	err := startFsformatterDriver()
	if err != nil {
		// if driver is not loaded already, skip.
		if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
			t.Skip()
		}
		t.Fatalf("Failed to start service: %v", err)
	}

	ctx := context.Background()
	initialInterfacesList, err := getDeviceInterfaceInstancesByClass(ctx, &devClassDiskGUID, false)
	if err != nil {
		t.Fatalf("failed to get initial disk interfaces: %v", err)
	}
	// We expect to see only one device initially
	if len(initialInterfacesList) != 1 {
		t.Fatalf("unexpected number of initial disk interfaces: %v", len(initialInterfacesList))
	}
	t.Logf("initial interface list: %+v\n", initialInterfacesList)

	// Create a fixed VHDX of 31 GB (refs needs size to be > 30GB)
	tempDir := t.TempDir()
	vhdxPath := filepath.Join(tempDir, "test.vhdx")
	if err := vhd.CreateVhdx(vhdxPath, diskSizeInGB, defaultVHDxBlockSizeMB); err != nil {
		t.Fatalf("failed to create VHDX: %v", err)
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

	err = vhd.AttachVirtualDisk(diskHandle, vhd.AttachVirtualDiskFlagNone, &vhd.AttachVirtualDiskParameters{Version: 1})
	if err != nil {
		t.Fatalf("failed to attach VHD: %s", err)
	}
	t.Cleanup(func() {
		if detachErr := vhd.DetachVirtualDisk(diskHandle); detachErr != nil {
			t.Logf("failed to detach vhd: %s", detachErr)
		}
	})
	// Disks might take time to show up. Add a small delay
	time.Sleep(1 * time.Second)

	interfaceListAfterVHDAttach, err := getDeviceInterfaceInstancesByClass(ctx, &devClassDiskGUID, false)
	if err != nil {
		t.Fatalf("failed to get initial disk interfaces: %v", err)
	}
	t.Logf("interface list after attaching VHD: %+v\n", interfaceListAfterVHDAttach)

	if len(initialInterfacesList) != (len(interfaceListAfterVHDAttach) - 1) {
		t.Fatalf("expected to find exactly 1 new interface in the returned interfaces list")
	}

	for _, iPath := range interfaceListAfterVHDAttach {
		// Take only the newly attached vhdx
		if iPath == initialInterfacesList[0] {
			continue
		}
		utf16Path, err := windows.UTF16PtrFromString(iPath)
		if err != nil {
			t.Fatalf("failed to convert interface path [%s] to utf16: %v", iPath, err)
		}

		handle, err := windows.CreateFile(utf16Path, windows.GENERIC_READ|windows.GENERIC_WRITE,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
			nil, windows.OPEN_EXISTING, 0, 0)
		if err != nil {
			t.Fatalf("failed to get handle to interface path [%s]: %v", iPath, err)
		}
		defer windows.Close(handle)

		deviceNumber, err := getStorageDeviceNumber(ctx, handle)
		if err != nil {
			t.Fatalf("failed to get physical device number: %v", err)
		}
		diskPath := fmt.Sprintf(fsformatter.VirtualDevObjectPathFormat, deviceNumber.DeviceNumber)
		t.Logf("diskPath %v", diskPath)

		// Invoke refs formatter and ensure it passes.
		mountedVolumePath, err := fsformatter.InvokeFsFormatter(ctx, diskPath)
		if err != nil {
			t.Fatalf("invoking refsFormatter failed: %v", err)
		}
		t.Logf("mountedVolumePath %v", mountedVolumePath)
	}
}
