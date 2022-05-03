//go:build windows

package errdefs

import "golang.org/x/sys/windows"

// HResult coalesces into an Errono on Windows, and uses the underlying Win32 errorcode.

func (r HResult) AsError() error {
	return windows.Errno(r.Errno())
}
