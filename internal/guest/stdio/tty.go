//go:build linux
// +build linux

package stdio

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

// NewConsole allocates a new console and returns the File for its master and
// path for its slave.
func NewConsole() (*os.File, string, error) {
	master, err := os.OpenFile("/dev/ptmx", syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, "", errors.Wrap(err, "failed to open master pseudoterminal file")
	}
	console, err := ptsname(master)
	if err != nil {
		return nil, "", err
	}
	if err := unlockpt(master); err != nil {
		return nil, "", err
	}
	// TODO: Do we need to keep this chmod call?
	if err := os.Chmod(console, 0600); err != nil {
		return nil, "", errors.Wrap(err, "failed to change permissions on the slave pseudoterminal file")
	}
	if err := os.Chown(console, 0, 0); err != nil {
		return nil, "", errors.Wrap(err, "failed to change ownership on the slave pseudoterminal file")
	}
	return master, console, nil
}

// ResizeConsole sends the appropriate resize to a pTTY FD
// Synchronization of pty should be handled in the callers context.
func ResizeConsole(pty *os.File, height, width uint16) error {
	type consoleSize struct {
		Height uint16
		Width  uint16
		x      uint16
		y      uint16
	}

	return ioctlFile(pty, uintptr(unix.TIOCSWINSZ), unsafe.Pointer(&consoleSize{Height: height, Width: width}))
}

// ioctlFile issues an ioctl on f without calling f.Fd().
//
// Fd puts the file back into blocking mode and removes it from the runtime
// poller for good, after which SetReadDeadline silently succeeds and does
// nothing. TtyRelay relies on a read deadline to interrupt a copier parked on
// the pty master when the host connections die during a live migration, so a
// single Fd call anywhere on the master would leave that relay wedged.
// SyscallConn keeps the registration intact.
//
// data stays an unsafe.Pointer up to the Syscall so the conversion happens in
// the call expression itself, which is what keeps the referent alive for the
// duration of the syscall.
func ioctlFile(f *os.File, flag uintptr, data unsafe.Pointer) error {
	rc, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var ioerr error
	if cerr := rc.Control(func(fd uintptr) {
		if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, flag, uintptr(data)); errno != 0 {
			ioerr = errno
		}
	}); cerr != nil {
		return cerr
	}
	return ioerr
}

// ptsname is a Go wrapper around the ptsname system call. It returns the name
// of the slave pseudoterminal device corresponding to the given master.
func ptsname(f *os.File) (string, error) {
	var n int32
	if err := ioctlFile(f, syscall.TIOCGPTN, unsafe.Pointer(&n)); err != nil {
		return "", errors.Wrap(err, "ioctl TIOCGPTN failed for ptsname")
	}
	return fmt.Sprintf("/dev/pts/%d", n), nil
}

// unlockpt is a Go wrapper around the unlockpt system call. It unlocks the
// slave pseudoterminal device corresponding to the given master.
func unlockpt(f *os.File) error {
	var u int32
	if err := ioctlFile(f, syscall.TIOCSPTLCK, unsafe.Pointer(&u)); err != nil {
		return errors.Wrap(err, "ioctl TIOCSPTLCK failed for unlockpt")
	}
	return nil
}
