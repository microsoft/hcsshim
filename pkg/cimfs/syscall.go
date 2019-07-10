package cimfs

import (
	"github.com/Microsoft/go-winio/pkg/guid"
)

type g = guid.GUID

//go:generate go run ../../mksyscall_windows.go -output zsyscall_windows.go syscall.go

//sys cimMountImage(cimPath string, volumeID *g) (hr error) = cimfs.CimMountImage
//sys cimDismountImage(volumeID *g) (hr error) = cimfs.CimDismountImage

//sys cimCreateImage(cimPath string, flags uint32, cimFSHandle *imageHandle) (hr error) = cimfs.CimCreateImage
//sys cimCloseImage(cimFSHandle imageHandle, cimPath string) (hr error) = cimfs.CimCloseImage

//sys cimCreateFile(cimFSHandle imageHandle, path string, file *fileInfoInternal, flags uint32, cimStreamHandle *streamHandle) (hr error) = cimfs.CimCreateFile
//sys cimCloseStream(cimStreamHandle streamHandle) (hr error) = cimfs.CimCloseStream
//sys cimWriteStream(cimStreamHandle streamHandle, buffer uintptr, bufferSize uint32) (hr error) = cimfs.CimWriteStream
//sys cimDeleteFile(cimFSHandle imageHandle, path string) (hr error) = cimfs.CimDeleteFile
//sys cimCreateHardLink(cimFSHandle imageHandle, existingPath string, targetPath string) (hr error) = cimfs.CimCreateHardLink
