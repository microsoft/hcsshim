//go:build windows

package main

import (
	"context"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	task "github.com/containerd/containerd/api/runtime/task/v2"
	"github.com/containerd/errdefs"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/Microsoft/hcsshim/internal/extendedtask"
	"github.com/Microsoft/hcsshim/internal/otelutil"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
)

type ServiceOptions struct {
	Events    publisher
	TID       string
	IsSandbox bool
}

type ServiceOption func(*ServiceOptions)

func WithEventPublisher(e publisher) ServiceOption {
	return func(o *ServiceOptions) {
		o.Events = e
	}
}
func WithTID(tid string) ServiceOption {
	return func(o *ServiceOptions) {
		o.TID = tid
	}
}
func WithIsSandbox(s bool) ServiceOption {
	return func(o *ServiceOptions) {
		o.IsSandbox = s
	}
}

type service struct {
	events publisher
	// tid is the original task id to be served. This can either be a single
	// task or represent the POD sandbox task id. The first call to Create MUST
	// match this id or the shim is considered to be invalid.
	//
	// This MUST be treated as readonly for the lifetime of the shim.
	tid string
	// isSandbox specifies if `tid` is a POD sandbox. If `false` the shim will
	// reject all calls to `Create` where `tid` does not match. If `true`
	// multiple calls to `Create` are allowed as long as the workload containers
	// all have the same parent task id.
	//
	// This MUST be treated as readonly for the lifetime of the shim.
	isSandbox bool

	// taskOrPod is either the `pod` this shim is tracking if `isSandbox ==
	// true` or it is the `task` this shim is tracking. If no call to `Create`
	// has taken place yet `taskOrPod.Load()` MUST return `nil`.
	taskOrPod atomic.Value

	// cl is the create lock. Since each shim MUST only track a single task or
	// POD. `cl` is used to create the task or POD sandbox. It SHOULD NOT be
	// taken when creating tasks in a POD sandbox as they can happen
	// concurrently.
	cl sync.Mutex

	// shutdown is closed to signal a shutdown request is received
	shutdown chan struct{}
	// shutdownOnce is responsible for closing `shutdown` and any other necessary cleanup
	shutdownOnce sync.Once
	// gracefulShutdown dictates whether to shutdown gracefully and clean up resources
	// or exit immediately
	gracefulShutdown bool
}

var _ task.TaskService = &service{}

func NewService(o ...ServiceOption) (svc *service, err error) {
	var opts ServiceOptions
	for _, op := range o {
		op(&opts)
	}

	svc = &service{
		events:    opts.Events,
		tid:       opts.TID,
		isSandbox: opts.IsSandbox,
		shutdown:  make(chan struct{}),
	}
	return svc, nil
}

