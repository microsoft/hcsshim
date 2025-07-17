//go:build linux
// +build linux

package runtime

import (
	"errors"
	"io"
	"syscall"

	"github.com/Microsoft/hcsshim/internal/bridgeutils/gcserr"
	"github.com/Microsoft/hcsshim/internal/guest/stdio"
	oci "github.com/opencontainers/runtime-spec/specs-go"
)

var (
	ErrContainerAlreadyExists = gcserr.WrapHresult(errors.New("container already exist"), gcserr.HrVmcomputeSystemAlreadyExists)
	ErrContainerDoesNotExist  = gcserr.WrapHresult(errors.New("container does not exist"), gcserr.HrVmcomputeSystemNotFound)
	ErrContainerStillRunning  = gcserr.WrapHresult(errors.New("container still running"), gcserr.HrVmcomputeInvalidState)
	ErrContainerNotRunning    = gcserr.WrapHresult(errors.New("container not running"), gcserr.HrVmcomputeSystemAlreadyStopped)
	ErrContainerNotStopped    = gcserr.WrapHresult(errors.New("container not stopped"), gcserr.HrVmcomputeInvalidState)
	ErrInvalidContainerID     = gcserr.WrapHresult(errors.New("invalid container ID"), gcserr.HrErrInvalidArg)
)

// ContainerState gives information about a container created by a Runtime.
type ContainerState struct {
	OCIVersion string
	ID         string
	Pid        int
	BundlePath string
	RootfsPath string
	Status     string
	Created    string
}

// ContainerProcessState gives information about a process created by a
// Runtime.
type ContainerProcessState struct {
	Pid              int
	Command          []string
	CreatedByRuntime bool
	IsZombie         bool
}

// StdioPipes contain the interfaces for reading from and writing to a
// process's stdio.
type StdioPipes struct {
	In  io.WriteCloser
	Out io.ReadCloser
	Err io.ReadCloser
}

// Process is an interface to manipulate process state.
type Process interface {
	Wait() (int, error)
	Pid() int
	Delete() error
	Tty() *stdio.TtyRelay
	PipeRelay() *stdio.PipeRelay
}

// Container is an interface to manipulate container state.
type Container interface {
	Process
	ID() string
	Exists() (bool, error)
	Start() error
	ExecProcess(process *oci.Process, stdioSet *stdio.ConnectionSet) (p Process, err error)
	Kill(signal syscall.Signal) error
	Pause() error
	Resume() error
	GetState() (*ContainerState, error)
	GetRunningProcesses() ([]ContainerProcessState, error)
	GetAllProcesses() ([]ContainerProcessState, error)
	GetInitProcess() (Process, error)
	Update(resources interface{}) error
}

// Runtime is the interface defining commands over an OCI container runtime,
// such as runC.
type Runtime interface {
	CreateContainer(id string, bundlePath string, stdioSet *stdio.ConnectionSet) (c Container, err error)
	ListContainerStates() ([]ContainerState, error)
}
