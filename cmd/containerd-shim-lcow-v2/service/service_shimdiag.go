//go:build windows

package service

import (
	"context"
	"os"
	"strings"

	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/shimdiag"

	"github.com/containerd/errdefs/pkg/errgrpc"
	"go.opencensus.io/trace"
)

// Ensure Service implements the ShimDiagService interface at compile time.
var _ shimdiag.ShimDiagService = &Service{}

// DiagExecInHost executes a process in the host namespace for diagnostic purposes.
// This method is part of the instrumentation layer and business logic is included in diagExecInHostInternal.
func (s *Service) DiagExecInHost(ctx context.Context, request *shimdiag.ExecProcessRequest) (resp *shimdiag.ExecProcessResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "DiagExecInHost")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute(logfields.SandboxID, s.sandboxID),
		trace.StringAttribute(logfields.Args, strings.Join(request.Args, " ")),
		trace.StringAttribute(logfields.Workdir, request.Workdir),
		trace.BoolAttribute(logfields.Terminal, request.Terminal),
		trace.StringAttribute(logfields.Stdin, request.Stdin),
		trace.StringAttribute(logfields.Stdout, request.Stdout),
		trace.StringAttribute(logfields.Stderr, request.Stderr))

	r, e := s.diagExecInHostInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// DiagTasks returns information about all tasks in the shim.
// This method is part of the instrumentation layer and business logic is included in diagTasksInternal.
func (s *Service) DiagTasks(ctx context.Context, request *shimdiag.TasksRequest) (resp *shimdiag.TasksResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "DiagTasks")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute(logfields.SandboxID, s.sandboxID),
		trace.BoolAttribute(logfields.Execs, request.Execs))

	r, e := s.diagTasksInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// DiagShare shares a directory from the host into the sandbox.
// This method is part of the instrumentation layer and business logic is included in diagShareInternal.
func (s *Service) DiagShare(ctx context.Context, request *shimdiag.ShareRequest) (resp *shimdiag.ShareResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "DiagShare")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute(logfields.SandboxID, s.sandboxID),
		trace.StringAttribute(logfields.HostPath, request.HostPath),
		trace.StringAttribute(logfields.UVMPath, request.UvmPath),
		trace.BoolAttribute(logfields.ReadOnly, request.ReadOnly))

	r, e := s.diagShareInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// DiagStacks returns the stack traces of all goroutines in the shim.
// This method is part of the instrumentation layer and business logic is included in diagStacksInternal.
func (s *Service) DiagStacks(ctx context.Context, request *shimdiag.StacksRequest) (resp *shimdiag.StacksResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "DiagStacks")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute(logfields.SandboxID, s.sandboxID))

	r, e := s.diagStacksInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// DiagPid returns the process ID (PID) of the shim for diagnostic purposes.
func (s *Service) DiagPid(ctx context.Context, _ *shimdiag.PidRequest) (resp *shimdiag.PidResponse, err error) {
	_, span := oc.StartSpan(ctx, "DiagPid")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute(logfields.SandboxID, s.sandboxID))

	return &shimdiag.PidResponse{
		Pid: int32(os.Getpid()),
	}, nil
}
