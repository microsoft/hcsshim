//go:build windows

package service

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/shimdiag"
	"github.com/containerd/errdefs"
)

// diagExecInHostInternal is the implementation for DiagExecInHost.
//
// It is used to create an exec session into the hosting UVM.
func (s *Service) diagExecInHostInternal(ctx context.Context, request *shimdiag.ExecProcessRequest) (*shimdiag.ExecProcessResponse, error) {
	ec, err := s.vmController.ExecIntoHost(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to exec into host: %w", err)
	}

	return &shimdiag.ExecProcessResponse{ExitCode: int32(ec)}, nil
}

func (s *Service) diagTasksInternal(ctx context.Context, request *shimdiag.TasksRequest) (*shimdiag.TasksResponse, error) {
	_ = ctx
	_ = request
	return nil, errdefs.ErrNotImplemented
}

func (s *Service) diagShareInternal(ctx context.Context, request *shimdiag.ShareRequest) (*shimdiag.ShareResponse, error) {
	_ = ctx
	_ = request
	return nil, errdefs.ErrNotImplemented
}

func (s *Service) diagStacksInternal(ctx context.Context, request *shimdiag.StacksRequest) (*shimdiag.StacksResponse, error) {
	_ = ctx
	_ = request
	return nil, errdefs.ErrNotImplemented
}
