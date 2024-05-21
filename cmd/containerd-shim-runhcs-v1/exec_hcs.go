//go:build windows

package main

import (
	"context"
	"sync"
	"time"

	eventstypes "github.com/containerd/containerd/api/events"
	task "github.com/containerd/containerd/api/runtime/task/v2"
	containerd_v1_types "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/runtime"
	"github.com/containerd/errdefs"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/otelutil"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/signals"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"
)

// newHcsExec creates an exec to track the lifetime of `spec` in `c` which is
// actually created on the call to `Start()`. If `id==tid` then this is the init
// exec and the exec will also start `c` on the call to `Start()` before execing
// the process `spec.Process`.
func newHcsExec(
	ctx context.Context,
	events publisher,
	tid string,
	host *uvm.UtilityVM,
	c cow.Container,
	id, bundle string,
	isWCOW bool,
	spec *specs.Process,
	io cmd.UpstreamIO) shimExec {
	log.G(ctx).WithFields(logrus.Fields{
		"tid":    tid,
		"eid":    id, // Init exec ID is always same as Task ID
		"bundle": bundle,
		"wcow":   isWCOW,
	}).Trace("newHcsExec")

	he := &hcsExec{
		events:      events,
		tid:         tid,
		host:        host,
		c:           c,
		id:          id,
		bundle:      bundle,
		isWCOW:      isWCOW,
		spec:        spec,
		io:          io,
		processDone: make(chan struct{}),
		state:       shimExecStateCreated,
		exitStatus:  255, // By design for non-exited process status.
		exited:      make(chan struct{}),
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
	// host is the hosting VM for `c`. If `host==nil` this exec MUST be a
	// process isolated WCOW exec.
	//
	// This MUST be treated as read only in the lifetime of the exec.
	host *uvm.UtilityVM
	// c is the hosting container for this exec.
	//
	// This MUST be treated as read only in the lifetime of the exec.
	c cow.Container
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
	// io is the upstream io connections used for copying between the upstream
	// io and the downstream io. The upstream IO MUST already be connected at
	// create time in order to be valid.
	//
	// This MUST be treated as read only in the lifetime of the exec.
	io              cmd.UpstreamIO
	processDone     chan struct{}
	processDoneOnce sync.Once

	// sl is the state lock that MUST be held to safely read/write any of the
	// following members.
	sl         sync.Mutex
	state      shimExecState
	pid        int
	exitStatus uint32
	exitedAt   time.Time
	p          *cmd.Cmd

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
		s = containerd_v1_types.Status_CREATED
	case shimExecStateRunning:
		s = containerd_v1_types.Status_RUNNING
	case shimExecStateExited:
		s = containerd_v1_types.Status_STOPPED
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
		ExitedAt:   timestamppb.New(he.exitedAt),
	}
}

func (he *hcsExec) startInternal(ctx context.Context, initializeContainer bool) (err error) {
	he.sl.Lock()
	defer he.sl.Unlock()
	if he.state != shimExecStateCreated {
		return newExecInvalidStateError(he.tid, he.id, he.state, "start")
	}
	defer func() {
		if err != nil {
			he.exitFromCreatedL(ctx, 1)
		}
	}()
	if initializeContainer {
		err = he.c.Start(ctx)
		if err != nil {
			return err
		}
		defer func() {
			if err != nil {
				_ = he.c.Terminate(ctx)
				he.c.Close()
			}
		}()
	}
	cmd := &cmd.Cmd{
		Host:   he.c,
		Stdin:  he.io.Stdin(),
		Stdout: he.io.Stdout(),
		Stderr: he.io.Stderr(),
		Log: log.G(ctx).WithFields(logrus.Fields{
			"tid": he.tid,
			"eid": he.id,
		}),
		CopyAfterExitTimeout: time.Second * 1,
	}
	if he.isWCOW || he.id != he.tid {
		// An init exec passes the process as part of the config. We only pass
		// the spec if this is a true exec.
		cmd.Spec = he.spec
	}
	err = cmd.Start()
	if err != nil {
		return err
	}
	he.p = cmd

	// Assign the PID and transition the state.
	he.pid = he.p.Process.Pid()
	he.state = shimExecStateRunning

	// Publish the task/exec start event. This MUST happen before waitForExit to
	// avoid publishing the exit previous to the start.
	if he.id != he.tid {
		if err := he.events.publishEvent(
			ctx,
			runtime.TaskExecStartedEventTopic,
			&eventstypes.TaskExecStarted{
				ContainerID: he.tid,
				ExecID:      he.id,
				Pid:         uint32(he.pid),
			}); err != nil {
			return err
		}
	} else {
		if err := he.events.publishEvent(
			ctx,
			runtime.TaskStartEventTopic,
			&eventstypes.TaskStart{
				ContainerID: he.tid,
				Pid:         uint32(he.pid),
			}); err != nil {
			return err
		}
	}

	// wait in the background for the exit.
	go he.waitForExit()
	return nil
}

