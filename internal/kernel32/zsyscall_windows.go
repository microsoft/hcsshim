// Code generated mksyscall_windows.exe DO NOT EDIT

package kernel32

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
	modkernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procLocalAlloc = modkernel32.NewProc("LocalAlloc")
	procLocalFree  = modkernel32.NewProc("LocalFree")
)

func LocalAlloc(flags uint32, size int) (ptr uintptr) {
	r0, _, _ := syscall.Syscall(procLocalAlloc.Addr(), 2, uintptr(flags), uintptr(size), 0)
	ptr = uintptr(r0)
	return
}

func LocalFree(ptr uintptr) {
	syscall.Syscall(procLocalFree.Addr(), 1, uintptr(ptr), 0, 0)
	return
}
