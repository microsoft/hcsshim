//go:build windows

package hcs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/hcs/schema1"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/jobobject"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/timeout"
	"github.com/Microsoft/hcsshim/internal/vmcompute"
	"go.opencensus.io/trace"
)

type System struct {
	handleLock     sync.RWMutex
	handle         vmcompute.HcsSystem
	id             string
	callbackNumber uintptr

	closedWaitOnce sync.Once
	waitBlock      chan struct{}
	waitError      error
	exitError      error
	os, typ, owner string
	startTime      time.Time
}

func newSystem(id string) *System {
	return &System{
		id:        id,
		waitBlock: make(chan struct{}),
	}
}

// Implementation detail for silo naming, this should NOT be relied upon very heavily.
func siloNameFmt(containerID string) string {
	return fmt.Sprintf(`\Container_%s`, containerID)
}

// CreateComputeSystem creates a new compute system with the given configuration but does not start it.
func CreateComputeSystem(ctx context.Context, id string, hcsDocumentInterface interface{}) (_ *System, err error) {
	operation := "hcs::CreateComputeSystem"

	// hcsCreateComputeSystemContext is an async operation. Start the outer span
	// here to measure the full create time.
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", id))

	computeSystem := newSystem(id)

	hcsDocumentB, err := json.Marshal(hcsDocumentInterface)
	if err != nil {
		return nil, err
	}

	hcsDocument := string(hcsDocumentB)

	var (
		identity    syscall.Handle
		resultJSON  string
		createError error
	)
	computeSystem.handle, resultJSON, createError = vmcompute.HcsCreateComputeSystem(ctx, id, hcsDocument, identity)
	if createError == nil || IsPending(createError) {
		defer func() {
			if err != nil {
				computeSystem.Close()
			}
		}()
		if err = computeSystem.registerCallback(ctx); err != nil {
			// Terminate the compute system if it still exists. We're okay to
			// ignore a failure here.
			_ = computeSystem.Terminate(ctx)
			return nil, makeSystemError(computeSystem, operation, err, nil)
		}
	}

	events, err := processAsyncHcsResult(ctx, createError, resultJSON, computeSystem.callbackNumber, hcsNotificationSystemCreateCompleted, &timeout.SystemCreate)
	if err != nil {
		if err == ErrTimeout {
			// Terminate the compute system if it still exists. We're okay to
			// ignore a failure here.
			_ = computeSystem.Terminate(ctx)
		}
		return nil, makeSystemError(computeSystem, operation, err, events)
	}
	go computeSystem.waitBackground()
	if err = computeSystem.getCachedProperties(ctx); err != nil {
		return nil, err
	}
	return computeSystem, nil
}

// OpenComputeSystem opens an existing compute system by ID.
func OpenComputeSystem(ctx context.Context, id string) (*System, error) {
	operation := "hcs::OpenComputeSystem"

	computeSystem := newSystem(id)
	handle, resultJSON, err := vmcompute.HcsOpenComputeSystem(ctx, id)
	events := processHcsResult(ctx, resultJSON)
	if err != nil {
		return nil, makeSystemError(computeSystem, operation, err, events)
	}
	computeSystem.handle = handle
	defer func() {
		if err != nil {
			computeSystem.Close()
		}
	}()
	if err = computeSystem.registerCallback(ctx); err != nil {
		return nil, makeSystemError(computeSystem, operation, err, nil)
	}
	go computeSystem.waitBackground()
	if err = computeSystem.getCachedProperties(ctx); err != nil {
		return nil, err
	}
	return computeSystem, nil
}

