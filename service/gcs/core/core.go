// Package core defines the interface representing the core functionality of a
// GCS-like program.
package core

import (
	"io"

	"github.com/Microsoft/opengcs/service/gcs/oslayer"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/runtime"
)

// StdioPipe is an interface describing a stdio pipe's available methods.
type StdioPipe interface {
	io.ReadWriteCloser
	CloseRead() error
	CloseWrite() error
}

// StdioSet is a structure defining the readers and writers the Core
// implementation should forward a process's stdio through.
type StdioSet struct {
	In  StdioPipe
	Out StdioPipe
	Err StdioPipe
}

// Core is the interface defining the core functionality of the GCS-like
// program. For a real implementation, this may include creating and configuring
// containers. However, it is also easily mocked out for testing.
type Core interface {
	CreateContainer(id string,
		info prot.VMHostedContainerSettings) error

	ExecProcess(id string,
		info prot.ProcessParameters,
		stdioSet *StdioSet) (pid int, err error)

	SignalContainer(id string, signal oslayer.Signal) error

	TerminateProcess(pid int) error

	ListProcesses(id string) ([]runtime.ContainerProcessState, error)

	RunExternalProcess(info prot.ProcessParameters,
		stdioSet *StdioSet) (pid int, err error)

	ModifySettings(id string,
		request prot.ResourceModificationRequestResponse) error

	RegisterContainerExitHook(id string,
		onExit func(oslayer.ProcessExitState)) error
	RegisterProcessExitHook(pid int,
		onExit func(oslayer.ProcessExitState)) error
}
