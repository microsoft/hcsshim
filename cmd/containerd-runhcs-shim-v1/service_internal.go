package shim

import (
	"context"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/runtime/v2/task"
	google_protobuf1 "github.com/gogo/protobuf/types"
)

func (s *service) stateInternal(ctx context.Context, req *task.StateRequest) (*task.StateResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) createInternal(ctx context.Context, req *task.CreateTaskRequest) (*task.CreateTaskResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) startInternal(ctx context.Context, req *task.StartRequest) (*task.StartResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) deleteInternal(ctx context.Context, req *task.DeleteRequest) (*task.DeleteResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) pidsInternal(ctx context.Context, req *task.PidsRequest) (*task.PidsResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) pauseInternal(ctx context.Context, req *task.PauseRequest) (*google_protobuf1.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) resumeInternal(ctx context.Context, req *task.ResumeRequest) (*google_protobuf1.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) checkpointInternal(ctx context.Context, req *task.CheckpointTaskRequest) (*google_protobuf1.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) killInternal(ctx context.Context, req *task.KillRequest) (*google_protobuf1.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) execInternal(ctx context.Context, req *task.ExecProcessRequest) (*google_protobuf1.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) resizePtyInternal(ctx context.Context, req *task.ResizePtyRequest) (*google_protobuf1.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) closeIOInternal(ctx context.Context, req *task.CloseIORequest) (*google_protobuf1.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) updateInternal(ctx context.Context, req *task.UpdateTaskRequest) (*google_protobuf1.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) waitInternal(ctx context.Context, req *task.WaitRequest) (*task.WaitResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) statsInternal(ctx context.Context, req *task.StatsRequest) (*task.StatsResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) connectInternal(ctx context.Context, req *task.ConnectRequest) (*task.ConnectResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) shutdownInternal(ctx context.Context, req *task.ShutdownRequest) (*google_protobuf1.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}
