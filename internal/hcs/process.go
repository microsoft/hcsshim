//go:build windows

package hcs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sync"
	"syscall"
	"time"

	"go.opencensus.io/trace"

	"github.com/Microsoft/hcsshim/internal/computecore"
	"github.com/Microsoft/hcsshim/internal/cow"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
)

type Process struct {
	handleLock          sync.RWMutex
	handle              computecore.HcsProcess
	processID           int
	system              *System
	hasCachedStdio      bool
	stdioLock           sync.Mutex
	stdin               io.WriteCloser
	stdout              io.ReadCloser
	stderr              io.ReadCloser
	killSignalDelivered bool

	// notificationID is the lookup key passed as the void* context to
	// HcsSetProcessCallback. Zero when no callback is registered.
	notificationID uint64
	// notify rendezvous between the HCS notification callback
	// (HcsEventProcessExited) and waitBackground. Close also signals it to
	// release waitBackground without publishing an exit.
	notify *notificationState

	closedWaitOnce sync.Once
	waitBlock      chan struct{}
	exitCode       int
	waitError      error
}

var _ cow.Process = &Process{}

func newProcess(process computecore.HcsProcess, processID int, computeSystem *System) *Process {
	return &Process{
		handle:    process,
		processID: processID,
		system:    computeSystem,
		waitBlock: make(chan struct{}),
		notify:    newNotificationState(),
	}
}

// Pid returns the process ID of the process within the container.
func (process *Process) Pid() int {
	return process.processID
}

// SystemID returns the ID of the process's compute system.
func (process *Process) SystemID() string {
	return process.system.ID()
}

func (process *Process) processSignalResult(ctx context.Context, err error) (bool, error) {
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrVmcomputeOperationInvalidState) || errors.Is(err, ErrComputeSystemDoesNotExist) || errors.Is(err, ErrElementNotFound) {
		if !process.stopped() {
			// The process should be gone, but we have not received the notification.
			// After a second, force unblock the process wait to work around a possible
			// deadlock in the HCS.
			go func() {
				time.Sleep(time.Second)
				process.closedWaitOnce.Do(func() {
					log.G(ctx).WithError(err).Warn("force unblocking process waits")
					process.exitCode = -1
					process.waitError = err
					close(process.waitBlock)
				})
			}()
		}
		return false, nil
	}
	return false, nil
}

// Signal signals the process with `options`.
//
// For LCOW `guestresource.SignalProcessOptionsLCOW`.
//
// For WCOW `guestresource.SignalProcessOptionsWCOW`.
func (process *Process) Signal(ctx context.Context, options interface{}) (bool, error) {
	process.handleLock.RLock()
	defer process.handleLock.RUnlock()

	operation := "hcs::Process::Signal"

	if process.handle == 0 {
		return false, makeProcessError(process, operation, ErrAlreadyClosed)
	}

	optionsb, err := json.Marshal(options)
	if err != nil {
		return false, err
	}
	optionsStr := string(optionsb)

	_, sigErr := runOperation(ctx, func(op computecore.HcsOperation) error {
		return computecore.HcsSignalProcess(ctx, process.handle, op, optionsStr)
	})
	delivered, sigErr := process.processSignalResult(ctx, sigErr)
	if sigErr != nil {
		sigErr = makeProcessError(process, operation, sigErr)
	}
	return delivered, sigErr
}

