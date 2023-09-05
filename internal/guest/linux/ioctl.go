//go:build linux
// +build linux

package linux

import (
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// 32 bits to describe an ioctl:
// 0-7:   NR (command for a given ioctl type)
// 8-15:  TYPE (ioctl type)
// 16-29: SIZE (payload size)
// 30-31: DIR (direction of ioctl, can be: none/write/read/write-read)
const (
	IocWrite    = 1
	IocRead     = 2
	IocNRBits   = 8
	IocTypeBits = 8
	IocSizeBits = 14
	IocDirBits  = 2

	IocNRMask    = (1 << IocNRBits) - 1
	IocTypeMask  = (1 << IocTypeBits) - 1
	IocSizeMask  = (1 << IocSizeBits) - 1
	IocDirMask   = (1 << IocDirBits) - 1
	IocTypeShift = IocNRBits
	IocSizeShift = IocTypeShift + IocTypeBits
	IocDirShift  = IocSizeShift + IocSizeBits
	IocWRBase    = (IocRead | IocWrite) << IocDirShift
)

// Ioctl makes a syscall described by `command` with data `dataPtr` to device
// driver file `f`.
func Ioctl(f *os.File, command int, dataPtr unsafe.Pointer) error {
	if _, _, err := unix.Syscall(
		unix.SYS_IOCTL,
		f.Fd(),
		uintptr(command),
		uintptr(dataPtr),
	); err != 0 {
		return err
	}
	return nil
}

// Ported from _IOWR macro.
// Returns value for `command` parameter in Ioctl().
func Iowr(subsystem, ID, parameterSize uint64) int {
	ioctlBase := IocWRBase | subsystem<<IocTypeShift | parameterSize<<IocSizeShift
	return int(ID | ioctlBase)
}
