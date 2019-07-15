package hcs

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"syscall"
	"time"

	"github.com/Microsoft/hcsshim/internal/interop"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
)

// ContainerError is an error encountered in HCS
type Process struct {
	handleLock     sync.RWMutex
	handle         hcsProcess
	processID      int
	system         *System
	stdin          io.WriteCloser
	stdout         io.ReadCloser
	stderr         io.ReadCloser
	callbackNumber uintptr

	closedWaitOnce sync.Once
	waitBlock      chan struct{}
	waitError      error
}

func newProcess(process hcsProcess, processID int, computeSystem *System) *Process {
	return &Process{
		handle:    process,
		processID: processID,
		system:    computeSystem,
		waitBlock: make(chan struct{}),
	}
}

type processModifyRequest struct {
	Operation   string
	ConsoleSize *consoleSize `json:",omitempty"`
	CloseHandle *closeHandle `json:",omitempty"`
}

type consoleSize struct {
	Height uint16
	Width  uint16
}

type closeHandle struct {
	Handle string
}

type processStatus struct {
	ProcessID      uint32
	Exited         bool
	ExitCode       uint32
	LastWaitResult int32
}

const (
	stdIn  string = "StdIn"
	stdOut string = "StdOut"
	stdErr string = "StdErr"
)

const (
	modifyConsoleSize string = "ConsoleSize"
	modifyCloseHandle string = "CloseHandle"
)

// Pid returns the process ID of the process within the container.
func (process *Process) Pid() int {
	return process.processID
}

// SystemID returns the ID of the process's compute system.
func (process *Process) SystemID() string {
	return process.system.ID()
}

func (process *Process) processSignalResult(err error) (bool, error) {
	switch err {
	case nil:
		return true, nil
	case ErrVmcomputeOperationInvalidState, ErrComputeSystemDoesNotExist, ErrElementNotFound:
		select {
		case <-process.waitBlock:
			// The process exit notification has already arrived.
		default:
			// The process should be gone, but we have not received the notification.
			// After a second, force unblock the process wait to work around a possible
			// deadlock in the HCS.
			go func() {
				time.Sleep(time.Second)
				process.closedWaitOnce.Do(func() {
					logrus.WithFields(logrus.Fields{
						logfields.ContainerID: process.SystemID(),
						logfields.ProcessID:   process.processID,
						logrus.ErrorKey:       err,
					}).Warn("hcsshim::Process::processSignalResult - Force unblocking process waits")
					process.waitError = err
					close(process.waitBlock)
				})
			}()
		}
		return false, nil
	default:
		return false, err
	}
}

// Signal signals the process with `options`.
//
// For LCOW `guestrequest.SignalProcessOptionsLCOW`.
//
// For WCOW `guestrequest.SignalProcessOptionsWCOW`.
func (process *Process) Signal(ctx context.Context, options interface{}) (_ bool, err error) {
	operation := "hcsshim::Process::Signal"
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute(logfields.SystemID, process.SystemID()),
		trace.Int64Attribute(logfields.ProcessID, int64(process.processID)))

	process.handleLock.RLock()
	defer process.handleLock.RUnlock()

	if process.handle == 0 {
		return false, makeProcessError(process, operation, ErrAlreadyClosed, nil)
	}

	optionsb, err := json.Marshal(options)
	if err != nil {
		return false, err
	}

	optionsStr := string(optionsb)

	var resultp *uint16
	syscallWatcher(ctx, func() {
		err = hcsSignalProcess(process.handle, optionsStr, &resultp)
	})
	events := processHcsResult(resultp)
	delivered, err := process.processSignalResult(err)
	if err != nil {
		err = makeProcessError(process, operation, err, events)
	}
	return delivered, err
}

// Kill signals the process to terminate but does not wait for it to finish terminating.
func (process *Process) Kill(ctx context.Context) (_ bool, err error) {
	operation := "hcsshim::Process::Kill"
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute(logfields.SystemID, process.SystemID()),
		trace.Int64Attribute(logfields.ProcessID, int64(process.processID)))

	process.handleLock.RLock()
	defer process.handleLock.RUnlock()

	if process.handle == 0 {
		return false, makeProcessError(process, operation, ErrAlreadyClosed, nil)
	}

	var resultp *uint16
	syscallWatcher(ctx, func() {
		err = hcsTerminateProcess(process.handle, &resultp)
	})
	events := processHcsResult(resultp)
	delivered, err := process.processSignalResult(err)
	if err != nil {
		err = makeProcessError(process, operation, err, events)
	}
	return delivered, err
}

