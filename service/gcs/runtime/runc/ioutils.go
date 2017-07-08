package runc

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/Sirupsen/logrus"

	"github.com/pkg/errors"
	"github.com/tonistiigi/fifo"
	"golang.org/x/net/context"
	"golang.org/x/sys/unix"

	"github.com/Microsoft/opengcs/service/gcs/runtime"
)

// ioSet is a set of stdio pipe handles, with a read and write version for
// each.
type ioSet struct {
	InR  *os.File
	InW  *os.File
	OutR *os.File
	OutW *os.File
	ErrR *os.File
	ErrW *os.File
}

// GetStdioPipes returns the stdio pipes used by the given process.
func (c *container) GetStdioPipes() (*runtime.StdioPipes, error) {
	return c.init.GetStdioPipes()
}

// GetStdioPipes returns the stdio pipes used by the given process.
func (p *process) GetStdioPipes() (*runtime.StdioPipes, error) {
	processDir := p.c.r.getProcessDir(p.c.id, p.pid)
	stdinPath := filepath.Join(processDir, "in")
	var stdin io.WriteCloser
	stdinPathExists, err := p.c.r.pathExists(stdinPath)
	if err != nil {
		return nil, err
	}
	if stdinPathExists {
		stdin, err = os.OpenFile(stdinPath, os.O_WRONLY, os.ModeNamedPipe)
		if err != nil {
			return nil, errors.Wrap(err, "failed to open stdin fifo file")
		}
	}

	stdoutPath := filepath.Join(processDir, "out")
	var stdout io.ReadCloser
	stdoutPathExists, err := p.c.r.pathExists(stdoutPath)
	if err != nil {
		return nil, err
	}
	if stdoutPathExists {
		stdout, err = os.OpenFile(stdoutPath, os.O_RDONLY, os.ModeNamedPipe)
		if err != nil {
			return nil, errors.Wrap(err, "failed to open stdout fifo file")
		}
	}

	stderrPath := filepath.Join(processDir, "err")
	var stderr io.ReadCloser
	stderrPathExists, err := p.c.r.pathExists(stderrPath)
	if err != nil {
		return nil, err
	}
	if stderrPathExists {
		stderr, err = os.OpenFile(stderrPath, os.O_RDONLY, os.ModeNamedPipe)
		if err != nil {
			return nil, errors.Wrap(err, "failed to open stderr fifo file")
		}
	}

	return &runtime.StdioPipes{In: stdin, Out: stdout, Err: stderr}, nil
}

// setupIOForTerminal gets the container's terminal master from the given unix
// socket listener and starts copying stdio for the process to and from the
// console.
func (r *runcRuntime) setupIOForTerminal(processDir string, stdioOptions runtime.StdioOptions, sockListener *net.UnixListener) error {
	master, err := r.getMasterFromSocket(sockListener)
	if err != nil {
		return err
	}

	if stdioOptions.CreateIn {
		stdinPath := filepath.Join(processDir, "in")
		stdin, err := r.openFifo(stdinPath, true, true)
		if err != nil {
			return errors.Wrapf(err, "failed to create stdin fifo %s", stdinPath)
		}
		r.beginCopying(master, false, stdin, true)
	}

	if stdioOptions.CreateOut {
		stdoutPath := filepath.Join(processDir, "out")
		stdout, err := r.openFifo(stdoutPath, false, true)
		if err != nil {
			return errors.Wrapf(err, "failed to create stdout fifo %s", stdoutPath)
		}
		r.beginCopying(stdout, true, master, true)
	}

	return nil
}

