package hcs

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/interop"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/schema1"
	"github.com/Microsoft/hcsshim/internal/timeout"
	"go.opencensus.io/trace"
)

// currentContainerStarts is used to limit the number of concurrent container
// starts.
var currentContainerStarts containerStarts

type containerStarts struct {
	maxParallel int
	inProgress  int
	sync.Mutex
}

func init() {
	mpsS := os.Getenv("HCSSHIM_MAX_PARALLEL_START")
	if len(mpsS) > 0 {
		mpsI, err := strconv.Atoi(mpsS)
		if err != nil || mpsI < 0 {
			return
		}
		currentContainerStarts.maxParallel = mpsI
	}
}

type System struct {
	handleLock     sync.RWMutex
	handle         hcsSystem
	id             string
	callbackNumber uintptr

	closedWaitOnce sync.Once
	waitBlock      chan struct{}
	waitError      error
	exitError      error

	os, typ string
}

func newSystem(id string) *System {
	return &System{
		id:        id,
		waitBlock: make(chan struct{}),
	}
}

// CreateComputeSystem creates a new compute system with the given configuration but does not start it.
func CreateComputeSystem(ctx context.Context, id string, hcsDocumentInterface interface{}) (_ *System, err error) {
	operation := "hcsshim::CreateComputeSystem"
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute(logfields.SystemID, id))

	computeSystem := newSystem(id)

	hcsDocumentB, err := json.Marshal(hcsDocumentInterface)
	if err != nil {
		return nil, err
	}

	hcsDocument := string(hcsDocumentB)

	log.G(ctx).WithField("configuration", hcsDocument).Debug("create system document")

	var (
		resultp     *uint16
		identity    syscall.Handle
		createError error
	)
	syscallWatcher(ctx, func() {
		createError = hcsCreateComputeSystem(id, hcsDocument, identity, &computeSystem.handle, &resultp)
	})

	if createError == nil || IsPending(createError) {
		defer func() {
			if err != nil {
				computeSystem.Close(ctx)
			}
		}()
		if err = computeSystem.registerCallback(); err != nil {
			// Terminate the compute system if it still exists. We're okay to
			// ignore a failure here.
			computeSystem.Terminate(ctx)
			return nil, makeSystemError(computeSystem, operation, "", err, nil)
		}
	}

	events, err := processAsyncHcsResult(createError, resultp, computeSystem.callbackNumber, hcsNotificationSystemCreateCompleted, &timeout.SystemCreate)
	if err != nil {
		if err == ErrTimeout {
			// Terminate the compute system if it still exists. We're okay to
			// ignore a failure here.
			computeSystem.Terminate(ctx)
		}
		return nil, makeSystemError(computeSystem, operation, hcsDocument, err, events)
	}
	go computeSystem.waitBackground()
	if err = computeSystem.getCachedProperties(ctx); err != nil {
		return nil, err
	}
	return computeSystem, nil
}

// OpenComputeSystem opens an existing compute system by ID.
func OpenComputeSystem(ctx context.Context, id string) (_ *System, err error) {
	operation := "hcsshim::OpenComputeSystem"
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() {
		if !IsNotExist(err) {
			oc.SetSpanStatus(span, err)
		}
	}()
	span.AddAttributes(trace.StringAttribute(logfields.SystemID, id))

	computeSystem := newSystem(id)

	var (
		handle  hcsSystem
		resultp *uint16
	)
	err = hcsOpenComputeSystem(id, &handle, &resultp)
	events := processHcsResult(resultp)
	if err != nil {
		return nil, makeSystemError(computeSystem, operation, "", err, events)
	}
	computeSystem.handle = handle
	defer func() {
		if err != nil {
			computeSystem.Close(ctx)
		}
	}()
	if err = computeSystem.registerCallback(); err != nil {
		return nil, makeSystemError(computeSystem, operation, "", err, nil)
	}
	go computeSystem.waitBackground()
	if err = computeSystem.getCachedProperties(ctx); err != nil {
		return nil, err
	}
	return computeSystem, nil
}

