// Code generated mksyscall_windows.exe DO NOT EDIT

package computestorage

import (
	"syscall"
	"unsafe"

	"github.com/Microsoft/hcsshim/internal/interop"
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
	modcomputestorage = windows.NewLazySystemDLL("computestorage.dll")

	procHcsImportLayer                   = modcomputestorage.NewProc("HcsImportLayer")
	procHcsExportLayer                   = modcomputestorage.NewProc("HcsExportLayer")
	prochcsExportLegacyWritableLayer     = modcomputestorage.NewProc("hcsExportLegacyWritableLayer")
	prochcsDestroyLayer                  = modcomputestorage.NewProc("hcsDestroyLayer")
	prochcsSetupBaseOSLayer              = modcomputestorage.NewProc("hcsSetupBaseOSLayer")
	prochcsInitializeWritableLayer       = modcomputestorage.NewProc("hcsInitializeWritableLayer")
	prochcsInitializeLegacyWritableLayer = modcomputestorage.NewProc("hcsInitializeLegacyWritableLayer")
	prochcsAttachLayerStorageFilter      = modcomputestorage.NewProc("hcsAttachLayerStorageFilter")
	prochcsDetachLayerStorageFilter      = modcomputestorage.NewProc("hcsDetachLayerStorageFilter")
	prochcsFormatWritableLayerVhd        = modcomputestorage.NewProc("hcsFormatWritableLayerVhd")
	prochcsGetLayerVhdMountPath          = modcomputestorage.NewProc("hcsGetLayerVhdMountPath")
)

func hcsImportLayer(layerPath string, sourceFolderPath string, layerData string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(layerPath)
	if hr != nil {
		return
	}
	var _p1 *uint16
	_p1, hr = syscall.UTF16PtrFromString(sourceFolderPath)
	if hr != nil {
		return
	}
	var _p2 *uint16
	_p2, hr = syscall.UTF16PtrFromString(layerData)
	if hr != nil {
		return
	}
	return _hcsImportLayer(_p0, _p1, _p2)
}

func _hcsImportLayer(layerPath *uint16, sourceFolderPath *uint16, layerData *uint16) (hr error) {
	if hr = procHcsImportLayer.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsImportLayer.Addr(), 3, uintptr(unsafe.Pointer(layerPath)), uintptr(unsafe.Pointer(sourceFolderPath)), uintptr(unsafe.Pointer(layerData)))
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsExportLayer(layerPath string, exportFolderPath string, layerData string, options string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(layerPath)
	if hr != nil {
		return
	}
	var _p1 *uint16
	_p1, hr = syscall.UTF16PtrFromString(exportFolderPath)
	if hr != nil {
		return
	}
	var _p2 *uint16
	_p2, hr = syscall.UTF16PtrFromString(layerData)
	if hr != nil {
		return
	}
	var _p3 *uint16
	_p3, hr = syscall.UTF16PtrFromString(options)
	if hr != nil {
		return
	}
	return _hcsExportLayer(_p0, _p1, _p2, _p3)
}

