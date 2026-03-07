//go:build windows

package service

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/oc"

	"github.com/containerd/containerd/api/runtime/task/v3"
	"github.com/containerd/errdefs/pkg/errgrpc"
	"go.opencensus.io/trace"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Ensure Service implements the TTRPCTaskService interface at compile time.
var _ task.TTRPCTaskService = &Service{}

// State returns the current state of a task or process.
func (s *Service) State(ctx context.Context, request *task.StateRequest) (resp *task.StateResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "State")
	defer span.End()
	defer func() {
		if resp != nil {
			span.AddAttributes(
				trace.StringAttribute("status", resp.Status.String()),
				trace.Int64Attribute("exit-status", int64(resp.ExitStatus)),
				trace.StringAttribute("exited-at", resp.ExitedAt.String()))
		}
		oc.SetSpanStatus(span, err)
	}()

	span.AddAttributes(
		trace.StringAttribute("sandbox-id", s.sandboxID),
		trace.StringAttribute("id", request.ID),
		trace.StringAttribute("exec-id", request.ExecID))

	r, e := s.stateInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Create creates a new task.
func (s *Service) Create(ctx context.Context, request *task.CreateTaskRequest) (resp *task.CreateTaskResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "Create")
	defer span.End()
	defer func() {
		if resp != nil {
			span.AddAttributes(trace.Int64Attribute("pid", int64(resp.Pid)))
		}
		oc.SetSpanStatus(span, err)
	}()

	span.AddAttributes(
		trace.StringAttribute("sandbox-id", s.sandboxID),
		trace.StringAttribute("id", request.ID),
		trace.StringAttribute("bundle", request.Bundle),
		trace.BoolAttribute("terminal", request.Terminal),
		trace.StringAttribute("stdin", request.Stdin),
		trace.StringAttribute("stdout", request.Stdout),
		trace.StringAttribute("stderr", request.Stderr),
		trace.StringAttribute("checkpoint", request.Checkpoint),
		trace.StringAttribute("parent-checkpoint", request.ParentCheckpoint))

	r, e := s.createInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Start starts a previously created task.
func (s *Service) Start(ctx context.Context, request *task.StartRequest) (resp *task.StartResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "Start")
	defer span.End()
	defer func() {
		if resp != nil {
			span.AddAttributes(trace.Int64Attribute("pid", int64(resp.Pid)))
		}
		oc.SetSpanStatus(span, err)
	}()

	span.AddAttributes(
		trace.StringAttribute("sandbox-id", s.sandboxID),
		trace.StringAttribute("id", request.ID),
		trace.StringAttribute("exec-id", request.ExecID))

	r, e := s.startInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Delete deletes a task and returns its exit status.
func (s *Service) Delete(ctx context.Context, request *task.DeleteRequest) (resp *task.DeleteResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "Delete")
	defer span.End()
	defer func() {
		if resp != nil {
			span.AddAttributes(
				trace.Int64Attribute("pid", int64(resp.Pid)),
				trace.Int64Attribute("exit-status", int64(resp.ExitStatus)),
				trace.StringAttribute("exited-at", resp.ExitedAt.String()))
		}
		oc.SetSpanStatus(span, err)
	}()

	span.AddAttributes(
		trace.StringAttribute("sandbox-id", s.sandboxID),
		trace.StringAttribute("id", request.ID),
		trace.StringAttribute("exec-id", request.ExecID))

	r, e := s.deleteInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Pids returns all process IDs for a task.
func (s *Service) Pids(ctx context.Context, request *task.PidsRequest) (resp *task.PidsResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "Pids")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("sandbox-id", s.sandboxID),
		trace.StringAttribute("id", request.ID))

	r, e := s.pidsInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Pause pauses a task.
func (s *Service) Pause(ctx context.Context, request *task.PauseRequest) (resp *emptypb.Empty, err error) {
	ctx, span := oc.StartSpan(ctx, "Pause")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("sandbox-id", s.sandboxID),
		trace.StringAttribute("id", request.ID))

	r, e := s.pauseInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Resume resumes a previously paused task.
func (s *Service) Resume(ctx context.Context, request *task.ResumeRequest) (resp *emptypb.Empty, err error) {
	ctx, span := oc.StartSpan(ctx, "Resume")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("sandbox-id", s.sandboxID),
		trace.StringAttribute("id", request.ID))

	r, e := s.resumeInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Checkpoint creates a checkpoint of a task.
func (s *Service) Checkpoint(ctx context.Context, request *task.CheckpointTaskRequest) (resp *emptypb.Empty, err error) {
	ctx, span := oc.StartSpan(ctx, "Checkpoint")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("sandbox-id", s.sandboxID),
		trace.StringAttribute("id", request.ID),
		trace.StringAttribute("path", request.Path))

	r, e := s.checkpointInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Kill sends a signal to a task or process.