// waitBackground waits for the process exit notification. Once received sets
// `process.waitError` (if any) and unblocks all `Wait` calls.
//
// This MUST be called exactly once per `process.handle` but `Wait` is safe to
// call multiple times.
func (process *Process) waitBackground() {
	operation := "hcsshim::Process::waitBackground"
	_, span := trace.StartSpan(context.Background(), operation)
	defer span.End()
	span.AddAttributes(
		trace.StringAttribute(logfields.SystemID, process.SystemID()),
		trace.Int64Attribute(logfields.ProcessID, int64(process.processID)))

	err := waitForNotification(process.callbackNumber, hcsNotificationProcessExited, nil)
	if err != nil {
		err = makeProcessError(process, operation, err, nil)
	}
	oc.SetSpanStatus(span, err)
	process.closedWaitOnce.Do(func() {
		process.waitError = err
		close(process.waitBlock)
	})
}

// Wait waits for the process to exit. If the process has already exited returns
// the pervious error (if any).
func (process *Process) Wait() (err error) {
	<-process.waitBlock
	return process.waitError
}

// ResizeConsole resizes the console of the process.
func (process *Process) ResizeConsole(ctx context.Context, width, height uint16) (err error) {
	operation := "hcssshim::Process::ResizeConsole"
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute(logfields.SystemID, process.SystemID()),
		trace.Int64Attribute(logfields.ProcessID, int64(process.processID)),
		trace.Int64Attribute("width", int64(width)),
		trace.Int64Attribute("height", int64(height)))

	process.handleLock.RLock()
	defer process.handleLock.RUnlock()

	if process.handle == 0 {
		return makeProcessError(process, operation, ErrAlreadyClosed, nil)
	}

	modifyRequest := processModifyRequest{
		Operation: modifyConsoleSize,
		ConsoleSize: &consoleSize{
			Height: height,
			Width:  width,
		},
	}

	modifyRequestb, err := json.Marshal(modifyRequest)
	if err != nil {
		return err
	}

	modifyRequestStr := string(modifyRequestb)

	var resultp *uint16
	err = hcsModifyProcess(process.handle, modifyRequestStr, &resultp)
	events := processHcsResult(resultp)
	if err != nil {
		return makeProcessError(process, operation, err, events)
	}

	return nil
}

func (process *Process) properties(ctx context.Context) (_ *processStatus, err error) {
	operation := "hcssshim::Process::properties"
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute(logfields.SystemID, process.SystemID()),
		trace.Int64Attribute(logfields.ProcessID, int64(process.processID)))

	process.handleLock.RLock()
	defer process.handleLock.RUnlock()

	if process.handle == 0 {
		return nil, makeProcessError(process, operation, ErrAlreadyClosed, nil)
	}

	var (
		resultp     *uint16
		propertiesp *uint16
	)
	syscallWatcher(ctx, func() {
		err = hcsGetProcessProperties(process.handle, &propertiesp, &resultp)
	})
	events := processHcsResult(resultp)
	if err != nil {
		return nil, makeProcessError(process, operation, err, events)
	}

	if propertiesp == nil {
		return nil, ErrUnexpectedValue
	}
	propertiesRaw := interop.ConvertAndFreeCoTaskMemBytes(propertiesp)

	properties := &processStatus{}
	if err := json.Unmarshal(propertiesRaw, properties); err != nil {
		return nil, makeProcessError(process, operation, err, nil)
	}

	return properties, nil
}

// ExitCode returns the exit code of the process. The process must have
// already terminated.
func (process *Process) ExitCode(ctx context.Context) (_ int, err error) {
	operation := "hcssshim::Process::ExitCode"
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute(logfields.SystemID, process.SystemID()),
		trace.Int64Attribute(logfields.ProcessID, int64(process.processID)))

	properties, err := process.properties(ctx)
	if err != nil {
		return -1, makeProcessError(process, operation, err, nil)
	}

	if properties.Exited == false {
		return -1, makeProcessError(process, operation, ErrInvalidProcessState, nil)
	}

	if properties.LastWaitResult != 0 {
		log.G(ctx).WithFields(logrus.Fields{
			logfields.SystemID:  process.SystemID(),
			logfields.ProcessID: process.processID,
			"wait-result":       properties.LastWaitResult,
		}).Warn("non-zero last wait result")
		return -1, nil
	}

	log.G(ctx).WithField("exitCode", properties.ExitCode).Debug("found exit code")
	return int(properties.ExitCode), nil
}