func _hcsExportLayer(layerPath *uint16, exportFolderPath *uint16, layerData *uint16, options *uint16) (hr error) {
	if hr = procHcsExportLayer.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall6(procHcsExportLayer.Addr(), 4, uintptr(unsafe.Pointer(layerPath)), uintptr(unsafe.Pointer(exportFolderPath)), uintptr(unsafe.Pointer(layerData)), uintptr(unsafe.Pointer(options)), 0, 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsExportLegacyWritableLayer(writableLayerMountPath string, writableLayerFolderPath string, exportFolderPath string, layerData string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(writableLayerMountPath)
	if hr != nil {
		return
	}
	var _p1 *uint16
	_p1, hr = syscall.UTF16PtrFromString(writableLayerFolderPath)
	if hr != nil {
		return
	}
	var _p2 *uint16
	_p2, hr = syscall.UTF16PtrFromString(exportFolderPath)
	if hr != nil {
		return
	}
	var _p3 *uint16
	_p3, hr = syscall.UTF16PtrFromString(layerData)
	if hr != nil {
		return
	}
	return _hcsExportLegacyWritableLayer(_p0, _p1, _p2, _p3)
}

func _hcsExportLegacyWritableLayer(writableLayerMountPath *uint16, writableLayerFolderPath *uint16, exportFolderPath *uint16, layerData *uint16) (hr error) {
	if hr = prochcsExportLegacyWritableLayer.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall6(prochcsExportLegacyWritableLayer.Addr(), 4, uintptr(unsafe.Pointer(writableLayerMountPath)), uintptr(unsafe.Pointer(writableLayerFolderPath)), uintptr(unsafe.Pointer(exportFolderPath)), uintptr(unsafe.Pointer(layerData)), 0, 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsDestroyLayer(layerPath string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(layerPath)
	if hr != nil {
		return
	}
	return _hcsDestroyLayer(_p0)
}

func _hcsDestroyLayer(layerPath *uint16) (hr error) {
	if hr = prochcsDestroyLayer.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(prochcsDestroyLayer.Addr(), 1, uintptr(unsafe.Pointer(layerPath)), 0, 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsSetupBaseOSLayer(layerPath string, vhdHandle syscall.Handle, options string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(layerPath)
	if hr != nil {
		return
	}
	var _p1 *uint16
	_p1, hr = syscall.UTF16PtrFromString(options)
	if hr != nil {
		return
	}
	return _hcsSetupBaseOSLayer(_p0, vhdHandle, _p1)
}

func _hcsSetupBaseOSLayer(layerPath *uint16, vhdHandle syscall.Handle, options *uint16) (hr error) {
	if hr = prochcsSetupBaseOSLayer.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(prochcsSetupBaseOSLayer.Addr(), 3, uintptr(unsafe.Pointer(layerPath)), uintptr(vhdHandle), uintptr(unsafe.Pointer(options)))
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsInitializeWritableLayer(writableLayerPath string, layerData string, options string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(writableLayerPath)
	if hr != nil {
		return
	}
	var _p1 *uint16
	_p1, hr = syscall.UTF16PtrFromString(layerData)
	if hr != nil {
		return
	}
	var _p2 *uint16
	_p2, hr = syscall.UTF16PtrFromString(options)
	if hr != nil {
		return
	}
	return _hcsInitializeWritableLayer(_p0, _p1, _p2)
}

func _hcsInitializeWritableLayer(writableLayerPath *uint16, layerData *uint16, options *uint16) (hr error) {
	if hr = prochcsInitializeWritableLayer.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(prochcsInitializeWritableLayer.Addr(), 3, uintptr(unsafe.Pointer(writableLayerPath)), uintptr(unsafe.Pointer(layerData)), uintptr(unsafe.Pointer(options)))
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsInitializeLegacyWritableLayer(writableLayerMountPath string, writableLayerFolderPath string, layerData string, options string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(writableLayerMountPath)
	if hr != nil {
		return
	}
	var _p1 *uint16
	_p1, hr = syscall.UTF16PtrFromString(writableLayerFolderPath)
	if hr != nil {
		return
	}
	var _p2 *uint16
	_p2, hr = syscall.UTF16PtrFromString(layerData)
	if hr != nil {
		return
	}
	var _p3 *uint16
	_p3, hr = syscall.UTF16PtrFromString(options)
	if hr != nil {
		return
	}
	return _hcsInitializeLegacyWritableLayer(_p0, _p1, _p2, _p3)
}

func _hcsInitializeLegacyWritableLayer(writableLayerMountPath *uint16, writableLayerFolderPath *uint16, layerData *uint16, options *uint16) (hr error) {
	if hr = prochcsInitializeLegacyWritableLayer.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall6(prochcsInitializeLegacyWritableLayer.Addr(), 4, uintptr(unsafe.Pointer(writableLayerMountPath)), uintptr(unsafe.Pointer(writableLayerFolderPath)), uintptr(unsafe.Pointer(layerData)), uintptr(unsafe.Pointer(options)), 0, 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsAttachLayerStorageFilter(layerPath string, layerData string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(layerPath)
	if hr != nil {
		return
	}
	var _p1 *uint16
	_p1, hr = syscall.UTF16PtrFromString(layerData)
	if hr != nil {
		return
	}
	return _hcsAttachLayerStorageFilter(_p0, _p1)
}

func _hcsAttachLayerStorageFilter(layerPath *uint16, layerData *uint16) (hr error) {
	if hr = prochcsAttachLayerStorageFilter.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(prochcsAttachLayerStorageFilter.Addr(), 2, uintptr(unsafe.Pointer(layerPath)), uintptr(unsafe.Pointer(layerData)), 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsDetachLayerStorageFilter(layerPath string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(layerPath)
	if hr != nil {
		return
	}
	return _hcsDetachLayerStorageFilter(_p0)
}

func _hcsDetachLayerStorageFilter(layerPath *uint16) (hr error) {
	if hr = prochcsDetachLayerStorageFilter.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(prochcsDetachLayerStorageFilter.Addr(), 1, uintptr(unsafe.Pointer(layerPath)), 0, 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsFormatWritableLayerVhd(vhdHandle syscall.Handle) (hr error) {
	if hr = prochcsFormatWritableLayerVhd.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(prochcsFormatWritableLayerVhd.Addr(), 1, uintptr(vhdHandle), 0, 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsGetLayerVhdMountPath(vhdHandle syscall.Handle, mountPath **uint16) (hr error) {
	if hr = prochcsGetLayerVhdMountPath.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(prochcsGetLayerVhdMountPath.Addr(), 2, uintptr(vhdHandle), uintptr(unsafe.Pointer(mountPath)), 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}
