// Package oslayer defines the interface between the GCS and operating system
// functionality such as filesystem access and networking.
package oslayer

import (
	"io"
	"net"
	"os"
	"syscall"
)

type Signal int

const (
	SIGKILL = Signal(syscall.SIGKILL)
	SIGTERM = Signal(syscall.SIGTERM)
)

// ProcessExitState is an interface describing the state of a process after it
// exits. Since os.ProcessState structs can only be obtained by an actual
// exited process, this interface can be mocked out for testing purposes to
// provide fake exit states.
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

// Link is an interface describing a network link.
type Link interface {
	Name() string
	Index() int
	SetUp() error
	SetDown() error
	SetNamespace(namespace Namespace) error
	Addrs(family int) ([]Addr, error)
	AddAddr(addr Addr) error
	GatewayRoutes(family int) ([]Route, error)
}

// Addr is an interface describing a network address.
type Addr interface {
	IP() net.IP
	String() string
}

// Route is an interface describing a network route.
type Route interface {
	Gw() Addr
	Metric() int
	SetMetric(metric int)
	LinkIndex() int
}

// Namespace is an interface describing a network namespace.
type Namespace interface {
	io.Closer
}

// OS is the interface describing operations that can be performed on and by
// the operating system, such as filesystem access and networking.
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

	// Networking
	GetLinkByName(name string) (Link, error)
	GetLinkByIndex(index int) (Link, error)
	GetCurrentNamespace() (Namespace, error)
	SetCurrentNamespace(namespace Namespace) error
	GetNamespaceFromPid(pid int) (Namespace, error)
	NewRoute(scope uint8, linkIndex int, gateway Addr) Route
	AddRoute(route Route) error
	AddGatewayRoute(gw Addr, link Link, metric int) error
	ParseAddr(s string) (Addr, error)

	// Processes
	Kill(pid int, sig syscall.Signal) error
}