// StdioLegacy returns the stdin, stdout, and stderr pipes, respectively. Closing
// these pipes does not close the underlying pipes; but this function can only
// be called once on each Process.
func (process *Process) StdioLegacy() (_ io.WriteCloser, _ io.ReadCloser, _ io.ReadCloser, err error) {
	operation := "hcssshim::Process::StdioLegacy"
	_, span := trace.StartSpan(context.Background(), operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute(logfields.SystemID, process.SystemID()),
		trace.Int64Attribute(logfields.ProcessID, int64(process.processID)))

	process.handleLock.RLock()
	defer process.handleLock.RUnlock()

	if process.handle == 0 {
		return nil, nil, nil, makeProcessError(process, operation, ErrAlreadyClosed, nil)
	}

	var (
		processInfo hcsProcessInformation
		resultp     *uint16
	)
	err = hcsGetProcessInfo(process.handle, &processInfo, &resultp)
	events := processHcsResult(resultp)
	if err != nil {
		return nil, nil, nil, makeProcessError(process, operation, err, events)
	}

	pipes, err := makeOpenFiles([]syscall.Handle{processInfo.StdInput, processInfo.StdOutput, processInfo.StdError})
	if err != nil {
		return nil, nil, nil, makeProcessError(process, operation, err, nil)
	}

	return pipes[0], pipes[1], pipes[2], nil
}

// Stdio returns the stdin, stdout, and stderr pipes, respectively.
// To close them, close the process handle.
func (process *Process) Stdio() (stdin io.Writer, stdout, stderr io.Reader) {
	return process.stdin, process.stdout, process.stderr
}

// CloseStdin closes the write side of the stdin pipe so that the process is
// notified on the read side that there is no more data in stdin.
func (process *Process) CloseStdin(ctx context.Context) (err error) {
	operation := "hcssshim::Process::CloseStdin"
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute(logfields.SystemID, process.SystemID()),
		trace.Int64Attribute(logfields.ProcessID, int64(process.processID)))

	process.handleLock.RLock()
	defer process.handleLock.RUnlock()

	if process.handle == 0 {
		return makeProcessError(process, operation, ErrAlreadyClosed, nil)
	}

	modifyRequest := processModifyRequest{
		Operation: modifyCloseHandle,
		CloseHandle: &closeHandle{
			Handle: stdIn,
		},
	}

	modifyRequestb, err := json.Marshal(modifyRequest)
	if err != nil {
		return err
	}

	modifyRequestStr := string(modifyRequestb)

	var resultp *uint16
	err = hcsModifyProcess(process.handle, modifyRequestStr, &resultp)
	events := processHcsResult(resultp)
	if err != nil {
		return makeProcessError(process, operation, err, events)
	}

	if process.stdin != nil {
		process.stdin.Close()
	}
	return nil
}

// Close cleans up any state associated with the process but does not kill
// or wait on it.
func (process *Process) Close(ctx context.Context) (err error) {
	operation := "hcssshim::Process::Close"
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute(logfields.SystemID, process.SystemID()),
		trace.Int64Attribute(logfields.ProcessID, int64(process.processID)))

	process.handleLock.Lock()
	defer process.handleLock.Unlock()

	// Don't double free this
	if process.handle == 0 {
		return nil
	}

	if process.stdin != nil {
		process.stdin.Close()
	}
	if process.stdout != nil {
		process.stdout.Close()
	}
	if process.stderr != nil {
		process.stderr.Close()
	}

	if err = process.unregisterCallback(); err != nil {
		return makeProcessError(process, operation, err, nil)
	}

	if err = hcsCloseProcess(process.handle); err != nil {
		return makeProcessError(process, operation, err, nil)
	}

	process.handle = 0
	process.closedWaitOnce.Do(func() {
		process.waitError = ErrAlreadyClosed
		close(process.waitBlock)
	})

	return nil
}

func (process *Process) registerCallback() error {
	context := &notifcationWatcherContext{
		channels:  newProcessChannels(),
		systemID:  process.SystemID(),
		processID: process.processID,
	}

	callbackMapLock.Lock()
	callbackNumber := nextCallback
	nextCallback++
	callbackMap[callbackNumber] = context
	callbackMapLock.Unlock()

	var callbackHandle hcsCallback
	err := hcsRegisterProcessCallback(process.handle, notificationWatcherCallback, callbackNumber, &callbackHandle)
	if err != nil {
		return err
	}
	context.handle = callbackHandle
	process.callbackNumber = callbackNumber

	return nil
}

func (process *Process) unregisterCallback() error {
	callbackNumber := process.callbackNumber

	callbackMapLock.RLock()
	context := callbackMap[callbackNumber]
	callbackMapLock.RUnlock()

	if context == nil {
		return nil
	}

	handle := context.handle

	if handle == 0 {
		return nil
	}

	// hcsUnregisterProcessCallback has its own syncronization
	// to wait for all callbacks to complete. We must NOT hold the callbackMapLock.
	err := hcsUnregisterProcessCallback(handle)
	if err != nil {
		return err
	}

	closeChannels(context.channels)

	callbackMapLock.Lock()
	delete(callbackMap, callbackNumber)
	callbackMapLock.Unlock()

	handle = 0

	return nil
}
