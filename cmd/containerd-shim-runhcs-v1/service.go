package main

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/runtime/v2/task"
	google_protobuf1 "github.com/gogo/protobuf/types"
	"go.opencensus.io/trace"
)

type cdevent struct {
	topic string
	event interface{}
}

var _ = (task.TaskService)(&service{})

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
	// POD. `cl` is used to create the task or POD sandbox. It SHOULD not be
	// taken when creating tasks in a POD sandbox as they can happen
	// concurrently.
	cl sync.Mutex
}

func (s *service) State(ctx context.Context, req *task.StateRequest) (resp *task.StateResponse, err error) {
	defer panicRecover()
	ctx, span := trace.StartSpan(ctx, "State")
	defer span.End()
	defer func() {
		if resp != nil {
			span.AddAttributes(
				trace.StringAttribute("status", resp.Status.String()),
				trace.Int64Attribute("exitStatus", int64(resp.ExitStatus)),
				trace.StringAttribute("exitedAt", resp.ExitedAt.String()))
		}
		oc.SetSpanStatus(span, err)
	}()

	span.AddAttributes(
		trace.StringAttribute("tid", req.ID),
		trace.StringAttribute("eid", req.ExecID))

	if s.isSandbox {
		span.AddAttributes(trace.StringAttribute("pod-id", s.tid))
	}

	r, e := s.stateInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Create(ctx context.Context, req *task.CreateTaskRequest) (resp *task.CreateTaskResponse, err error) {
	defer panicRecover()
	ctx, span := trace.StartSpan(ctx, "Create")
	defer span.End()
	defer func() {
		if resp != nil {
			span.AddAttributes(trace.Int64Attribute("pid", int64(resp.Pid)))
		}
		oc.SetSpanStatus(span, err)
	}()

	span.AddAttributes(
		trace.StringAttribute("tid", req.ID),
		trace.StringAttribute("bundle", req.Bundle),
		// trace.StringAttribute("rootfs", req.Rootfs), TODO: JTERRY75 -
		// OpenCensus doesnt support slice like our logrus hook
		trace.BoolAttribute("terminal", req.Terminal),
		trace.StringAttribute("stdin", req.Stdin),
		trace.StringAttribute("stdout", req.Stdout),
		trace.StringAttribute("stderr", req.Stderr),
		trace.StringAttribute("checkpoint", req.Checkpoint),
		trace.StringAttribute("parentcheckpoint", req.ParentCheckpoint))

	if s.isSandbox {
		span.AddAttributes(trace.StringAttribute("pod-id", s.tid))
	}

	r, e := s.createInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Start(ctx context.Context, req *task.StartRequest) (resp *task.StartResponse, err error) {
	defer panicRecover()
	ctx, span := trace.StartSpan(ctx, "Start")
	defer span.End()
	defer func() {
		if resp != nil {
			span.AddAttributes(trace.Int64Attribute("pid", int64(resp.Pid)))
		}
		oc.SetSpanStatus(span, err)
	}()

	span.AddAttributes(
		trace.StringAttribute("tid", req.ID),
		trace.StringAttribute("eid", req.ExecID))

	if s.isSandbox {
		span.AddAttributes(trace.StringAttribute("pod-id", s.tid))
	}

	r, e := s.startInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Delete(ctx context.Context, req *task.DeleteRequest) (resp *task.DeleteResponse, err error) {
	defer panicRecover()
	ctx, span := trace.StartSpan(ctx, "Delete")
	defer span.End()
	defer func() {
		if resp != nil {
			span.AddAttributes(
				trace.Int64Attribute("pid", int64(resp.Pid)),
				trace.Int64Attribute("exitStatus", int64(resp.ExitStatus)),
				trace.StringAttribute("exitedAt", resp.ExitedAt.String()))
		}
		oc.SetSpanStatus(span, err)
	}()

	span.AddAttributes(
		trace.StringAttribute("tid", req.ID),
		trace.StringAttribute("eid", req.ExecID))

	if s.isSandbox {
		span.AddAttributes(trace.StringAttribute("pod-id", s.tid))
	}

	r, e := s.deleteInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Pids(ctx context.Context, req *task.PidsRequest) (_ *task.PidsResponse, err error) {
	defer panicRecover()
	ctx, span := trace.StartSpan(ctx, "Pids")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute("tid", req.ID))

	if s.isSandbox {
		span.AddAttributes(trace.StringAttribute("pod-id", s.tid))
	}

	r, e := s.pidsInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Pause(ctx context.Context, req *task.PauseRequest) (_ *google_protobuf1.Empty, err error) {
	defer panicRecover()
	ctx, span := trace.StartSpan(ctx, "Pause")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute("tid", req.ID))

	if s.isSandbox {
		span.AddAttributes(trace.StringAttribute("pod-id", s.tid))
	}

	r, e := s.pauseInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Resume(ctx context.Context, req *task.ResumeRequest) (_ *google_protobuf1.Empty, err error) {
	defer panicRecover()
	ctx, span := trace.StartSpan(ctx, "Resume")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute("tid", req.ID))

	if s.isSandbox {
		span.AddAttributes(trace.StringAttribute("pod-id", s.tid))
	}

	r, e := s.resumeInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Checkpoint(ctx context.Context, req *task.CheckpointTaskRequest) (_ *google_protobuf1.Empty, err error) {
	defer panicRecover()
	ctx, span := trace.StartSpan(ctx, "Checkpoint")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("tid", req.ID),
		trace.StringAttribute("path", req.Path))

	if s.isSandbox {
		span.AddAttributes(trace.StringAttribute("pod-id", s.tid))
	}

	r, e := s.checkpointInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Kill(ctx context.Context, req *task.KillRequest) (_ *google_protobuf1.Empty, err error) {
	defer panicRecover()
	ctx, span := trace.StartSpan(ctx, "Kill")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("tid", req.ID),
		trace.StringAttribute("eid", req.ExecID),
		trace.Int64Attribute("signal", int64(req.Signal)),
		trace.BoolAttribute("all", req.All))

	if s.isSandbox {
		span.AddAttributes(trace.StringAttribute("pod-id", s.tid))
	}

	r, e := s.killInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Exec(ctx context.Context, req *task.ExecProcessRequest) (_ *google_protobuf1.Empty, err error) {
	defer panicRecover()
	ctx, span := trace.StartSpan(ctx, "Exec")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("tid", req.ID),
		trace.StringAttribute("eid", req.ExecID),
		trace.BoolAttribute("terminal", req.Terminal),
		trace.StringAttribute("stdin", req.Stdin),
		trace.StringAttribute("stdout", req.Stdout),
		trace.StringAttribute("stderr", req.Stderr))

	if s.isSandbox {
		span.AddAttributes(trace.StringAttribute("pod-id", s.tid))
	}

	r, e := s.execInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) DiagExecInHost(ctx context.Context, req *shimdiag.ExecProcessRequest) (_ *shimdiag.ExecProcessResponse, err error) {
	defer panicRecover()
	ctx, span := trace.StartSpan(ctx, "DiagExecInHost")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("args", strings.Join(req.Args, " ")),
		trace.StringAttribute("workdir", req.Workdir),
		trace.BoolAttribute("terminal", req.Terminal),
		trace.StringAttribute("stdin", req.Stdin),
		trace.StringAttribute("stdout", req.Stdout),
		trace.StringAttribute("stderr", req.Stderr))

	if s.isSandbox {
		span.AddAttributes(trace.StringAttribute("pod-id", s.tid))
	}

	r, e := s.diagExecInHostInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) DiagShare(ctx context.Context, req *shimdiag.ShareRequest) (_ *shimdiag.ShareResponse, err error) {
	defer panicRecover()
	ctx, span := trace.StartSpan(ctx, "DiagShare")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("hostpath", req.HostPath),
		trace.StringAttribute("uvmpath", req.UvmPath),
		trace.BoolAttribute("readonly", req.ReadOnly))

	if s.isSandbox {
		span.AddAttributes(trace.StringAttribute("pod-id", s.tid))
	}

	r, e := s.diagShareInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) ResizePty(ctx context.Context, req *task.ResizePtyRequest) (_ *google_protobuf1.Empty, err error) {
	defer panicRecover()
	ctx, span := trace.StartSpan(ctx, "ResizePty")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("tid", req.ID),
		trace.StringAttribute("eid", req.ExecID),
		trace.Int64Attribute("width", int64(req.Width)),
		trace.Int64Attribute("height", int64(req.Height)))

	if s.isSandbox {
		span.AddAttributes(trace.StringAttribute("pod-id", s.tid))
	}

	r, e := s.resizePtyInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) CloseIO(ctx context.Context, req *task.CloseIORequest) (_ *google_protobuf1.Empty, err error) {
	defer panicRecover()
	ctx, span := trace.StartSpan(ctx, "CloseIO")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("tid", req.ID),
		trace.StringAttribute("eid", req.ExecID),
		trace.BoolAttribute("stdin", req.Stdin))

	if s.isSandbox {
		span.AddAttributes(trace.StringAttribute("pod-id", s.tid))
	}

	r, e := s.closeIOInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Update(ctx context.Context, req *task.UpdateTaskRequest) (_ *google_protobuf1.Empty, err error) {
	defer panicRecover()
	ctx, span := trace.StartSpan(ctx, "Update")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute("tid", req.ID))

	if s.isSandbox {
		span.AddAttributes(trace.StringAttribute("pod-id", s.tid))
	}

	r, e := s.updateInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Wait(ctx context.Context, req *task.WaitRequest) (resp *task.WaitResponse, err error) {
	defer panicRecover()
	ctx, span := trace.StartSpan(ctx, "Wait")
	defer span.End()
	defer func() {
		if resp != nil {
			span.AddAttributes(
				trace.Int64Attribute("exitStatus", int64(resp.ExitStatus)),
				trace.StringAttribute("exitedAt", resp.ExitedAt.String()))
		}
		oc.SetSpanStatus(span, err)
	}()

	span.AddAttributes(
		trace.StringAttribute("tid", req.ID),
		trace.StringAttribute("eid", req.ExecID))

	if s.isSandbox {
		span.AddAttributes(trace.StringAttribute("pod-id", s.tid))
	}

	r, e := s.waitInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Stats(ctx context.Context, req *task.StatsRequest) (_ *task.StatsResponse, err error) {
	defer panicRecover()
	ctx, span := trace.StartSpan(ctx, "Stats")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute("tid", req.ID))

	if s.isSandbox {
		span.AddAttributes(trace.StringAttribute("pod-id", s.tid))
	}

	r, e := s.statsInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Connect(ctx context.Context, req *task.ConnectRequest) (resp *task.ConnectResponse, err error) {
	defer panicRecover()
	ctx, span := trace.StartSpan(ctx, "Connect")
	defer span.End()
	defer func() {
		if resp != nil {
			span.AddAttributes(
				trace.Int64Attribute("shimPid", int64(resp.ShimPid)),
				trace.Int64Attribute("taskPid", int64(resp.TaskPid)),
				trace.StringAttribute("version", resp.Version))
		}
		oc.SetSpanStatus(span, err)
	}()

	span.AddAttributes(trace.StringAttribute("tid", req.ID))

	if s.isSandbox {
		span.AddAttributes(trace.StringAttribute("pod-id", s.tid))
	}

	r, e := s.connectInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) Shutdown(ctx context.Context, req *task.ShutdownRequest) (_ *google_protobuf1.Empty, err error) {
	defer panicRecover()
	ctx, span := trace.StartSpan(ctx, "Shutdown")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute("tid", req.ID))

	if s.isSandbox {
		span.AddAttributes(trace.StringAttribute("pod-id", s.tid))
	}

	r, e := s.shutdownInternal(ctx, req)
	return r, errdefs.ToGRPC(e)
}

func (s *service) DiagStacks(ctx context.Context, req *shimdiag.StacksRequest) (*shimdiag.StacksResponse, error) {
	if s == nil {
		return nil, nil
	}
	defer panicRecover()
	ctx, span := trace.StartSpan(ctx, "DiagStacks")
	defer span.End()

	span.AddAttributes(trace.StringAttribute("tid", s.tid))

	if s.isSandbox {
		span.AddAttributes(trace.StringAttribute("pod-id", s.tid))
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
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		resp.GuestStacks = t.DumpGuestStacks(ctx)
	}
	return resp, nil
}
