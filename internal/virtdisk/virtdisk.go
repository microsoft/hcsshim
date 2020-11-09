// Package virtdisk is a wrapper around the APIs exported by virtdisk.dll
package virtdisk

import (
	"context"
	"fmt"

	"github.com/Microsoft/go-winio/pkg/guid"
	"golang.org/x/sys/windows"
)

//go:generate go run ../../mksyscall_windows.go -output zsyscall_windows.go virtdisk.go

// CreateDiffVhd creates a differencing virtual disk.
func CreateDiffVhd(ctx context.Context, diffVhdPath, baseVhdPath string) error {
	// Setting `ParentPath` is how to signal to create a differencing disk.
	createParams := &CreateVirtualDiskParameters{
		Version: 2,
		Version2: CreateVersion2{
			ParentPath:       windows.StringToUTF16Ptr(baseVhdPath),
			BlockSizeInBytes: 1 * 1024 * 1024,
			OpenFlags:        uint32(OpenVirtualDiskFlagCachedIO),
		},
	}

	vhdHandle, err := CreateVirtualDisk(ctx, diffVhdPath, VirtualDiskAccessFlagNone, CreateVirtualDiskFlagNone, createParams)
	if err != nil {
		return fmt.Errorf("failed to create differencing vhd: %s", err)
	}
	if err := windows.CloseHandle(vhdHandle); err != nil {
		return fmt.Errorf("failed to close differencing vhd handle: %s", err)
	}
	return nil
}

type (
	CreateVirtualDiskFlag uint32
	OpenVirtualDiskFlag   uint32
	AttachVirtualDiskFlag uint32
	DetachVirtualDiskFlag uint32
	VirtualDiskAccessMask uint32
)

type VirtualStorageType struct {
	DeviceID uint32
	VendorID guid.GUID
}

type CreateVersion2 struct {
	UniqueID                 guid.GUID
	MaximumSize              uint64
	BlockSizeInBytes         uint32
	SectorSizeInBytes        uint32
	PhysicalSectorSizeInByte uint32
	ParentPath               *uint16 // string
	SourcePath               *uint16 // string
	OpenFlags                uint32
	ParentVirtualStorageType VirtualStorageType
	SourceVirtualStorageType VirtualStorageType
	ResiliencyGUID           guid.GUID
}

type CreateVirtualDiskParameters struct {
	Version  uint32 // Must always be set to 2
	Version2 CreateVersion2
}

type OpenVersion2 struct {
	GetInfoOnly    bool
	ReadOnly       bool
	ResiliencyGUID guid.GUID
}

type OpenVirtualDiskParameters struct {
	Version  uint32 // Must always be set to 2
	Version2 OpenVersion2
}

type AttachVersion2 struct {
	RestrictedOffset uint64
	RestrictedLength uint64
}

type AttachVirtualDiskParameters struct {
	Version  uint32 // Must always be set to 2
	Version2 AttachVersion2
}