func (computeSystem *System) getCachedProperties(ctx context.Context) (err error) {
	ctx, span := trace.StartSpan(ctx, "hcsshim::System::getCachedProperties")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute(logfields.SystemID, computeSystem.id))

	props, err := computeSystem.Properties(ctx)
	if err != nil {
		return err
	}
	computeSystem.typ = strings.ToLower(props.SystemType)
	computeSystem.os = strings.ToLower(props.RuntimeOSType)
	if computeSystem.os == "" && computeSystem.typ == "container" {
		// Pre-RS5 HCS did not return the OS, but it only supported containers
		// that ran Windows.
		computeSystem.os = "windows"
	}
	return nil
}

// OS returns the operating system of the compute system, "linux" or "windows".
func (computeSystem *System) OS() string {
	return computeSystem.os
}

// IsOCI returns whether processes in the compute system should be created via
// OCI.
func (computeSystem *System) IsOCI() bool {
	return computeSystem.os == "linux" && computeSystem.typ == "container"
}

// GetComputeSystems gets a list of the compute systems on the system that match the query
func GetComputeSystems(ctx context.Context, q schema1.ComputeSystemQuery) (_ []schema1.ContainerProperties, err error) {
	operation := "hcsshim::GetComputeSystems"
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	queryb, err := json.Marshal(q)
	if err != nil {
		return nil, err
	}

	query := string(queryb)

	log.G(ctx).WithField("query", query).Debug("query document")

	var (
		resultp         *uint16
		computeSystemsp *uint16
	)

	syscallWatcher(ctx, func() {
		err = hcsEnumerateComputeSystems(query, &computeSystemsp, &resultp)
	})
	events := processHcsResult(resultp)
	if err != nil {
		return nil, &HcsError{Op: operation, Err: err, Events: events}
	}

	if computeSystemsp == nil {
		return nil, ErrUnexpectedValue
	}
	computeSystemsRaw := interop.ConvertAndFreeCoTaskMemBytes(computeSystemsp)
	computeSystems := []schema1.ContainerProperties{}
	if err = json.Unmarshal(computeSystemsRaw, &computeSystems); err != nil {
		return nil, err
	}

	return computeSystems, nil
}

// Start synchronously starts the computeSystem.
func (computeSystem *System) Start(ctx context.Context) (err error) {
	operation := "hcsshim::System::Start"
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute(logfields.SystemID, computeSystem.id))

	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	if computeSystem.handle == 0 {
		return makeSystemError(computeSystem, operation, "", ErrAlreadyClosed, nil)
	}

	// This is a very simple backoff-retry loop to limit the number
	// of parallel container starts if environment variable
	// HCSSHIM_MAX_PARALLEL_START is set to a positive integer.
	// It should generally only be used as a workaround to various
	// platform issues that exist between RS1 and RS4 as of Aug 2018
	if currentContainerStarts.maxParallel > 0 {
		for {
			currentContainerStarts.Lock()
			if currentContainerStarts.inProgress < currentContainerStarts.maxParallel {
				currentContainerStarts.inProgress++
				currentContainerStarts.Unlock()
				break
			}
			if currentContainerStarts.inProgress == currentContainerStarts.maxParallel {
				currentContainerStarts.Unlock()
				time.Sleep(100 * time.Millisecond)
			}
		}
		// Make sure we decrement the count when we are done.
		defer func() {
			currentContainerStarts.Lock()
			currentContainerStarts.inProgress--
			currentContainerStarts.Unlock()
		}()
	}

	var resultp *uint16
	syscallWatcher(ctx, func() {
		err = hcsStartComputeSystem(computeSystem.handle, "", &resultp)
	})
	events, err := processAsyncHcsResult(err, resultp, computeSystem.callbackNumber, hcsNotificationSystemStartCompleted, &timeout.SystemStart)
	if err != nil {
		return makeSystemError(computeSystem, operation, "", err, events)
	}

	return nil
}

// ID returns the compute system's identifier.
func (computeSystem *System) ID() string {
	return computeSystem.id
}

// Shutdown requests a compute system shutdown.
func (computeSystem *System) Shutdown(ctx context.Context) (err error) {
	operation := "hcsshim::System::Shutdown"
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute(logfields.SystemID, computeSystem.id))

	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	if computeSystem.handle == 0 {
		return nil
	}

	var resultp *uint16
	syscallWatcher(ctx, func() {
		err = hcsShutdownComputeSystem(computeSystem.handle, "", &resultp)
	})
	events := processHcsResult(resultp)
	switch err {
	case nil, ErrVmcomputeAlreadyStopped, ErrComputeSystemDoesNotExist, ErrVmcomputeOperationPending:
	default:
		return makeSystemError(computeSystem, operation, "", err, events)
	}
	return nil
}