// Kill signals the process to terminate but does not wait for it to finish terminating.
func (process *Process) Kill(ctx context.Context) (bool, error) {
	process.handleLock.RLock()
	defer process.handleLock.RUnlock()

	operation := "hcs::Process::Kill"

	if process.handle == 0 {
		return false, makeProcessError(process, operation, ErrAlreadyClosed)
	}

	if process.stopped() {
		return false, makeProcessError(process, operation, ErrProcessAlreadyStopped)
	}

	if process.killSignalDelivered {
		// A kill signal has already been sent to this process. Sending a second
		// one offers no real benefit, as processes cannot stop themselves from
		// being terminated, once a TerminateProcess has been issued. Sending a
		// second kill may result in a number of errors (two of which detailed bellow)
		// and which we can avoid handling.
		return true, nil
	}

	// HCS serializes the signals sent to a target pid per compute system handle.
	// To avoid SIGKILL being serialized behind other signals, we open a new compute
	// system handle to deliver the kill signal.
	// If the calls to opening a new compute system handle fail, we forcefully
	// terminate the container itself so that no container is left behind
	hcsSystem, err := OpenComputeSystem(ctx, process.system.id)
	if err != nil {
		// log error and force termination of container
		log.G(ctx).WithField("err", err).Error("OpenComputeSystem() call failed")
		err = process.system.Terminate(ctx)
		// if the Terminate() call itself ever failed, log and return error
		if err != nil {
			log.G(ctx).WithField("err", err).Error("Terminate() call failed")
			return false, err
		}
		process.system.Close()
		return true, nil
	}
	defer hcsSystem.Close()

	newProcessHandle, err := hcsSystem.OpenProcess(ctx, process.Pid())
	if err != nil {
		// Return true only if the target process has either already
		// exited, or does not exist.
		if IsAlreadyStopped(err) {
			return true, nil
		} else {
			return false, err
		}
	}
	defer newProcessHandle.Close()

	_, killErr := runOperation(ctx, func(op computecore.HcsOperation) error {
		return computecore.HcsTerminateProcess(ctx, newProcessHandle.handle, op, "")
	})
	if killErr != nil {
		// We still need to check these two cases, as processes may still be killed by an
		// external actor (human operator, OOM, random script etc).
		if errors.Is(killErr, os.ErrPermission) || IsAlreadyStopped(killErr) {
			// There are two cases where it should be safe to ignore an error returned
			// by HcsTerminateProcess. The first one is cause by the fact that
			// HcsTerminateProcess ends up calling TerminateProcess in the context
			// of a container. According to the TerminateProcess documentation:
			// https://docs.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-terminateprocess#remarks
			// After a process has terminated, call to TerminateProcess with open
			// handles to the process fails with ERROR_ACCESS_DENIED (5) error code.
			// It's safe to ignore this error here. HCS should always have permissions
			// to kill processes inside any container. So an ERROR_ACCESS_DENIED
			// is unlikely to be anything else than what the ending remarks in the
			// documentation states.
			//
			// The second case is generated by hcs itself, if for any reason HcsTerminateProcess
			// is called twice in a very short amount of time. In such cases, hcs may return
			// HCS_E_PROCESS_ALREADY_STOPPED.
			return true, nil
		}
	}
	delivered, killErr := newProcessHandle.processSignalResult(ctx, killErr)
	if killErr != nil {
		killErr = makeProcessError(newProcessHandle, operation, killErr)
	}

	process.killSignalDelivered = delivered
	return delivered, killErr
}

// waitBackground blocks until either the HCS callback delivers
// HcsEventProcessExited (real exit, publish exit code) or Close fires
// (release without publishing). It then sets `process.waitError` (if any)
// and unblocks all `Wait` calls.
//
// HCS does not deliver a final notification on HcsCloseProcess — it just
// unregisters the callback — so Close needs its own signal. Publishing a
// synthetic exit on Close would report exit_code=255 to containerd for
// processes that are still running.
//
// This MUST be called exactly once per `process.handle` but `Wait` is safe
// to call multiple times.
func (process *Process) waitBackground() {
	operation := "hcs::Process::waitBackground"
	ctx, span := oc.StartSpan(context.Background(), operation)
	defer span.End()
	span.AddAttributes(
		trace.StringAttribute("cid", process.SystemID()),
		trace.Int64Attribute("pid", int64(process.processID)))

	var raw json.RawMessage
	exitCode := -1
	var err error
	select {
	case <-process.waitBlock:
		log.G(ctx).Debug("process waitBackground returning without exit notification (handle closed)")
		return
	case raw = <-process.notify.exit:
	case abortErr := <-process.notify.abort:
		err = makeProcessError(process, operation, abortErr)
	}

	if err == nil && len(raw) > 0 {
		properties := &hcsschema.ProcessStatus{}
		if uErr := json.Unmarshal(raw, properties); uErr != nil {
			err = makeProcessError(process, operation, uErr)
		} else if properties.LastWaitResult != 0 {
			log.G(ctx).WithField("wait-result", properties.LastWaitResult).Warning("non-zero last wait result")
		} else {
			exitCode = int(properties.ExitCode)
		}
	}
	log.G(ctx).WithField("exitCode", exitCode).Debug("process exited")

	process.closedWaitOnce.Do(func() {
		process.exitCode = exitCode
		process.waitError = err
		close(process.waitBlock)
	})
	oc.SetSpanStatus(span, err)
}

