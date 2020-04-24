// Code generated mksyscall_windows.exe DO NOT EDIT

package cfgmgr

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
	modcfgmgr32 = windows.NewLazySystemDLL("cfgmgr32.dll")

	procCM_Get_Device_ID_List_SizeA = modcfgmgr32.NewProc("CM_Get_Device_ID_List_SizeA")
	procCM_Get_Device_ID_ListA      = modcfgmgr32.NewProc("CM_Get_Device_ID_ListA")
	procCM_Locate_DevNodeW          = modcfgmgr32.NewProc("CM_Locate_DevNodeW")
	procCM_Get_DevNode_PropertyW    = modcfgmgr32.NewProc("CM_Get_DevNode_PropertyW")
)

func cmGetDeviceIDListSize(pulLen *uint32, pszFilter *byte, uFlags uint64) (hr error) {
	r0, _, _ := syscall.Syscall(procCM_Get_Device_ID_List_SizeA.Addr(), 3, uintptr(unsafe.Pointer(pulLen)), uintptr(unsafe.Pointer(pszFilter)), uintptr(uFlags))
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}

func cmGetDeviceIDList(pszFilter *byte, buffer *byte, bufferLen uint64, uFlags uint64) (hr error) {
	r0, _, _ := syscall.Syscall6(procCM_Get_Device_ID_ListA.Addr(), 4, uintptr(unsafe.Pointer(pszFilter)), uintptr(unsafe.Pointer(buffer)), uintptr(bufferLen), uintptr(uFlags), 0, 0)
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}

func cmLocateDevNode(pdnDevInst *uint32, pDeviceID string, uFlags uint64) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(pDeviceID)
	if hr != nil {
		return
	}
	return _cmLocateDevNode(pdnDevInst, _p0, uFlags)
}

func _cmLocateDevNode(pdnDevInst *uint32, pDeviceID *uint16, uFlags uint64) (hr error) {
	r0, _, _ := syscall.Syscall(procCM_Locate_DevNodeW.Addr(), 3, uintptr(unsafe.Pointer(pdnDevInst)), uintptr(unsafe.Pointer(pDeviceID)), uintptr(uFlags))
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}

func cmGetDevNodeProperty(dnDevInst uint32, propertyKey *devPropKey, propertyType *uint64, propertyBuffer *uint16, propertyBufferSize *uint64, uFlags uint64) (hr error) {
	r0, _, _ := syscall.Syscall6(procCM_Get_DevNode_PropertyW.Addr(), 6, uintptr(dnDevInst), uintptr(unsafe.Pointer(propertyKey)), uintptr(unsafe.Pointer(propertyType)), uintptr(unsafe.Pointer(propertyBuffer)), uintptr(unsafe.Pointer(propertyBufferSize)), uintptr(uFlags))
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}
