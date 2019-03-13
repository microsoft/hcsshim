package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/Microsoft/go-winio/vhd"
	"github.com/Microsoft/hcsshim/internal/guestrequest"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/lcow"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/signals"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"
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

const (
	// processStopTimeout is the amount of time after signaling the process with
	// a signal expected to kill the process that the exec must wait before
	// forcibly terminating the process.
	//
	// For example, sending a SIGKILL is expected to kill a process. If the
	// process does not stop within `processStopTimeout` we will forcibly
	// terminate the process without a signal.
	processStopTimeout = time.Second * 5
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
	c *hcs.System,
	id, bundle string,
	isWCOW bool,
	spec *specs.Spec,
	io upstreamIO) shimExec {
	logrus.WithFields(logrus.Fields{
		"tid": tid,
		"eid": id,
	}).Debug("newHcsExec")

	processCtx, processDoneCancel := context.WithCancel(context.Background())
	he := &hcsExec{
		events:            events,
		tid:               tid,
		host:              host,
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
	// host is the hosting VM for `c`. If `host==nil` this exec MUST be a
	// process isolated WCOW exec.
	//
	// This MUST be treated as read only in the lifetime of the exec.
	host *uvm.UtilityVM
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
	// spec is the OCI spec that was passed in at create time. This is
	// stored because we don't actually create the process until the call to
	// `Start`. Generally, we only need to process part, but due to the
	// ignore flushes optimisation, easiest way is to pass in the whole spec.
	//
	// This MUST be treated as read only in the lifetime of the exec.
	spec *specs.Spec
	// io is the upstream io connections used for copying between the upstream
	// io and the downstream io. The upstream IO MUST already be connected at
	// create time in order to be valid.
	//
	// This MUST be treated as read only in the lifetime of the exec.
	io                upstreamIO
	ioWg              sync.WaitGroup
	processCtx        context.Context
	processDoneCancel context.CancelFunc

	// sl is the state lock that MUST be held to safely read/write any of the
	// following members.
	sl             sync.Mutex
	state          shimExecState
	pid            int
	exitStatus     uint32
	exitedAt       time.Time
	p              *hcs.Process
	stdout, stderr io.Closer

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

type WriteCacheMode uint16

const (
	// Write Cache Mode for a VHD.
	WriteCacheModeCacheMetadata         WriteCacheMode = 0
	WriteCacheModeWriteInternalMetadata WriteCacheMode = 1
	WriteCacheModeWriteMetadata         WriteCacheMode = 2
	WriteCacheModeCommitAll             WriteCacheMode = 3
	WriteCacheModeDisableFlushing       WriteCacheMode = 4
)

// setVhdWriteCacheMode sets the WriteCacheMode for a VHD. The handle
// to the VHD should be opened with Access: None, Flags: ParentCachedIO |
// IgnoreRelativeParentLocator. Use DisableFlushing for optimisation during
// first boot, and CacheMetadata following container start
func setVhdWriteCacheMode(handle syscall.Handle, wcm WriteCacheMode) error {
	type storageSetSurfaceCachePolicyRequest struct {
		RequestLevel uint32
		CacheMode    uint16
		pad          uint16 // For 4-byte alignment
	}
	const ioctlSetSurfaceCachePolicy uint32 = 0x2d1a10
	request := storageSetSurfaceCachePolicyRequest{
		RequestLevel: 1,
		CacheMode:    uint16(wcm),
		pad:          0,
	}
	var bytesReturned uint32
	return syscall.DeviceIoControl(
		handle,
		ioctlSetSurfaceCachePolicy,
		(*byte)(unsafe.Pointer(&request)),
		uint32(unsafe.Sizeof(request)),
		nil,
		0,
		&bytesReturned,
		nil)
}

// startFlushDisableRequired determines if the optimisations in
// preStartFlushDisable and postStartFlushEnable are required.
func (he *hcsExec) startFlushDisableRequired() bool {
	// No-op pre-RS5 or post-18855. Pre-RS5 doesn't use v2. Post 18855 has
	// these optimisations in the platform for v2 callers. Only for WCOW.
	osv := osversion.Get()
	if osv.Build < 17763 || osv.Build >= 18855 ||
		!he.isWCOW ||
		!he.spec.Windows.IgnoreFlushesDuringBoot ||
		he.spec.Windows.HyperV != nil { // TODO @jhowardmsft Remove this when xenon WCOW bit implemented
		return false
	}
	return true
}

// preStartFlushDisable conditionally disables flushing if required
// prior to the computesystem Start() call. This is necessary only on
// certain Windows builds where HCS does not implement this functionality
// itself.
func (he *hcsExec) preStartFlushDisable() (syscall.Handle, error) {
	if !he.startFlushDisableRequired() {
		return 0, nil
	}

	if he.spec.Windows.HyperV == nil {
		// Operating on the scratch disk
		path := filepath.Join(he.spec.Windows.LayerFolders[len(he.spec.Windows.LayerFolders)-1], "sandbox.vhdx")

		logrus.WithFields(logrus.Fields{
			"tid":  he.tid,
			"eid":  he.id,
			"path": path,
		}).Debug("hcsExec::Start Disabling VHD flushing")

		handle, err := vhd.OpenVirtualDisk(path, vhd.VirtualDiskAccessNone, vhd.OpenVirtualDiskFlagParentCachedIO|vhd.OpenVirtualDiskFlagIgnoreRelativeParentLocator)
		if err != nil {
			syscall.CloseHandle(handle)
			return 0, errors.Wrap(err, fmt.Sprintf("failed to open %s", path))
		}
		if err := setVhdWriteCacheMode(handle, WriteCacheModeDisableFlushing); err != nil {
			syscall.CloseHandle(handle)
			return 0, errors.Wrap(err, fmt.Sprintf("failed to disable flushing on %s", path))
		}
		return handle, nil

	}

	// TODO @jhowardmsft - Extend for xenon WCOW
	return 0, nil
}

// postStartFlushEnable conditionally enables flushing if required
// after the computesystem Start() call. It effectively reverses anything
// that might have been done in `preStartFlushDisable()`.
func (he *hcsExec) postStartFlushEnable(handle syscall.Handle) {
	if he.spec.Windows.HyperV == nil {
		if handle == 0 {
			return
		}

		logrus.WithFields(logrus.Fields{
			"tid": he.tid,
			"eid": he.id,
		}).Debug("hcsExec::Start Re-enabling VHD flushing")

		setVhdWriteCacheMode(handle, WriteCacheModeCacheMetadata)
		syscall.CloseHandle(handle)
	}

	// TODO @jhowardmsft - Extend for xenon WCOW

}

func (he *hcsExec) Start(ctx context.Context) (err error) {
	logrus.WithFields(logrus.Fields{
		"tid": he.tid,
		"eid": he.id,
	}).Debug("hcsExec::Start")

	he.sl.Lock()
	defer he.sl.Unlock()
	if he.state != shimExecStateCreated {
		return newExecInvalidStateError(he.tid, he.id, he.state, "start")
	}
	defer func() {
		if err != nil {
			he.exitFromCreatedL(1)
		}
	}()
	if he.id == he.tid {

		var handle syscall.Handle
		handle, err = he.preStartFlushDisable()
		if err != nil {
			return err
		}

		// This is the init exec. We need to start the container itself
		err = he.c.Start()
		he.postStartFlushEnable(handle) // Regardless of successful start or not
		if err != nil {
			return err
		}
		defer func() {
			if err != nil {
				he.c.Terminate()
			}
		}()
	}
	var (
		proc *hcs.Process
	)
	if he.isWCOW {
		wpp := &hcsschema.ProcessParameters{
			CommandLine:      he.spec.Process.CommandLine,
			User:             he.spec.Process.User.Username,
			WorkingDirectory: he.spec.Process.Cwd,
			EmulateConsole:   he.spec.Process.Terminal,
			CreateStdInPipe:  he.io.StdinPath() != "",
			CreateStdOutPipe: he.io.StdoutPath() != "",
			CreateStdErrPipe: he.io.StderrPath() != "",
		}

		if he.spec.Process.CommandLine == "" {
			wpp.CommandLine = escapeArgs(he.spec.Process.Args)
		}

		environment := make(map[string]string)
		for _, v := range he.spec.Process.Env {
			s := strings.SplitN(v, "=", 2)
			if len(s) == 2 && len(s[1]) > 0 {
				environment[s[0]] = s[1]
			}
		}
		wpp.Environment = environment

		if he.spec.Process.ConsoleSize != nil {
			wpp.ConsoleSize = []int32{
				int32(he.spec.Process.ConsoleSize.Height),
				int32(he.spec.Process.ConsoleSize.Width),
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
	defer func() {
		if err != nil {
			he.p.Kill()
			he.p.Close()
		}
	}()

	in, out, serr, err := he.p.Stdio()
	if err != nil {
		return err
	}

	if he.io.StdinPath() != "" {
		go func() {
			io.Copy(in, he.io.Stdin())
			logrus.WithFields(logrus.Fields{
				"tid": he.tid,
				"eid": he.id,
			}).Debug("hcsExec::Start::Stdin - Copy completed")
			in.Close()
			he.p.CloseStdin()
			he.io.CloseStdin()
		}()
	}

	if he.io.StdoutPath() != "" {
		he.stdout = out
		he.ioWg.Add(1)
		go func() {
			io.Copy(he.io.Stdout(), out)
			logrus.WithFields(logrus.Fields{
				"tid": he.tid,
				"eid": he.id,
			}).Debug("hcsExec::Start::Stdout - Copy completed")
			he.ioWg.Done()

			// Close the stdout io handle if not closed.
			he.sl.Lock()
			if he.stdout != nil {
				he.stdout.Close()
				he.stdout = nil
			}
			he.sl.Unlock()
		}()
	}

	if he.io.StderrPath() != "" {
		he.stderr = serr
		he.ioWg.Add(1)
		go func() {
			io.Copy(he.io.Stderr(), serr)
			logrus.WithFields(logrus.Fields{
				"tid": he.tid,
				"eid": he.id,
			}).Debug("hcsExec::Start::Stderr - Copy completed")
			he.ioWg.Done()

			// Close the stderr io handle if not closed.
			he.sl.Lock()
			if he.stderr != nil {
				he.stderr.Close()
				he.stderr = nil
			}
			he.sl.Unlock()
		}()
	}

	// Assign the PID and transition the state.
	he.pid = he.p.Pid()
	he.state = shimExecStateRunning

	// Publish the task/exec start event. This MUST happen before waitForExit to
	// avoid publishing the exit previous to the start.
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
		he.exitFromCreatedL(1)
		return nil
	case shimExecStateRunning:
		supported := false
		if osversion.Get().Build >= osversion.RS5 {
			supported = he.host == nil || he.host.SignalProcessSupported()
		}
		sig, err := signals.Validate(int(signal), supported, !he.isWCOW)
		if err != nil {
			return err
		}
		if supported {
			if signals.ShouldKill(signal) {
				go func() {
					select {
					case <-time.After(processStopTimeout):
						logrus.WithFields(logrus.Fields{
							"tid":    he.tid,
							"eid":    he.id,
							"signal": signal,
						}).Warning("hcsExec::Kill - timed out waiting for expected process stop")
						if err := he.p.Kill(); err != nil && !hcs.IsAlreadyClosed(err) && !hcs.IsAlreadyStopped(err) {
							logrus.WithFields(logrus.Fields{
								"tid":           he.tid,
								"eid":           he.id,
								"signal":        signal,
								logrus.ErrorKey: err,
							}).Error("hcsExec::Kill - failed to forcibly terminate process after timeout period")
						}
					case <-he.processCtx.Done():
						// Process exited. This is the normal case.
					}
				}()
			}
			return he.p.Signal(guestrequest.SignalProcessOptions{
				Signal: sig,
			})
		}
		// legacy path before signals support.
		return he.p.Kill()
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

	// If we have any upstream IO we close the upstream connection. This will
	// unblock the `io.Copy` in the `Start()` call which will signal
	// `he.p.CloseStdin()`. If `he.io.Stdin()` is already closed this is safe to
	// call multiple times.
	he.io.CloseStdin()
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

func (he *hcsExec) ForceExit(status int) {
	he.sl.Lock()
	defer he.sl.Unlock()
	if he.state != shimExecStateExited {
		// Avoid logging the force if we already exited gracefully
		logrus.WithFields(logrus.Fields{
			"tid":    he.tid,
			"eid":    he.id,
			"status": status,
		}).Debug("hcsExec::ForceExit")
		switch he.state {
		case shimExecStateCreated:
			he.exitFromCreatedL(status)
		case shimExecStateRunning:
			// Kill the process to unblock `he.waitForExit`
			he.p.Kill()
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
// `he.waitForContainerExit()``.
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
func (he *hcsExec) exitFromCreatedL(status int) {
	if he.state != shimExecStateExited {
		// Unblock the container exit goroutine
		he.processDoneCancel()
		// Transition this exec
		he.state = shimExecStateExited
		he.exitStatus = uint32(status)
		he.exitedAt = time.Now()
		// Release all upstream IO connections (if any)
		he.io.Close()
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
	err := he.p.Wait()
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"tid":           he.tid,
			"eid":           he.id,
			logrus.ErrorKey: err,
		}).Error("hcsExec::waitForExit - Failed process Wait")
	}

	// Issue the process cancellation to unblock the container wait as early as
	// possible.
	he.processDoneCancel()

	code, err := he.p.ExitCode()
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"tid":           he.tid,
			"eid":           he.id,
			logrus.ErrorKey: err,
		}).Error("hcsExec::waitForExit - Failed to get ExitCode")
	} else {
		logrus.WithFields(logrus.Fields{
			"tid":      he.tid,
			"eid":      he.id,
			"exitCode": code,
		}).Debug("hcsExec::waitForExit - Exited")
	}

	// Close the process handle (we will never reference it again)
	he.p.Close()

	he.sl.Lock()
	he.state = shimExecStateExited
	he.exitStatus = uint32(code)
	he.exitedAt = time.Now()
	he.sl.Unlock()

	go func() {
		// processCopyTimeout is the amount of time after process exit we allow the
		// stdout, stderr relay's to continue before forcibly closing them if not
		// already completed. This is primarily a safety step against the HCS when
		// it fails to send a close on the stdout, stderr pipes when the process
		// exits and blocks the relay wait groups forever.
		const processCopyTimeout = time.Second * 1

		time.Sleep(processCopyTimeout)
		he.sl.Lock()
		defer he.sl.Unlock()
		if he.stdout != nil || he.stderr != nil {
			logrus.WithFields(logrus.Fields{
				"tid": he.tid,
				"eid": he.id,
			}).Warn("hcsExec::waitForExit - timed out waiting for ioRelay to complete")

			if he.stdout != nil {
				he.stdout.Close()
				he.stdout = nil
			}
			if he.stderr != nil {
				he.stderr.Close()
				he.stderr = nil
			}
		}
	}()

	// Wait for all IO copies to complete and free the resources.
	he.ioWg.Wait()
	he.io.Close()

	// We had a valid process so send the exited notification.
	he.events(
		runtime.TaskExitEventTopic,
		&eventstypes.TaskExit{
			ContainerID: he.tid,
			ID:          he.id,
			Pid:         uint32(he.pid),
			ExitStatus:  he.exitStatus,
			ExitedAt:    he.exitedAt,
		})
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
	cexit := make(chan error)
	go func() {
		cexit <- he.c.Wait()
	}()
	select {
	case <-cexit:
		// Container exited first. We need to force the process into the exited
		// state and cleanup any resources
		he.sl.Lock()
		switch he.state {
		case shimExecStateCreated:
			he.exitFromCreatedL(1)
		case shimExecStateRunning:
			// Kill the process to unblock `he.waitForExit`.
			he.p.Kill()
		}
		he.sl.Unlock()
	case <-he.processCtx.Done():
		// Process exited first. This is the normal case do nothing because
		// `he.waitForExit` will release any waiters.
	}
}

// escapeArgs makes a Windows-style escaped command line from a set of arguments
func escapeArgs(args []string) string {
	escapedArgs := make([]string, len(args))
	for i, a := range args {
		escapedArgs[i] = windows.EscapeArg(a)
	}
	return strings.Join(escapedArgs, " ")
}
