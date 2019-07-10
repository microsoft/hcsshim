package cimfs

import (
	"unsafe"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

// FileInfo represents the metadata for a single file in the image.
type FileInfo struct {
	Size int64

	CreationTime   windows.Filetime
	LastWriteTime  windows.Filetime
	ChangeTime     windows.Filetime
	LastAccessTime windows.Filetime

	Attributes uint32

	SecurityDescriptor []byte
	ReparseData        []byte
	EAs                []winio.ExtendedAttribute
}

type fileInfoInternal struct {
	Size     uint32
	FileSize int64

	CreationTime   windows.Filetime
	LastWriteTime  windows.Filetime
	ChangeTime     windows.Filetime
	LastAccessTime windows.Filetime

	Attributes uint32

	SecurityDescriptorBuffer unsafe.Pointer
	SecurityDescriptorSize   uint32

	ReparseDataBuffer unsafe.Pointer
	ReparseDataSize   uint32

	EAs     unsafe.Pointer
	EACount uint32
}

type eaInternal struct {
	Name       unsafe.Pointer
	NameLength uint32

	Flags uint8

	Buffer     unsafe.Pointer
	BufferSize uint32
}

type imageHandle uintptr
type streamHandle uintptr

// Image represents a single CimFS filesystem image. On disk, the image is
// composed of a filesystem file and several object ID and region files.
type Image struct {
	handle       imageHandle
	activeStream streamHandle
}

// Open opens an existing CimFS image, or creates one if it doesn't exist.
func Open(path string) (*Image, error) {
	var handle imageHandle
	if err := cimCreateImage(path, 0, &handle); err != nil {
		return nil, err
	}

	return &Image{handle: handle}, nil
}

// AddFile adds an entry for a file to the image. The file is added at the
// specified path. After calling this function, the file is set as the active
// stream for the image, so data can be written by calling `Write`.
func (cim *Image) AddFile(path string, info *FileInfo) error {
	infoInternal := &fileInfoInternal{
		FileSize:       info.Size,
		CreationTime:   info.CreationTime,
		LastWriteTime:  info.LastWriteTime,
		ChangeTime:     info.ChangeTime,
		LastAccessTime: info.LastAccessTime,
		Attributes:     info.Attributes,
	}

	if len(info.SecurityDescriptor) > 0 {
		infoInternal.SecurityDescriptorBuffer = unsafe.Pointer(&info.SecurityDescriptor[0])
		infoInternal.SecurityDescriptorSize = uint32(len(info.SecurityDescriptor))
	}

	if len(info.ReparseData) > 0 {
		infoInternal.ReparseDataBuffer = unsafe.Pointer(&info.ReparseData[0])
		infoInternal.ReparseDataSize = uint32(len(info.ReparseData))
	}

	easInternal := []eaInternal{}
	for _, ea := range info.EAs {
		nameBytes, err := windows.BytePtrFromString(ea.Name)
		if err != nil {
			return errors.Wrap(err, "failed to convert EA name to bytes")
		}
		eaInternal := eaInternal{
			Name:       unsafe.Pointer(nameBytes),
			NameLength: uint32(len(ea.Name)),
			Flags:      ea.Flags,
		}

		if len(ea.Value) > 0 {
			eaInternal.Buffer = unsafe.Pointer(&ea.Value[0])
			eaInternal.BufferSize = uint32(len(ea.Value))
		}

		easInternal = append(easInternal, eaInternal)
	}
	if len(easInternal) > 0 {
		infoInternal.EAs = unsafe.Pointer(&easInternal[0])
		infoInternal.EACount = uint32(len(easInternal))
	}

	return cimCreateFile(cim.handle, path, infoInternal, 0, &cim.activeStream)
}

// Write writes bytes to the active stream.
func (cim *Image) Write(p []byte) (int, error) {
	if cim.activeStream == 0 {
		return 0, errors.New("No active stream")
	}

	// TODO: pass p directly to gen'd syscall
	err := cimWriteStream(cim.activeStream, uintptr(unsafe.Pointer(&p[0])), uint32(len(p)))
	if err != nil {
		return 0, err
	}

	return len(p), nil
}

// CloseStream closes the active stream.
func (cim *Image) CloseStream() error {
	if cim.activeStream == 0 {
		return errors.New("No active stream")
	}

	return cimCloseStream(cim.activeStream)
}

// Close closes the CimFS image.
func (cim *Image) Close(path string) error {
	return cimCloseImage(cim.handle, path)
}

// RemoveFile deletes the file at `path` from the image.
func (cim *Image) RemoveFile(path string) error {
	return cimDeleteFile(cim.handle, path)
}

// AddLink adds a hard link from `oldname` to `newname` in the image.
func (cim *Image) AddLink(oldname string, newname string) error {
	return cimCreateHardLink(cim.handle, oldname, newname)
}

// MountImage mounts the CimFS image at `path` to the volume `volumeGUID`.
func MountImage(path string, volumeGUID guid.GUID) error {
	return cimMountImage(path, &volumeGUID)
}

// UnmountImage unmounts the CimFS volume `volumeGUID`.
func UnmountImage(volumeGUID guid.GUID) error {
	return cimDismountImage(&volumeGUID)
}
