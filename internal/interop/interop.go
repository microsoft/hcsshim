//go:build windows

package interop

import (
	"syscall"
	"unsafe"
)

//go:generate go tool github.com/Microsoft/go-winio/tools/mkwinsyscall -output zsyscall_windows.go interop.go

//sys coTaskMemFree(buffer unsafe.Pointer) = api_ms_win_core_com_l1_1_0.CoTaskMemFree

func ConvertAndFreeCoTaskMemString(buffer *uint16) string {
	str := ConvertString(buffer)
	coTaskMemFree(unsafe.Pointer(buffer))
	return str
}

// Converts a PWSTR to a string, duplicating the underlying data and leaving the original unmodified.
func ConvertString(buffer *uint16) string {
	return syscall.UTF16ToString((*[1 << 29]uint16)(unsafe.Pointer(buffer))[:])
}

func Win32FromHresult(hr uintptr) syscall.Errno {
	if hr&0x1fff0000 == 0x00070000 {
		return syscall.Errno(hr & 0xffff)
	}
	return syscall.Errno(hr)
}
