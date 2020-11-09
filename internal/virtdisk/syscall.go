package virtdisk

import (
	"context"
	"fmt"

	"golang.org/x/sys/windows"
)

// OpenVirtualDisk opens a virtual disk in either vhd or vhdx format and returns a handle to the
// disk.
func OpenVirtualDisk(ctx context.Context, vhdPath string, virtualDiskAccessMask VirtualDiskAccessMask, openVirtualDiskFlags OpenVirtualDiskFlag, parameters *OpenVirtualDiskParameters) (_ windows.Handle, err error) {
	var handle windows.Handle
	if parameters.Version != 2 {
		return handle, fmt.Errorf("only version 2 VHDs are supported, found version: %d", parameters.Version)
	}

	err = openVirtualDisk(&vhdxVirtualStorageType, vhdPath, uint32(virtualDiskAccessMask), uint32(openVirtualDiskFlags), parameters, &handle)
	if err != nil {
		return handle, fmt.Errorf("failed to open virtual disk: %s", err)
	}
	return handle, nil
}

// CreateVirtualDisk creates a virtual harddisk and returns a handle to the disk.
func CreateVirtualDisk(ctx context.Context, path string, virtualDiskAccessMask VirtualDiskAccessMask, createVirtualDiskFlags CreateVirtualDiskFlag, parameters *CreateVirtualDiskParameters) (_ windows.Handle, err error) {
	var handle windows.Handle
	if parameters.Version != 2 {
		return handle, fmt.Errorf("only version 2 VHDs are supported, found version: %d", parameters.Version)
	}

	err = createVirtualDisk(&vhdxVirtualStorageType, path, uint32(virtualDiskAccessMask), 0, uint32(createVirtualDiskFlags), 0, parameters, nil, &handle)
	if err != nil {
		return handle, fmt.Errorf("failed to create virtual disk: %s", err)
	}
	return handle, nil
}

// GetVirtualDiskPhysicalPath takes a handle to a virtual hard disk and returns the physical
// path of the disk on the machine.
func GetVirtualDiskPhysicalPath(ctx context.Context, handle windows.Handle) (_ string, err error) {
	var (
		diskPathSizeInBytes uint32 = 256 * 2 // max path length 256 wide chars
		diskPhysicalPathBuf [256]uint16
	)

	err = getVirtualDiskPhysicalPath(handle, &diskPathSizeInBytes, &diskPhysicalPathBuf[0])
	if err != nil {
		return "", fmt.Errorf("failed to get disk physical path: %s", err)
	}
	return windows.UTF16ToString(diskPhysicalPathBuf[:]), nil
}

// AttachVirtualDisk attaches a virtual hard disk for use.
func AttachVirtualDisk(ctx context.Context, handle windows.Handle, attachVirtualDiskFlag AttachVirtualDiskFlag, parameters *AttachVirtualDiskParameters) (err error) {
	if parameters.Version != 2 {
		return fmt.Errorf("only version 2 VHDs are supported, found version: %d", parameters.Version)
	}

	err = attachVirtualDisk(handle, 0, uint32(attachVirtualDiskFlag), 0, parameters, nil)
	if err != nil {
		return fmt.Errorf("failed to attach virtual disk: %s", err)
	}
	return nil
}

// DetachVirtualDisk detaches a virtual hard disk.
func DetachVirtualDisk(ctx context.Context, handle windows.Handle) (err error) {
	err = detachVirtualDisk(handle, 0, 0)
	if err != nil {
		return fmt.Errorf("failed to detach virtual disk: %s", err)
	}
	return nil
}
