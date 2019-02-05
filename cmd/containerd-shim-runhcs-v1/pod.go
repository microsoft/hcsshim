package main

import (
	"context"
	"sync"

	"github.com/containerd/containerd/errdefs"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// shimPod represents the logical grouping of all tasks in a single set of
// shared namespaces. The pod sandbox (container) is represented by the task
// that matches the `shimPod.ID()`
type shimPod interface {
	// ID is the id of the task representing the pause (sandbox) container.
	ID() string
	// GetTask returns a task in this pod that matches `tid`.
	//
	// If `tid` is not found, this pod MUST return `errdefs.ErrNotFound`.
	GetTask(tid string) (shimTask, error)
	// KillTask sends `signal` to task that matches `tid`.
	//
	// If `tid` is not found, this pod MUST return `errdefs.ErrNotFound`.
	//
	// If `tid==ID() && eid == "" && all == true` this pod will send `signal` to
	// all tasks in the pod and lastly send `signal` to the sandbox itself.
	//
	// If `all == true && eid != ""` this pod MUST return
	// `errdefs.ErrFailedPrecondition`.
	//
	// A call to `KillTask` is only valid when the exec found by `tid,eid` is in
	// the `shimExecStateRunning` state. If the exec is not in this state this
	// pod MUST return `errdefs.ErrFailedPrecondition`.
	KillTask(ctx context.Context, tid, eid string, signal uint32, all bool) error
}

var _ = (shimPod)(&pod{})

type pod struct {
	// id is the id of the sandbox task when the pod is created.
	//
	// It MUST be treated as read only in the lifetime of the pod.
	id string
	// sandboxTask is the task that represents the sandbox.
	//
	// Note: The invariant `id==sandboxTask.ID()` MUST be true.
	//
	// It MUST be treated as read only in the lifetime of the pod.
	sandboxTask shimTask

	workloadTasks sync.Map
}

func (p *pod) ID() string {
	return p.id
}

func (p *pod) GetTask(tid string) (shimTask, error) {
	if tid == p.id {
		return p.sandboxTask, nil
	}
	raw, loaded := p.workloadTasks.Load(tid)
	if !loaded {
		return nil, errors.Wrapf(errdefs.ErrNotFound, "task with id: '%s' not found", tid)
	}
	return raw.(shimTask), nil
}

func (p *pod) KillTask(ctx context.Context, tid, eid string, signal uint32, all bool) error {
	logrus.WithFields(logrus.Fields{
		"tid":    tid,
		"eid":    eid,
		"signal": signal,
		"all":    all,
	}).Debug("pod::KillTask")

	t, err := p.GetTask(tid)
	if err != nil {
		return err
	}
	if all && eid != "" {
		return errors.Wrapf(errdefs.ErrFailedPrecondition, "cannot signal all with non empty ExecID: '%s'", eid)
	}
	eg := errgroup.Group{}
	if all && tid == p.id {
		// We are in a kill all on the sandbox task. Signal everything.
		p.workloadTasks.Range(func(key, value interface{}) bool {
			wt := value.(shimTask)
			eg.Go(func() error {
				return wt.KillExec(ctx, eid, signal, all)
			})

			// iterate all
			return false
		})
	}
	eg.Go(func() error {
		return t.KillExec(ctx, eid, signal, all)
	})
	return eg.Wait()
}