func (he *hcsExec) Start(ctx context.Context) (err error) {
	// If he.id == he.tid then this is the init exec.
	// We need to initialize the container itself before starting this exec.
	return he.startInternal(ctx, he.id == he.tid)
}

func (he *hcsExec) Kill(ctx context.Context, signal uint32) error {
	he.sl.Lock()
	defer he.sl.Unlock()
	switch he.state {
	case shimExecStateCreated:
		he.exitFromCreatedL(ctx, 1)
		return nil
	case shimExecStateRunning:
		supported := false
		if osversion.Build() >= osversion.RS5 {
			supported = he.host == nil || he.host.SignalProcessSupported()
		}
		var options interface{}
		var err error
		if he.isWCOW {
			var opt *guestresource.SignalProcessOptionsWCOW
			opt, err = signals.ValidateWCOW(int(signal), supported)
			if opt != nil {
				options = opt
			}
		} else {
			var opt *guestresource.SignalProcessOptionsLCOW
			opt, err = signals.ValidateLCOW(int(signal), supported)
			if opt != nil {
				options = opt
			}
		}
		if err != nil {
			return errors.Wrapf(errdefs.ErrFailedPrecondition, "signal %d: %v", signal, err)
		}
		var delivered bool
		if supported && options != nil {
			if he.isWCOW {
				// Servercore images block on signaling and wait until the target process
				// is terminated to return to the caller. This causes issues when graceful
				// termination of containers is requested (Bug36689012).
				// To fix this, we deliver the signal to the target process in a separate background
				// thread so that the caller can wait for the desired timeout before sending
				// a SIGKILL to the process.
				// TODO: We can get rid of these changes once the fix to support graceful termination is
				// made in windows.
				go func() {
					signalDelivered, deliveryErr := he.p.Process.Signal(ctx, options)

					if deliveryErr != nil {
						if !hcs.IsAlreadyStopped(deliveryErr) {
							// Process is not already stopped and there was a signal delivery error to this process
							log.G(ctx).WithField("err", deliveryErr).Errorf("Error in delivering signal %d, to pid: %d", signal, he.pid)
						}
					}
					if !signalDelivered {
						log.G(ctx).Errorf("Error: NotFound; exec: '%s' in task: '%s' not found", he.id, he.tid)
					}
				}()
				delivered, err = true, nil
			} else {
				delivered, err = he.p.Process.Signal(ctx, options)
			}
		} else {
			// legacy path before signals support OR if WCOW with signals
			// support needs to issue a terminate.
			delivered, err = he.p.Process.Kill(ctx)
		}
		if err != nil {
			if hcs.IsAlreadyStopped(err) {
				// Desired state is actual state. No use in erroring out just because we couldn't kill
				// an already dead process.
				return nil
			}
			return err
		}
		if !delivered {
			return errors.Wrapf(errdefs.ErrNotFound, "exec: '%s' in task: '%s' not found", he.id, he.tid)
		}
		return nil
	case shimExecStateExited:
		return errors.Wrapf(errdefs.ErrNotFound, "exec: '%s' in task: '%s' not found", he.id, he.tid)
	default:
		return newExecInvalidStateError(he.tid, he.id, he.state, "kill")
	}
}

func (he *hcsExec) ResizePty(ctx context.Context, width, height uint32) error {
	he.sl.Lock()
	defer he.sl.Unlock()
	if !he.io.Terminal() {
		return errors.Wrapf(errdefs.ErrFailedPrecondition, "exec: '%s' in task: '%s' is not a tty", he.id, he.tid)
	}

	if he.state == shimExecStateRunning {
		return he.p.Process.ResizeConsole(ctx, uint16(width), uint16(height))
	}
	return nil
}

func (he *hcsExec) CloseIO(ctx context.Context, stdin bool) error {
	// If we have any upstream IO we close the upstream connection. This will
	// unblock the `io.Copy` in the `Start()` call which will signal
	// `he.p.CloseStdin()`. If `he.io.Stdin()` is already closed this is safe to
	// call multiple times.
	he.io.CloseStdin(ctx)
	return nil
}

func (he *hcsExec) Wait() *task.StateResponse {
	<-he.exited
	return he.Status()
}

func (he *hcsExec) ForceExit(ctx context.Context, status int) {
	he.sl.Lock()
	defer he.sl.Unlock()
	if he.state != shimExecStateExited {
		switch he.state {
		case shimExecStateCreated:
			he.exitFromCreatedL(ctx, status)
		case shimExecStateRunning:
			// Kill the process to unblock `he.waitForExit`
			_, _ = he.p.Process.Kill(ctx)
		}
	}
}

