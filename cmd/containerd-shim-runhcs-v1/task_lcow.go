package main

import (
	"context"
	"sync"
	"time"

	"github.com/containerd/containerd/errdefs"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

var _ = (shimTask)(&lcowTask{})

type lcowTask struct {
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
	init shimExec

	execs sync.Map
}

func (lt *lcowTask) ID() string {
	return lt.id
}

func (lt *lcowTask) GetExec(eid string) (shimExec, error) {
	if eid == "" {
		return lt.init, nil
	}
	raw, loaded := lt.execs.Load(eid)
	if !loaded {
		return nil, errors.Wrapf(errdefs.ErrNotFound, "exec: '%s' in task: '%s' not found", eid, lt.id)
	}
	return raw.(shimExec), nil
}

func (lt *lcowTask) KillExec(ctx context.Context, eid string, signal uint32, all bool) error {
	logrus.WithFields(logrus.Fields{
		"tid":    lt.id,
		"eid":    eid,
		"signal": signal,
		"all":    all,
	}).Debug("lcowTask::KillExec")

	e, err := lt.GetExec(eid)
	if err != nil {
		return err
	}
	if all && eid != "" {
		return errors.Wrapf(errdefs.ErrFailedPrecondition, "cannot signal all for non-empty exec: '%s'", eid)
	}
	eg := errgroup.Group{}
	if all {
		// We are in a kill all on the init task. Signal everything.
		lt.execs.Range(func(key, value interface{}) bool {
			ex := value.(shimExec)
			eg.Go(func() error {
				return ex.Kill(ctx, signal)
			})

			// iterate all
			return false
		})
	} else if eid == "" {
		// We are in a kill of the init task. Verify all exec's are in the
		// non-running state.
		invalid := false
		lt.execs.Range(func(key, value interface{}) bool {
			ex := value.(shimExec)
			if ex.State() != shimExecStateExited {
				invalid = true
				// we have an invalid state. Stop iteration.
				return true
			}
			// iterate next valid
			return false
		})
		if invalid {
			return errors.Wrap(errdefs.ErrFailedPrecondition, "cannot signal init exec with un-exited additional exec's")
		}
	}
	eg.Go(func() error {
		return e.Kill(ctx, signal)
	})
	err = eg.Wait()
	if err != nil {
		return err
	}
	if eid == "" {
		// We just killed the init process. Tear down the container too.

		// TODO: JTERRY75 we need to kill the container here.
	}
	return nil
}

func (lt *lcowTask) DeleteExec(ctx context.Context, eid string) (int, uint32, time.Time, error) {
	logrus.WithFields(logrus.Fields{
		"tid": lt.id,
		"eid": eid,
	}).Debug("lcowTask::DeleteExec")

	e, err := lt.GetExec(eid)
	if err != nil {
		return 0, 0, time.Time{}, err
	}
	if eid == "" {
		// We are deleting the init exec. Verify all additional exec's are exited as well
		invalid := false
		lt.execs.Range(func(key, value interface{}) bool {
			ex := value.(shimExec)
			if ex.State() != shimExecStateExited {
				invalid = true
				// we have an invalid state. Stop iteration.
				return true
			}
			// iterate next valid
			return false
		})
		if invalid {
			return 0, 0, time.Time{}, errors.Wrap(errdefs.ErrFailedPrecondition, "cannot delete init exec with un-exited additional exec's")
		}
	}
	state := e.State()
	if state != shimExecStateExited {
		return 0, 0, time.Time{}, newExecInvalidStateError(lt.id, eid, state, "delete")
	}
	if eid != "" {
		lt.execs.Delete(eid)
	}
	status := e.Status()
	return int(status.Pid), status.ExitStatus, status.ExitedAt, nil
}

func (lt *lcowTask) Pids(ctx context.Context) ([]shimTaskPidPair, error) {
	logrus.WithFields(logrus.Fields{
		"tid": lt.id,
	}).Debug("lcowTask::Pids")

	return nil, errdefs.ErrNotImplemented
}
