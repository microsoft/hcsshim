package main

import (
	"context"
	"sync"
	"time"

	"github.com/Microsoft/hcsshim/internal/guestrequest"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/lcow"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	containerd_v1_types "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/runtime/v2/task"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func newLcowExec(
	tid string,
	c *hcs.System,
	id, bundle, stdin, stdout, stderr string,
	terminal bool,
	spec *specs.Process,
	io *iorelay) shimExec {
	le := &lcowExec{
		tid:        tid,
		c:          c,
		id:         id,
		bundle:     bundle,
		stdin:      stdin,
		stdout:     stdout,
		stderr:     stderr,
		terminal:   terminal,
		spec:       spec,
		io:         io,
		state:      shimExecStateCreated,
		exitStatus: 255, // By design for non-exited process status.
		exited:     make(chan struct{}),
	}
	go le.waitForContainerExit()
	return le
}

var _ = (shimExec)(&lcowExec{})

type lcowExec struct {
	// tid is the task id of the container hosting this process.
	//
	// This MUST be treated as read only in the lifetime of the exec.
	tid string
	// c is the hosting container for this exec.
	//
	// This MUST be treated as read only in the lifetime of the exec.
	c *hcs.System
	// id is the id of this process.
	//
	// This MUST be treated as read only in the lifetime of the exec.
	id string
	// bundle is the on disk path to the folder containing the `process.json`
	// describing this process. If `id==""` the process is described in the
	// `config.json`.
	//
	// This MUST be treated as read only in the lifetime of the exec.
	bundle string
	// stdin is the path of the `stdin` connection passed at create time. If
	// `stdin==""` then no `stdin` connection was requested.
	//
	// This MUST be treated as read only in the lifetime of the exec.
	stdin string
	// stdout is the path of the `stdout` connection passed at create time. If
	// `stdout==""` then no `stdout` connection was requested.
	//
	// This MUST be treated as read only in the lifetime of the exec.
	stdout string
	// stderr is the path of the `stderr` connection passed at create time. If
	// `stderr==""` then no `stderr` connection was requested.
	//
	// This MUST be treated as read only in the lifetime of the exec.
	stderr string
	// terminal signifies if this process is emulating a terminal connection. If
	// `terminal==true` then `stderr` MUST equal `""`.
	//
	// This MUST be treated as read only in the lifetime of the exec.
	terminal bool
	// spec is the OCI Process spec that was passed in at create time. This is
	// stored because we don't actually create the process until the call to
	// `Start`.
	//
	// This MUST be treated as read only in the lifetime of the exec.
	spec *specs.Process
	// io is the relay for copying between the upstream io and the downstream
	// io. The upstream IO MUST already be connected at create time in order to
	// be valid.
	//
	// This MUST be treated as read only in the lifetime of the exec.
	io *iorelay

	// sl is the state lock that MUST be held to safely read/write any of the
	// following members.
	sl         sync.Mutex
	state      shimExecState
	pid        int
	exitStatus uint32
	exitedAt   time.Time
	p          *hcs.Process
	closedIO   bool

	// exited is a wait block which waits async for the process to exit.
	exited     chan struct{}
	exitedOnce sync.Once
}

func (le *lcowExec) ID() string {
	return le.id
}

func (le *lcowExec) Pid() int {
	le.sl.Lock()
	defer le.sl.Unlock()
	return le.pid
}

func (le *lcowExec) State() shimExecState {
	le.sl.Lock()
	defer le.sl.Unlock()
	return le.state
}

func (le *lcowExec) Status() *task.StateResponse {
	le.sl.Lock()
	defer le.sl.Unlock()

	var s containerd_v1_types.Status
	switch le.state {
	case shimExecStateCreated:
		s = containerd_v1_types.StatusCreated
	case shimExecStateRunning:
		s = containerd_v1_types.StatusRunning
	case shimExecStateExited:
		s = containerd_v1_types.StatusStopped
	}

	return &task.StateResponse{
		ID:         le.id,
		Bundle:     le.bundle,
		Pid:        uint32(le.pid),
		Status:     s,
		Stdin:      le.stdin,
		Stdout:     le.stdout,
		Stderr:     le.stderr,
		Terminal:   le.terminal,
		ExitStatus: le.exitStatus,
		ExitedAt:   le.exitedAt,
	}
}

