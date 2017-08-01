// Package core defines the interface representing the core functionality of a
// GCS-like program.
package core

import (
	"github.com/Microsoft/opengcs/service/gcs/oslayer"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/runtime"
	"github.com/Microsoft/opengcs/service/gcs/stdio"
)

// Core is the interface defining the core functionality of the GCS-like
// program. For a real implementation, this may include creating and configuring
// containers. However, it is also easily mocked out for testing.
type Core interface {
	CreateContainer(id string, info prot.VMHostedContainerSettings) error
	ExecProcess(id string, info prot.ProcessParameters, stdioSet *stdio.ConnectionSet) (pid int, err error)
	SignalContainer(id string, signal oslayer.Signal) error
	SignalProcess(pid int, options prot.SignalProcessOptions) error
	ListProcesses(id string) ([]runtime.ContainerProcessState, error)
	RunExternalProcess(info prot.ProcessParameters, stdioSet *stdio.ConnectionSet) (pid int, err error)
	ModifySettings(id string, request prot.ResourceModificationRequestResponse) error
	RegisterContainerExitHook(id string, onExit func(oslayer.ProcessExitState)) error
	RegisterProcessExitHook(pid int, onExit func(oslayer.ProcessExitState)) error
	ResizeConsole(pid int, height, width uint16) error
}
