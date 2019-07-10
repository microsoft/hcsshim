// Code generated mksyscall_windows.exe DO NOT EDIT

package cimfs

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var _ unsafe.Pointer

// Do the interface allocations only once for common
// Errno values.
const (
	errnoERROR_IO_PENDING = 997
)

var (
	errERROR_IO_PENDING error = syscall.Errno(errnoERROR_IO_PENDING)
)

// errnoErr returns common boxed Errno values, to prevent
// allocations at runtime.
func errnoErr(e syscall.Errno) error {
	switch e {
	case 0:
		return nil
	case errnoERROR_IO_PENDING:
		return errERROR_IO_PENDING
	}
	// TODO: add more here, after collecting data on the common
	// error values see on Windows. (perhaps when running
	// all.bat?)
	return e
}

var (
	modcimfs = windows.NewLazySystemDLL("cimfs.dll")

	procCimMountImage     = modcimfs.NewProc("CimMountImage")
	procCimDismountImage  = modcimfs.NewProc("CimDismountImage")
	procCimCreateImage    = modcimfs.NewProc("CimCreateImage")
	procCimCloseImage     = modcimfs.NewProc("CimCloseImage")
	procCimCreateFile     = modcimfs.NewProc("CimCreateFile")
	procCimCloseStream    = modcimfs.NewProc("CimCloseStream")
	procCimWriteStream    = modcimfs.NewProc("CimWriteStream")
	procCimDeleteFile     = modcimfs.NewProc("CimDeleteFile")
	procCimCreateHardLink = modcimfs.NewProc("CimCreateHardLink")
)

func cimMountImage(cimPath string, volumeID *g) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(cimPath)
	if hr != nil {
		return
	}
	return _cimMountImage(_p0, volumeID)
}

func _cimMountImage(cimPath *uint16, volumeID *g) (hr error) {
	r0, _, _ := syscall.Syscall(procCimMountImage.Addr(), 2, uintptr(unsafe.Pointer(cimPath)), uintptr(unsafe.Pointer(volumeID)), 0)
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}

func cimDismountImage(volumeID *g) (hr error) {
	r0, _, _ := syscall.Syscall(procCimDismountImage.Addr(), 1, uintptr(unsafe.Pointer(volumeID)), 0, 0)
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}

func cimCreateImage(cimPath string, flags uint32, cimFSHandle *imageHandle) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(cimPath)
	if hr != nil {
		return
	}
	return _cimCreateImage(_p0, flags, cimFSHandle)
}

func _cimCreateImage(cimPath *uint16, flags uint32, cimFSHandle *imageHandle) (hr error) {
	r0, _, _ := syscall.Syscall(procCimCreateImage.Addr(), 3, uintptr(unsafe.Pointer(cimPath)), uintptr(flags), uintptr(unsafe.Pointer(cimFSHandle)))
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}

func cimCloseImage(cimFSHandle imageHandle, cimPath string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(cimPath)
	if hr != nil {
		return
	}
	return _cimCloseImage(cimFSHandle, _p0)
}

func _cimCloseImage(cimFSHandle imageHandle, cimPath *uint16) (hr error) {
	r0, _, _ := syscall.Syscall(procCimCloseImage.Addr(), 2, uintptr(cimFSHandle), uintptr(unsafe.Pointer(cimPath)), 0)
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}

func cimCreateFile(cimFSHandle imageHandle, path string, file *fileInfoInternal, flags uint32, cimStreamHandle *streamHandle) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(path)
	if hr != nil {
		return
	}
	return _cimCreateFile(cimFSHandle, _p0, file, flags, cimStreamHandle)
}

func _cimCreateFile(cimFSHandle imageHandle, path *uint16, file *fileInfoInternal, flags uint32, cimStreamHandle *streamHandle) (hr error) {
	r0, _, _ := syscall.Syscall6(procCimCreateFile.Addr(), 5, uintptr(cimFSHandle), uintptr(unsafe.Pointer(path)), uintptr(unsafe.Pointer(file)), uintptr(flags), uintptr(unsafe.Pointer(cimStreamHandle)), 0)
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}

func cimCloseStream(cimStreamHandle streamHandle) (hr error) {
	r0, _, _ := syscall.Syscall(procCimCloseStream.Addr(), 1, uintptr(cimStreamHandle), 0, 0)
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}

func cimWriteStream(cimStreamHandle streamHandle, buffer uintptr, bufferSize uint32) (hr error) {
	r0, _, _ := syscall.Syscall(procCimWriteStream.Addr(), 3, uintptr(cimStreamHandle), uintptr(buffer), uintptr(bufferSize))
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}

func cimDeleteFile(cimFSHandle imageHandle, path string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(path)
	if hr != nil {
		return
	}
	return _cimDeleteFile(cimFSHandle, _p0)
}

func _cimDeleteFile(cimFSHandle imageHandle, path *uint16) (hr error) {
	r0, _, _ := syscall.Syscall(procCimDeleteFile.Addr(), 2, uintptr(cimFSHandle), uintptr(unsafe.Pointer(path)), 0)
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}

func cimCreateHardLink(cimFSHandle imageHandle, existingPath string, targetPath string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(existingPath)
	if hr != nil {
		return
	}
	var _p1 *uint16
	_p1, hr = syscall.UTF16PtrFromString(targetPath)
	if hr != nil {
		return
	}
	return _cimCreateHardLink(cimFSHandle, _p0, _p1)
}

func _cimCreateHardLink(cimFSHandle imageHandle, existingPath *uint16, targetPath *uint16) (hr error) {
	r0, _, _ := syscall.Syscall(procCimCreateHardLink.Addr(), 3, uintptr(cimFSHandle), uintptr(unsafe.Pointer(existingPath)), uintptr(unsafe.Pointer(targetPath)))
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}
