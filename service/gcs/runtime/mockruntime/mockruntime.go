// Package mockruntime defines a mock implementation of the Runtime interface.
package mockruntime

import (
	oci "github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/opengcs/service/gcs/oslayer"
	"github.com/Microsoft/opengcs/service/gcs/oslayer/mockos"
	"github.com/Microsoft/opengcs/service/gcs/runtime"
)

// mockRuntime is an implementation of the Runtime interface which uses runC as
// the container runtime.
type mockRuntime struct{}

// NewRuntime constructs a new mockRuntime with the default settings.
func NewRuntime() *mockRuntime {
	return &mockRuntime{}
}

func (r *mockRuntime) CreateContainer(id string, bundlePath string, stdioOptions runtime.StdioOptions) (pid int, err error) {
	return 101, nil
}

func (r *mockRuntime) StartContainer(id string) error {
	return nil
}

func (r *mockRuntime) ExecProcess(id string, process oci.Process, stdioOptions runtime.StdioOptions) (pid int, err error) {
	return 101, nil
}

func (r *mockRuntime) KillContainer(id string, signal oslayer.Signal) error {
	return nil
}

func (r *mockRuntime) DeleteContainer(id string) error {
	return nil
}

func (r *mockRuntime) DeleteProcess(id string, pid int) error {
	return nil
}

func (r *mockRuntime) PauseContainer(id string) error {
	return nil
}

func (r *mockRuntime) ResumeContainer(id string) error {
	return nil
}

func (r *mockRuntime) GetContainerState(id string) (*runtime.ContainerState, error) {
	state := &runtime.ContainerState{
		OCIVersion: "v1",
		ID:         "abcdef",
		Pid:        123,
		BundlePath: "/path/to/bundle",
		RootfsPath: "/path/to/rootfs",
		Status:     "running",
		Created:    "tuesday",
	}
	return state, nil
}

func (r *mockRuntime) ContainerExists(id string) (bool, error) {
	return true, nil
}

func (r *mockRuntime) ListContainerStates() ([]runtime.ContainerState, error) {
	states := []runtime.ContainerState{
		runtime.ContainerState{
			OCIVersion: "v1",
			ID:         "abcdef",
			Pid:        123,
			BundlePath: "/path/to/bundle",
			RootfsPath: "/path/to/rootfs",
			Status:     "running",
			Created:    "tuesday",
		},
	}
	return states, nil
}

func (r *mockRuntime) GetRunningContainerProcesses(id string) ([]runtime.ContainerProcessState, error) {
	states := []runtime.ContainerProcessState{
		runtime.ContainerProcessState{
			Pid:              123,
			Command:          []string{"cat", "file"},
			CreatedByRuntime: true,
			IsZombie:         true,
		},
	}
	return states, nil
}

func (r *mockRuntime) GetAllContainerProcesses(id string) ([]runtime.ContainerProcessState, error) {
	states := []runtime.ContainerProcessState{
		runtime.ContainerProcessState{
			Pid:              123,
			Command:          []string{"cat", "file"},
			CreatedByRuntime: true,
			IsZombie:         true,
		},
	}
	return states, nil
}

func (r *mockRuntime) WaitOnProcess(id string, pid int) (oslayer.ProcessExitState, error) {
	state := mockos.NewProcessExitState(123)
	return state, nil
}

func (r *mockRuntime) WaitOnContainer(id string) (oslayer.ProcessExitState, error) {
	state := mockos.NewProcessExitState(123)
	return state, nil
}

func (r *mockRuntime) GetInitPid(id string) (pid int, err error) {
	return 123, nil
}

func (r *mockRuntime) GetStdioPipes(id string, pid int) (*runtime.StdioPipes, error) {
	return &runtime.StdioPipes{}, nil
}