// Wait waits for the process to exit. If the process has already exited returns
// the previous error (if any).
func (process *Process) Wait() error {
	<-process.waitBlock
	return process.waitError
}

// Exited returns if the process has stopped
func (process *Process) stopped() bool {
	select {
	case <-process.waitBlock:
		return true
	default:
		return false
	}
}

// ResizeConsole resizes the console of the process.
func (process *Process) ResizeConsole(ctx context.Context, width, height uint16) error {
	process.handleLock.RLock()
	defer process.handleLock.RUnlock()

	operation := "hcs::Process::ResizeConsole"

	if process.handle == 0 {
		return makeProcessError(process, operation, ErrAlreadyClosed)
	}
	modifyRequest := hcsschema.ProcessModifyRequest{
		Operation: guestrequest.ModifyProcessConsoleSize,
		ConsoleSize: &hcsschema.ConsoleSize{
			Height: height,
			Width:  width,
		},
	}

	modifyRequestb, err := json.Marshal(modifyRequest)
	if err != nil {
		return err
	}
	modifyRequestStr := string(modifyRequestb)

	_, modErr := runOperation(ctx, func(op computecore.HcsOperation) error {
		return computecore.HcsModifyProcess(ctx, process.handle, op, modifyRequestStr)
	})
	if modErr != nil {
		return makeProcessError(process, operation, modErr)
	}

	return nil
}

// ExitCode returns the exit code of the process. The process must have
// already terminated.
func (process *Process) ExitCode() (int, error) {
	if !process.stopped() {
		return -1, makeProcessError(process, "hcs::Process::ExitCode", ErrInvalidProcessState)
	}
	if process.waitError != nil {
		return -1, process.waitError
	}
	return process.exitCode, nil
}

// StdioLegacy returns the stdin, stdout, and stderr pipes, respectively. Closing
// these pipes does not close the underlying pipes. Once returned, these pipes
// are the responsibility of the caller to close.
func (process *Process) StdioLegacy() (_ io.WriteCloser, _ io.ReadCloser, _ io.ReadCloser, err error) {
	operation := "hcs::Process::StdioLegacy"
	ctx, span := oc.StartSpan(context.Background(), operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute("cid", process.SystemID()),
		trace.Int64Attribute("pid", int64(process.processID)))

	process.handleLock.RLock()
	defer process.handleLock.RUnlock()

	if process.handle == 0 {
		return nil, nil, nil, makeProcessError(process, operation, ErrAlreadyClosed)
	}

	process.stdioLock.Lock()
	defer process.stdioLock.Unlock()
	if process.hasCachedStdio {
		stdin, stdout, stderr := process.stdin, process.stdout, process.stderr
		process.stdin, process.stdout, process.stderr = nil, nil, nil
		process.hasCachedStdio = false
		return stdin, stdout, stderr, nil
	}

	processInfo, _, err := runProcessOperation(ctx, func(op computecore.HcsOperation) error {
		return computecore.HcsGetProcessInfo(ctx, process.handle, op)
	})
	if err != nil {
		return nil, nil, nil, makeProcessError(process, operation, err)
	}

	pipes, err := makeOpenFiles([]syscall.Handle{processInfo.StdInput, processInfo.StdOutput, processInfo.StdError})
	if err != nil {
		return nil, nil, nil, makeProcessError(process, operation, err)
	}

	return pipes[0], pipes[1], pipes[2], nil
}

// Stdio returns the stdin, stdout, and stderr pipes, respectively.
// To close them, close the process handle, or use the `CloseStd*` functions.
func (process *Process) Stdio() (stdin io.Writer, stdout, stderr io.Reader) {
	process.stdioLock.Lock()
	defer process.stdioLock.Unlock()
	return process.stdin, process.stdout, process.stderr
}

