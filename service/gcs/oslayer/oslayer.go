// Package oslayer defines the interface between the GCS and operating system
// functionality such as filesystem access.
package oslayer

import (
	"io"
	"os"
	"syscall"
)

// Signal represents signals which may be sent to processes, such as SIGKILL or
// SIGTERM.
type Signal int

const (
	// SIGKILL defines the Signal for a non-ignorable exit.
	SIGKILL = Signal(syscall.SIGKILL)
	// SIGTERM defines the Signal for an exit request.
	SIGTERM = Signal(syscall.SIGTERM)
)

// ProcessExitState is an interface describing the state of a process after it
// exits. Since os.ProcessState structs can only be obtained by an actual exited
// process, this interface can be mocked out for testing purposes to provide
// fake exit states.
type ProcessExitState interface {
	ExitCode() int
}

// File is an interface describing the methods exposed by a file on the system.
type File interface {
	io.ReadWriteCloser
}

// Process is an interface describing the methods exposed by a process on the
// system.
type Process interface {
	Pid() int
}

// Cmd is an interface describing a command which can be run on the system.
type Cmd interface {
	SetDir(dir string)
	SetEnv(env []string)
	StdinPipe() (io.WriteCloser, error)
	StdoutPipe() (io.ReadCloser, error)
	StderrPipe() (io.ReadCloser, error)
	SetStdin(stdin io.Reader)
	SetStdout(stdout io.Writer)
	SetStderr(stderr io.Writer)
	ExitState() ProcessExitState
	Process() Process
	Start() error
	Wait() error
	Run() error
	Output() ([]byte, error)
	CombinedOutput() ([]byte, error)
}

// OS is the interface describing operations that can be performed on and by the
// operating system, such as filesystem access and networking.
type OS interface {
	// Filesystem
	OpenFile(name string, flag int, perm os.FileMode) (File, error)
	Command(name string, arg ...string) Cmd
	MkdirAll(path string, perm os.FileMode) error
	RemoveAll(path string) error
	Create(name string) (File, error)
	ReadDir(dirname string) ([]os.FileInfo, error)
	Mount(source string, target string, fstype string, flags uintptr, data string) (err error)
	Unmount(target string, flags int) (err error)
	PathExists(name string) (bool, error)
	PathIsMounted(name string) (bool, error)
	Link(oldname, newname string) error

	// Processes
	Kill(pid int, sig syscall.Signal) error
}
