//go:build windows && lcow

package service

import (
	"context"
	"os"

	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/ot"

	"github.com/containerd/containerd/api/runtime/task/v3"
	"github.com/containerd/errdefs/pkg/errgrpc"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Ensure Service implements the TTRPCTaskService interface at compile time.
var _ task.TTRPCTaskService = &Service{}

// State returns the current state of a task or process.
// This method is part of the instrumentation layer and business logic is included in stateInternal.
func (s *Service) State(ctx context.Context, request *task.StateRequest) (resp *task.StateResponse, err error) {
	ctx, span := ot.StartSpan(ctx, "State")
	defer span.End()
	defer func() {
		if resp != nil {
			span.SetAttributes(
				attribute.String(logfields.Status, resp.Status.String()),
				attribute.Int64(logfields.ExitStatus, int64(resp.ExitStatus)),
				attribute.String(logfields.ExitedAt, resp.ExitedAt.String()))
		}
		ot.SetSpanStatus(span, err)
	}()

	span.SetAttributes(
		attribute.String(logfields.SandboxID, s.sandboxID),
		attribute.String(logfields.ID, request.ID),
		attribute.String(logfields.ExecID, request.ExecID))

	r, e := s.stateInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Create creates a new task.
// This method is part of the instrumentation layer and business logic is included in createInternal.
func (s *Service) Create(ctx context.Context, request *task.CreateTaskRequest) (resp *task.CreateTaskResponse, err error) {
	ctx, span := ot.StartSpan(ctx, "Create")
	defer span.End()
	defer func() {
		if resp != nil {
			span.SetAttributes(attribute.Int64(logfields.ProcessID, int64(resp.Pid)))
		}
		ot.SetSpanStatus(span, err)
	}()

	span.SetAttributes(
		attribute.String(logfields.SandboxID, s.sandboxID),
		attribute.String(logfields.ID, request.ID),
		attribute.String(logfields.Bundle, request.Bundle),
		attribute.Bool(logfields.Terminal, request.Terminal),
		attribute.String(logfields.Stdin, request.Stdin),
		attribute.String(logfields.Stdout, request.Stdout),
		attribute.String(logfields.Stderr, request.Stderr),
		attribute.String(logfields.Checkpoint, request.Checkpoint),
		attribute.String(logfields.ParentCheckpoint, request.ParentCheckpoint))

	r, e := s.createInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Start starts a previously created task.
// This method is part of the instrumentation layer and business logic is included in startInternal.
func (s *Service) Start(ctx context.Context, request *task.StartRequest) (resp *task.StartResponse, err error) {
	ctx, span := ot.StartSpan(ctx, "Start")
	defer span.End()
	defer func() {
		if resp != nil {
			span.SetAttributes(attribute.Int64(logfields.ProcessID, int64(resp.Pid)))
		}
		ot.SetSpanStatus(span, err)
	}()

	span.SetAttributes(
		attribute.String(logfields.SandboxID, s.sandboxID),
		attribute.String(logfields.ID, request.ID),
		attribute.String(logfields.ExecID, request.ExecID))

	r, e := s.startInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Delete deletes a task and returns its exit status.
// This method is part of the instrumentation layer and business logic is included in deleteInternal.
func (s *Service) Delete(ctx context.Context, request *task.DeleteRequest) (resp *task.DeleteResponse, err error) {
	ctx, span := ot.StartSpan(ctx, "Delete")
	defer span.End()
	defer func() {
		if resp != nil {
			span.SetAttributes(
				attribute.Int64(logfields.ProcessID, int64(resp.Pid)),
				attribute.Int64(logfields.ExitStatus, int64(resp.ExitStatus)),
				attribute.String(logfields.ExitedAt, resp.ExitedAt.String()))
		}
		ot.SetSpanStatus(span, err)
	}()

	span.SetAttributes(
		attribute.String(logfields.SandboxID, s.sandboxID),
		attribute.String(logfields.ID, request.ID),
		attribute.String(logfields.ExecID, request.ExecID))

	r, e := s.deleteInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Pids returns all process IDs for a task.
// This method is part of the instrumentation layer and business logic is included in pidsInternal.
func (s *Service) Pids(ctx context.Context, request *task.PidsRequest) (resp *task.PidsResponse, err error) {
	ctx, span := ot.StartSpan(ctx, "Pids")
	defer span.End()
	defer func() { ot.SetSpanStatus(span, err) }()

	span.SetAttributes(
		attribute.String(logfields.SandboxID, s.sandboxID),
		attribute.String(logfields.ID, request.ID))

	r, e := s.pidsInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Pause pauses a task.
// This method is part of the instrumentation layer and business logic is included in pauseInternal.
func (s *Service) Pause(ctx context.Context, request *task.PauseRequest) (resp *emptypb.Empty, err error) {
	ctx, span := ot.StartSpan(ctx, "Pause")
	defer span.End()
	defer func() { ot.SetSpanStatus(span, err) }()

	span.SetAttributes(
		attribute.String(logfields.SandboxID, s.sandboxID),
		attribute.String(logfields.ID, request.ID))

	r, e := s.pauseInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Resume resumes a previously paused task.
// This method is part of the instrumentation layer and business logic is included in resumeInternal.
func (s *Service) Resume(ctx context.Context, request *task.ResumeRequest) (resp *emptypb.Empty, err error) {
	ctx, span := ot.StartSpan(ctx, "Resume")
	defer span.End()
	defer func() { ot.SetSpanStatus(span, err) }()

	span.SetAttributes(
		attribute.String(logfields.SandboxID, s.sandboxID),
		attribute.String(logfields.ID, request.ID))

	r, e := s.resumeInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Checkpoint creates a checkpoint of a task.
// This method is part of the instrumentation layer and business logic is included in checkpointInternal.
func (s *Service) Checkpoint(ctx context.Context, request *task.CheckpointTaskRequest) (resp *emptypb.Empty, err error) {
	ctx, span := ot.StartSpan(ctx, "Checkpoint")
	defer span.End()
	defer func() { ot.SetSpanStatus(span, err) }()

	span.SetAttributes(
		attribute.String(logfields.SandboxID, s.sandboxID),
		attribute.String(logfields.ID, request.ID),
		attribute.String(logfields.Path, request.Path))

	r, e := s.checkpointInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Kill sends a signal to a task or process.
// This method is part of the instrumentation layer and business logic is included in killInternal.
func (s *Service) Kill(ctx context.Context, request *task.KillRequest) (resp *emptypb.Empty, err error) {
	ctx, span := ot.StartSpan(ctx, "Kill")
	defer span.End()
	defer func() { ot.SetSpanStatus(span, err) }()

	span.SetAttributes(
		attribute.String(logfields.SandboxID, s.sandboxID),
		attribute.String(logfields.ID, request.ID),
		attribute.String(logfields.ExecID, request.ExecID),
		attribute.Int64(logfields.Signal, int64(request.Signal)),
		attribute.Bool(logfields.All, request.All))

	r, e := s.killInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Exec executes an additional process inside a task.
// This method is part of the instrumentation layer and business logic is included in execInternal.
func (s *Service) Exec(ctx context.Context, request *task.ExecProcessRequest) (resp *emptypb.Empty, err error) {
	ctx, span := ot.StartSpan(ctx, "Exec")
	defer span.End()
	defer func() { ot.SetSpanStatus(span, err) }()

	span.SetAttributes(
		attribute.String(logfields.SandboxID, s.sandboxID),
		attribute.String(logfields.ID, request.ID),
		attribute.String(logfields.ExecID, request.ExecID),
		attribute.Bool(logfields.Terminal, request.Terminal),
		attribute.String(logfields.Stdin, request.Stdin),
		attribute.String(logfields.Stdout, request.Stdout),
		attribute.String(logfields.Stderr, request.Stderr))

	r, e := s.execInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// ResizePty resizes the terminal of a process.
// This method is part of the instrumentation layer and business logic is included in resizePtyInternal.
func (s *Service) ResizePty(ctx context.Context, request *task.ResizePtyRequest) (resp *emptypb.Empty, err error) {
	ctx, span := ot.StartSpan(ctx, "ResizePty")
	defer span.End()
	defer func() { ot.SetSpanStatus(span, err) }()

	span.SetAttributes(
		attribute.String(logfields.SandboxID, s.sandboxID),
		attribute.String(logfields.ID, request.ID),
		attribute.String(logfields.ExecID, request.ExecID),
		attribute.Int64(logfields.Width, int64(request.Width)),
		attribute.Int64(logfields.Height, int64(request.Height)))

	r, e := s.resizePtyInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// CloseIO closes the IO for a process.
// This method is part of the instrumentation layer and business logic is included in closeIOInternal.
func (s *Service) CloseIO(ctx context.Context, request *task.CloseIORequest) (resp *emptypb.Empty, err error) {
	ctx, span := ot.StartSpan(ctx, "CloseIO")
	defer span.End()
	defer func() { ot.SetSpanStatus(span, err) }()

	span.SetAttributes(
		attribute.String(logfields.SandboxID, s.sandboxID),
		attribute.String(logfields.ID, request.ID),
		attribute.String(logfields.ExecID, request.ExecID),
		attribute.Bool(logfields.Stdin, request.Stdin))

	r, e := s.closeIOInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Update updates a running task with new resource constraints.
// This method is part of the instrumentation layer and business logic is included in updateInternal.
func (s *Service) Update(ctx context.Context, request *task.UpdateTaskRequest) (resp *emptypb.Empty, err error) {
	ctx, span := ot.StartSpan(ctx, "Update")
	defer span.End()
	defer func() { ot.SetSpanStatus(span, err) }()

	span.SetAttributes(
		attribute.String(logfields.SandboxID, s.sandboxID),
		attribute.String(logfields.ID, request.ID))

	r, e := s.updateInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Wait waits for a task or process to exit.
// This method is part of the instrumentation layer and business logic is included in waitInternal.
func (s *Service) Wait(ctx context.Context, request *task.WaitRequest) (resp *task.WaitResponse, err error) {
	ctx, span := ot.StartSpan(ctx, "Wait")
	defer span.End()
	defer func() {
		if resp != nil {
			span.SetAttributes(
				attribute.Int64(logfields.ExitStatus, int64(resp.ExitStatus)),
				attribute.String(logfields.ExitedAt, resp.ExitedAt.String()))
		}
		ot.SetSpanStatus(span, err)
	}()

	span.SetAttributes(
		attribute.String(logfields.SandboxID, s.sandboxID),
		attribute.String(logfields.ID, request.ID),
		attribute.String(logfields.ExecID, request.ExecID))

	r, e := s.waitInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Stats returns resource usage statistics for a task.
// This method is part of the instrumentation layer and business logic is included in statsInternal.
func (s *Service) Stats(ctx context.Context, request *task.StatsRequest) (resp *task.StatsResponse, err error) {
	ctx, span := ot.StartSpan(ctx, "Stats")
	defer span.End()
	defer func() { ot.SetSpanStatus(span, err) }()

	span.SetAttributes(
		attribute.String(logfields.SandboxID, s.sandboxID),
		attribute.String(logfields.ID, request.ID))

	r, e := s.statsInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Connect reconnects to a running task.
// This method is part of the instrumentation layer and business logic is included in connectInternal.
func (s *Service) Connect(ctx context.Context, request *task.ConnectRequest) (resp *task.ConnectResponse, err error) {
	_, span := ot.StartSpan(ctx, "Connect")
	defer span.End()
	defer func() {
		if resp != nil {
			span.SetAttributes(
				attribute.Int64(logfields.ShimPid, int64(resp.ShimPid)),
				attribute.Int64(logfields.TaskPid, int64(resp.TaskPid)),
				attribute.String(logfields.Version, resp.Version))
		}
		ot.SetSpanStatus(span, err)
	}()

	span.SetAttributes(
		attribute.String(logfields.SandboxID, s.sandboxID),
		attribute.String(logfields.ID, request.ID))

	// We treat the shim/task as the same pid on the Windows host.
	pid := uint32(os.Getpid())

	return &task.ConnectResponse{
		ShimPid: pid,
		TaskPid: pid,
	}, nil
}

// Shutdown gracefully shuts down the Service.
// This method is part of the instrumentation layer and business logic is included in shutdownInternal.
func (s *Service) Shutdown(ctx context.Context, request *task.ShutdownRequest) (resp *emptypb.Empty, err error) {
	ctx, span := ot.StartSpan(ctx, "Shutdown")
	defer span.End()
	defer func() { ot.SetSpanStatus(span, err) }()

	span.SetAttributes(
		attribute.String(logfields.SandboxID, s.sandboxID),
		attribute.String(logfields.ID, request.ID))

	r, e := s.shutdownInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}
