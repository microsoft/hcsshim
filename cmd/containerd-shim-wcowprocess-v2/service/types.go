//go:build windows && wcowprocess

package service

import (
	"context"

	"github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats"
	"github.com/Microsoft/hcsshim/internal/controller/process"
	container "github.com/Microsoft/hcsshim/internal/controller/windowscontainer/processisolated"

	"github.com/containerd/containerd/api/runtime/task/v3"
	containerdtypes "github.com/containerd/containerd/api/types/task"
	"github.com/opencontainers/runtime-spec/specs-go"
)

// containerController is the subset of the container controller that Service
// depends on. Implemented by [*processisolated.Controller].
type containerController interface {
	Create(ctx context.Context, spec *specs.Spec, req *task.CreateTaskRequest, copts *container.CreateOpts) error
	Start(ctx context.Context, events chan interface{}) (uint32, error)
	Wait(ctx context.Context)
	Update(ctx context.Context, resources interface{}) error
	NewProcess(execID string) (*process.Controller, error)
	GetProcess(execID string) (*process.Controller, error)
	Pids(ctx context.Context) ([]*containerdtypes.ProcessInfo, error)
	Stats(ctx context.Context) (*stats.Statistics, error)
	KillProcess(ctx context.Context, execID string, signal uint32, all bool) error
	DeleteProcess(ctx context.Context, execID string) (*task.StateResponse, error)
}
