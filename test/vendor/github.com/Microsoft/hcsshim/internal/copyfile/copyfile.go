package copyfile

import (
	"context"
	"fmt"
	"syscall"
	"unsafe"

	"github.com/Microsoft/hcsshim/internal/oc"
	"go.opencensus.io/trace"
)

var (
	modkernel32   = syscall.NewLazyDLL("kernel32.dll")
	procCopyFileW = modkernel32.NewProc("CopyFileW")
)

// CopyFile is a utility for copying a file using CopyFileW win32 API for
// performance.
func CopyFile(ctx context.Context, srcFile, destFile string, overwrite bool) (err error) {
	ctx, span := trace.StartSpan(ctx, "copyfile::CopyFile") //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute("srcFile", srcFile),
		trace.StringAttribute("destFile", destFile),
		trace.BoolAttribute("overwrite", overwrite))

	var bFailIfExists uint32 = 1
	if overwrite {
		bFailIfExists = 0
	}

	lpExistingFileName, err := syscall.UTF16PtrFromString(srcFile)
	if err != nil {
		return err
	}
	lpNewFileName, err := syscall.UTF16PtrFromString(destFile)
	if err != nil {
		return err
	}
	r1, _, err := syscall.Syscall(
		procCopyFileW.Addr(),
		3,
		uintptr(unsafe.Pointer(lpExistingFileName)),
		uintptr(unsafe.Pointer(lpNewFileName)),
		uintptr(bFailIfExists))
	if r1 == 0 {
		return fmt.Errorf("failed CopyFileW Win32 call from '%s' to '%s': %s", srcFile, destFile, err)
	}
	return nil
}