func (s *service) State(ctx context.Context, req *task.StateRequest) (resp *task.StateResponse, err error) {
	ctx, span := otelutil.StartSpan(ctx, "State", trace.WithAttributes(
		attribute.String("tid", req.ID),
		attribute.String("eid", req.ExecID)))
	defer span.End()
	defer func() {
		if resp != nil {
			span.SetAttributes(
				attribute.String("status", resp.Status.String()),
				attribute.Int64("exitStatus", int64(resp.ExitStatus)),
				attribute.String("exitedAt", resp.ExitedAt.String()))
		}
		otelutil.SetSpanStatus(span, err)
	}()

	if s.isSandbox {
		span.SetAttributes(attribute.String("pod-id", s.tid))
	}

	r, e := s.stateInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Create(ctx context.Context, req *task.CreateTaskRequest) (resp *task.CreateTaskResponse, err error) {
	ctx, span := otelutil.StartSpan(ctx, "Create", trace.WithAttributes(
		attribute.String("tid", req.ID),
		attribute.String("bundle", req.Bundle),
		// attribute.String("rootfs", req.Rootfs), TODO: JTERRY75 -
		// OpenCensus doesnt support slice like our logrus hook
		attribute.Bool("terminal", req.Terminal),
		attribute.String("stdin", req.Stdin),
		attribute.String("stdout", req.Stdout),
		attribute.String("stderr", req.Stderr),
		attribute.String("checkpoint", req.Checkpoint),
		attribute.String("parentcheckpoint", req.ParentCheckpoint)))
	defer span.End()
	defer func() {
		if resp != nil {
			span.SetAttributes(attribute.Int64("pid", int64(resp.Pid)))
		}
		otelutil.SetSpanStatus(span, err)
	}()

	if s.isSandbox {
		span.SetAttributes(attribute.String("pod-id", s.tid))
	}

	r, e := s.createInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Start(ctx context.Context, req *task.StartRequest) (resp *task.StartResponse, err error) {
	ctx, span := otelutil.StartSpan(ctx, "Start", trace.WithAttributes(
		attribute.String("tid", req.ID),
		attribute.String("eid", req.ExecID)))
	defer span.End()
	defer func() {
		if resp != nil {
			span.SetAttributes(attribute.Int64("pid", int64(resp.Pid)))
		}
		otelutil.SetSpanStatus(span, err)
	}()

	if s.isSandbox {
		span.SetAttributes(attribute.String("pod-id", s.tid))
	}

	r, e := s.startInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Delete(ctx context.Context, req *task.DeleteRequest) (resp *task.DeleteResponse, err error) {
	ctx, span := otelutil.StartSpan(ctx, "Delete", trace.WithAttributes(
		attribute.String("tid", req.ID),
		attribute.String("eid", req.ExecID)))
	defer span.End()
	defer func() {
		if resp != nil {
			span.SetAttributes(
				attribute.Int64("pid", int64(resp.Pid)),
				attribute.Int64("exitStatus", int64(resp.ExitStatus)),
				attribute.String("exitedAt", resp.ExitedAt.String()))
		}
		otelutil.SetSpanStatus(span, err)
	}()

	if s.isSandbox {
		span.SetAttributes(attribute.String("pod-id", s.tid))
	}

	r, e := s.deleteInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Pids(ctx context.Context, req *task.PidsRequest) (_ *task.PidsResponse, err error) {
	ctx, span := otelutil.StartSpan(ctx, "Pids", trace.WithAttributes(
		attribute.String("tid", req.ID)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if s.isSandbox {
		span.SetAttributes(attribute.String("pod-id", s.tid))
	}

	r, e := s.pidsInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Pause(ctx context.Context, req *task.PauseRequest) (_ *emptypb.Empty, err error) {
	ctx, span := otelutil.StartSpan(ctx, "Pause", trace.WithAttributes(
		attribute.String("tid", req.ID)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if s.isSandbox {
		span.SetAttributes(attribute.String("pod-id", s.tid))
	}

	r, e := s.pauseInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Resume(ctx context.Context, req *task.ResumeRequest) (_ *emptypb.Empty, err error) {
	ctx, span := otelutil.StartSpan(ctx, "Resume", trace.WithAttributes(
		attribute.String("tid", req.ID)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if s.isSandbox {
		span.SetAttributes(attribute.String("pod-id", s.tid))
	}

	r, e := s.resumeInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Checkpoint(ctx context.Context, req *task.CheckpointTaskRequest) (_ *emptypb.Empty, err error) {
	ctx, span := otelutil.StartSpan(ctx, "Checkpoint", trace.WithAttributes(
		attribute.String("tid", req.ID),
		attribute.String("path", req.Path)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if s.isSandbox {
		span.SetAttributes(attribute.String("pod-id", s.tid))
	}

	r, e := s.checkpointInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Kill(ctx context.Context, req *task.KillRequest) (_ *emptypb.Empty, err error) {
	ctx, span := otelutil.StartSpan(ctx, "Kill", trace.WithAttributes(
		attribute.String("tid", req.ID),
		attribute.String("eid", req.ExecID),
		attribute.Int64("signal", int64(req.Signal)),
		attribute.Bool("all", req.All)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if s.isSandbox {
		span.SetAttributes(attribute.String("pod-id", s.tid))
	}

	r, e := s.killInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Exec(ctx context.Context, req *task.ExecProcessRequest) (_ *emptypb.Empty, err error) {
	ctx, span := otelutil.StartSpan(ctx, "Exec", trace.WithAttributes(
		attribute.String("tid", req.ID),
		attribute.String("eid", req.ExecID),
		attribute.Bool("terminal", req.Terminal),
		attribute.String("stdin", req.Stdin),
		attribute.String("stdout", req.Stdout),
		attribute.String("stderr", req.Stderr)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if s.isSandbox {
		span.SetAttributes(attribute.String("pod-id", s.tid))
	}

	r, e := s.execInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) DiagExecInHost(ctx context.Context, req *shimdiag.ExecProcessRequest) (_ *shimdiag.ExecProcessResponse, err error) {
	ctx, span := otelutil.StartSpan(ctx, "DiagExecInHost", trace.WithAttributes(
		attribute.String("args", strings.Join(req.Args, " ")),
		attribute.String("workdir", req.Workdir),
		attribute.Bool("terminal", req.Terminal),
		attribute.String("stdin", req.Stdin),
		attribute.String("stdout", req.Stdout),
		attribute.String("stderr", req.Stderr)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if s.isSandbox {
		span.SetAttributes(attribute.String("pod-id", s.tid))
	}

	r, e := s.diagExecInHostInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) DiagShare(ctx context.Context, req *shimdiag.ShareRequest) (_ *shimdiag.ShareResponse, err error) {
	ctx, span := otelutil.StartSpan(ctx, "DiagShare", trace.WithAttributes(
		attribute.String("hostpath", req.HostPath),
		attribute.String("uvmpath", req.UvmPath),
		attribute.Bool("readonly", req.ReadOnly)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if s.isSandbox {
		span.SetAttributes(attribute.String("pod-id", s.tid))
	}

	r, e := s.diagShareInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) DiagTasks(ctx context.Context, req *shimdiag.TasksRequest) (_ *shimdiag.TasksResponse, err error) {
	ctx, span := otelutil.StartSpan(ctx, "DiagTasks", trace.WithAttributes(
		attribute.Bool("execs", req.Execs)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if s.isSandbox {
		span.SetAttributes(attribute.String("pod-id", s.tid))
	}

	r, e := s.diagTasksInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) ResizePty(ctx context.Context, req *task.ResizePtyRequest) (_ *emptypb.Empty, err error) {
	ctx, span := otelutil.StartSpan(ctx, "ResizePty", trace.WithAttributes(
		attribute.String("tid", req.ID),
		attribute.String("eid", req.ExecID),
		attribute.Int64("width", int64(req.Width)),
		attribute.Int64("height", int64(req.Height))))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if s.isSandbox {
		span.SetAttributes(attribute.String("pod-id", s.tid))
	}

	r, e := s.resizePtyInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) CloseIO(ctx context.Context, req *task.CloseIORequest) (_ *emptypb.Empty, err error) {
	ctx, span := otelutil.StartSpan(ctx, "CloseIO", trace.WithAttributes(
		attribute.String("tid", req.ID),
		attribute.String("eid", req.ExecID),
		attribute.Bool("stdin", req.Stdin)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if s.isSandbox {
		span.SetAttributes(attribute.String("pod-id", s.tid))
	}

	r, e := s.closeIOInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Update(ctx context.Context, req *task.UpdateTaskRequest) (_ *emptypb.Empty, err error) {
	ctx, span := otelutil.StartSpan(ctx, "Update", trace.WithAttributes(
		attribute.String("tid", req.ID)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if s.isSandbox {
		span.SetAttributes(attribute.String("pod-id", s.tid))
	}

	r, e := s.updateInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Wait(ctx context.Context, req *task.WaitRequest) (resp *task.WaitResponse, err error) {
	ctx, span := otelutil.StartSpan(ctx, "Wait", trace.WithAttributes(
		attribute.String("tid", req.ID),
		attribute.String("eid", req.ExecID)))
	defer span.End()
	defer func() {
		if resp != nil {
			span.SetAttributes(
				attribute.Int64("exitStatus", int64(resp.ExitStatus)),
				attribute.String("exitedAt", resp.ExitedAt.String()))
		}
		otelutil.SetSpanStatus(span, err)
	}()

	if s.isSandbox {
		span.SetAttributes(attribute.String("pod-id", s.tid))
	}

	r, e := s.waitInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Stats(ctx context.Context, req *task.StatsRequest) (_ *task.StatsResponse, err error) {
	ctx, span := otelutil.StartSpan(ctx, "Stats", trace.WithAttributes(
		attribute.String("tid", req.ID)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if s.isSandbox {
		span.SetAttributes(attribute.String("pod-id", s.tid))
	}

	r, e := s.statsInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Connect(ctx context.Context, req *task.ConnectRequest) (resp *task.ConnectResponse, err error) {
	ctx, span := otelutil.StartSpan(ctx, "Connect", trace.WithAttributes(
		attribute.String("tid", req.ID)))
	defer span.End()
	defer func() {
		if resp != nil {
			span.SetAttributes(
				attribute.Int64("shimPid", int64(resp.ShimPid)),
				attribute.Int64("taskPid", int64(resp.TaskPid)),
				attribute.String("version", resp.Version))
		}
		otelutil.SetSpanStatus(span, err)
	}()

	if s.isSandbox {
		span.SetAttributes(attribute.String("pod-id", s.tid))
	}

	r, e := s.connectInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Shutdown(ctx context.Context, req *task.ShutdownRequest) (_ *emptypb.Empty, err error) {
	ctx, span := otelutil.StartSpan(ctx, "Shutdown", trace.WithAttributes(
		attribute.String("tid", req.ID)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if s.isSandbox {
		span.SetAttributes(attribute.String("pod-id", s.tid))
	}

	r, e := s.shutdownInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) DiagStacks(ctx context.Context, req *shimdiag.StacksRequest) (*shimdiag.StacksResponse, error) {
	if s == nil {
		return nil, nil
	}
	ctx, span := otelutil.StartSpan(ctx, "DiagStacks", trace.WithAttributes(
		attribute.String("tid", s.tid)))
	defer span.End()

	if s.isSandbox {
		span.SetAttributes(attribute.String("pod-id", s.tid))
	}

	buf := make([]byte, 4096)
	for {
		buf = buf[:runtime.Stack(buf, true)]
		if len(buf) < cap(buf) {
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	resp := &shimdiag.StacksResponse{Stacks: string(buf)}

	t, _ := s.getTask(s.tid)
	if t != nil {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second) //nolint:govet // shadow
		defer cancel()
		resp.GuestStacks = t.DumpGuestStacks(ctx)
	}
	return resp, nil
}

func (s *service) DiagPid(ctx context.Context, req *shimdiag.PidRequest) (*shimdiag.PidResponse, error) {
	if s == nil {
		return nil, nil
	}
	ctx, span := otelutil.StartSpan(ctx, "DiagPid", trace.WithAttributes(
		attribute.String("tid", s.tid))) //nolint:ineffassign,staticcheck
	defer span.End()

	return &shimdiag.PidResponse{
		Pid: int32(os.Getpid()),
	}, nil
}

func (s *service) ComputeProcessorInfo(ctx context.Context, req *extendedtask.ComputeProcessorInfoRequest) (*extendedtask.ComputeProcessorInfoResponse, error) {
	ctx, span := otelutil.StartSpan(ctx, "ComputeProcessorInfo", trace.WithAttributes(
		attribute.String("tid", s.tid)))
	defer span.End()

	r, e := s.computeProcessorInfoInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Done() <-chan struct{} {
	return s.shutdown
}

func (s *service) IsShutdown() bool {
	select {
	case <-s.shutdown:
		return true
	default:
		return false
	}
}