func (computeSystem *System) getCachedProperties(ctx context.Context) error {
	props, err := computeSystem.Properties(ctx)
	if err != nil {
		return err
	}
	computeSystem.typ = strings.ToLower(props.SystemType)
	computeSystem.os = strings.ToLower(props.RuntimeOSType)
	computeSystem.owner = strings.ToLower(props.Owner)
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
func GetComputeSystems(ctx context.Context, q schema1.ComputeSystemQuery) ([]schema1.ContainerProperties, error) {
	operation := "hcs::GetComputeSystems"

	queryb, err := json.Marshal(q)
	if err != nil {
		return nil, err
	}

	computeSystemsJSON, resultJSON, err := vmcompute.HcsEnumerateComputeSystems(ctx, string(queryb))
	events := processHcsResult(ctx, resultJSON)
	if err != nil {
		return nil, &HcsError{Op: operation, Err: err, Events: events}
	}

	if computeSystemsJSON == "" {
		return nil, ErrUnexpectedValue
	}
	computeSystems := []schema1.ContainerProperties{}
	if err = json.Unmarshal([]byte(computeSystemsJSON), &computeSystems); err != nil {
		return nil, err
	}

	return computeSystems, nil
}

// Start synchronously starts the computeSystem.
func (computeSystem *System) Start(ctx context.Context) (err error) {
	operation := "hcs::System::Start"

	// hcsStartComputeSystemContext is an async operation. Start the outer span
	// here to measure the full start time.
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", computeSystem.id))

	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	if computeSystem.handle == 0 {
		return makeSystemError(computeSystem, operation, ErrAlreadyClosed, nil)
	}

	resultJSON, err := vmcompute.HcsStartComputeSystem(ctx, computeSystem.handle, "")
	events, err := processAsyncHcsResult(ctx, err, resultJSON, computeSystem.callbackNumber, hcsNotificationSystemStartCompleted, &timeout.SystemStart)
	if err != nil {
		return makeSystemError(computeSystem, operation, err, events)
	}
	computeSystem.startTime = time.Now()
	return nil
}

// ID returns the compute system's identifier.
func (computeSystem *System) ID() string {
	return computeSystem.id
}

// Shutdown requests a compute system shutdown.
func (computeSystem *System) Shutdown(ctx context.Context) error {
	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	operation := "hcs::System::Shutdown"

	if computeSystem.handle == 0 {
		return nil
	}

	resultJSON, err := vmcompute.HcsShutdownComputeSystem(ctx, computeSystem.handle, "")
	events := processHcsResult(ctx, resultJSON)
	switch err {
	case nil, ErrVmcomputeAlreadyStopped, ErrComputeSystemDoesNotExist, ErrVmcomputeOperationPending:
	default:
		return makeSystemError(computeSystem, operation, err, events)
	}
	return nil
}

// Terminate requests a compute system terminate.
func (computeSystem *System) Terminate(ctx context.Context) error {
	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	operation := "hcs::System::Terminate"

	if computeSystem.handle == 0 {
		return nil
	}

	resultJSON, err := vmcompute.HcsTerminateComputeSystem(ctx, computeSystem.handle, "")
	events := processHcsResult(ctx, resultJSON)
	switch err {
	case nil, ErrVmcomputeAlreadyStopped, ErrComputeSystemDoesNotExist, ErrVmcomputeOperationPending:
	default:
		return makeSystemError(computeSystem, operation, err, events)
	}
	return nil
}

// waitBackground waits for the compute system exit notification. Once received
// sets `computeSystem.waitError` (if any) and unblocks all `Wait` calls.
//
// This MUST be called exactly once per `computeSystem.handle` but `Wait` is
// safe to call multiple times.
func (computeSystem *System) waitBackground() {
	operation := "hcs::System::waitBackground"
	ctx, span := trace.StartSpan(context.Background(), operation)
	defer span.End()
	span.AddAttributes(trace.StringAttribute("cid", computeSystem.id))

	err := waitForNotification(ctx, computeSystem.callbackNumber, hcsNotificationSystemExited, nil)
	switch err {
	case nil:
		log.G(ctx).Debug("system exited")
	case ErrVmcomputeUnexpectedExit:
		log.G(ctx).Debug("unexpected system exit")
		computeSystem.exitError = makeSystemError(computeSystem, operation, err, nil)
		err = nil
	default:
		err = makeSystemError(computeSystem, operation, err, nil)
	}
	computeSystem.closedWaitOnce.Do(func() {
		computeSystem.waitError = err
		close(computeSystem.waitBlock)
	})
	oc.SetSpanStatus(span, err)
}

// Wait synchronously waits for the compute system to shutdown or terminate. If
// the compute system has already exited returns the previous error (if any).
func (computeSystem *System) Wait() error {
	<-computeSystem.waitBlock
	return computeSystem.waitError
}

// ExitError returns an error describing the reason the compute system terminated.
func (computeSystem *System) ExitError() error {
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

// Properties returns the requested container properties targeting a V1 schema container.
func (computeSystem *System) Properties(ctx context.Context, types ...schema1.PropertyType) (*schema1.ContainerProperties, error) {
	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	operation := "hcs::System::Properties"

	queryBytes, err := json.Marshal(schema1.PropertyQuery{PropertyTypes: types})
	if err != nil {
		return nil, makeSystemError(computeSystem, operation, err, nil)
	}

	propertiesJSON, resultJSON, err := vmcompute.HcsGetComputeSystemProperties(ctx, computeSystem.handle, string(queryBytes))
	events := processHcsResult(ctx, resultJSON)
	if err != nil {
		return nil, makeSystemError(computeSystem, operation, err, events)
	}

	if propertiesJSON == "" {
		return nil, ErrUnexpectedValue
	}
	properties := &schema1.ContainerProperties{}
	if err := json.Unmarshal([]byte(propertiesJSON), properties); err != nil {
		return nil, makeSystemError(computeSystem, operation, err, nil)
	}

	return properties, nil
}

// statisticsInProc emulates what HCS does to grab statistics for a given container with a small
// change to make grabbing the private working set total much more efficient.
func (computeSystem *System) statisticsInProc(ctx context.Context) (*hcsschema.Statistics, error) {
	// Start timestamp for these stats before we grab them to match HCS
	timestamp := time.Now()

	// In the future we can make use of some new functionality in the HCS that allows you
	// to pass a job object for HCS to use for the container. Currently, the only way we'll
	// be able to open the job/silo is if we're running as SYSTEM.
	jobOptions := &jobobject.Options{
		UseNTVariant: true,
		Name:         siloNameFmt(computeSystem.id),
	}
	job, err := jobobject.Open(ctx, jobOptions)
	if err != nil {
		return nil, err
	}
	defer job.Close()

	memInfo, err := job.QueryMemoryStats()
	if err != nil {
		return nil, err
	}

	processorInfo, err := job.QueryProcessorStats()
	if err != nil {
		return nil, err
	}

	storageInfo, err := job.QueryStorageStats()
	if err != nil {
		return nil, err
	}

	// This calculates the private working set more efficiently than HCS does. HCS calls NtQuerySystemInformation
	// with the class SystemProcessInformation which returns an array containing system information for *every*
	// process running on the machine. They then grab the pids that are running in the container and filter down
	// the entries in the array to only what's running in that silo and start tallying up the total. This doesn't
	// work well as performance should get worse if more processess are running on the machine in general and not
	// just in the container. All of the additional information besides the WorkingSetPrivateSize field is ignored
	// as well which isn't great and is wasted work to fetch.
	//
	// HCS only let's you grab statistics in an all or nothing fashion, so we can't just grab the private
	// working set ourselves and ask for everything else seperately. The optimization we can make here is
	// to open the silo ourselves and do the same queries for the rest of the info, as well as calculating
	// the private working set in a more efficient manner by:
	//
	// 1. Find the pids running in the silo
	// 2. Get a process handle for every process (only need PROCESS_QUERY_LIMITED_INFORMATION access)
	// 3. Call NtQueryInformationProcess on each process with the class ProcessVmCounters
	// 4. Tally up the total using the field PrivateWorkingSetSize in VM_COUNTERS_EX2.
	privateWorkingSet, err := job.PrivateWorkingSet()
	if err != nil {
		return nil, err
	}

	return &hcsschema.Statistics{
		Timestamp:          timestamp,
		ContainerStartTime: computeSystem.startTime,
		Uptime100ns:        uint64(time.Since(computeSystem.startTime).Nanoseconds()) / 100,
		Memory: &hcsschema.MemoryStats{
			MemoryUsageCommitBytes:            memInfo.JobMemory,
			MemoryUsageCommitPeakBytes:        memInfo.PeakJobMemoryUsed,
			MemoryUsagePrivateWorkingSetBytes: privateWorkingSet,
		},
		Processor: &hcsschema.ProcessorStats{
			RuntimeKernel100ns: uint64(processorInfo.TotalKernelTime),
			RuntimeUser100ns:   uint64(processorInfo.TotalUserTime),
			TotalRuntime100ns:  uint64(processorInfo.TotalKernelTime + processorInfo.TotalUserTime),
		},
		Storage: &hcsschema.StorageStats{
			ReadCountNormalized:  uint64(storageInfo.ReadStats.IoCount),
			ReadSizeBytes:        storageInfo.ReadStats.TotalSize,
			WriteCountNormalized: uint64(storageInfo.WriteStats.IoCount),
			WriteSizeBytes:       storageInfo.WriteStats.TotalSize,
		},
	}, nil
}

// PropertiesV2 returns the requested container properties targeting a V2 schema container.
func (computeSystem *System) PropertiesV2(ctx context.Context, types ...hcsschema.PropertyType) (_ *hcsschema.Properties, err error) {
	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	operation := "hcs::System::PropertiesV2"

	// Check if any of the queries are for stats. We can grab these in process and skip the hop to
	// HCS.
	var statsPresent, onlyStats bool
	for i, prop := range types {
		if prop == hcsschema.PTStatistics {
			statsPresent = true
			onlyStats = len(types) == 1
			// Remove stats from the query types.
			types = append(types[:i], types[i+1:]...)
		}
	}

	properties := &hcsschema.Properties{}
	if statsPresent && computeSystem.typ == "container" {
		properties.Statistics, err = computeSystem.statisticsInProc(ctx)
		if err == nil {
			// Early return if this was the only thing we were querying for.
			if onlyStats {
				properties.Id = computeSystem.id
				properties.SystemType = computeSystem.typ
				properties.RuntimeOsType = computeSystem.os
				properties.Owner = computeSystem.owner
				return properties, nil
			}
		} else {
			logTxt := "failed to grab statistics in process - falling back to HCS"
			log.G(ctx).WithError(err).WithField(logfields.ContainerID, computeSystem.id).Warn(logTxt)
			types = append(types, hcsschema.PTStatistics)
		}
	}

	queryBytes, err := json.Marshal(hcsschema.PropertyQuery{PropertyTypes: types})
	if err != nil {
		return nil, makeSystemError(computeSystem, operation, err, nil)
	}

	propertiesJSON, resultJSON, err := vmcompute.HcsGetComputeSystemProperties(ctx, computeSystem.handle, string(queryBytes))
	events := processHcsResult(ctx, resultJSON)
	if err != nil {
		return nil, makeSystemError(computeSystem, operation, err, events)
	}

	if propertiesJSON == "" {
		return nil, ErrUnexpectedValue
	}
	hcsProps := &hcsschema.Properties{}
	if err := json.Unmarshal([]byte(propertiesJSON), hcsProps); err != nil {
		return nil, makeSystemError(computeSystem, operation, err, nil)
	}
	// Copy over stats if we might've failed the inProc query.
	if properties.Statistics != nil {
		hcsProps.Statistics = properties.Statistics
		hcsProps.Owner = computeSystem.owner
	}

	return hcsProps, nil
}

// Pause pauses the execution of the computeSystem. This feature is not enabled in TP5.
func (computeSystem *System) Pause(ctx context.Context) (err error) {
	operation := "hcs::System::Pause"

	// hcsPauseComputeSystemContext is an async peration. Start the outer span
	// here to measure the full pause time.
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", computeSystem.id))

	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	if computeSystem.handle == 0 {
		return makeSystemError(computeSystem, operation, ErrAlreadyClosed, nil)
	}

	resultJSON, err := vmcompute.HcsPauseComputeSystem(ctx, computeSystem.handle, "")
	events, err := processAsyncHcsResult(ctx, err, resultJSON, computeSystem.callbackNumber, hcsNotificationSystemPauseCompleted, &timeout.SystemPause)
	if err != nil {
		return makeSystemError(computeSystem, operation, err, events)
	}

	return nil
}

// Resume resumes the execution of the computeSystem. This feature is not enabled in TP5.
func (computeSystem *System) Resume(ctx context.Context) (err error) {
	operation := "hcs::System::Resume"

	// hcsResumeComputeSystemContext is an async operation. Start the outer span
	// here to measure the full restore time.
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", computeSystem.id))

	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	if computeSystem.handle == 0 {
		return makeSystemError(computeSystem, operation, ErrAlreadyClosed, nil)
	}

	resultJSON, err := vmcompute.HcsResumeComputeSystem(ctx, computeSystem.handle, "")
	events, err := processAsyncHcsResult(ctx, err, resultJSON, computeSystem.callbackNumber, hcsNotificationSystemResumeCompleted, &timeout.SystemResume)
	if err != nil {
		return makeSystemError(computeSystem, operation, err, events)
	}

	return nil
}

// Save the compute system
func (computeSystem *System) Save(ctx context.Context, options interface{}) (err error) {
	operation := "hcs::System::Save"

	// hcsSaveComputeSystemContext is an async peration. Start the outer span
	// here to measure the full save time.
	ctx, span := trace.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", computeSystem.id))

	saveOptions, err := json.Marshal(options)
	if err != nil {
		return err
	}

	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	if computeSystem.handle == 0 {
		return makeSystemError(computeSystem, operation, ErrAlreadyClosed, nil)
	}

	result, err := vmcompute.HcsSaveComputeSystem(ctx, computeSystem.handle, string(saveOptions))
	events, err := processAsyncHcsResult(ctx, err, result, computeSystem.callbackNumber, hcsNotificationSystemSaveCompleted, &timeout.SystemSave)
	if err != nil {
		return makeSystemError(computeSystem, operation, err, events)
	}

	return nil
}

func (computeSystem *System) createProcess(ctx context.Context, operation string, c interface{}) (*Process, *vmcompute.HcsProcessInformation, error) {
	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	if computeSystem.handle == 0 {
		return nil, nil, makeSystemError(computeSystem, operation, ErrAlreadyClosed, nil)
	}

	configurationb, err := json.Marshal(c)
	if err != nil {
		return nil, nil, makeSystemError(computeSystem, operation, err, nil)
	}

	configuration := string(configurationb)
	processInfo, processHandle, resultJSON, err := vmcompute.HcsCreateProcess(ctx, computeSystem.handle, configuration)
	events := processHcsResult(ctx, resultJSON)
	if err != nil {
		return nil, nil, makeSystemError(computeSystem, operation, err, events)
	}

	log.G(ctx).WithField("pid", processInfo.ProcessId).Debug("created process pid")
	return newProcess(processHandle, int(processInfo.ProcessId), computeSystem), &processInfo, nil
}

// CreateProcess launches a new process within the computeSystem.
func (computeSystem *System) CreateProcess(ctx context.Context, c interface{}) (cow.Process, error) {
	operation := "hcs::System::CreateProcess"
	process, processInfo, err := computeSystem.createProcess(ctx, operation, c)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			process.Close()
		}
	}()

	pipes, err := makeOpenFiles([]syscall.Handle{processInfo.StdInput, processInfo.StdOutput, processInfo.StdError})
	if err != nil {
		return nil, makeSystemError(computeSystem, operation, err, nil)
	}
	process.stdin = pipes[0]
	process.stdout = pipes[1]
	process.stderr = pipes[2]
	process.hasCachedStdio = true

	if err = process.registerCallback(ctx); err != nil {
		return nil, makeSystemError(computeSystem, operation, err, nil)
	}
	go process.waitBackground()

	return process, nil
}

