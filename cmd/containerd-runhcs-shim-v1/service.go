package shim

import (
	"context"

	"github.com/containerd/containerd/runtime/v2/task"
	google_protobuf1 "github.com/gogo/protobuf/types"
	"github.com/sirupsen/logrus"
)

func beginActivity(activity string, fields logrus.Fields) {
	logrus.WithFields(fields).Info(activity)
}

func endActivity(activity string, fields logrus.Fields, err error) {
	if err != nil {
		fields["result"] = "Error"
		fields[logrus.ErrorKey] = err
		logrus.WithFields(fields).Error(activity)
	} else {
		fields["result"] = "Success"
		logrus.WithFields(fields).Info(activity)
	}
}

var _ = (task.TaskService)(&service{})

type service struct {
}

func (s *service) State(ctx context.Context, req *task.StateRequest) (_ *task.StateResponse, err error) {
	const activity = "State"
	af := logrus.Fields{
		"tid": req.ID,
		"eid": req.ExecID,
	}
	beginActivity(activity, af)
	defer endActivity(activity, af, err)

	return s.stateInternal(ctx, req)
}

func (s *service) Create(ctx context.Context, req *task.CreateTaskRequest) (_ *task.CreateTaskResponse, err error) {
	const activity = "Create"
	beginActivity(activity, logrus.Fields{
		"tid":              req.ID,
		"bundle":           req.Bundle,
		"rootfs":           req.Rootfs,
		"terminal":         req.Terminal,
		"stdin":            req.Stdin,
		"stdout":           req.Stdout,
		"stderr":           req.Stderr,
		"checkpoint":       req.Checkpoint,
		"parentcheckpoint": req.ParentCheckpoint,
	})
	defer endActivity(activity, logrus.Fields{
		"tid": req.ID,
	}, err)

	return s.createInternal(ctx, req)
}

func (s *service) Start(ctx context.Context, req *task.StartRequest) (_ *task.StartResponse, err error) {
	const activity = "Start"
	af := logrus.Fields{
		"tid": req.ID,
		"eid": req.ExecID,
	}
	beginActivity(activity, af)
	defer endActivity(activity, af, err)

	return s.startInternal(ctx, req)
}

func (s *service) Delete(ctx context.Context, req *task.DeleteRequest) (_ *task.DeleteResponse, err error) {
	const activity = "Delete"
	af := logrus.Fields{
		"tid": req.ID,
		"eid": req.ExecID,
	}
	beginActivity(activity, af)
	defer endActivity(activity, af, err)

	return s.deleteInternal(ctx, req)
}

func (s *service) Pids(ctx context.Context, req *task.PidsRequest) (_ *task.PidsResponse, err error) {
	const activity = "Pids"
	af := logrus.Fields{
		"tid": req.ID,
	}
	beginActivity(activity, af)
	defer endActivity(activity, af, err)

	return s.pidsInternal(ctx, req)
}

func (s *service) Pause(ctx context.Context, req *task.PauseRequest) (_ *google_protobuf1.Empty, err error) {
	const activity = "Pause"
	af := logrus.Fields{
		"tid": req.ID,
	}
	beginActivity(activity, af)
	defer endActivity(activity, af, err)

	return s.pauseInternal(ctx, req)
}

func (s *service) Resume(ctx context.Context, req *task.ResumeRequest) (_ *google_protobuf1.Empty, err error) {
	const activity = "Resume"
	af := logrus.Fields{
		"tid": req.ID,
	}
	beginActivity(activity, af)
	defer endActivity(activity, af, err)

	return s.resumeInternal(ctx, req)
}

func (s *service) Checkpoint(ctx context.Context, req *task.CheckpointTaskRequest) (_ *google_protobuf1.Empty, err error) {
	const activity = "Checkpoint"
	af := logrus.Fields{
		"tid":  req.ID,
		"path": req.Path,
	}
	beginActivity(activity, af)
	defer endActivity(activity, af, err)

	return s.checkpointInternal(ctx, req)
}

func (s *service) Kill(ctx context.Context, req *task.KillRequest) (_ *google_protobuf1.Empty, err error) {
	const activity = "Kill"
	af := logrus.Fields{
		"tid":    req.ID,
		"eid":    req.ExecID,
		"signal": req.Signal,
		"all":    req.All,
	}
	beginActivity(activity, af)
	defer endActivity(activity, af, err)

	return s.killInternal(ctx, req)
}

func (s *service) Exec(ctx context.Context, req *task.ExecProcessRequest) (_ *google_protobuf1.Empty, err error) {
	const activity = "Exec"
	af := logrus.Fields{
		"tid":      req.ID,
		"eid":      req.ExecID,
		"terminal": req.Terminal,
		"stdin":    req.Stdin,
		"stdout":   req.Stdout,
		"stderr":   req.Stderr,
	}
	beginActivity(activity, af)
	defer endActivity(activity, af, err)

	return s.execInternal(ctx, req)
}

func (s *service) ResizePty(ctx context.Context, req *task.ResizePtyRequest) (_ *google_protobuf1.Empty, err error) {
	const activity = "ResizePty"
	af := logrus.Fields{
		"tid":    req.ID,
		"eid":    req.ExecID,
		"width":  req.Width,
		"height": req.Height,
	}
	beginActivity(activity, af)
	defer endActivity(activity, af, err)

	return s.resizePtyInternal(ctx, req)
}

func (s *service) CloseIO(ctx context.Context, req *task.CloseIORequest) (_ *google_protobuf1.Empty, err error) {
	const activity = "CloseIO"
	af := logrus.Fields{
		"tid":   req.ID,
		"eid":   req.ExecID,
		"stdin": req.Stdin,
	}
	beginActivity(activity, af)
	defer endActivity(activity, af, err)

	return s.closeIOInternal(ctx, req)
}

func (s *service) Update(ctx context.Context, req *task.UpdateTaskRequest) (_ *google_protobuf1.Empty, err error) {
	const activity = "Update"
	af := logrus.Fields{
		"tid": req.ID,
	}
	beginActivity(activity, af)
	defer endActivity(activity, af, err)

	return s.updateInternal(ctx, req)
}

func (s *service) Wait(ctx context.Context, req *task.WaitRequest) (_ *task.WaitResponse, err error) {
	const activity = "Wait"
	af := logrus.Fields{
		"tid": req.ID,
		"eid": req.ExecID,
	}
	beginActivity(activity, af)
	defer endActivity(activity, af, err)

	return s.waitInternal(ctx, req)
}

func (s *service) Stats(ctx context.Context, req *task.StatsRequest) (_ *task.StatsResponse, err error) {
	const activity = "Stats"
	af := logrus.Fields{
		"tid": req.ID,
	}
	beginActivity(activity, af)
	defer endActivity(activity, af, err)

	return s.statsInternal(ctx, req)
}

func (s *service) Connect(ctx context.Context, req *task.ConnectRequest) (_ *task.ConnectResponse, err error) {
	const activity = "Connect"
	af := logrus.Fields{
		"tid": req.ID,
	}
	beginActivity(activity, af)
	defer endActivity(activity, af, err)

	return s.connectInternal(ctx, req)
}

func (s *service) Shutdown(ctx context.Context, req *task.ShutdownRequest) (_ *google_protobuf1.Empty, err error) {
	const activity = "Shutdown"
	af := logrus.Fields{
		"tid": req.ID,
		"now": req.Now,
	}
	beginActivity(activity, af)
	defer endActivity(activity, af, err)

	return s.shutdownInternal(ctx, req)
}