// exitFromCreatedL transitions the shim to the exited state from the created
// state. It is the callers responsibility to hold `he.sl` for the durration of
// this transition.
//
// This call is idempotent and will not affect any state if the shim is already
// in the `shimExecStateExited` state.
//
// To transition for a created state the following must be done:
//
// 1. Issue `he.processDoneCancel` to unblock the goroutine
// `he.waitForContainerExit()`.
//
// 2. Set `he.state`, `he.exitStatus` and `he.exitedAt` to the exited values.
//
// 3. Release any upstream IO resources that were never used in a copy.
//
// 4. Close `he.exited` channel to unblock any waiters who might have called
// `Create`/`Wait`/`Start` which is a valid pattern.
//
// We DO NOT send the async `TaskExit` event because we never would have sent
// the `TaskStart`/`TaskExecStarted` event.
func (he *hcsExec) exitFromCreatedL(ctx context.Context, status int) {
	if he.state != shimExecStateExited {
		// Avoid logging the force if we already exited gracefully
		log.G(ctx).WithField("status", status).Debug("hcsExec::exitFromCreatedL")

		// Unblock the container exit goroutine
		he.processDoneOnce.Do(func() { close(he.processDone) })
		// Transition this exec
		he.state = shimExecStateExited
		he.exitStatus = uint32(status)
		he.exitedAt = time.Now()
		// Release all upstream IO connections (if any)
		he.io.Close(ctx)
		// Free any waiters
		he.exitedOnce.Do(func() {
			close(he.exited)
		})
	}
}

// waitForExit waits for the `he.p` to exit. This MUST only be called after a
// successful call to `Create` and MUST not be called more than once.
//
// This MUST be called via a goroutine.
//
// In the case of an exit from a running process the following must be done:
//
// 1. Wait for `he.p` to exit.
//
// 2. Issue `he.processDoneCancel` to unblock the goroutine
// `he.waitForContainerExit()` (if still running). We do this early to avoid the
// container exit also attempting to kill the process. However this race
// condition is safe and handled.
//
// 3. Capture the process exit code and set `he.state`, `he.exitStatus` and
// `he.exitedAt` to the exited values.
//
// 4. Wait for all IO to complete and release any upstream IO connections.
//
// 5. Send the async `TaskExit` to upstream listeners of any events.
//
// 6. Close `he.exited` channel to unblock any waiters who might have called
// `Create`/`Wait`/`Start` which is a valid pattern.
func (he *hcsExec) waitForExit() {
	var err error // this will only save the last error, since we dont return early on error
	ctx, span := otelutil.StartSpan(context.Background(), "hcsExec::waitForExit", trace.WithAttributes(
		attribute.String("tid", he.tid),
		attribute.String("eid", he.id)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	err = he.p.Process.Wait()
	if err != nil {
		log.G(ctx).WithError(err).Error("failed process Wait")
	}

	// Issue the process cancellation to unblock the container wait as early as
	// possible.
	he.processDoneOnce.Do(func() { close(he.processDone) })

	code, err := he.p.Process.ExitCode()
	if err != nil {
		log.G(ctx).WithError(err).Error("failed to get ExitCode")
	} else {
		log.G(ctx).WithField("exitCode", code).Debug("exited")
	}

	he.sl.Lock()
	he.state = shimExecStateExited
	he.exitStatus = uint32(code)
	he.exitedAt = time.Now()
	he.sl.Unlock()

	// Wait for all IO copies to complete and free the resources.
	_ = he.p.Wait()
	he.io.Close(ctx)

	// Only send the `runtime.TaskExitEventTopic` notification if this is a true
	// exec. For the `init` exec this is handled in task teardown.
	if he.tid != he.id {
		// We had a valid process so send the exited notification.
		if err := he.events.publishEvent(
			ctx,
			runtime.TaskExitEventTopic,
			&eventstypes.TaskExit{
				ContainerID: he.tid,
				ID:          he.id,
				Pid:         uint32(he.pid),
				ExitStatus:  he.exitStatus,
				ExitedAt:    timestamppb.New(he.exitedAt),
			}); err != nil {
			log.G(ctx).WithError(err).Error("failed to publish TaskExitEvent")
		}
	}

	// Free any waiters.
	he.exitedOnce.Do(func() {
		close(he.exited)
	})
}

// waitForContainerExit waits for `he.c` to exit. Depending on the exec's state
// will forcibly transition this exec to the exited state and unblock any
// waiters.
//
// This MUST be called via a goroutine at exec create.
func (he *hcsExec) waitForContainerExit() {
	ctx, span := otelutil.StartSpan(context.Background(), "hcsExec::waitForContainerExit", trace.WithAttributes(
		attribute.String("tid", he.tid),
		attribute.String("eid", he.id)))
	defer span.End()

	// wait for container or process to exit and ckean up resrources
	select {
	case <-he.c.WaitChannel():
		// Container exited first. We need to force the process into the exited
		// state and cleanup any resources
		he.sl.Lock()
		switch he.state {
		case shimExecStateCreated:
			he.exitFromCreatedL(ctx, 1)
		case shimExecStateRunning:
			// Kill the process to unblock `he.waitForExit`.
			_, _ = he.p.Process.Kill(ctx)
		}
		he.sl.Unlock()
	case <-he.processDone:
		// Process exited first. This is the normal case do nothing because
		// `he.waitForExit` will release any waiters.
	}
}
