//go:build windows

package service

import (
	"context"
	"os"
	"strings"

	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/shimdiag"

	"github.com/containerd/errdefs/pkg/errgrpc"
	"go.opencensus.io/trace"
)

// Ensure Service implements the ShimDiagService interface at compile time.
var _ shimdiag.ShimDiagService = &Service{}

// DiagExecInHost executes a process in the host namespace for diagnostic purposes.
func (s *Service) DiagExecInHost(ctx context.Context, request *shimdiag.ExecProcessRequest) (resp *shimdiag.ExecProcessResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "DiagExecInHost")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("sandbox-id", s.sandboxID),
		trace.StringAttribute("args", strings.Join(request.Args, " ")),
		trace.StringAttribute("workdir", request.Workdir),
		trace.BoolAttribute("terminal", request.Terminal),
		trace.StringAttribute("stdin", request.Stdin),
		trace.StringAttribute("stdout", request.Stdout),
		trace.StringAttribute("stderr", request.Stderr))

	r, e := s.diagExecInHostInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// DiagTasks returns information about all tasks in the shim.
func (s *Service) DiagTasks(ctx context.Context, request *shimdiag.TasksRequest) (resp *shimdiag.TasksResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "DiagTasks")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("sandbox-id", s.sandboxID),
		trace.BoolAttribute("execs", request.Execs))

	r, e := s.diagTasksInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// DiagShare shares a directory from the host into the sandbox.
func (s *Service) DiagShare(ctx context.Context, request *shimdiag.ShareRequest) (resp *shimdiag.ShareResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "DiagShare")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("sandbox-id", s.sandboxID),
		trace.StringAttribute("host-path", request.HostPath),
		trace.StringAttribute("uvm-path", request.UvmPath),
		trace.BoolAttribute("readonly", request.ReadOnly))

	r, e := s.diagShareInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// DiagStacks returns the stack traces of all goroutines in the shim.
func (s *Service) DiagStacks(ctx context.Context, request *shimdiag.StacksRequest) (resp *shimdiag.StacksResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "DiagStacks")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute("sandbox-id", s.sandboxID))

	r, e := s.diagStacksInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// DiagPid returns the process ID (PID) of the shim for diagnostic purposes.
func (s *Service) DiagPid(ctx context.Context, _ *shimdiag.PidRequest) (resp *shimdiag.PidResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "DiagPid")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute("sandbox-id", s.sandboxID))

	return &shimdiag.PidResponse{
		Pid: int32(os.Getpid()),
	}, nil
}
