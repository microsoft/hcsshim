package main

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/Microsoft/hcsshim/internal/guestrequest"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/lcow"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	eventstypes "github.com/containerd/containerd/api/events"
	containerd_v1_types "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/runtime"
	"github.com/containerd/containerd/runtime/v2/task"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

// newHcsExec creates an exec to track the lifetime of `spec` in `c` which is
// actually created on the call to `Start()`. If `id==tid` then this is the init
// exec and the exec will also start `c` on the call to `Start()` before execing
// the process `spec.Process`.
func newHcsExec(
	ctx context.Context,
	events publisher,
	tid string,
	c *hcs.System,
	id, bundle string,
	isWCOW bool,
	spec *specs.Process,
	io *iorelay) shimExec {
	logrus.WithFields(logrus.Fields{
		"tid": tid,
		"eid": id,
	}).Debug("newHcsExec")

	processCtx, processDoneCancel := context.WithCancel(context.Background())
	he := &hcsExec{
		events:            events,
		tid:               tid,
		c:                 c,
		id:                id,
		bundle:            bundle,
		isWCOW:            isWCOW,
		spec:              spec,
		io:                io,
		processCtx:        processCtx,
		processDoneCancel: processDoneCancel,
		state:             shimExecStateCreated,
		exitStatus:        255, // By design for non-exited process status.
		exited:            make(chan struct{}),
	}
	go he.waitForContainerExit()
	return he
}

var _ = (shimExec)(&hcsExec{})

