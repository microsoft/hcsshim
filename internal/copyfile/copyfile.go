//go:build windows

package copyfile

import (
	"context"
	"fmt"
	"syscall"

	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/winapi"
	"go.opencensus.io/trace"
)

// CopyFile is a utility for copying a file using CopyFileW win32 API for
// performance.
func CopyFile(ctx context.Context, srcFile, destFile string, overwrite bool) (err error) {
	ctx, span := oc.StartSpan(ctx, "copyfile::CopyFile") //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute("srcFile", srcFile),
		trace.StringAttribute("destFile", destFile),
		trace.BoolAttribute("overwrite", overwrite))

	var bFailIfExists int32 = 1
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
	if err := winapi.CopyFileW(lpExistingFileName, lpNewFileName, bFailIfExists); err != nil {
		return fmt.Errorf("failed CopyFileW Win32 call from '%s' to '%s': %s", srcFile, destFile, err)
	}
	return nil
}
