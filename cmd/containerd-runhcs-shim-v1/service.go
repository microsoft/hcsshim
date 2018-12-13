package shim

import (
	"context"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/runtime/v2/task"
	google_protobuf1 "github.com/gogo/protobuf/types"
)

var _ = (task.TaskService)(&service{})

type service struct {
}

func (s *service) State(ctx context.Context, req *task.StateRequest) (*task.StateResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) Create(ctx context.Context, req *task.CreateTaskRequest) (*task.CreateTaskResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) Start(ctx context.Context, req *task.StartRequest) (*task.StartResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) Delete(ctx context.Context, req *task.DeleteRequest) (*task.DeleteResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) Pids(ctx context.Context, req *task.PidsRequest) (*task.PidsResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) Pause(ctx context.Context, req *task.PauseRequest) (*google_protobuf1.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) Resume(ctx context.Context, req *task.ResumeRequest) (*google_protobuf1.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) Checkpoint(ctx context.Context, req *task.CheckpointTaskRequest) (*google_protobuf1.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) Kill(ctx context.Context, req *task.KillRequest) (*google_protobuf1.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) Exec(ctx context.Context, req *task.ExecProcessRequest) (*google_protobuf1.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) ResizePty(ctx context.Context, req *task.ResizePtyRequest) (*google_protobuf1.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) CloseIO(ctx context.Context, req *task.CloseIORequest) (*google_protobuf1.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) Update(ctx context.Context, req *task.UpdateTaskRequest) (*google_protobuf1.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) Wait(ctx context.Context, req *task.WaitRequest) (*task.WaitResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) Stats(ctx context.Context, req *task.StatsRequest) (*task.StatsResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) Connect(ctx context.Context, req *task.ConnectRequest) (*task.ConnectResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) Shutdown(ctx context.Context, req *task.ShutdownRequest) (*google_protobuf1.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}
