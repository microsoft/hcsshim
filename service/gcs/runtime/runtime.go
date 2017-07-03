// Package runtime defines the interface between the GCS and an OCI container
// runtime.
package runtime

import (
	"io"

	oci "github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/opengcs/service/gcs/oslayer"
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

// StdioOptions specify how the runtime should handle stdio for the process.
type StdioOptions struct {
	CreateIn  bool
	CreateOut bool
	CreateErr bool
}

// StdioPipes contain the interfaces for reading from and writing to a
// process's stdio.
type StdioPipes struct {
	In  io.WriteCloser
	Out io.ReadCloser
	Err io.ReadCloser
}

// Runtime is the interface defining commands over an OCI container runtime,
// such as runC.
type Runtime interface {
	CreateContainer(id string, bundlePath string, stdioOptions StdioOptions) (pid int, err error)
	StartContainer(id string) error
	ExecProcess(id string, process oci.Process, stdioOptions StdioOptions) (pid int, err error)
	KillContainer(id string, signal oslayer.Signal) error
	DeleteContainer(id string) error
	DeleteProcess(id string, pid int) error
	PauseContainer(id string) error
	ResumeContainer(id string) error
	GetContainerState(id string) (*ContainerState, error)
	ContainerExists(id string) (bool, error)
	ListContainerStates() ([]ContainerState, error)
	GetRunningContainerProcesses(id string) ([]ContainerProcessState, error)
	GetAllContainerProcesses(id string) ([]ContainerProcessState, error)
	WaitOnProcess(id string, pid int) (oslayer.ProcessExitState, error)
	WaitOnContainer(id string) (oslayer.ProcessExitState, error)

	GetInitPid(id string) (pid int, err error)
	GetStdioPipes(id string, pid int) (*StdioPipes, error)
}
