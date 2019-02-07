package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/hcsoci"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/runtime/v2/task"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

func newLcowStandaloneTask(ctx context.Context, req *task.CreateTaskRequest, s *specs.Spec) (shimTask, error) {
	ct, _, err := oci.GetSandboxTypeAndID(s.Annotations)
	if err != nil {
		return nil, err
	}
	if ct != oci.KubernetesContainerTypeNone {
		return nil, errors.Wrapf(
			errdefs.ErrFailedPrecondition,
			"cannot create standalone task, expected no annotation: '%s': got '%s'",
			oci.KubernetesContainerTypeAnnotation,
			ct)
	}

	owner, err := os.Executable()
	if err != nil {
		return nil, err
	}

	var parent *uvm.UtilityVM
	if oci.IsIsolated(s) {
		// Create the UVM parent
		opts, err := oci.SpecToUVMCreateOpts(s, fmt.Sprintf("%s@vm", req.ID), owner)
		if err != nil {
			return nil, err
		}
		switch opts.(type) {
		case *uvm.OptionsLCOW:
			lopts := (opts).(*uvm.OptionsLCOW)
			parent, err = uvm.CreateLCOW(lopts)
			if err != nil {
				return nil, err
			}
		case *uvm.OptionsWCOW:
			wopts := (opts).(*uvm.OptionsWCOW)
			parent, err = uvm.CreateWCOW(wopts)
			if err != nil {
				return nil, err
			}
		}
		err = parent.Start()
		if err != nil {
			parent.Close()
		}
	} else if !oci.IsWCOW(s) {
		return nil, errors.Wrap(errdefs.ErrFailedPrecondition, "oci spec does not contain WCOW or LCOW spec")
	}

	shim, err := newLcowTask(parent, true, req, s)
	if err != nil {
		if parent != nil {
			parent.Close()
		}
		return nil, err
	}
	return shim, nil
}

// newLcowTask creates a container within `parent` and its init exec process in
// the `shimExecCreated` state and returns the task that tracks its lifetime.
//
// If `parent == nil` the container is created on the host.
func newLcowTask(parent *uvm.UtilityVM, ownsParent bool, req *task.CreateTaskRequest, s *specs.Spec) (shimTask, error) {
	owner, err := os.Executable()
	if err != nil {
		return nil, err
	}

	io, err := newRelay(req.Stdin, req.Stdout, req.Stderr, req.Terminal)
	if err != nil {
		return nil, err
	}

	var netNS string
	if s.Windows != nil &&
		s.Windows.Network != nil {
		netNS = s.Windows.Network.NetworkNamespace
	}
	opts := hcsoci.CreateOptions{
		ID:               req.ID,
		Owner:            owner,
		Spec:             s,
		HostingSystem:    parent,
		NetworkNamespace: netNS,
	}
	system, resources, err := hcsoci.CreateContainer(&opts)
	if err != nil {
		return nil, err
	}

	lt := &lcowTask{
		id: req.ID,
		c:  system,
		cr: resources,
	}
	if ownsParent {
		lt.ownsHost = true
		lt.host = parent
	}
	lt.init = newLcowExec(
		req.ID,
		system,
		"",
		req.Bundle,
		s.Process,
		io)
	return lt, nil
}

var _ = (shimTask)(&lcowTask{})

type lcowTask struct {
	// id is the id of this task when it is created.
	//
	// It MUST be treated as read only in the liftetime of the task.
	id string
	// c is the container backing this task.
	//
	// It MUST be treated as read only in the lifetime of this task EXCEPT after
	// a Kill to the init task in which it must be shutdown.
	c *hcs.System
	// cr is the container resources this task is holding.
	//
	// It MUST be treated as read only in the lifetime of this task EXCEPT after
	// a Kill to the init task in which all resources must be released.
	cr *hcsoci.Resources
	// init is the init process of the container.
	//
	// Note: the invariant `container state == init.State()` MUST be true. IE:
	// if the init process exits the container as a whole and all exec's MUST
	// exit.
	//
	// It MUST be treated as read only in the lifetime of the task.
	init shimExec
	// ownsHost is `true` if this task owns `host`. If so when this tasks init
	// exec shuts down it is required that `host` be shut down as well.
	ownsHost bool
	// host is the hosting VM for this exec if hypervisor isolated. If
	// `host==nil` this is an Argon task so no UVM cleanup is required.
	host *uvm.UtilityVM

	execs sync.Map

	closeOnce sync.Once
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
		lt.closeOnce.Do(func() {
			serr := lt.c.Shutdown()
			if serr != nil {
				if hcs.IsPending(serr) {
					const shutdownTimeout = time.Minute * 5
					werr := lt.c.WaitTimeout(shutdownTimeout)
					if err != nil {
						if hcs.IsTimeout(werr) {
							// TODO: Log this?
						}
						lt.c.Terminate()
					}
				} else {
					lt.c.Terminate()
				}
			}
			hcsoci.ReleaseResources(lt.cr, lt.host, true)
			if lt.ownsHost && lt.host != nil {
				lt.host.Close()
			}
		})
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