// setupIOWithoutTerminal provides the set of stdio pipes to be used for the
// given process, as well as starts copying stdio for the process to and from
// the pipes.
func (r *runcRuntime) setupIOWithoutTerminal(id string, processDir string, stdioOptions runtime.StdioOptions) (*ioSet, error) {
	ioSet, err := initializeIOSet(stdioOptions)
	if err != nil {
		return nil, err
	}

	if stdioOptions.CreateIn {
		stdinPath := filepath.Join(processDir, "in")
		stdin, err := r.openFifo(stdinPath, true, true)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create stdin fifo %s", stdinPath)
		}
		r.beginCopying(ioSet.InW, true, stdin, true)
	}

	if stdioOptions.CreateOut {
		stdoutPath := filepath.Join(processDir, "out")
		stdout, err := r.openFifo(stdoutPath, false, true)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create stdout fifo %s", stdoutPath)
		}
		r.beginCopying(stdout, true, ioSet.OutR, true)
	}

	if stdioOptions.CreateErr {
		stderrPath := filepath.Join(processDir, "err")
		stderr, err := r.openFifo(stderrPath, false, true)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create stderr fifo %s", stderrPath)
		}
		r.beginCopying(stderr, true, ioSet.ErrR, true)
	}

	return ioSet, nil
}

// initializeIOSet initializes the ioSet to be used for a process without a
// terminal.
func initializeIOSet(stdioOptions runtime.StdioOptions) (*ioSet, error) {
	// These must be declared here so that err can be used in the cleanup go
	// routine below. This same err instance must then be assigned to in all
	// the conditionals below, instead of creating a new err in each one.
	var err error
	var read *os.File
	var write *os.File
	ioSet := &ioSet{}

	// If the function returns an error, make sure to close everything added to
	// the ioSet so far.
	defer func() {
		if err != nil {
			if ioSet.InR != nil {
				ioSet.InR.Close()
			}
			if ioSet.InW != nil {
				ioSet.InW.Close()
			}
			if ioSet.OutR != nil {
				ioSet.OutR.Close()
			}
			if ioSet.OutW != nil {
				ioSet.OutW.Close()
			}
			if ioSet.ErrR != nil {
				ioSet.ErrR.Close()
			}
			if ioSet.ErrW != nil {
				ioSet.ErrW.Close()
			}
		}
	}()

	if stdioOptions.CreateIn {
		read, write, err = os.Pipe()
		if err != nil {
			return nil, errors.Wrap(err, "failed call to os.Pipe for stdin")
		}
		ioSet.InR, ioSet.InW = read, write
	}

	if stdioOptions.CreateOut {
		read, write, err = os.Pipe()
		if err != nil {
			return nil, errors.Wrap(err, "failed call to os.Pipe for stdout")
		}
		ioSet.OutW, ioSet.OutR = write, read
	}

	if stdioOptions.CreateErr {
		read, write, err = os.Pipe()
		if err != nil {
			return nil, errors.Wrap(err, "failed call to os.Pipe for stderr")
		}
		ioSet.ErrW, ioSet.ErrR = write, read
	}

	return ioSet, nil
}

// beginCopying starts copying bytes from src to dest in a separate go routine.
// closeDest and closeSource can also be set to specify whether src or dest
// should be closed after the copy has finished.
func (r *runcRuntime) beginCopying(dest io.WriteCloser, closeDest bool, src io.ReadCloser, closeSource bool) {
	go func() {
		if _, err := io.Copy(dest, src); err != nil {
			logrus.Error(err)
		}
		if closeSource {
			src.Close()
		}
		if closeDest {
			dest.Close()
		}
	}()
}

// openFifo opens a FIFO with the given name and readonly setting, creating it
// if specified.
func (r *runcRuntime) openFifo(name string, readonly bool, create bool) (io.ReadWriteCloser, error) {
	var mode int
	if readonly {
		mode = syscall.O_RDONLY
	} else {
		mode = syscall.O_WRONLY
	}
	if create {
		mode |= syscall.O_CREAT
	}
	mode |= syscall.O_NONBLOCK
	pipe, err := fifo.OpenFifo(context.Background(), name, mode, 0)
	if err != nil {
		return nil, errors.Wrap(err, "failed call to fifo.OpenFifo")
	}
	return pipe, nil
}

