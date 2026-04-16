//go:build windows && lcow

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

func (s *Service) diagTasksInternal(_ context.Context, _ *shimdiag.TasksRequest) (*shimdiag.TasksResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *Service) diagShareInternal(_ context.Context, _ *shimdiag.ShareRequest) (*shimdiag.ShareResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *Service) diagStacksInternal(_ context.Context, _ *shimdiag.StacksRequest) (*shimdiag.StacksResponse, error) {
	return nil, errdefs.ErrNotImplemented
}
