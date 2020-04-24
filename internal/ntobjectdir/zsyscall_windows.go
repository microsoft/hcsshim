// Code generated mksyscall_windows.exe DO NOT EDIT

package ntobjectdir

import (
	"syscall"
	"unsafe"

	"github.com/Microsoft/hcsshim/internal/ntconstants"
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
	modntdll = windows.NewLazySystemDLL("ntdll.dll")

	procNtOpenDirectoryObject  = modntdll.NewProc("NtOpenDirectoryObject")
	procNtQueryDirectoryObject = modntdll.NewProc("NtQueryDirectoryObject")
)

func ntOpenDirectoryObject(handle *uintptr, accessMask uint32, oa *ntconstants.ObjectAttributes) (status uint32) {
	r0, _, _ := syscall.Syscall(procNtOpenDirectoryObject.Addr(), 3, uintptr(unsafe.Pointer(handle)), uintptr(accessMask), uintptr(unsafe.Pointer(oa)))
	status = uint32(r0)
	return
}

func ntQueryDirectoryObject(handle uintptr, buffer *byte, length uint32, singleEntry bool, restartScan bool, context *uint32, returnLength *uint32) (status uint32) {
	var _p0 uint32
	if singleEntry {
		_p0 = 1
	} else {
		_p0 = 0
	}
	var _p1 uint32
	if restartScan {
		_p1 = 1
	} else {
		_p1 = 0
	}
	r0, _, _ := syscall.Syscall9(procNtQueryDirectoryObject.Addr(), 7, uintptr(handle), uintptr(unsafe.Pointer(buffer)), uintptr(length), uintptr(_p0), uintptr(_p1), uintptr(unsafe.Pointer(context)), uintptr(unsafe.Pointer(returnLength)), 0, 0)
	status = uint32(r0)
	return
}
