package main

import (
	"context"
	"sync"
	"time"

	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/containerd/containerd/errdefs"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func newWcowPodSandboxTask(id, bundle string, parent *uvm.UtilityVM) shimTask {
	wpst := &wcowPodSandboxTask{
		id:   id,
		init: newWcowPodSandboxExec(id, bundle),
	}
	if parent != nil {
		go func() {
			werr := parent.Wait()
			if werr != nil && !hcs.IsAlreadyClosed(werr) {
				logrus.WithFields(logrus.Fields{
					"tid":           id,
					logrus.ErrorKey: werr,
				}).Error("newWcowPodSandboxTask - UVM Wait failed")
			}
			// The UVM came down. Force transition the init task (if it wasn't
			// already) to unblock any waiters since the platform wont send any
			// events for this fake process.
			wpst.init.Kill(context.Background(), 0x0)
		}()
	}
	return wpst
}

var _ = (shimTask)(&wcowPodSandboxTask{})

// wcowPodSandboxTask is a special task type that actually holds no real
// resources due to various design differences between Linux/Windows.
//
// For more information on why we can have this stub and in what invariant cases
// it makes sense please see `wcowPodExec`.
//
// Note: If this is a Hypervisor Isolated WCOW sandbox then we do actually track
// the lifetime of the UVM for a WCOW POD but the UVM will have no WCOW
// container/exec init representing the actual POD Sandbox task.
type wcowPodSandboxTask struct {
	// id is the id of this task when it is created.
	//
	// It MUST be treated as read only in the liftetime of the task.
	id string
	// init is the init process of the container.
	//
	// Note: the invariant `container state == init.State()` MUST be true. IE:
	// if the init process exits the container as a whole and all exec's MUST
	// exit.
	//
	// It MUST be treated as read only in the lifetime of the task.
	init *wcowPodSandboxExec
	// host is the hosting VM for this task if hypervisor isolated. If
	// `host==nil` this is an Argon task so no UVM cleanup is required.
	host *uvm.UtilityVM

	closeOnce sync.Once
}

func (wpst *wcowPodSandboxTask) ID() string {
	return wpst.id
}

func (wpst *wcowPodSandboxTask) GetExec(eid string) (shimExec, error) {
	if eid == "" {
		return wpst.init, nil
	}
	// Cannot exec in an a WCOW sandbox container so all non-init calls fail here.
	return nil, errors.Wrapf(errdefs.ErrNotFound, "exec: '%s' in task: '%s' not found", eid, wpst.id)
}

func (wpst *wcowPodSandboxTask) KillExec(ctx context.Context, eid string, signal uint32, all bool) error {
	logrus.WithFields(logrus.Fields{
		"tid":    wpst.id,
		"eid":    eid,
		"signal": signal,
		"all":    all,
	}).Debug("wcowPodSandboxTask::KillExec")

	e, err := wpst.GetExec(eid)
	if err != nil {
		return err
	}
	if all && eid != "" {
		return errors.Wrapf(errdefs.ErrFailedPrecondition, "cannot signal all for non-empty exec: '%s'", eid)
	}
	err = e.Kill(ctx, signal)
	if err != nil {
		return err
	}
	if eid == "" {
		// We killed the fake init task. Bring down the uvm.
		wpst.closeOnce.Do(func() {
			if wpst.host != nil {
				wpst.host.Close()
			}
		})
	}
	return nil
}

func (wpst *wcowPodSandboxTask) DeleteExec(ctx context.Context, eid string) (int, uint32, time.Time, error) {
	logrus.WithFields(logrus.Fields{
		"tid": wpst.id,
		"eid": eid,
	}).Debug("wcowPodSandboxTask::DeleteExec")

	e, err := wpst.GetExec(eid)
	if err != nil {
		return 0, 0, time.Time{}, err
	}
	state := e.State()
	if state != shimExecStateExited {
		return 0, 0, time.Time{}, newExecInvalidStateError(wpst.id, eid, state, "delete")
	}
	status := e.Status()
	return int(status.Pid), status.ExitStatus, status.ExitedAt, nil
}

func (wpst *wcowPodSandboxTask) Pids(ctx context.Context) ([]shimTaskPidPair, error) {
	logrus.WithFields(logrus.Fields{
		"tid": wpst.id,
	}).Debug("wcowPodSandboxTask::Pids")

	return []shimTaskPidPair{
		shimTaskPidPair{
			Pid:    wpst.init.Pid(),
			ExecID: wpst.init.ID(),
		},
	}, nil
}
