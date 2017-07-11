// Package mockcore defines a mock implementation of the Core interface.
package mockcore

import (
	"github.com/Microsoft/opengcs/service/gcs/core"
	"github.com/Microsoft/opengcs/service/gcs/oslayer"
	"github.com/Microsoft/opengcs/service/gcs/oslayer/mockos"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/runtime"
)

// CreateContainerCall captures the arguments of CreateContainer.
type CreateContainerCall struct {
	ID       string
	Settings prot.VMHostedContainerSettings
}

// ExecProcessCall captures the arguments of ExecProcess.
type ExecProcessCall struct {
	ID       string
	Params   prot.ProcessParameters
	StdioSet *core.StdioSet
}

// SignalContainerCall captures the arguments of SignalContainer.
type SignalContainerCall struct {
	ID     string
	Signal oslayer.Signal
}

// TerminateProcessCall captures the arguments of TerminateProcess.
type TerminateProcessCall struct {
	Pid int
}

// ListProcessesCall captures the arguments of ListProcesses.
type ListProcessesCall struct {
	ID string
}

// RunExternalProcessCall captures the arguments of RunExternalProcess.
type RunExternalProcessCall struct {
	Params   prot.ProcessParameters
	StdioSet *core.StdioSet
}

// ModifySettingsCall captures the arguments of ModifySettings.
type ModifySettingsCall struct {
	ID      string
	Request prot.ResourceModificationRequestResponse
}

// RegisterContainerExitHookCall captures the arguments of
// RegisterContainerExitHook.
type RegisterContainerExitHookCall struct {
	ID       string
	ExitHook func(oslayer.ProcessExitState)
}

// RegisterProcessExitHookCall captures the arguments of
// RegisterProcessExitHook.
type RegisterProcessExitHookCall struct {
	Pid      int
	ExitHook func(oslayer.ProcessExitState)
}

// MockCore serves as an argument capture mechanism which implements the Core
// interface. Arguments passed to one of its methods are stored to be queried
// later.
type MockCore struct {
	LastCreateContainer           CreateContainerCall
	LastExecProcess               ExecProcessCall
	LastSignalContainer           SignalContainerCall
	LastTerminateProcess          TerminateProcessCall
	LastListProcesses             ListProcessesCall
	LastRunExternalProcess        RunExternalProcessCall
	LastModifySettings            ModifySettingsCall
	LastRegisterContainerExitHook RegisterContainerExitHookCall
	LastRegisterProcessExitHook   RegisterProcessExitHookCall
}

// CreateContainer captures its arguments and returns a nil error.
func (c *MockCore) CreateContainer(id string, settings prot.VMHostedContainerSettings) error {
	c.LastCreateContainer = CreateContainerCall{
		ID:       id,
		Settings: settings,
	}
	return nil
}

// ExecProcess captures its arguments and returns pid 101 and a nil error.
func (c *MockCore) ExecProcess(id string, params prot.ProcessParameters, stdioSet *core.StdioSet) (pid int, err error) {
	c.LastExecProcess = ExecProcessCall{
		ID:       id,
		Params:   params,
		StdioSet: stdioSet,
	}
	return 101, nil
}

// SignalContainer captures its arguments and returns a nil error.
func (c *MockCore) SignalContainer(id string, signal oslayer.Signal) error {
	c.LastSignalContainer = SignalContainerCall{ID: id, Signal: signal}
	return nil
}

// TerminateProcess captures its arguments and returns a nil error.
func (c *MockCore) TerminateProcess(pid int) error {
	c.LastTerminateProcess = TerminateProcessCall{Pid: pid}
	return nil
}

// ListProcesses captures its arguments. It then returns a process with pid
// 101, command "sh -c testexe", CreatedByRuntime true, and IsZombie true, as
// well as a nil error.
func (c *MockCore) ListProcesses(id string) ([]runtime.ContainerProcessState, error) {
	c.LastListProcesses = ListProcessesCall{ID: id}
	return []runtime.ContainerProcessState{
		runtime.ContainerProcessState{
			Pid:              101,
			Command:          []string{"sh", "-c", "testexe"},
			CreatedByRuntime: true,
			IsZombie:         true,
		},
	}, nil
}

// RunExternalProcess captures its arguments and returns pid 101 and a nil
// error.
func (c *MockCore) RunExternalProcess(params prot.ProcessParameters, stdioSet *core.StdioSet) (pid int, err error) {
	c.LastRunExternalProcess = RunExternalProcessCall{
		Params:   params,
		StdioSet: stdioSet,
	}
	return 101, nil
}

// ModifySettings captures its arguments and returns a nil error.
func (c *MockCore) ModifySettings(id string, request prot.ResourceModificationRequestResponse) error {
	c.LastModifySettings = ModifySettingsCall{
		ID:      id,
		Request: request,
	}
	return nil
}

// RegisterContainerExitHook captures its arguments and returns a nil error.
func (c *MockCore) RegisterContainerExitHook(id string, exitHook func(oslayer.ProcessExitState)) error {
	c.LastRegisterContainerExitHook = RegisterContainerExitHookCall{
		ID:       id,
		ExitHook: exitHook,
	}
	return nil
}

// RegisterProcessExitHook captures its arguments, runs the given exit hook on
// a process exit state with exit code 103, and returns a nil error.
func (c *MockCore) RegisterProcessExitHook(pid int, exitHook func(oslayer.ProcessExitState)) error {
	c.LastRegisterProcessExitHook = RegisterProcessExitHookCall{
		Pid:      pid,
		ExitHook: exitHook,
	}
	exitHook(mockos.NewProcessExitState(103))
	return nil
}