const (
	VIRTUAL_STORAGE_TYPE_DEVICE_VHDX = 0x3

	VirtualDiskAccessFlagNone     VirtualDiskAccessMask = 0x00000000
	VirtualDiskAccessFlagAttachRO VirtualDiskAccessMask = 0x00010000
	VirtualDiskAccessFlagAttachRW VirtualDiskAccessMask = 0x00020000
	VirtualDiskAccessFlagDetach   VirtualDiskAccessMask = 0x00040000
	VirtualDiskAccessFlagGetInfo  VirtualDiskAccessMask = 0x00080000
	VirtualDiskAccessFlagCreate   VirtualDiskAccessMask = 0x00100000
	VirtualDiskAccessFlagMetaOps  VirtualDiskAccessMask = 0x00200000
	VirtualDiskAccessFlagRead     VirtualDiskAccessMask = 0x000d0000
	VirtualDiskAccessFlagAll      VirtualDiskAccessMask = 0x003f0000
	VirtualDiskAccessFlagWritable VirtualDiskAccessMask = 0x00320000

	CreateVirtualDiskFlagNone                              CreateVirtualDiskFlag = 0x0
	CreateVirtualDiskFlagFullPhysicalAllocation            CreateVirtualDiskFlag = 0x1
	CreateVirtualDiskFlagPreventWritesToSourceDisk         CreateVirtualDiskFlag = 0x2
	CreateVirtualDiskFlagDoNotCopyMetadataFromParent       CreateVirtualDiskFlag = 0x4
	CreateVirtualDiskFlagCreateBackingStorage              CreateVirtualDiskFlag = 0x8
	CreateVirtualDiskFlagUseChangeTrackingSourceLimit      CreateVirtualDiskFlag = 0x10
	CreateVirtualDiskFlagPreserveParentChangeTrackingState CreateVirtualDiskFlag = 0x20
	CreateVirtualDiskFlagVhdSetUseOriginalBackingStorage   CreateVirtualDiskFlag = 0x40
	CreateVirtualDiskFlagSparseFile                        CreateVirtualDiskFlag = 0x80
	CreateVirtualDiskFlagPmemCompatible                    CreateVirtualDiskFlag = 0x100
	CreateVirtualDiskFlagSupportCompressedVolumes          CreateVirtualDiskFlag = 0x200

	OpenVirtualDiskFlagNone                        OpenVirtualDiskFlag = 0x00000000
	OpenVirtualDiskFlagNoParents                   OpenVirtualDiskFlag = 0x00000001
	OpenVirtualDiskFlagBlankFile                   OpenVirtualDiskFlag = 0x00000002
	OpenVirtualDiskFlagBootDrive                   OpenVirtualDiskFlag = 0x00000004
	OpenVirtualDiskFlagCachedIO                    OpenVirtualDiskFlag = 0x00000008
	OpenVirtualDiskFlagCustomDiffChain             OpenVirtualDiskFlag = 0x00000010
	OpenVirtualDiskFlagParentCachedIO              OpenVirtualDiskFlag = 0x00000020
	OpenVirtualDiskFlagVhdsetFileOnly              OpenVirtualDiskFlag = 0x00000040
	OpenVirtualDiskFlagIgnoreRelativeParentLocator OpenVirtualDiskFlag = 0x00000080
	OpenVirtualDiskFlagNoWriteHardening            OpenVirtualDiskFlag = 0x00000100
	OpenVirtualDiskFlagSupportCompressedVolumes    OpenVirtualDiskFlag = 0x00000200

	AttachVirtualDiskFlagNone                          AttachVirtualDiskFlag = 0x00000000
	AttachVirtualDiskFlagReadOnly                      AttachVirtualDiskFlag = 0x00000001
	AttachVirtualDiskFlagNoDriveLetter                 AttachVirtualDiskFlag = 0x00000002
	AttachVirtualDiskFlagPermanentLifetime             AttachVirtualDiskFlag = 0x00000004
	AttachVirtualDiskFlagNoLocalHost                   AttachVirtualDiskFlag = 0x00000008
	AttachVirtualDiskFlagNoSecurityDescriptor          AttachVirtualDiskFlag = 0x00000010
	AttachVirtualDiskFlagBypassDefaultEncryptionPolicy AttachVirtualDiskFlag = 0x00000020
	AttachVirtualDiskFlagNonPnp                        AttachVirtualDiskFlag = 0x00000040
	AttachVirtualDiskFlagRestrictedRange               AttachVirtualDiskFlag = 0x00000080
	AttachVirtualDiskFlagSinglePartition               AttachVirtualDiskFlag = 0x00000100
	AttachVirtualDiskFlagRegisterVolume                AttachVirtualDiskFlag = 0x00000200

	DetachVirtualDiskFlagNone DetachVirtualDiskFlag = 0x0
)

var (
	VIRTUAL_STORAGE_TYPE_VENDOR_MICROSOFT = guid.GUID{
		Data1: 0xec984aec,
		Data2: 0xa0f9,
		Data3: 0x47e9,
		Data4: [8]byte{0x90, 0x1f, 0x71, 0x41, 0x5a, 0x66, 0x34, 0x5b},
	}

	vhdxVirtualStorageType = VirtualStorageType{
		DeviceID: VIRTUAL_STORAGE_TYPE_DEVICE_VHDX,
		VendorID: VIRTUAL_STORAGE_TYPE_VENDOR_MICROSOFT,
	}
)

//sys createVirtualDisk(virtualStorageType *VirtualStorageType, path string, virtualDiskAccessMask uint32, securityDescriptor uintptr, createVirtualDiskFlags uint32, providerSpecificFlags uint32, parameters *CreateVirtualDiskParameters, overlapped *windows.Overlapped, handle *windows.Handle) (err error) [failretval != 0] = virtdisk.CreateVirtualDisk
//sys openVirtualDisk(virtualStorageType *VirtualStorageType, path string, virtualDiskAccessMask uint32, openVirtualDiskFlags uint32, parameters *OpenVirtualDiskParameters, handle *windows.Handle) (err error) [failretval != 0] = virtdisk.OpenVirtualDisk
//sys attachVirtualDisk(handle windows.Handle, securityDescriptor uintptr, attachVirtualDiskFlag uint32, providerSpecificFlags uint32, parameters *AttachVirtualDiskParameters, overlapped *windows.Overlapped) (err error) [failretval != 0] = virtdisk.AttachVirtualDisk
//sys detachVirtualDisk(handle windows.Handle, detachVirtualDiskFlags uint32, providerSpecificFlags uint32) (err error) [failretval != 0] = virtdisk.DetachVirtualDisk
//sys getVirtualDiskPhysicalPath(handle windows.Handle, diskPathSizeInBytes *uint32, buffer *uint16) (err error) [failretval != 0] = virtdisk.GetVirtualDiskPhysicalPath