func (s *Service) Kill(ctx context.Context, request *task.KillRequest) (resp *emptypb.Empty, err error) {
	ctx, span := oc.StartSpan(ctx, "Kill")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("sandbox-id", s.sandboxID),
		trace.StringAttribute("id", request.ID),
		trace.StringAttribute("exec-id", request.ExecID),
		trace.Int64Attribute("signal", int64(request.Signal)),
		trace.BoolAttribute("all", request.All))

	r, e := s.killInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Exec executes an additional process inside a task.
func (s *Service) Exec(ctx context.Context, request *task.ExecProcessRequest) (resp *emptypb.Empty, err error) {
	ctx, span := oc.StartSpan(ctx, "Exec")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("sandbox-id", s.sandboxID),
		trace.StringAttribute("id", request.ID),
		trace.StringAttribute("exec-id", request.ExecID),
		trace.BoolAttribute("terminal", request.Terminal),
		trace.StringAttribute("stdin", request.Stdin),
		trace.StringAttribute("stdout", request.Stdout),
		trace.StringAttribute("stderr", request.Stderr))

	r, e := s.execInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// ResizePty resizes the terminal of a process.
func (s *Service) ResizePty(ctx context.Context, request *task.ResizePtyRequest) (resp *emptypb.Empty, err error) {
	ctx, span := oc.StartSpan(ctx, "ResizePty")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("sandbox-id", s.sandboxID),
		trace.StringAttribute("id", request.ID),
		trace.StringAttribute("exec-id", request.ExecID),
		trace.Int64Attribute("width", int64(request.Width)),
		trace.Int64Attribute("height", int64(request.Height)))

	r, e := s.resizePtyInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// CloseIO closes the IO for a process.
func (s *Service) CloseIO(ctx context.Context, request *task.CloseIORequest) (resp *emptypb.Empty, err error) {
	ctx, span := oc.StartSpan(ctx, "CloseIO")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("sandbox-id", s.sandboxID),
		trace.StringAttribute("id", request.ID),
		trace.StringAttribute("exec-id", request.ExecID),
		trace.BoolAttribute("stdin", request.Stdin))

	r, e := s.closeIOInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Update updates a running task with new resource constraints.
func (s *Service) Update(ctx context.Context, request *task.UpdateTaskRequest) (resp *emptypb.Empty, err error) {
	ctx, span := oc.StartSpan(ctx, "Update")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("sandbox-id", s.sandboxID),
		trace.StringAttribute("id", request.ID))

	r, e := s.updateInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Wait waits for a task or process to exit.
func (s *Service) Wait(ctx context.Context, request *task.WaitRequest) (resp *task.WaitResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "Wait")
	defer span.End()
	defer func() {
		if resp != nil {
			span.AddAttributes(
				trace.Int64Attribute("exit-status", int64(resp.ExitStatus)),
				trace.StringAttribute("exited-at", resp.ExitedAt.String()))
		}
		oc.SetSpanStatus(span, err)
	}()

	span.AddAttributes(
		trace.StringAttribute("sandbox-id", s.sandboxID),
		trace.StringAttribute("id", request.ID),
		trace.StringAttribute("exec-id", request.ExecID))

	r, e := s.waitInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Stats returns resource usage statistics for a task.
func (s *Service) Stats(ctx context.Context, request *task.StatsRequest) (resp *task.StatsResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "Stats")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("sandbox-id", s.sandboxID),
		trace.StringAttribute("id", request.ID))

	r, e := s.statsInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Connect reconnects to a running task.
func (s *Service) Connect(ctx context.Context, request *task.ConnectRequest) (resp *task.ConnectResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "Connect")
	defer span.End()
	defer func() {
		if resp != nil {
			span.AddAttributes(
				trace.Int64Attribute("shim-pid", int64(resp.ShimPid)),
				trace.Int64Attribute("task-pid", int64(resp.TaskPid)),
				trace.StringAttribute("version", resp.Version))
		}
		oc.SetSpanStatus(span, err)
	}()

	span.AddAttributes(
		trace.StringAttribute("sandbox-id", s.sandboxID),
		trace.StringAttribute("id", request.ID))

	r, e := s.connectInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Shutdown gracefully shuts down the Service.
func (s *Service) Shutdown(ctx context.Context, request *task.ShutdownRequest) (resp *emptypb.Empty, err error) {
	ctx, span := oc.StartSpan(ctx, "Shutdown")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("sandbox-id", s.sandboxID),
		trace.StringAttribute("id", request.ID))

	r, e := s.shutdownInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}