// OpenProcess gets an interface to an existing process within the computeSystem.
func (computeSystem *System) OpenProcess(ctx context.Context, pid int) (*Process, error) {
	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	operation := "hcs::System::OpenProcess"

	if computeSystem.handle == 0 {
		return nil, makeSystemError(computeSystem, operation, ErrAlreadyClosed, nil)
	}

	processHandle, resultJSON, err := vmcompute.HcsOpenProcess(ctx, computeSystem.handle, uint32(pid))
	events := processHcsResult(ctx, resultJSON)
	if err != nil {
		return nil, makeSystemError(computeSystem, operation, err, events)
	}

	process := newProcess(processHandle, pid, computeSystem)
	if err = process.registerCallback(ctx); err != nil {
		return nil, makeSystemError(computeSystem, operation, err, nil)
	}
	go process.waitBackground()

	return process, nil
}

// Close cleans up any state associated with the compute system but does not terminate or wait for it.
func (computeSystem *System) Close() (err error) {
	operation := "hcs::System::Close"
	ctx, span := trace.StartSpan(context.Background(), operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", computeSystem.id))

	computeSystem.handleLock.Lock()
	defer computeSystem.handleLock.Unlock()

	// Don't double free this
	if computeSystem.handle == 0 {
		return nil
	}

	if err = computeSystem.unregisterCallback(ctx); err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}

	err = vmcompute.HcsCloseComputeSystem(ctx, computeSystem.handle)
	if err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}

	computeSystem.handle = 0
	computeSystem.closedWaitOnce.Do(func() {
		computeSystem.waitError = ErrAlreadyClosed
		close(computeSystem.waitBlock)
	})

	return nil
}

