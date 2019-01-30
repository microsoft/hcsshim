package main

import (
	"context"
	"os"

	containerd_v1_types "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/errdefs"
	runcopts "github.com/containerd/containerd/runtime/v2/runc/options"
	"github.com/containerd/containerd/runtime/v2/task"
	"github.com/containerd/typeurl"
	google_protobuf1 "github.com/gogo/protobuf/types"
	"github.com/pkg/errors"
)

var empty = &google_protobuf1.Empty{}

// getPod returns the pod this shim is tracking or else returns `nil`. It is the
// callers responsibility to verify that `s.isSandbox == true` before calling
// this method.
//
//
// If `pod==nil` returns `errdefs.ErrFailedPrecondition`.
func (s *service) getPod() (shimPod, error) {
	raw := s.z.Load()
	if raw == nil {
		return nil, errors.Wrapf(errdefs.ErrFailedPrecondition, "task with id: '%s' must be created first", s.tid)
	}
	return raw.(shimPod), nil
}

// getTask returns a task matching `tid` or else returns `nil`. This properly
// handles a task in a pod or a singular task shim.
//
// If `tid` is not found will return `errdefs.ErrNotFound`.
func (s *service) getTask(tid string) (shimTask, error) {
	raw := s.z.Load()
	if raw == nil {
		return nil, errors.Wrapf(errdefs.ErrNotFound, "task with id: '%s' not found", tid)
	}
	if s.isSandbox {
		p := raw.(shimPod)
		return p.GetTask(tid)
	}
	// When its not a sandbox only the init task is a valid id.
	if s.tid != tid {
		return nil, errors.Wrapf(errdefs.ErrNotFound, "task with id: '%s' not found", tid)
	}
	return raw.(shimTask), nil
}

func (s *service) stateInternal(ctx context.Context, req *task.StateRequest) (*task.StateResponse, error) {
	t, err := s.getTask(req.ID)
	if err != nil {
		return nil, err
	}
	e, err := t.GetExec(req.ExecID)
	if err != nil {
		return nil, err
	}
	return e.Status(), nil
}

func (s *service) createInternal(ctx context.Context, req *task.CreateTaskRequest) (*task.CreateTaskResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) startInternal(ctx context.Context, req *task.StartRequest) (*task.StartResponse, error) {
	t, err := s.getTask(req.ID)
	if err != nil {
		return nil, err
	}
	e, err := t.GetExec(req.ExecID)
	if err != nil {
		return nil, err
	}
	err = e.Start(ctx)
	if err != nil {
		return nil, err
	}
	return &task.StartResponse{
		Pid: uint32(e.Pid()),
	}, nil
}

func (s *service) deleteInternal(ctx context.Context, req *task.DeleteRequest) (*task.DeleteResponse, error) {
	// TODO: JTERRY75 we need to send this to the POD for isSandbox

	t, err := s.getTask(req.ID)
	if err != nil {
		return nil, err
	}
	pid, exitStatus, exitedAt, err := t.DeleteExec(ctx, req.ExecID)
	if err != nil {
		return nil, err
	}
	return &task.DeleteResponse{
		Pid:        uint32(pid),
		ExitStatus: exitStatus,
		ExitedAt:   exitedAt,
	}, nil
}

func (s *service) pidsInternal(ctx context.Context, req *task.PidsRequest) (*task.PidsResponse, error) {
	t, err := s.getTask(req.ID)
	if err != nil {
		return nil, err
	}
	pids, err := t.Pids(ctx)
	if err != nil {
		return nil, err
	}
	processes := make([]*containerd_v1_types.ProcessInfo, len(pids))
	for i, p := range pids {
		proc := &containerd_v1_types.ProcessInfo{
			Pid: uint32(p.Pid),
		}
		if p.ExecID != "" {
			d := &runcopts.ProcessDetails{
				ExecID: p.ExecID,
			}
			a, err := typeurl.MarshalAny(d)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to marshal ProcessDetails for process: %s, task: %s", p.ExecID, req.ID)
			}
			proc.Info = a
		}
		processes[i] = proc
	}
	return &task.PidsResponse{
		Processes: processes,
	}, nil
}

func (s *service) pauseInternal(ctx context.Context, req *task.PauseRequest) (*google_protobuf1.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) resumeInternal(ctx context.Context, req *task.ResumeRequest) (*google_protobuf1.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) checkpointInternal(ctx context.Context, req *task.CheckpointTaskRequest) (*google_protobuf1.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) killInternal(ctx context.Context, req *task.KillRequest) (*google_protobuf1.Empty, error) {
	if s.isSandbox {
		pod, err := s.getPod()
		if err != nil {
			return nil, errors.Wrapf(errdefs.ErrNotFound, "%v: task with id: '%s' not found", err, req.ID)
		}
		// Send it to the POD and let it cascade on its own through all tasks.
		err = pod.KillTask(ctx, req.ID, req.ExecID, req.Signal, req.All)
		if err != nil {
			return nil, err
		}
		return empty, nil
	}
	t, err := s.getTask(req.ID)
	if err != nil {
		return nil, err
	}
	// Send it to the task and let it cascade on its own through all exec's
	err = t.KillExec(ctx, req.ExecID, req.Signal, req.All)
	if err != nil {
		return nil, err
	}
	return empty, nil
}

func (s *service) execInternal(ctx context.Context, req *task.ExecProcessRequest) (*google_protobuf1.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) resizePtyInternal(ctx context.Context, req *task.ResizePtyRequest) (*google_protobuf1.Empty, error) {
	t, err := s.getTask(req.ID)
	if err != nil {
		return nil, err
	}
	e, err := t.GetExec(req.ExecID)
	if err != nil {
		return nil, err
	}
	err = e.ResizePty(ctx, req.Width, req.Height)
	if err != nil {
		return nil, err
	}
	return empty, nil
}

func (s *service) closeIOInternal(ctx context.Context, req *task.CloseIORequest) (*google_protobuf1.Empty, error) {
	t, err := s.getTask(req.ID)
	if err != nil {
		return nil, err
	}
	e, err := t.GetExec(req.ExecID)
	if err != nil {
		return nil, err
	}
	err = e.CloseIO(ctx, req.Stdin)
	if err != nil {
		return nil, err
	}
	return empty, nil
}

func (s *service) updateInternal(ctx context.Context, req *task.UpdateTaskRequest) (*google_protobuf1.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) waitInternal(ctx context.Context, req *task.WaitRequest) (*task.WaitResponse, error) {
	t, err := s.getTask(req.ID)
	if err != nil {
		return nil, err
	}
	e, err := t.GetExec(req.ExecID)
	if err != nil {
		return nil, err
	}
	state := e.Wait(ctx)
	return &task.WaitResponse{
		ExitStatus: state.ExitStatus,
		ExitedAt:   state.ExitedAt,
	}, nil
}

func (s *service) statsInternal(ctx context.Context, req *task.StatsRequest) (*task.StatsResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *service) connectInternal(ctx context.Context, req *task.ConnectRequest) (*task.ConnectResponse, error) {
	// We treat the shim/task as the same pid on the Windows host.
	pid := uint32(os.Getpid())
	return &task.ConnectResponse{
		ShimPid: pid,
		TaskPid: pid,
	}, nil
}

func (s *service) shutdownInternal(ctx context.Context, req *task.ShutdownRequest) (*google_protobuf1.Empty, error) {
	if req.Now {
		os.Exit(0)
	}
	// TODO: JTERRY75 if we dont use `now` issue a Shutdown to the ttrpc
	// connection to drain any active requests.
	return nil, errdefs.ErrNotImplemented
}