// Terminate requests a compute system terminate.
func (computeSystem *System) Terminate(ctx context.Context) (err error) {
	operation := "hcsshim::System::Terminate"
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute(logfields.SystemID, computeSystem.id))

	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	if computeSystem.handle == 0 {
		return nil
	}

	var resultp *uint16
	syscallWatcher(ctx, func() {
		err = hcsTerminateComputeSystem(computeSystem.handle, "", &resultp)
	})
	events := processHcsResult(resultp)
	switch err {
	case nil, ErrVmcomputeAlreadyStopped, ErrComputeSystemDoesNotExist, ErrVmcomputeOperationPending:
	default:
		return makeSystemError(computeSystem, operation, "", err, events)
	}
	return nil
}

// waitBackground waits for the compute system exit notification. Once received
// sets `computeSystem.waitError` (if any) and unblocks all `Wait` calls.
//
// This MUST be called exactly once per `computeSystem.handle` but `Wait` is
// safe to call multiple times.
func (computeSystem *System) waitBackground() {
	operation := "hcsshim::System::waitBackground"
	ctx, span := trace.StartSpan(context.Background(), operation)
	defer span.End()
	span.AddAttributes(trace.StringAttribute(logfields.SystemID, computeSystem.id))

	err := waitForNotification(computeSystem.callbackNumber, hcsNotificationSystemExited, nil)
	switch err {
	case nil:
	case ErrVmcomputeUnexpectedExit:
		log.G(ctx).WithField(logfields.SystemID, computeSystem.id).Debug("unexpected system exit")
		computeSystem.exitError = makeSystemError(computeSystem, operation, "", err, nil)
		err = nil
	default:
		err = makeSystemError(computeSystem, operation, "", err, nil)
	}
	oc.SetSpanStatus(span, err)
	computeSystem.closedWaitOnce.Do(func() {
		computeSystem.waitError = err
		close(computeSystem.waitBlock)
	})
}

// Wait synchronously waits for the compute system to shutdown or terminate. If
// the compute system has already exited returns the previous error (if any).
func (computeSystem *System) Wait() (err error) {
	<-computeSystem.waitBlock
	return computeSystem.waitError
}

// ExitError returns an error describing the reason the compute system terminated.
func (computeSystem *System) ExitError() (err error) {
	select {
	case <-computeSystem.waitBlock:
		if computeSystem.waitError != nil {
			return computeSystem.waitError
		}
		return computeSystem.exitError
	default:
		return errors.New("container not exited")
	}
}

func (computeSystem *System) Properties(ctx context.Context, types ...schema1.PropertyType) (_ *schema1.ContainerProperties, err error) {
	operation := "hcsshim::System::Properties"
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute(logfields.SystemID, computeSystem.id))

	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	queryBytes, err := json.Marshal(schema1.PropertyQuery{PropertyTypes: types})
	if err != nil {
		return nil, makeSystemError(computeSystem, operation, "", err, nil)
	}

	queryString := string(queryBytes)
	log.G(ctx).WithField("propertyQuery", queryString).Debug("query document")

	var resultp, propertiesp *uint16
	syscallWatcher(ctx, func() {
		err = hcsGetComputeSystemProperties(computeSystem.handle, string(queryString), &propertiesp, &resultp)
	})
	events := processHcsResult(resultp)
	if err != nil {
		return nil, makeSystemError(computeSystem, operation, "", err, events)
	}

	if propertiesp == nil {
		return nil, ErrUnexpectedValue
	}
	propertiesRaw := interop.ConvertAndFreeCoTaskMemBytes(propertiesp)
	properties := &schema1.ContainerProperties{}
	if err := json.Unmarshal(propertiesRaw, properties); err != nil {
		return nil, makeSystemError(computeSystem, operation, "", err, nil)
	}

	return properties, nil
}

