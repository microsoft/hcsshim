// Package mockcore defines a mock implementation of the Core interface.
package mockcore

import (
	"github.com/Microsoft/opengcs/service/gcs/core"
	"github.com/Microsoft/opengcs/service/gcs/oslayer"
	"github.com/Microsoft/opengcs/service/gcs/oslayer/mockos"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/runtime"
)

type CreateContainerCall struct {
	ID       string
	Settings prot.VmHostedContainerSettings
}

type ExecProcessCall struct {
	ID       string
	Params   prot.ProcessParameters
	StdioSet *core.StdioSet
}

type SignalContainerCall struct {
	ID     string
	Signal oslayer.Signal
}

type TerminateProcessCall struct {
	Pid int
}

type ListProcessesCall struct {
	ID string
}

type RunExternalProcessCall struct {
	Params   prot.ProcessParameters
	StdioSet *core.StdioSet
}

type ModifySettingsCall struct {
	ID      string
	Request prot.ResourceModificationRequestResponse
}

type RegisterContainerExitHookCall struct {
	ID       string
	ExitHook func(oslayer.ProcessExitState)
}

type RegisterProcessExitHookCall struct {
	Pid      int
	ExitHook func(oslayer.ProcessExitState)
}

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

func (c *MockCore) CreateContainer(id string, settings prot.VmHostedContainerSettings) error {
	c.LastCreateContainer = CreateContainerCall{
		ID:       id,
		Settings: settings,
	}
	return nil
}

func (c *MockCore) ExecProcess(id string, params prot.ProcessParameters, stdioSet *core.StdioSet) (pid int, err error) {
	c.LastExecProcess = ExecProcessCall{
		ID:       id,
		Params:   params,
		StdioSet: stdioSet,
	}
	return 101, nil
}

func (c *MockCore) SignalContainer(id string, signal oslayer.Signal) error {
	c.LastSignalContainer = SignalContainerCall{ID: id, Signal: signal}
	return nil
}

func (c *MockCore) TerminateProcess(pid int) error {
	c.LastTerminateProcess = TerminateProcessCall{Pid: pid}
	return nil
}

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

func (c *MockCore) RunExternalProcess(params prot.ProcessParameters, stdioSet *core.StdioSet) (pid int, err error) {
	c.LastRunExternalProcess = RunExternalProcessCall{
		Params:   params,
		StdioSet: stdioSet,
	}
	return 101, nil
}

func (c *MockCore) ModifySettings(id string, request prot.ResourceModificationRequestResponse) error {
	c.LastModifySettings = ModifySettingsCall{
		ID:      id,
		Request: request,
	}
	return nil
}

func (c *MockCore) RegisterContainerExitHook(id string, exitHook func(oslayer.ProcessExitState)) error {
	c.LastRegisterContainerExitHook = RegisterContainerExitHookCall{
		ID:       id,
		ExitHook: exitHook,
	}
	return nil
}

func (c *MockCore) RegisterProcessExitHook(pid int, exitHook func(oslayer.ProcessExitState)) error {
	c.LastRegisterProcessExitHook = RegisterProcessExitHookCall{
		Pid:      pid,
		ExitHook: exitHook,
	}
	exitHook(mockos.NewProcessExitState(103))
	return nil
}

func (c *MockCore) CleanupContainer(id string) error {
	return nil
}
