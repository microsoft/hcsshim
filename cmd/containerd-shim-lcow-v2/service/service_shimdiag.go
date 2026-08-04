//go:build windows && lcow

package service

import (
	"context"
	"os"
	"strings"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/ot"
	"github.com/Microsoft/hcsshim/internal/shimdiag"

	"github.com/containerd/errdefs/pkg/errgrpc"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/attribute"
)

// Ensure Service implements the ShimDiagService interface at compile time.
var _ shimdiag.ShimDiagService = &Service{}

// DiagExecInHost executes a process in the host namespace for diagnostic purposes.
// This method is part of the instrumentation layer and business logic is included in diagExecInHostInternal.
func (s *Service) DiagExecInHost(ctx context.Context, request *shimdiag.ExecProcessRequest) (resp *shimdiag.ExecProcessResponse, err error) {
	ctx, span := ot.StartSpan(ctx, "DiagExecInHost")
	defer span.End()
	defer func() { ot.SetSpanStatus(span, err) }()

	span.SetAttributes(
		attribute.String(logfields.SandboxID, s.sandboxID),
		attribute.String(logfields.Args, strings.Join(request.Args, " ")),
		attribute.String(logfields.Workdir, request.Workdir),
		attribute.Bool(logfields.Terminal, request.Terminal),
		attribute.String(logfields.Stdin, request.Stdin),
		attribute.String(logfields.Stdout, request.Stdout),
		attribute.String(logfields.Stderr, request.Stderr))

	// Set the sandbox ID in the logger context for all subsequent logs in this request.
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, s.sandboxID))

	r, e := s.diagExecInHostInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// DiagTasks returns information about all tasks in the shim.
// This method is part of the instrumentation layer and business logic is included in diagTasksInternal.
func (s *Service) DiagTasks(ctx context.Context, request *shimdiag.TasksRequest) (resp *shimdiag.TasksResponse, err error) {
	ctx, span := ot.StartSpan(ctx, "DiagTasks")
	defer span.End()
	defer func() { ot.SetSpanStatus(span, err) }()

	span.SetAttributes(
		attribute.String(logfields.SandboxID, s.sandboxID),
		attribute.Bool(logfields.Execs, request.Execs))

	// Set the sandbox ID in the logger context for all subsequent logs in this request.
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, s.sandboxID))

	r, e := s.diagTasksInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// DiagShare shares a directory from the host into the sandbox.
// This method is part of the instrumentation layer and business logic is included in diagShareInternal.
func (s *Service) DiagShare(ctx context.Context, request *shimdiag.ShareRequest) (resp *shimdiag.ShareResponse, err error) {
	ctx, span := ot.StartSpan(ctx, "DiagShare")
	defer span.End()
	defer func() { ot.SetSpanStatus(span, err) }()

	span.SetAttributes(
		attribute.String(logfields.SandboxID, s.sandboxID),
		attribute.String(logfields.HostPath, request.HostPath),
		attribute.String(logfields.UVMPath, request.UvmPath),
		attribute.Bool(logfields.ReadOnly, request.ReadOnly))

	// Set the sandbox ID in the logger context for all subsequent logs in this request.
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, s.sandboxID))

	r, e := s.diagShareInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// DiagStacks returns the stack traces of all goroutines in the shim.
// This method is part of the instrumentation layer and business logic is included in diagStacksInternal.
func (s *Service) DiagStacks(ctx context.Context, _ *shimdiag.StacksRequest) (resp *shimdiag.StacksResponse, err error) {
	ctx, span := ot.StartSpan(ctx, "DiagStacks")
	defer span.End()
	defer func() { ot.SetSpanStatus(span, err) }()

	span.SetAttributes(attribute.String(logfields.SandboxID, s.sandboxID))

	// Set the sandbox ID in the logger context for all subsequent logs in this request.
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, s.sandboxID))

	r, e := s.diagStacksInternal(ctx)
	return r, errgrpc.ToGRPC(e)
}

// DiagPid returns the process ID (PID) of the shim for diagnostic purposes.
func (s *Service) DiagPid(ctx context.Context, _ *shimdiag.PidRequest) (resp *shimdiag.PidResponse, err error) {
	_, span := ot.StartSpan(ctx, "DiagPid")
	defer span.End()
	defer func() { ot.SetSpanStatus(span, err) }()

	span.SetAttributes(attribute.String(logfields.SandboxID, s.sandboxID))

	return &shimdiag.PidResponse{
		Pid: int32(os.Getpid()),
	}, nil
}