// Pause pauses the execution of the computeSystem. This feature is not enabled in TP5.
func (computeSystem *System) Pause(ctx context.Context) (err error) {
	operation := "hcsshim::System::Pause"
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute(logfields.SystemID, computeSystem.id))

	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	if computeSystem.handle == 0 {
		return makeSystemError(computeSystem, operation, "", ErrAlreadyClosed, nil)
	}

	var resultp *uint16
	syscallWatcher(ctx, func() {
		err = hcsPauseComputeSystem(computeSystem.handle, "", &resultp)
	})
	events, err := processAsyncHcsResult(err, resultp, computeSystem.callbackNumber, hcsNotificationSystemPauseCompleted, &timeout.SystemPause)
	if err != nil {
		return makeSystemError(computeSystem, operation, "", err, events)
	}

	return nil
}

// Resume resumes the execution of the computeSystem. This feature is not enabled in TP5.
func (computeSystem *System) Resume(ctx context.Context) (err error) {
	operation := "hcsshim::System::Resume"
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute(logfields.SystemID, computeSystem.id))

	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	if computeSystem.handle == 0 {
		return makeSystemError(computeSystem, operation, "", ErrAlreadyClosed, nil)
	}

	var resultp *uint16
	syscallWatcher(ctx, func() {
		err = hcsResumeComputeSystem(computeSystem.handle, "", &resultp)
	})
	events, err := processAsyncHcsResult(err, resultp, computeSystem.callbackNumber, hcsNotificationSystemResumeCompleted, &timeout.SystemResume)
	if err != nil {
		return makeSystemError(computeSystem, operation, "", err, events)
	}

	return nil
}

func (computeSystem *System) createProcess(ctx context.Context, c interface{}) (_ *Process, _ *hcsProcessInformation, err error) {
	operation := "hcsshim::System::createProcess"
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute(logfields.SystemID, computeSystem.id))

	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	var (
		processInfo   hcsProcessInformation
		processHandle hcsProcess
		resultp       *uint16
	)

	if computeSystem.handle == 0 {
		return nil, nil, makeSystemError(computeSystem, operation, "", ErrAlreadyClosed, nil)
	}

	configurationb, err := json.Marshal(c)
	if err != nil {
		return nil, nil, makeSystemError(computeSystem, operation, "", err, nil)
	}

	configuration := string(configurationb)

	log.G(ctx).WithField("processParameters", configuration).Debug("create process document")

	syscallWatcher(ctx, func() {
		err = hcsCreateProcess(computeSystem.handle, configuration, &processInfo, &processHandle, &resultp)
	})
	events := processHcsResult(resultp)
	if err != nil {
		return nil, nil, makeSystemError(computeSystem, operation, configuration, err, events)
	}

	log.G(ctx).WithField(logfields.ProcessID, processInfo.ProcessId).Debug("created process pid")
	return newProcess(processHandle, int(processInfo.ProcessId), computeSystem), &processInfo, nil
}

// CreateProcessNoStdio launches a new process within the computeSystem. The
// Stdio handles are not cached on the process struct.
func (computeSystem *System) CreateProcessNoStdio(ctx context.Context, c interface{}) (_ cow.Process, err error) {
	operation := "hcsshim::System::CreateProcessNoStdio"
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute(logfields.SystemID, computeSystem.id))

	process, processInfo, err := computeSystem.createProcess(ctx, c)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			process.Close(ctx)
		}
	}()

	// We don't do anything with these handles. Close them so they don't leak.
	syscall.Close(processInfo.StdInput)
	syscall.Close(processInfo.StdOutput)
	syscall.Close(processInfo.StdError)

	if err = process.registerCallback(); err != nil {
		return nil, makeSystemError(computeSystem, operation, "", err, nil)
	}
	go process.waitBackground()

	return process, nil
}

// CreateProcess launches a new process within the computeSystem.
func (computeSystem *System) CreateProcess(ctx context.Context, c interface{}) (_ cow.Process, err error) {
	operation := "hcsshim::System::CreateProcess"
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute(logfields.SystemID, computeSystem.id))

	process, processInfo, err := computeSystem.createProcess(ctx, c)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			process.Close(ctx)
		}
	}()

	pipes, err := makeOpenFiles([]syscall.Handle{processInfo.StdInput, processInfo.StdOutput, processInfo.StdError})
	if err != nil {
		return nil, makeSystemError(computeSystem, operation, "", err, nil)
	}
	process.stdin = pipes[0]
	process.stdout = pipes[1]
	process.stderr = pipes[2]

	if err = process.registerCallback(); err != nil {
		return nil, makeSystemError(computeSystem, operation, "", err, nil)
	}
	go process.waitBackground()

	return process, nil
}