func (computeSystem *System) registerCallback(ctx context.Context) error {
	callbackContext := &notificationWatcherContext{
		channels: newSystemChannels(),
		systemID: computeSystem.id,
	}

	callbackMapLock.Lock()
	callbackNumber := nextCallback
	nextCallback++
	callbackMap[callbackNumber] = callbackContext
	callbackMapLock.Unlock()

	callbackHandle, err := vmcompute.HcsRegisterComputeSystemCallback(ctx, computeSystem.handle, notificationWatcherCallback, callbackNumber)
	if err != nil {
		return err
	}
	callbackContext.handle = callbackHandle
	computeSystem.callbackNumber = callbackNumber

	return nil
}

func (computeSystem *System) unregisterCallback(ctx context.Context) error {
	callbackNumber := computeSystem.callbackNumber

	callbackMapLock.RLock()
	callbackContext := callbackMap[callbackNumber]
	callbackMapLock.RUnlock()

	if callbackContext == nil {
		return nil
	}

	handle := callbackContext.handle

	if handle == 0 {
		return nil
	}

	// hcsUnregisterComputeSystemCallback has its own synchronization
	// to wait for all callbacks to complete. We must NOT hold the callbackMapLock.
	err := vmcompute.HcsUnregisterComputeSystemCallback(ctx, handle)
	if err != nil {
		return err
	}

	closeChannels(callbackContext.channels)

	callbackMapLock.Lock()
	delete(callbackMap, callbackNumber)
	callbackMapLock.Unlock()

	handle = 0 //nolint:ineffassign

	return nil
}

// Modify the System by sending a request to HCS
func (computeSystem *System) Modify(ctx context.Context, config interface{}) error {
	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	operation := "hcs::System::Modify"

	if computeSystem.handle == 0 {
		return makeSystemError(computeSystem, operation, ErrAlreadyClosed, nil)
	}

	requestBytes, err := json.Marshal(config)
	if err != nil {
		return err
	}

	requestJSON := string(requestBytes)
	resultJSON, err := vmcompute.HcsModifyComputeSystem(ctx, computeSystem.handle, requestJSON)
	events := processHcsResult(ctx, resultJSON)
	if err != nil {
		return makeSystemError(computeSystem, operation, err, events)
	}

	return nil
}