// createConsoleSocket creates a unix socket in the given process directory and
// returns its path and a listener to it. This socket can then be used to
// receive the container's terminal master file descriptor.
func (r *runcRuntime) createConsoleSocket(processDir string) (listener *net.UnixListener, socketPath string, err error) {
	socketPath = filepath.Join(processDir, "master.sock")
	addr, err := net.ResolveUnixAddr("unix", socketPath)
	if err != nil {
		return nil, "", errors.Wrapf(err, "failed to resolve unix socket at address %s", socketPath)
	}
	listener, err = net.ListenUnix("unix", addr)
	if err != nil {
		return nil, "", errors.Wrapf(err, "failed to listen on unix socket at address %s", socketPath)
	}
	return listener, socketPath, nil
}

// getMasterFromSocket blocks on the given listener's socket until a message is
// sent, then parses the file descriptor representing the terminal master out
// of the message and returns it as a file.
func (r *runcRuntime) getMasterFromSocket(listener *net.UnixListener) (master *os.File, err error) {
	// Accept the listener's connection.
	conn, err := listener.Accept()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get terminal master file descriptor from socket")
	}
	defer conn.Close()
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return nil, errors.New("connection returned from Accept was not a unix socket")
	}

	const maxNameLen = 4096
	var oobSpace = unix.CmsgSpace(4)

	name := make([]byte, maxNameLen)
	oob := make([]byte, oobSpace)

	// Read a message from the unix socket. This blocks until the message is
	// sent.
	n, oobn, _, _, err := unixConn.ReadMsgUnix(name, oob)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read message from unix socket")
	}
	if n >= maxNameLen || oobn != oobSpace {
		return nil, errors.Errorf("read an invalid number of bytes (n=%d oobn=%d)", n, oobn)
	}

	// Truncate the data returned from the message.
	name = name[:n]
	oob = oob[:oobn]

	// Parse the out-of-band data in the message.
	messages, err := unix.ParseSocketControlMessage(oob)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse socket control message for oob %v", oob)
	}
	if len(messages) == 0 {
		return nil, errors.New("did not receive any socket control messages")
	}
	if len(messages) > 1 {
		return nil, errors.Errorf("received more than one socket control message: received %d", len(messages))
	}
	message := messages[0]

	// Parse the file descriptor out of the out-of-band data in the message.
	fds, err := unix.ParseUnixRights(&message)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse file descriptors out of message %v", message)
	}
	if len(fds) == 0 {
		return nil, errors.New("did not receive any file descriptors")
	}
	if len(fds) > 1 {
		return nil, errors.Errorf("received more than one file descriptor: received %d", len(fds))
	}
	fd := uintptr(fds[0])

	return os.NewFile(fd, string(name)), nil
}

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

func ioctl(fd uintptr, flag, data uintptr) error {
	if _, _, err := unix.Syscall(unix.SYS_IOCTL, fd, flag, data); err != 0 {
		return err
	}
	return nil
}

// ptsname is a Go wrapper around the ptsname system call. It returns the name
// of the slave pseudoterminal device corresponding to the given master.
func ptsname(f *os.File) (string, error) {
	var n int32
	if err := ioctl(f.Fd(), unix.TIOCGPTN, uintptr(unsafe.Pointer(&n))); err != nil {
		return "", errors.Wrap(err, "ioctl TIOCGPTN failed for ptsname")
	}
	return fmt.Sprintf("/dev/pts/%d", n), nil
}

// unlockpt is a Go wrapper around the unlockpt system call. It unlocks the
// slave pseudoterminal device corresponding to the given master.
func unlockpt(f *os.File) error {
	var u int32
	if err := ioctl(f.Fd(), unix.TIOCSPTLCK, uintptr(unsafe.Pointer(&u))); err != nil {
		return errors.Wrap(err, "ioctl TIOCSPTLCK failed for unlockpt")
	}
	return nil
}

// pathExists returns true if the given path exists, false if not.
func (r *runcRuntime) pathExists(pathToCheck string) (bool, error) {
	_, err := os.Stat(pathToCheck)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, errors.Wrapf(err, "failed call to Stat for path %s", pathToCheck)
	}
	return true, nil
}