// OpenProcess gets an interface to an existing process within the computeSystem.
func (computeSystem *System) OpenProcess(ctx context.Context, pid int) (_ *Process, err error) {
	operation := "hcsshim::System::OpenProcess"
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute(logfields.SystemID, computeSystem.id),
		trace.Int64Attribute(logfields.ProcessID, int64(pid)))

	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	var (
		processHandle hcsProcess
		resultp       *uint16
	)

	if computeSystem.handle == 0 {
		return nil, makeSystemError(computeSystem, operation, "", ErrAlreadyClosed, nil)
	}

	syscallWatcher(ctx, func() {
		err = hcsOpenProcess(computeSystem.handle, uint32(pid), &processHandle, &resultp)
	})
	events := processHcsResult(resultp)
	if err != nil {
		return nil, makeSystemError(computeSystem, operation, "", err, events)
	}

	process := newProcess(processHandle, pid, computeSystem)
	if err = process.registerCallback(); err != nil {
		return nil, makeSystemError(computeSystem, operation, "", err, nil)
	}
	go process.waitBackground()

	return process, nil
}

// Close cleans up any state associated with the compute system but does not terminate or wait for it.
func (computeSystem *System) Close(ctx context.Context) (err error) {
	operation := "hcsshim::System::Close"
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute(logfields.SystemID, computeSystem.id))

	computeSystem.handleLock.Lock()
	defer computeSystem.handleLock.Unlock()

	// Don't double free this
	if computeSystem.handle == 0 {
		return nil
	}

	if err = computeSystem.unregisterCallback(); err != nil {
		return makeSystemError(computeSystem, operation, "", err, nil)
	}

	syscallWatcher(ctx, func() {
		err = hcsCloseComputeSystem(computeSystem.handle)
	})
	if err != nil {
		return makeSystemError(computeSystem, operation, "", err, nil)
	}

	computeSystem.handle = 0
	computeSystem.closedWaitOnce.Do(func() {
		computeSystem.waitError = ErrAlreadyClosed
		close(computeSystem.waitBlock)
	})

	return nil
}

func (computeSystem *System) registerCallback() error {
	context := &notifcationWatcherContext{
		channels: newSystemChannels(),
		systemID: computeSystem.id,
	}

	callbackMapLock.Lock()
	callbackNumber := nextCallback
	nextCallback++
	callbackMap[callbackNumber] = context
	callbackMapLock.Unlock()

	var callbackHandle hcsCallback
	err := hcsRegisterComputeSystemCallback(computeSystem.handle, notificationWatcherCallback, callbackNumber, &callbackHandle)
	if err != nil {
		return err
	}
	context.handle = callbackHandle
	computeSystem.callbackNumber = callbackNumber

	return nil
}

func (computeSystem *System) unregisterCallback() error {
	callbackNumber := computeSystem.callbackNumber

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

	// hcsUnregisterComputeSystemCallback has its own syncronization
	// to wait for all callbacks to complete. We must NOT hold the callbackMapLock.
	err := hcsUnregisterComputeSystemCallback(handle)
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

// Modify the System by sending a request to HCS
func (computeSystem *System) Modify(ctx context.Context, config interface{}) (err error) {
	operation := "hcsshim::System::Modify"
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute(logfields.SystemID, computeSystem.id))

	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	if computeSystem.handle == 0 {
		return makeSystemError(computeSystem, operation, "", ErrAlreadyClosed, nil)
	}

	requestJSON, err := json.Marshal(config)
	if err != nil {
		return err
	}

	requestString := string(requestJSON)

	log.G(ctx).WithField("configuration", requestString).Debug("modify document")

	var resultp *uint16
	syscallWatcher(ctx, func() {
		err = hcsModifyComputeSystem(computeSystem.handle, requestString, &resultp)
	})
	events := processHcsResult(resultp)
	if err != nil {
		return makeSystemError(computeSystem, operation, requestString, err, events)
	}

	return nil
}
