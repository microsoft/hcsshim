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

var _ runtime.Runtime = &mockRuntime{}

// NewRuntime constructs a new mockRuntime with the default settings.
func NewRuntime() *mockRuntime {
	return &mockRuntime{}
}

type container struct {
	id string
}

func (r *mockRuntime) CreateContainer(id string, bundlePath string, stdioOptions runtime.StdioOptions) (c runtime.Container, err error) {
	return &container{id: id}, nil
}

func (c *container) Start() error {
	return nil
}

func (c *container) ID() string {
	return c.id
}

func (c *container) Pid() int {
	return 101
}

func (c *container) ExecProcess(process oci.Process, stdioOptions runtime.StdioOptions) (p runtime.Process, err error) {
	return &container{}, nil
}

func (c *container) Kill(signal oslayer.Signal) error {
	return nil
}

func (c *container) Delete() error {
	return nil
}

func (c *container) Pause() error {
	return nil
}

func (c *container) Resume() error {
	return nil
}

func (c *container) GetState() (*runtime.ContainerState, error) {
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

func (c *container) Exists() (bool, error) {
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

func (c *container) GetRunningProcesses() ([]runtime.ContainerProcessState, error) {
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

func (c *container) GetAllProcesses() ([]runtime.ContainerProcessState, error) {
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

func (c *container) Wait() (oslayer.ProcessExitState, error) {
	state := mockos.NewProcessExitState(123)
	return state, nil
}

func (c *container) GetStdioPipes() (*runtime.StdioPipes, error) {
	return &runtime.StdioPipes{}, nil
}
