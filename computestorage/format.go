package computestorage

import (
	"context"
	"os"
	"syscall"

	"github.com/Microsoft/go-winio/vhd"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"golang.org/x/sys/windows"
)

func openDisk(path string) (windows.Handle, error) {
	u16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	h, err := windows.CreateFile(
		u16,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_NO_BUFFERING,
		0,
	)
	if err != nil {
		return 0, &os.PathError{
			Op:   "CreateFile",
			Path: path,
			Err:  err,
		}
	}
	return h, nil
}

// FormatWritableLayerVhd formats a virtual disk for use as a writable container layer.
//
// If the VHD is not mounted it will be temporarily mounted.
func FormatWritableLayerVhd(ctx context.Context, vhdHandle windows.Handle) (err error) {
	title := "hcsshim.FormatWritableLayerVhd"
	ctx, span := trace.StartSpan(ctx, title) //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	h := vhdHandle
	// On RS5 HcsFormatWritableLayerVhd expects to receive a disk handle instead of a vhd handle.
	if osversion.Build() < osversion.V19H1 {
		if err := vhd.AttachVirtualDisk(syscall.Handle(vhdHandle), vhd.AttachVirtualDiskFlagNone, &vhd.AttachVirtualDiskParameters{Version: 1}); err != nil {
			return err
		}
		defer func() {
			if detachErr := vhd.DetachVirtualDisk(syscall.Handle(vhdHandle)); err == nil && detachErr != nil {
				err = detachErr
			}
		}()
		diskPath, err := vhd.GetVirtualDiskPhysicalPath(syscall.Handle(vhdHandle))
		if err != nil {
			return err
		}
		diskHandle, err := openDisk(diskPath)
		if err != nil {
			return err
		}
		defer windows.CloseHandle(diskHandle) // nolint: errcheck
		h = diskHandle
	}
	err = hcsFormatWritableLayerVhd(h)
	if err != nil {
		return errors.Wrap(err, "failed to format writable layer vhd")
	}
	return
}