type hcsExec struct {
	events publisher
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
	// describing this process. If `id==tid` the process is described in the
	// `config.json`.
	//
	// This MUST be treated as read only in the lifetime of the exec.
	bundle string
	// isWCOW is set to `true` when this process is part of a Windows OCI spec.
	//
	// This MUST be treated as read only in the lifetime of the exec.
	isWCOW bool
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
	io                *iorelay
	processCtx        context.Context
	processDoneCancel context.CancelFunc

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

func (he *hcsExec) ID() string {
	return he.id
}

func (he *hcsExec) Pid() int {
	he.sl.Lock()
	defer he.sl.Unlock()
	return he.pid
}

func (he *hcsExec) State() shimExecState {
	he.sl.Lock()
	defer he.sl.Unlock()
	return he.state
}

func (he *hcsExec) Status() *task.StateResponse {
	he.sl.Lock()
	defer he.sl.Unlock()

	var s containerd_v1_types.Status
	switch he.state {
	case shimExecStateCreated:
		s = containerd_v1_types.StatusCreated
	case shimExecStateRunning:
		s = containerd_v1_types.StatusRunning
	case shimExecStateExited:
		s = containerd_v1_types.StatusStopped
	}

	return &task.StateResponse{
		ID:         he.tid,
		ExecID:     he.id,
		Bundle:     he.bundle,
		Pid:        uint32(he.pid),
		Status:     s,
		Stdin:      he.io.StdinPath(),
		Stdout:     he.io.StdoutPath(),
		Stderr:     he.io.StderrPath(),
		Terminal:   he.io.Terminal(),
		ExitStatus: he.exitStatus,
		ExitedAt:   he.exitedAt,
	}
}

func (he *hcsExec) Start(ctx context.Context) error {
	logrus.WithFields(logrus.Fields{
		"tid": he.tid,
		"eid": he.id,
	}).Debug("hcsExec::Start")

	he.sl.Lock()
	defer he.sl.Unlock()
	if he.state != shimExecStateCreated {
		return newExecInvalidStateError(he.tid, he.id, he.state, "start")
	}
	if he.id == he.tid {
		// This is the init exec. We need to start the container itself
		err := he.c.Start()
		if err != nil {
			return err
		}
	}
	var (
		proc *hcs.Process
		err  error
	)
	if he.isWCOW {
		wpp := &hcsschema.ProcessParameters{
			CommandLine:      he.spec.CommandLine,
			User:             he.spec.User.Username,
			WorkingDirectory: he.spec.Cwd,
			EmulateConsole:   he.spec.Terminal,
			CreateStdInPipe:  he.io.StdinPath() != "",
			CreateStdOutPipe: he.io.StdoutPath() != "",
			CreateStdErrPipe: he.io.StderrPath() != "",
		}

		if he.spec.CommandLine == "" {
			wpp.CommandLine = escapeArgs(he.spec.Args)
		}

		environment := make(map[string]string)
		for _, v := range he.spec.Env {
			s := strings.SplitN(v, "=", 2)
			if len(s) == 2 && len(s[1]) > 0 {
				environment[s[0]] = s[1]
			}
		}
		wpp.Environment = environment

		if he.spec.ConsoleSize != nil {
			wpp.ConsoleSize = []int32{
				int32(he.spec.ConsoleSize.Height),
				int32(he.spec.ConsoleSize.Width),
			}
		}
		proc, err = he.c.CreateProcess(wpp)
	} else {
		lpp := &lcow.ProcessParameters{
			ProcessParameters: hcsschema.ProcessParameters{
				CreateStdInPipe:  he.io.StdinPath() != "",
				CreateStdOutPipe: he.io.StdoutPath() != "",
				CreateStdErrPipe: he.io.StderrPath() != "",
			},
		}
		if he.id != he.tid {
			// An init exec passes the process as part of the config. We only pass
			// the spec if this is a true exec.
			lpp.OCIProcess = he.spec
		}
		proc, err = he.c.CreateProcess(lpp)
	}
	if err != nil {
		return err
	}
	he.p = proc

	in, out, serr, err := he.p.Stdio()
	if err != nil {
		he.p.Kill()
		he.setExitedL(1)
		he.close()

		if he.id == he.tid {
			// We just started the container as well. Force kill it here
			he.c.Terminate()
		}
		return err
	}

	he.io.BeginRelay(in, out, serr)

	// Assign the PID and transition the state.
	he.pid = he.p.Pid()
	he.state = shimExecStateRunning

	// Publish the task/exec start event. This MUST happen before waitForExit to
	// avoid publishing the exit pervious to the start.
	if he.id != he.tid {
		he.events(
			runtime.TaskExecStartedEventTopic,
			&eventstypes.TaskExecStarted{
				ContainerID: he.tid,
				ExecID:      he.id,
				Pid:         uint32(he.pid),
			})
	} else {
		he.events(
			runtime.TaskStartEventTopic,
			&eventstypes.TaskStart{
				ContainerID: he.tid,
				Pid:         uint32(he.pid),
			})
	}

	// wait in the background for the exit.
	go he.waitForExit()
	return nil
}

func (he *hcsExec) Kill(ctx context.Context, signal uint32) error {
	logrus.WithFields(logrus.Fields{
		"tid":    he.tid,
		"eid":    he.id,
		"signal": signal,
	}).Debug("hcsExec::Kill")

	he.sl.Lock()
	defer he.sl.Unlock()
	switch he.state {
	case shimExecStateCreated:
		// Created state kill is just a state transition
		// TODO: What are the right values here?
		he.state = shimExecStateExited
		he.exitStatus = 1
		he.exitedAt = time.Now()
		return nil
	case shimExecStateRunning:
		// TODO: We need to detect that the guest supports Signal process else
		// issue a kill here.
		return he.p.Signal(guestrequest.SignalProcessOptions{
			Signal: int(signal),
		})
	case shimExecStateExited:
		return nil
	default:
		return newExecInvalidStateError(he.tid, he.id, he.state, "kill")
	}
}

func (he *hcsExec) ResizePty(ctx context.Context, width, height uint32) error {
	logrus.WithFields(logrus.Fields{
		"tid":    he.tid,
		"eid":    he.id,
		"width":  width,
		"height": height,
	}).Debug("hcsExec::ResizePty")

	he.sl.Lock()
	defer he.sl.Unlock()
	if he.state != shimExecStateRunning {
		return newExecInvalidStateError(he.tid, he.id, he.state, "resizepty")
	}
	if !he.io.Terminal() {
		return errors.Wrapf(errdefs.ErrFailedPrecondition, "exec: '%s' in task: '%s' is not a tty", he.id, he.tid)
	}

	return he.p.ResizeConsole(uint16(width), uint16(height))
}

func (he *hcsExec) CloseIO(ctx context.Context, stdin bool) error {
	logrus.WithFields(logrus.Fields{
		"tid":   he.tid,
		"eid":   he.id,
		"stdin": stdin,
	}).Debug("hcsExec::CloseIO")

	he.sl.Lock()
	defer he.sl.Unlock()

	if !he.closedIO {
		he.closedIO = true
		he.io.CloseStdin()
		if he.p != nil {
			err := he.p.CloseStdin()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (he *hcsExec) Wait(ctx context.Context) *task.StateResponse {
	logrus.WithFields(logrus.Fields{
		"tid": he.tid,
		"eid": he.id,
	}).Debug("hcsExec::Wait")

	<-he.exited
	return he.Status()
}

// waitForExit asynchronously waits for the `he.p` to exit. Once exited will set
// `he.state`, `he.exitStatus`, and `he.exitedAt` if not already set.
//
// This MUST be called via a goroutine.
func (he *hcsExec) waitForExit() {
	err := he.p.Wait()
	code := 1
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"tid":           he.tid,
			"eid":           he.id,
			logrus.ErrorKey: err,
		}).Error("hcsExec::waitForExit::Wait")
	} else {
		c, err := he.p.ExitCode()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"tid":           he.tid,
				"eid":           he.id,
				logrus.ErrorKey: err,
			}).Error("hcsExec::waitForExit::ExitCode")
		} else {
			code = c
			logrus.WithFields(logrus.Fields{
				"tid":      he.tid,
				"eid":      he.id,
				"exitCode": code,
			}).Debug("hcsExec::waitForExit::ExitCode")
		}
	}
	he.sl.Lock()
	// If the exec closes before the container we set status here.
	he.setExitedL(code)
	he.sl.Unlock()
	he.close()
}

