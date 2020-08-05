// Code generated mksyscall_windows.exe DO NOT EDIT

package winapi

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
	modadvapi32 = windows.NewLazySystemDLL("advapi32.dll")

	procIsProcessInJob                       = modkernel32.NewProc("IsProcessInJob")
	procQueryInformationJobObject            = modkernel32.NewProc("QueryInformationJobObject")
	procOpenJobObjectW                       = modkernel32.NewProc("OpenJobObjectW")
	procSetIoRateControlInformationJobObject = modkernel32.NewProc("SetIoRateControlInformationJobObject")
	procSearchPathW                          = modkernel32.NewProc("SearchPathW")
	procLogonUserW                           = modadvapi32.NewProc("LogonUserW")
	procRtlMoveMemory                        = modkernel32.NewProc("RtlMoveMemory")
	procGetActiveProcessorCount              = modkernel32.NewProc("GetActiveProcessorCount")
)

func IsProcessInJob(procHandle windows.Handle, jobHandle uintptr, result *bool) (err error) {
	r1, _, e1 := syscall.Syscall(procIsProcessInJob.Addr(), 3, uintptr(procHandle), uintptr(jobHandle), uintptr(unsafe.Pointer(result)))
	if r1 == 0 {
		if e1 != 0 {
			err = errnoErr(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func QueryInformationJobObject(jobHandle windows.Handle, infoClass uint32, jobObjectInfo uintptr, jobObjectInformationLength uint32, lpReturnLength uint32) (err error) {
	r1, _, e1 := syscall.Syscall6(procQueryInformationJobObject.Addr(), 5, uintptr(jobHandle), uintptr(infoClass), uintptr(jobObjectInfo), uintptr(jobObjectInformationLength), uintptr(lpReturnLength), 0)
	if r1 == 0 {
		if e1 != 0 {
			err = errnoErr(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func OpenJobObject(desiredAccess uint32, inheritHandle uint32, lpName *uint16) (handle windows.Handle, err error) {
	r0, _, e1 := syscall.Syscall(procOpenJobObjectW.Addr(), 3, uintptr(desiredAccess), uintptr(inheritHandle), uintptr(unsafe.Pointer(lpName)))
	handle = windows.Handle(r0)
	if handle == 0 {
		if e1 != 0 {
			err = errnoErr(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func SetIoRateControlInformationJobObject(jobHandle windows.Handle, ioRateControlInfo *JOBOBJECT_IO_RATE_CONTROL_INFORMATION) (ret uint32, err error) {
	r0, _, e1 := syscall.Syscall(procSetIoRateControlInformationJobObject.Addr(), 2, uintptr(jobHandle), uintptr(unsafe.Pointer(ioRateControlInfo)), 0)
	ret = uint32(r0)
	if ret == 0 {
		if e1 != 0 {
			err = errnoErr(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func SearchPath(lpPath *uint16, lpFileName *uint16, lpExtension *uint16, nBufferLength uint32, lpBuffer *uint16, lpFilePath **uint16) (size uint32, err error) {
	r0, _, e1 := syscall.Syscall6(procSearchPathW.Addr(), 6, uintptr(unsafe.Pointer(lpPath)), uintptr(unsafe.Pointer(lpFileName)), uintptr(unsafe.Pointer(lpExtension)), uintptr(nBufferLength), uintptr(unsafe.Pointer(lpBuffer)), uintptr(unsafe.Pointer(lpFilePath)))
	size = uint32(r0)
	if size == 0 {
		if e1 != 0 {
			err = errnoErr(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func LogonUser(username *uint16, domain *uint16, password *uint16, logonType uint32, logonProvider uint32, token *windows.Token) (err error) {
	r1, _, e1 := syscall.Syscall6(procLogonUserW.Addr(), 6, uintptr(unsafe.Pointer(username)), uintptr(unsafe.Pointer(domain)), uintptr(unsafe.Pointer(password)), uintptr(logonType), uintptr(logonProvider), uintptr(unsafe.Pointer(token)))
	if r1 == 0 {
		if e1 != 0 {
			err = errnoErr(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func RtlMoveMemory(destination *byte, source *byte, length uintptr) (err error) {
	r1, _, e1 := syscall.Syscall(procRtlMoveMemory.Addr(), 3, uintptr(unsafe.Pointer(destination)), uintptr(unsafe.Pointer(source)), uintptr(length))
	if r1 == 0 {
		if e1 != 0 {
			err = errnoErr(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func GetActiveProcessorCount(groupNumber uint16) (amount uint32) {
	r0, _, _ := syscall.Syscall(procGetActiveProcessorCount.Addr(), 1, uintptr(groupNumber), 0, 0)
	amount = uint32(r0)
	return
}
