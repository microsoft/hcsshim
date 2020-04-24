// Code generated mksyscall_windows.exe DO NOT EDIT

package safefile

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

	procNtCreateFile         = modntdll.NewProc("NtCreateFile")
	procNtSetInformationFile = modntdll.NewProc("NtSetInformationFile")
)

func ntCreateFile(handle *uintptr, accessMask uint32, oa *ntconstants.ObjectAttributes, iosb *ntconstants.IOStatusBlock, allocationSize *uint64, fileAttributes uint32, shareAccess uint32, createDisposition uint32, createOptions uint32, eaBuffer *byte, eaLength uint32) (status uint32) {
	r0, _, _ := syscall.Syscall12(procNtCreateFile.Addr(), 11, uintptr(unsafe.Pointer(handle)), uintptr(accessMask), uintptr(unsafe.Pointer(oa)), uintptr(unsafe.Pointer(iosb)), uintptr(unsafe.Pointer(allocationSize)), uintptr(fileAttributes), uintptr(shareAccess), uintptr(createDisposition), uintptr(createOptions), uintptr(unsafe.Pointer(eaBuffer)), uintptr(eaLength), 0)
	status = uint32(r0)
	return
}

func ntSetInformationFile(handle uintptr, iosb *ntconstants.IOStatusBlock, information uintptr, length uint32, class uint32) (status uint32) {
	r0, _, _ := syscall.Syscall6(procNtSetInformationFile.Addr(), 5, uintptr(handle), uintptr(unsafe.Pointer(iosb)), uintptr(information), uintptr(length), uintptr(class), 0)
	status = uint32(r0)
	return
}