func (le *lcowExec) Start(ctx context.Context) error {
	logrus.WithFields(logrus.Fields{
		"tid": le.tid,
		"id":  le.id,
	}).Debug("lcowExec::Start")

	le.sl.Lock()
	defer le.sl.Unlock()
	if le.state != shimExecStateCreated {
		return newExecInvalidStateError(le.tid, le.id, le.state, "start")
	}
	lpp := &lcow.ProcessParameters{
		ProcessParameters: hcsschema.ProcessParameters{
			CreateStdInPipe:  le.stdin != "",
			CreateStdOutPipe: le.stdout != "",
			CreateStdErrPipe: le.stderr != "",
		},
	}
	if le.id != "" {
		// An init exec passes the process as part of the config. We only pass
		// the spec if this is a true exec.
		lpp.OCIProcess = le.spec
	}
	p, err := le.c.CreateProcess(lpp)
	if err != nil {
		return err
	}

	in, out, serr, err := le.p.Stdio()
	if err != nil {
		p.Kill()
		return err
	}

	le.io.BeginRelay(in, out, serr)

	le.p = p
	le.pid = le.p.Pid()
	le.state = shimExecStateRunning

	// wait in the background for the exit.
	go le.waitForExit()
	return nil
}

func (le *lcowExec) Kill(ctx context.Context, signal uint32) error {
	logrus.WithFields(logrus.Fields{
		"tid":    le.tid,
		"id":     le.id,
		"signal": signal,
	}).Debug("lcowExec::Kill")

	le.sl.Lock()
	defer le.sl.Unlock()
	switch le.state {
	case shimExecStateCreated:
		// Created state kill is just a state transition
		// TODO: What are the right values here?
		le.state = shimExecStateExited
		le.exitStatus = 1
		le.exitedAt = time.Now()
		return nil
	case shimExecStateRunning:
		// TODO: We don't support a version of LCOW that doesn't support signals. We
		// should likely be reading the guest properties but we don't actually need
		// to.
		return le.p.Signal(guestrequest.SignalProcessOptions{
			Signal: int(signal),
		})
	default:
		return newExecInvalidStateError(le.tid, le.id, le.state, "kill")
	}
}

func (le *lcowExec) ResizePty(ctx context.Context, width, height uint32) error {
	logrus.WithFields(logrus.Fields{
		"tid":    le.tid,
		"id":     le.id,
		"width":  width,
		"height": height,
	}).Debug("lcowExec::ResizePty")

	le.sl.Lock()
	defer le.sl.Unlock()
	if le.state != shimExecStateRunning {
		return newExecInvalidStateError(le.tid, le.id, le.state, "resizepty")
	}
	if !le.terminal {
		return errors.Wrapf(errdefs.ErrFailedPrecondition, "exec: '%s' in task: '%s' is not a tty", le.id, le.tid)
	}

	return le.p.ResizeConsole(uint16(width), uint16(height))
}

func (le *lcowExec) CloseIO(ctx context.Context, stdin bool) error {
	logrus.WithFields(logrus.Fields{
		"tid":   le.tid,
		"id":    le.id,
		"stdin": stdin,
	}).Debug("lcowExec::CloseIO")

	le.sl.Lock()
	defer le.sl.Unlock()

	if !le.closedIO {
		le.io.CloseStdin()
		err := le.p.CloseStdin()
		if err != nil {
			return err
		}
		le.closedIO = true
	}
	return nil
}

func (le *lcowExec) Wait(ctx context.Context) *task.StateResponse {
	logrus.WithFields(logrus.Fields{
		"tid": le.tid,
		"id":  le.id,
	}).Debug("lcowExec::Wait")

	<-le.exited
	return le.Status()
}

// waitForExit asynchronously waits for the `le.p` to exit. Once exited will set
// `le.state`, `le.exitStatus`, and `le.exitedAt` if not already set.
//
// This MUST be called via a goroutine.
func (le *lcowExec) waitForExit() {
	err := le.p.Wait()
	code := 1
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"tid":           le.tid,
			"eid":           le.id,
			logrus.ErrorKey: err,
		}).Error("lcowExec::waitForExit::Wait")
	} else {
		code, err = le.p.ExitCode()
		logrus.WithFields(logrus.Fields{
			"tid":           le.tid,
			"eid":           le.id,
			logrus.ErrorKey: err,
		}).Error("lcowExec::waitForExit::ExitCode")
	}
	le.sl.Lock()
	// If the exec closes before the container we set status here.
	if le.state != shimExecStateExited {
		le.state = shimExecStateExited
		le.pid = 0
		le.exitStatus = uint32(code)
		le.exitedAt = time.Now()
		le.p.Close()
	}
	le.sl.Unlock()
	le.exitedOnce.Do(func() {
		le.io.Wait()
		close(le.exited)
	})
}

// waitForContainerExit asynchronously waits for `le.c` to exit. Once exited
// will set `le.state`, `le.exitStatus`, and `le.exitedAt` if not already set.
//
// This MUST be called via a goroutine at exec create.
func (le *lcowExec) waitForContainerExit() {
	le.c.Wait()
	le.sl.Lock()
	if le.state != shimExecStateExited {
		// If the container closes before the exec we set status here.
		le.state = shimExecStateExited
		le.pid = 0
		le.exitStatus = 1
		le.exitedAt = time.Now()
	}
	le.sl.Unlock()
	le.exitedOnce.Do(func() {
		le.io.Wait()
		close(le.exited)
	})
}
