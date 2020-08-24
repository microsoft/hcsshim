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
	modcfgmgr32 = windows.NewLazySystemDLL("cfgmgr32.dll")
	modntdll    = windows.NewLazySystemDLL("ntdll.dll")

	procInitializeProcThreadAttributeList = modkernel32.NewProc("InitializeProcThreadAttributeList")
	procUpdateProcThreadAttribute         = modkernel32.NewProc("UpdateProcThreadAttribute")
	procDeleteProcThreadAttributeList     = modkernel32.NewProc("DeleteProcThreadAttributeList")
	procGetProcessHeap                    = modkernel32.NewProc("GetProcessHeap")
	procHeapAlloc                         = modkernel32.NewProc("HeapAlloc")
	procHeapFree                          = modkernel32.NewProc("HeapFree")
	procCreatePseudoConsole               = modkernel32.NewProc("CreatePseudoConsole")
	procResizePseudoConsole               = modkernel32.NewProc("ResizePseudoConsole")
	procClosePseudoConsole                = modkernel32.NewProc("ClosePseudoConsole")
	procCM_Get_Device_ID_List_SizeA       = modcfgmgr32.NewProc("CM_Get_Device_ID_List_SizeA")
	procCM_Get_Device_ID_ListA            = modcfgmgr32.NewProc("CM_Get_Device_ID_ListA")
	procCM_Locate_DevNodeW                = modcfgmgr32.NewProc("CM_Locate_DevNodeW")
	procCM_Get_DevNode_PropertyW          = modcfgmgr32.NewProc("CM_Get_DevNode_PropertyW")
	procLocalAlloc                        = modkernel32.NewProc("LocalAlloc")
	procLocalFree                         = modkernel32.NewProc("LocalFree")
	procNtCreateFile                      = modntdll.NewProc("NtCreateFile")
	procNtSetInformationFile              = modntdll.NewProc("NtSetInformationFile")
	procNtOpenDirectoryObject             = modntdll.NewProc("NtOpenDirectoryObject")
	procNtQueryDirectoryObject            = modntdll.NewProc("NtQueryDirectoryObject")
	procRtlNtStatusToDosError             = modntdll.NewProc("RtlNtStatusToDosError")
)

