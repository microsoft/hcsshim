//go:build windows

package copyfile

import (
	"context"
	"fmt"
	"syscall"

	"github.com/Microsoft/hcsshim/internal/otelutil"
	"github.com/Microsoft/hcsshim/internal/winapi"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// CopyFile is a utility for copying a file using CopyFileW win32 API for
// performance.
func CopyFile(ctx context.Context, srcFile, destFile string, overwrite bool) (err error) {
	ctx, span := otelutil.StartSpan(ctx, "copyfile::CopyFile", trace.WithAttributes(
		attribute.String("srcFile", srcFile),
		attribute.String("destFile", destFile),
		attribute.Bool("overwrite", overwrite))) //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

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
		return fmt.Errorf("failed CopyFileW Win32 call from '%s' to '%s': %w", srcFile, destFile, err)
	}
	return nil
}