// setExitedL sets the process exit state.
//
// The caller MUST hold `he.sl` previous to calling this method.
//
// The caller MUST free `he.exited` in order to unblock waiters depending on
// `he.io` state the caller MAY optionally wait for `he.io` to complete.
func (he *hcsExec) setExitedL(code int) {
	if he.state != shimExecStateExited {
		he.state = shimExecStateExited
		he.exitStatus = uint32(code)
		he.exitedAt = time.Now()
		if he.p != nil {
			he.p.Close()
		}

		// Issue the process done cancellation so that the container wait is
		// cleaned up.
		he.processDoneCancel()
	}
}

// waitForContainerExit asynchronously waits for `he.c` to exit. Once exited
// will set `he.state`, `he.exitStatus`, and `he.exitedAt` if not already set.
//
// This MUST be called via a goroutine at exec create.
func (he *hcsExec) waitForContainerExit() {
	cexit := make(chan error)
	go func() {
		cexit <- he.c.Wait()
	}()
	select {
	case <-cexit:
		// Container exited first. We need to force the process into the exited
		// state and cleanup any resources
		he.sl.Lock()
		he.setExitedL(1)
		he.sl.Unlock()
		he.close()
	case <-he.processCtx.Done():
		// Process exited first. This is the normal case do nothing.
	}
}

func (he *hcsExec) close() {
	he.exitedOnce.Do(func() {
		he.io.Wait()

		// Publish the exited event
		status := he.Status()
		he.events(
			runtime.TaskExitEventTopic,
			&eventstypes.TaskExit{
				ContainerID: he.tid,
				ID:          he.id,
				Pid:         status.Pid,
				ExitStatus:  status.ExitStatus,
				ExitedAt:    status.ExitedAt,
			})

		close(he.exited)
	})
}

// escapeArgs makes a Windows-style escaped command line from a set of arguments
func escapeArgs(args []string) string {
	escapedArgs := make([]string, len(args))
	for i, a := range args {
		escapedArgs[i] = windows.EscapeArg(a)
	}
	return strings.Join(escapedArgs, " ")
}