// CloseStdin closes the write side of the stdin pipe so that the process is
// notified on the read side that there is no more data in stdin.
func (process *Process) CloseStdin(ctx context.Context) (err error) {
	operation := "hcs::Process::CloseStdin"
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute("cid", process.SystemID()),
		trace.Int64Attribute("pid", int64(process.processID)))

	process.handleLock.RLock()
	defer process.handleLock.RUnlock()

	if process.handle == 0 {
		return makeProcessError(process, operation, ErrAlreadyClosed)
	}

	// HcsModifyProcess request to close stdin will fail if the process has already exited
	if !process.stopped() {
		modifyRequest := hcsschema.ProcessModifyRequest{
			Operation: guestrequest.CloseProcessHandle,
			CloseHandle: &hcsschema.CloseHandle{
				Handle: guestrequest.STDInHandle,
			},
		}

		modifyRequestb, err := json.Marshal(modifyRequest)
		if err != nil {
			return err
		}
		modifyRequestStr := string(modifyRequestb)

		_, modErr := runOperation(ctx, func(op computecore.HcsOperation) error {
			return computecore.HcsModifyProcess(ctx, process.handle, op, modifyRequestStr)
		})
		if modErr != nil {
			return makeProcessError(process, operation, modErr)
		}
	}

	process.stdioLock.Lock()
	defer process.stdioLock.Unlock()
	if process.stdin != nil {
		process.stdin.Close()
		process.stdin = nil
	}

	return nil
}

func (process *Process) CloseStdout(ctx context.Context) (err error) {
	ctx, span := oc.StartSpan(ctx, "hcs::Process::CloseStdout") //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute("cid", process.SystemID()),
		trace.Int64Attribute("pid", int64(process.processID)))

	process.handleLock.Lock()
	defer process.handleLock.Unlock()

	if process.handle == 0 {
		return nil
	}

	process.stdioLock.Lock()
	defer process.stdioLock.Unlock()
	if process.stdout != nil {
		process.stdout.Close()
		process.stdout = nil
	}
	return nil
}

func (process *Process) CloseStderr(ctx context.Context) (err error) {
	ctx, span := oc.StartSpan(ctx, "hcs::Process::CloseStderr") //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute("cid", process.SystemID()),
		trace.Int64Attribute("pid", int64(process.processID)))

	process.handleLock.Lock()
	defer process.handleLock.Unlock()

	if process.handle == 0 {
		return nil
	}

	process.stdioLock.Lock()
	defer process.stdioLock.Unlock()
	if process.stderr != nil {
		process.stderr.Close()
		process.stderr = nil
	}
	return nil
}

// Close cleans up any state associated with the process but does not kill
// or wait on it.
func (process *Process) Close() (err error) {
	operation := "hcs::Process::Close"
	ctx, span := oc.StartSpan(context.Background(), operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute("cid", process.SystemID()),
		trace.Int64Attribute("pid", int64(process.processID)))

	process.handleLock.Lock()
	defer process.handleLock.Unlock()

	// Don't double free this
	if process.handle == 0 {
		return nil
	}

	process.stdioLock.Lock()
	if process.stdin != nil {
		process.stdin.Close()
		process.stdin = nil
	}
	if process.stdout != nil {
		process.stdout.Close()
		process.stdout = nil
	}
	if process.stderr != nil {
		process.stderr.Close()
		process.stderr = nil
	}
	process.stdioLock.Unlock()

	// HcsCloseProcess internally unregisters our notification callback
	// and drains in-flight invocations before tearing the handle down.
	computecore.HcsCloseProcess(ctx, process.handle)
	unregisterNotificationContext(process.notificationID)
	process.notificationID = 0

	// Release Wait/ExitCode callers with ErrAlreadyClosed
	// and unblock waitBackground.
	process.handle = 0
	process.closedWaitOnce.Do(func() {
		process.exitCode = -1
		process.waitError = ErrAlreadyClosed
		close(process.waitBlock)
	})

	return nil
}

// registerNotification registers the package-wide HCS notification callback
// on this process handle. Must be called BEFORE waitBackground starts so
// notifications are not missed.
func (process *Process) registerNotification(ctx context.Context) error {
	id := registerNotificationContext(process.SystemID(), process.processID, process.notify, nil)
	if err := computecore.HcsSetProcessCallback(
		ctx, process.handle,
		computecore.HcsEventOptionNone,
		uintptr(id), notificationCallback,
	); err != nil {
		unregisterNotificationContext(id)
		return err
	}
	process.notificationID = id
	return nil
}