func InitializeProcThreadAttributeList(lpAttributeList uintptr, dwAttributeCount uint32, dwFlags uint32, lpSize *uintptr) (err error) {
	r1, _, e1 := syscall.Syscall6(procInitializeProcThreadAttributeList.Addr(), 4, uintptr(lpAttributeList), uintptr(dwAttributeCount), uintptr(dwFlags), uintptr(unsafe.Pointer(lpSize)), 0, 0)
	if r1 == 0 {
		if e1 != 0 {
			err = errnoErr(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func UpdateProcThreadAttribute(lpAttributeList uintptr, dwFlags uint32, attribute uintptr, lpValue *uintptr, cbSize uintptr, lpPreviousValue *uintptr, lpReturnSize *uintptr) (err error) {
	r1, _, e1 := syscall.Syscall9(procUpdateProcThreadAttribute.Addr(), 7, uintptr(lpAttributeList), uintptr(dwFlags), uintptr(attribute), uintptr(unsafe.Pointer(lpValue)), uintptr(cbSize), uintptr(unsafe.Pointer(lpPreviousValue)), uintptr(unsafe.Pointer(lpReturnSize)), 0, 0)
	if r1 == 0 {
		if e1 != 0 {
			err = errnoErr(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func DeleteProcThreadAttributeList(lpAttributeList uintptr) {
	syscall.Syscall(procDeleteProcThreadAttributeList.Addr(), 1, uintptr(lpAttributeList), 0, 0)
	return
}

func GetProcessHeap() (procHeap windows.Handle, err error) {
	r0, _, e1 := syscall.Syscall(procGetProcessHeap.Addr(), 0, 0, 0, 0)
	procHeap = windows.Handle(r0)
	if procHeap == 0 {
		if e1 != 0 {
			err = errnoErr(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func HeapAlloc(hHeap windows.Handle, dwFlags uint32, dwBytes uintptr) (lpMem uintptr, err error) {
	r0, _, e1 := syscall.Syscall(procHeapAlloc.Addr(), 3, uintptr(hHeap), uintptr(dwFlags), uintptr(dwBytes))
	lpMem = uintptr(r0)
	if lpMem == 0 {
		if e1 != 0 {
			err = errnoErr(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func HeapFree(hHeap windows.Handle, dwFlags uint32, lpMem uintptr) (err error) {
	r1, _, e1 := syscall.Syscall(procHeapFree.Addr(), 3, uintptr(hHeap), uintptr(dwFlags), uintptr(lpMem))
	if r1 == 0 {
		if e1 != 0 {
			err = errnoErr(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func CreatePseudoConsole(size uintptr, hInput windows.Handle, hOutput windows.Handle, dwFlags uint32, phPC *windows.Handle) (hr error) {
	r0, _, _ := syscall.Syscall6(procCreatePseudoConsole.Addr(), 5, uintptr(size), uintptr(hInput), uintptr(hOutput), uintptr(dwFlags), uintptr(unsafe.Pointer(phPC)), 0)
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}

func ResizePseudoConsole(hPC windows.Handle, size uintptr) (hr error) {
	r0, _, _ := syscall.Syscall(procResizePseudoConsole.Addr(), 2, uintptr(hPC), uintptr(size), 0)
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}

func ClosePseudoConsole(hPC windows.Handle) (err error) {
	r1, _, e1 := syscall.Syscall(procClosePseudoConsole.Addr(), 1, uintptr(hPC), 0, 0)
	if r1 == 0 {
		if e1 != 0 {
			err = errnoErr(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func CMGetDeviceIDListSize(pulLen *uint32, pszFilter *byte, uFlags uint32) (hr error) {
	r0, _, _ := syscall.Syscall(procCM_Get_Device_ID_List_SizeA.Addr(), 3, uintptr(unsafe.Pointer(pulLen)), uintptr(unsafe.Pointer(pszFilter)), uintptr(uFlags))
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}

func CMGetDeviceIDList(pszFilter *byte, buffer *byte, bufferLen uint32, uFlags uint32) (hr error) {
	r0, _, _ := syscall.Syscall6(procCM_Get_Device_ID_ListA.Addr(), 4, uintptr(unsafe.Pointer(pszFilter)), uintptr(unsafe.Pointer(buffer)), uintptr(bufferLen), uintptr(uFlags), 0, 0)
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}

func CMLocateDevNode(pdnDevInst *uint32, pDeviceID string, uFlags uint32) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(pDeviceID)
	if hr != nil {
		return
	}
	return _CMLocateDevNode(pdnDevInst, _p0, uFlags)
}

func _CMLocateDevNode(pdnDevInst *uint32, pDeviceID *uint16, uFlags uint32) (hr error) {
	r0, _, _ := syscall.Syscall(procCM_Locate_DevNodeW.Addr(), 3, uintptr(unsafe.Pointer(pdnDevInst)), uintptr(unsafe.Pointer(pDeviceID)), uintptr(uFlags))
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}

func CMGetDevNodeProperty(dnDevInst uint32, propertyKey *DevPropKey, propertyType *uint32, propertyBuffer *uint16, propertyBufferSize *uint32, uFlags uint32) (hr error) {
	r0, _, _ := syscall.Syscall6(procCM_Get_DevNode_PropertyW.Addr(), 6, uintptr(dnDevInst), uintptr(unsafe.Pointer(propertyKey)), uintptr(unsafe.Pointer(propertyType)), uintptr(unsafe.Pointer(propertyBuffer)), uintptr(unsafe.Pointer(propertyBufferSize)), uintptr(uFlags))
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}

func LocalAlloc(flags uint32, size int) (ptr uintptr) {
	r0, _, _ := syscall.Syscall(procLocalAlloc.Addr(), 2, uintptr(flags), uintptr(size), 0)
	ptr = uintptr(r0)
	return
}

func LocalFree(ptr uintptr) {
	syscall.Syscall(procLocalFree.Addr(), 1, uintptr(ptr), 0, 0)
	return
}

func NtCreateFile(handle *uintptr, accessMask uint32, oa *ObjectAttributes, iosb *IOStatusBlock, allocationSize *uint64, fileAttributes uint32, shareAccess uint32, createDisposition uint32, createOptions uint32, eaBuffer *byte, eaLength uint32) (status uint32) {
	r0, _, _ := syscall.Syscall12(procNtCreateFile.Addr(), 11, uintptr(unsafe.Pointer(handle)), uintptr(accessMask), uintptr(unsafe.Pointer(oa)), uintptr(unsafe.Pointer(iosb)), uintptr(unsafe.Pointer(allocationSize)), uintptr(fileAttributes), uintptr(shareAccess), uintptr(createDisposition), uintptr(createOptions), uintptr(unsafe.Pointer(eaBuffer)), uintptr(eaLength), 0)
	status = uint32(r0)
	return
}

func NtSetInformationFile(handle uintptr, iosb *IOStatusBlock, information uintptr, length uint32, class uint32) (status uint32) {
	r0, _, _ := syscall.Syscall6(procNtSetInformationFile.Addr(), 5, uintptr(handle), uintptr(unsafe.Pointer(iosb)), uintptr(information), uintptr(length), uintptr(class), 0)
	status = uint32(r0)
	return
}

func NtOpenDirectoryObject(handle *uintptr, accessMask uint32, oa *ObjectAttributes) (status uint32) {
	r0, _, _ := syscall.Syscall(procNtOpenDirectoryObject.Addr(), 3, uintptr(unsafe.Pointer(handle)), uintptr(accessMask), uintptr(unsafe.Pointer(oa)))
	status = uint32(r0)
	return
}

func NtQueryDirectoryObject(handle uintptr, buffer *byte, length uint32, singleEntry bool, restartScan bool, context *uint32, returnLength *uint32) (status uint32) {
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

func RtlNtStatusToDosError(status uint32) (winerr error) {
	r0, _, _ := syscall.Syscall(procRtlNtStatusToDosError.Addr(), 1, uintptr(status), 0, 0)
	if r0 != 0 {
		winerr = syscall.Errno(r0)
	}
	return
}
