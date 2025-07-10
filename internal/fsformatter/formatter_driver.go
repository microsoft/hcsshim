//go:build windows
// +build windows

package fsformatter

import (
	"context"
	"encoding/binary"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

// This file contains all the supporting structures needed to make
// an ioctl call to RefsFormatter.
const (
	ioctlKernelFormatVolumeFormat = 0x40001000
	// This is used to construct the disk path that refsFormatter
	// understands. `harddisk%d` here refers to the disk number
	// associated with the corresponding lun of the attached
	// scsi device.
	VirtualDevObjectPathFormat                      = "\\device\\harddisk%d\\partition0"
	checksumTypeSha256                              = uint16(4)
	refsChecksumType                                = checksumTypeSha256
	maxSizeOfKernelFormatVolumeFormatRefsParameters = 16 * 8 // 128 bytes
	sizeOfWchar                                     = int(unsafe.Sizeof(uint16(0)))
	kernelFormatVolumeMaxVolumeLabelLength          = uint32(33 * sizeOfWchar)
	kernelFormatVolumeWin32DriverPath               = "\\\\?\\KernelFSFormatter"
	// Allocate large enough buffer for output from fsFormatter
	maxSizeOfOutputBuffer = uint32(512)

	// KERNEL_FORMAT_VOLUME_FORMAT_REFS_PARAMETERS member offsets
	clusterSizeOffset      = 0
	checksumTypeOffset     = 4
	useDataIntegrityOffset = 6
	majorVersionOffset     = 8
	minorVersionOffset     = 10
)

type kernelFormatVolumeFilesystemTypes uint32

const (
	kernelFormatVolumeFilesystemTypeInvalid = kernelFormatVolumeFilesystemTypes(iota)
	kernelFormatVolumeFilesystemTypeRefs    = kernelFormatVolumeFilesystemTypes(1)
	kernelFormatVolumeFilesystemTypeMax     = kernelFormatVolumeFilesystemTypes(2)
)

// We only want to allow refs formatting
func (filesystemType kernelFormatVolumeFilesystemTypes) String() string {
	switch filesystemType {
	case kernelFormatVolumeFilesystemTypeRefs:
		return "KERNEL_FORMAT_VOLUME_FILESYSTEM_TYPE_REFS"
	default:
		return "Unknown"
	}
}

type kernelFormatVolumeFormatInputBufferFlags uint32

const (
	kernelFormatVolumeFormatInputBufferFlagNone        = kernelFormatVolumeFormatInputBufferFlags(0x00000000)
	kernelFormatVolumeFormatInputBufferFlagSuperFloppy = kernelFormatVolumeFormatInputBufferFlags(0x00000001)
)

func (flag kernelFormatVolumeFormatInputBufferFlags) String() string {
	switch flag {
	case kernelFormatVolumeFormatInputBufferFlagNone:
		return "kernelFormatVolumeFormatInputBufferFlagNone"
	case kernelFormatVolumeFormatInputBufferFlagSuperFloppy:
		return "kernelFormatVolumeFormatInputBufferFlagSuperFloppy"
	default:
		return "Unknown"
	}
}

type KernelFormatVolumeFormatRefsParameters struct {
	ClusterSize          uint32
	MetadataChecksumType uint16
	UseDataIntegrity     bool
	MajorVersion         uint16
	MinorVersion         uint16
}

type KernelFormatVolumeFormatFsParameters struct {
	FileSystemType kernelFormatVolumeFilesystemTypes
	// Represents a WCHAR character array
	VolumeLabel [kernelFormatVolumeMaxVolumeLabelLength / uint32(sizeOfWchar)]uint16
	// Length of volume label in bytes
	VolumeLabelLength uint16
	// RefsFormatterParams represents the following union
	/*
	   union {

	       KERNEL_FORMAT_VOLUME_FORMAT_REFS_PARAMETERS RefsParameters;

	       //
	       //  This structure can't grow in size nor change in alignment. 16 ULONGLONGs
	       //  should be more than enough for supporting other filesystems down the
	       //  line. This also serves to enforce 8 byte alignment.
	       //
	       Reserved [16]uint64
	   };
	*/
	RefsFormatterParams [128]byte
}

type KernelFormatVolumeFormatInputBuffer struct {
	Size         uint64
	FsParameters KernelFormatVolumeFormatFsParameters
	Flags        kernelFormatVolumeFormatInputBufferFlags
	Reserved     [4]uint32
	// Size of DiskPathBuffer in bytes
	DiskPathLength uint16
	// DiskPathBuffer holds the disk path. It represents a
	// variable size WCHAR character array
	DiskPathBuffer []uint16
}

type kernelFormatVolumeFormatOutputBufferFlags uint32

const kernelFormatVolumeFormatOutputBufferFlagsNone = kernelFormatVolumeFormatOutputBufferFlags(0x00000000)

func (flag kernelFormatVolumeFormatOutputBufferFlags) String() string {
	switch flag {
	case kernelFormatVolumeFormatOutputBufferFlagsNone:
		return "kernelFormatVolumeFormatOutputBufferFlagsNone"
	default:
		return "Unknown"
	}
}

type KernelFormarVolumeFormatOutputBuffer struct {
	Size     uint32
	Flags    kernelFormatVolumeFormatOutputBufferFlags
	Reserved [4]uint32
	// VolumePathLength holds size of VolumePathBuffer
	// in bytes
	VolumePathLength uint16
	// VolumePathBuffer holds the mounted volume path
	// as returned from refsFormatter. It represents
	// a variable size WCHAR character array
	VolumePathBuffer []uint16
}

// GetVolumePathBufferOffset gets offset to KernelFormarVolumeFormatOutputBuffer{}.VolumePathBuffer
func GetVolumePathBufferOffset() uint32 {
	volPathBufferOffset := uint32(unsafe.Sizeof(KernelFormarVolumeFormatOutputBuffer{}.Size) +
		unsafe.Sizeof(KernelFormarVolumeFormatOutputBuffer{}.Flags) +
		unsafe.Sizeof(KernelFormarVolumeFormatOutputBuffer{}.Reserved) +
		unsafe.Sizeof(KernelFormarVolumeFormatOutputBuffer{}.VolumePathLength))

	return volPathBufferOffset
}

// getInputBufferSize gets the total size needed for input buffer
func getInputBufferSize(wcharDiskPathLength uint16) uint32 {
	bufferSize := uint32(unsafe.Sizeof(KernelFormatVolumeFormatInputBuffer{}.Size)+
		/* This is specifically for the union in KernelFormatVolumeFormatFsParameters */
		unsafe.Offsetof(KernelFormatVolumeFormatFsParameters{}.RefsFormatterParams)+
		maxSizeOfKernelFormatVolumeFormatRefsParameters+
		unsafe.Sizeof(KernelFormatVolumeFormatInputBuffer{}.Flags)+
		unsafe.Sizeof(KernelFormatVolumeFormatInputBuffer{}.Reserved)+
		unsafe.Sizeof(KernelFormatVolumeFormatInputBuffer{}.DiskPathLength)) +
		uint32(wcharDiskPathLength)

	return bufferSize
}

// getInputBufferDiskPathBufferOffset gets offset to KernelFormatVolumeFormatInputBuffer{}.DiskPathBuffer
func getInputBufferDiskPathBufferOffset() uint32 {
	diskPathBufferOffset := uint32(unsafe.Sizeof(KernelFormatVolumeFormatInputBuffer{}.Size) +
		unsafe.Offsetof(KernelFormatVolumeFormatFsParameters{}.RefsFormatterParams) +
		maxSizeOfKernelFormatVolumeFormatRefsParameters +
		unsafe.Sizeof(KernelFormatVolumeFormatInputBuffer{}.Flags) +
		unsafe.Sizeof(KernelFormatVolumeFormatInputBuffer{}.Reserved) +
		unsafe.Sizeof(KernelFormatVolumeFormatInputBuffer{}.DiskPathLength))

	return diskPathBufferOffset
}

// KmFmtCreateFormatOutputBuffer formats an output buffer as expected
// by the fsFormatter driver
func KmFmtCreateFormatOutputBuffer() *KernelFormarVolumeFormatOutputBuffer {
	buf := make([]uint16, maxSizeOfOutputBuffer)
	outputBuffer := (*KernelFormarVolumeFormatOutputBuffer)(unsafe.Pointer(&buf[0]))
	outputBuffer.Size = uint32(maxSizeOfOutputBuffer)

	return outputBuffer
}

func toUTF16(s string) []uint16 {
	return utf16.Encode([]rune(s))
}

// KmFmtCreateFormatInputBuffer formats an input buffer as expected
// by the refsFormatter driver.
// diskPath represents disk path in VirtualDevObjectPathFormat.
func KmFmtCreateFormatInputBuffer(diskPath string) *KernelFormatVolumeFormatInputBuffer {
	refsParametersBuf := make([]byte, unsafe.Sizeof(KernelFormatVolumeFormatRefsParameters{}))
	refsParameters := (*KernelFormatVolumeFormatRefsParameters)(unsafe.Pointer(&refsParametersBuf[0]))

	utf16DiskPath := toUTF16(diskPath)
	wcharDiskPathLength := uint16(len(utf16DiskPath) * sizeOfWchar)

	refsParameters.ClusterSize = 0x1000
	refsParameters.MetadataChecksumType = refsChecksumType
	refsParameters.UseDataIntegrity = true
	refsParameters.MajorVersion = uint16(3)
	refsParameters.MinorVersion = uint16(14)

	bufferSize := getInputBufferSize(wcharDiskPathLength)
	buf := make([]byte, bufferSize)
	inputBuffer := (*KernelFormatVolumeFormatInputBuffer)(unsafe.Pointer(&buf[0]))

	inputBuffer.Size = uint64(bufferSize)
	inputBuffer.Flags = kernelFormatVolumeFormatInputBufferFlagSuperFloppy

	inputBuffer.FsParameters.FileSystemType = kernelFormatVolumeFilesystemTypeRefs
	inputBuffer.FsParameters.VolumeLabelLength = 0
	inputBuffer.FsParameters.VolumeLabel = [33]uint16{}

	// Write KERNEL_FORMAT_VOLUME_FORMAT_REFS_PARAMETERS
	binary.LittleEndian.PutUint32(inputBuffer.FsParameters.RefsFormatterParams[clusterSizeOffset:], refsParameters.ClusterSize)
	binary.LittleEndian.PutUint16(inputBuffer.FsParameters.RefsFormatterParams[checksumTypeOffset:], refsParameters.MetadataChecksumType)
	if refsParameters.UseDataIntegrity {
		inputBuffer.FsParameters.RefsFormatterParams[useDataIntegrityOffset] = 1
	} else {
		inputBuffer.FsParameters.RefsFormatterParams[useDataIntegrityOffset] = 0
	}
	binary.LittleEndian.PutUint16(inputBuffer.FsParameters.RefsFormatterParams[majorVersionOffset:], refsParameters.MajorVersion)
	binary.LittleEndian.PutUint16(inputBuffer.FsParameters.RefsFormatterParams[minorVersionOffset:], refsParameters.MinorVersion)

	// Finally write the diskPathLength and diskPathBuffer with the input disk path
	inputBuffer.DiskPathLength = wcharDiskPathLength
	// DiskBuffer writing
	ptr := unsafe.Add(unsafe.Pointer(inputBuffer), getInputBufferDiskPathBufferOffset())
	// Convert the string to UTF-16 slice
	utf16Array := toUTF16(diskPath)
	diskPathBuf := unsafe.Slice((*uint16)(ptr), len(utf16Array))
	copy(diskPathBuf, utf16Array)

	return inputBuffer
}

// InvokeFsFormatter makes an ioctl call to the fsFormatter driver and returns
// a path to the mountedVolume
func InvokeFsFormatter(ctx context.Context, diskPath string) (string, error) {
	// Prepare input and output buffers as expected by refsFormatter
	inputBuffer := KmFmtCreateFormatInputBuffer(diskPath)
	outputBuffer := KmFmtCreateFormatOutputBuffer()

	utf16DriverPath, _ := windows.UTF16PtrFromString(kernelFormatVolumeWin32DriverPath)
	deviceHandle, err := windows.CreateFile(utf16DriverPath,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		0,
		nil,
		windows.OPEN_EXISTING,
		0,
		0)
	if err != nil {
		return "", errors.Wrap(err, "failed to get handle to refsFormatter driver")
	}
	defer windows.Close(deviceHandle)

	// Ioctl to fsFormatter driver
	var bytesReturned uint32
	if err := windows.DeviceIoControl(
		deviceHandle,
		ioctlKernelFormatVolumeFormat,
		(*byte)(unsafe.Pointer(inputBuffer)),
		uint32(inputBuffer.Size),
		(*byte)(unsafe.Pointer(outputBuffer)),
		outputBuffer.Size,
		&bytesReturned,
		nil,
	); err != nil {
		return "", errors.Wrap(err, "ioctl to refsFormatter driver failed")
	}

	// Read the returned volume path from the corresponding offset in outputBuffer
	ptr := unsafe.Pointer(uintptr(unsafe.Pointer(outputBuffer)) + uintptr(GetVolumePathBufferOffset()))
	utf16Data := unsafe.Slice((*uint16)(ptr), outputBuffer.VolumePathLength/2)
	mountedVolumePath := syscall.UTF16ToString(utf16Data)
	return mountedVolumePath, err
}
