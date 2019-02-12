package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

func newHcsStandaloneTask(ctx context.Context, req *task.CreateTaskRequest, s *specs.Spec) (shimTask, error) {
	logrus.WithFields(logrus.Fields{
		"tid": req.ID,
	}).Debug("newHcsStandloneTask")

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

			// In order for the UVM sandbox.vhdx not to collide with the actual
			// nested Argon sandbox.vhdx we append the \vm folder to the last
			// entry in the list.
			layersLen := len(s.Windows.LayerFolders)
			layers := make([]string, layersLen)
			copy(layers, s.Windows.LayerFolders)

			vmPath := filepath.Join(layers[layersLen-1], "vm")
			err := os.MkdirAll(vmPath, 0)
			if err != nil {
				return nil, err
			}
			layers[layersLen-1] = vmPath
			wopts.LayerFolders = layers

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

	shim, err := newHcsTask(ctx, parent, true, req, s)
	if err != nil {
		if parent != nil {
			parent.Close()
		}
		return nil, err
	}
	return shim, nil
}

// newHcsTask creates a container within `parent` and its init exec process in
// the `shimExecCreated` state and returns the task that tracks its lifetime.
//
// If `parent == nil` the container is created on the host.
func newHcsTask(ctx context.Context, parent *uvm.UtilityVM, ownsParent bool, req *task.CreateTaskRequest, s *specs.Spec) (shimTask, error) {
	logrus.WithFields(logrus.Fields{
		"tid":        req.ID,
		"ownsParent": ownsParent,
	}).Debug("newHcsTask")

	owner, err := os.Executable()
	if err != nil {
		return nil, err
	}

	io, err := newRelay(ctx, req.Stdin, req.Stdout, req.Stderr, req.Terminal)
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

	ht := &hcsTask{
		id:     req.ID,
		isWCOW: oci.IsWCOW(s),
		c:      system,
		cr:     resources,
	}
	if ownsParent {
		ht.ownsHost = true
		ht.host = parent
	}
	ht.init = newHcsExec(
		ctx,
		req.ID,
		system,
		"",
		req.Bundle,
		ht.isWCOW,
		s.Process,
		io)
	return ht, nil
}

var _ = (shimTask)(&hcsTask{})

// hcsTask is a generic task that represents a WCOW Container (process or
// hypervisor isolated), or a LCOW Container. This task MAY own the UVM the
// container is in but in the case of a POD it may just track the UVM for
// container lifetime management. In the case of ownership when the init
// task/exec is stopped the UVM itself will be stopped as well.
type hcsTask struct {
	// id is the id of this task when it is created.
	//
	// It MUST be treated as read only in the liftetime of the task.
	id string
	// isWCOW is set to `true` if this is a task representing a Windows container.
	//
	// It MUST be treated as read only in the liftetime of the task.
	isWCOW bool
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

	// ecl is the exec create lock for all non-init execs and MUST be held
	// durring create to prevent ID duplication.
	ecl   sync.Mutex
	execs sync.Map

	closeOnce sync.Once
}

func (ht *hcsTask) ID() string {
	return ht.id
}

func (ht *hcsTask) CreateExec(ctx context.Context, req *task.ExecProcessRequest, spec *specs.Process) error {
	logrus.WithFields(logrus.Fields{
		"tid": ht.id,
		"eid": req.ExecID,
	}).Debug("hcsTask::CreateExec")

	ht.ecl.Lock()
	defer ht.ecl.Unlock()

	// If the task exists or we got a request for "" which is the init task
	// fail.
	if _, loaded := ht.execs.Load(req.ExecID); loaded || req.ExecID == "" {
		return errors.Wrapf(errdefs.ErrAlreadyExists, "exec: '%s' in task: '%s' already exists", req.ExecID, ht.id)
	}

	if ht.init.State() != shimExecStateRunning {
		return errors.Wrapf(errdefs.ErrFailedPrecondition, "exec: '' in task: '%s' must be running to create additional execs", ht.id)
	}

	io, err := newRelay(ctx, req.Stdin, req.Stdout, req.Stderr, req.Terminal)
	if err != nil {
		return err
	}
	he := newHcsExec(ctx, ht.id, ht.c, req.ExecID, ht.init.Status().Bundle, ht.isWCOW, spec, io)
	ht.execs.Store(req.ExecID, he)
	return nil
}

func (ht *hcsTask) GetExec(eid string) (shimExec, error) {
	if eid == "" {
		return ht.init, nil
	}
	raw, loaded := ht.execs.Load(eid)
	if !loaded {
		return nil, errors.Wrapf(errdefs.ErrNotFound, "exec: '%s' in task: '%s' not found", eid, ht.id)
	}
	return raw.(shimExec), nil
}

func (ht *hcsTask) KillExec(ctx context.Context, eid string, signal uint32, all bool) error {
	logrus.WithFields(logrus.Fields{
		"tid":    ht.id,
		"eid":    eid,
		"signal": signal,
		"all":    all,
	}).Debug("hcsTask::KillExec")

	e, err := ht.GetExec(eid)
	if err != nil {
		return err
	}
	if all && eid != "" {
		return errors.Wrapf(errdefs.ErrFailedPrecondition, "cannot signal all for non-empty exec: '%s'", eid)
	}
	eg := errgroup.Group{}
	if all {
		// We are in a kill all on the init task. Signal everything.
		ht.execs.Range(func(key, value interface{}) bool {
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
		ht.execs.Range(func(key, value interface{}) bool {
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
		ht.closeOnce.Do(func() {
			serr := ht.c.Shutdown()
			if serr != nil {
				if hcs.IsPending(serr) {
					const shutdownTimeout = time.Minute * 5
					werr := ht.c.WaitTimeout(shutdownTimeout)
					if err != nil {
						if hcs.IsTimeout(werr) {
							// TODO: Log this?
						}
						ht.c.Terminate()
					}
				} else {
					ht.c.Terminate()
				}
			}
			hcsoci.ReleaseResources(ht.cr, ht.host, true)
			if ht.ownsHost && ht.host != nil {
				ht.host.Close()
			}
		})
	}
	return nil
}

func (ht *hcsTask) DeleteExec(ctx context.Context, eid string) (int, uint32, time.Time, error) {
	logrus.WithFields(logrus.Fields{
		"tid": ht.id,
		"eid": eid,
	}).Debug("hcsTask::DeleteExec")

	e, err := ht.GetExec(eid)
	if err != nil {
		return 0, 0, time.Time{}, err
	}
	if eid == "" {
		// We are deleting the init exec. Verify all additional exec's are exited as well
		invalid := false
		ht.execs.Range(func(key, value interface{}) bool {
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
		return 0, 0, time.Time{}, newExecInvalidStateError(ht.id, eid, state, "delete")
	}
	if eid != "" {
		ht.execs.Delete(eid)
	}
	status := e.Status()
	return int(status.Pid), status.ExitStatus, status.ExitedAt, nil
}

func (ht *hcsTask) Pids(ctx context.Context) ([]shimTaskPidPair, error) {
	logrus.WithFields(logrus.Fields{
		"tid": ht.id,
	}).Debug("hcsTask::Pids")

	return nil, errdefs.ErrNotImplemented
}
