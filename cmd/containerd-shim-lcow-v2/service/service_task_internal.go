//go:build windows

package service

import (
	"context"

	"github.com/containerd/containerd/api/runtime/task/v3"
	"github.com/containerd/errdefs"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *Service) stateInternal(_ context.Context, _ *task.StateRequest) (*task.StateResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *Service) createInternal(_ context.Context, _ *task.CreateTaskRequest) (*task.CreateTaskResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *Service) startInternal(_ context.Context, _ *task.StartRequest) (*task.StartResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *Service) deleteInternal(_ context.Context, _ *task.DeleteRequest) (*task.DeleteResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *Service) pidsInternal(_ context.Context, _ *task.PidsRequest) (*task.PidsResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *Service) pauseInternal(_ context.Context, _ *task.PauseRequest) (*emptypb.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *Service) resumeInternal(_ context.Context, _ *task.ResumeRequest) (*emptypb.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *Service) checkpointInternal(_ context.Context, _ *task.CheckpointTaskRequest) (*emptypb.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *Service) killInternal(_ context.Context, _ *task.KillRequest) (*emptypb.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *Service) execInternal(_ context.Context, _ *task.ExecProcessRequest) (*emptypb.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *Service) resizePtyInternal(_ context.Context, _ *task.ResizePtyRequest) (*emptypb.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *Service) closeIOInternal(_ context.Context, _ *task.CloseIORequest) (*emptypb.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *Service) updateInternal(_ context.Context, _ *task.UpdateTaskRequest) (*emptypb.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *Service) waitInternal(_ context.Context, _ *task.WaitRequest) (*task.WaitResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *Service) statsInternal(_ context.Context, _ *task.StatsRequest) (*task.StatsResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *Service) connectInternal(_ context.Context, _ *task.ConnectRequest) (*task.ConnectResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *Service) shutdownInternal(_ context.Context, _ *task.ShutdownRequest) (*emptypb.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}
