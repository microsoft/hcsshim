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

	"github.com/Microsoft/hcsshim/internal/computecore"
	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/hcs/schema1"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/jobobject"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/timeout"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
)

type System struct {
	handleLock sync.RWMutex
	handle     computecore.HcsSystem
	id         string

	// notificationID is the lookup key passed as the void* context to
	// HcsSetComputeSystemCallback. Zero when no callback is registered.
	notificationID uint64
	// notify rendezvous between the HCS notification callback
	// (HcsEventSystemExited) and waitBackground. Close also signals it to
	// release waitBackground without publishing an exit.
	notify *notificationState

	closedWaitOnce sync.Once
	waitBlock      chan struct{}
	waitError      error
	exitError      error
	os, typ, owner string
	startTime      time.Time
	stopTime       time.Time

	// migrationNotifyCh delivers live migration events from
	// notificationHandler. Never closed (callers signal end-of-stream
	// via their own context); sends are non-blocking and drop on overflow.
	migrationNotifyCh chan hcsschema.OperationSystemMigrationNotificationInfo
}

var _ cow.Container = &System{}
var _ cow.ProcessHost = &System{}

func newSystem(id string) *System {
	return &System{
		id:                id,
		waitBlock:         make(chan struct{}),
		notify:            newNotificationState(),
		migrationNotifyCh: make(chan hcsschema.OperationSystemMigrationNotificationInfo, migrationNotificationBufferSize),
	}
}

// Implementation detail for silo naming, this should NOT be relied upon very heavily.
func siloNameFmt(containerID string) string {
	return fmt.Sprintf(`\Container_%s`, containerID)
}

// CreateComputeSystem creates a new compute system with the given configuration but does not start it.
func CreateComputeSystem(ctx context.Context, id string, hcsDocumentInterface interface{}) (_ *System, err error) {
	operation := "hcs::CreateComputeSystem"

	ctx, span := oc.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", id))

	computeSystem := newSystem(id)

	hcsDocumentB, err := json.Marshal(hcsDocumentInterface)
	if err != nil {
		return nil, err
	}

	hcsDocument := string(hcsDocumentB)

	// On any error after this point, tear down the compute system and
	// release the handle. Terminate is guarded (no-op when handle == 0
	// or already stopped) so this is safe on every failure path,
	// including a synchronous create failure.
	defer func() {
		if err != nil {
			_ = computeSystem.Terminate(ctx)
			computeSystem.Close()
		}
	}()

	createCtx, cancel := context.WithTimeout(ctx, timeout.SystemCreate)
	defer cancel()
	_, createErr := runOperation(createCtx, func(op computecore.HcsOperation) error {
		var hErr error
		computeSystem.handle, hErr = computecore.HcsCreateComputeSystem(ctx, id, hcsDocument, op, nil)
		return hErr
	})
	if createErr != nil {
		return nil, makeSystemError(computeSystem, operation, createErr)
	}

	if err = computeSystem.registerNotification(ctx); err != nil {
		return nil, makeSystemError(computeSystem, operation, err)
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
	handle, err := computecore.HcsOpenComputeSystem(ctx, id, syscall.GENERIC_ALL)
	if err != nil {
		return nil, makeSystemError(computeSystem, operation, err)
	}

	computeSystem.handle = handle
	defer func() {
		if err != nil {
			computeSystem.Close()
		}
	}()
	if err = computeSystem.registerNotification(ctx); err != nil {
		return nil, makeSystemError(computeSystem, operation, err)
	}

	go computeSystem.waitBackground()
	if err = computeSystem.getCachedProperties(ctx); err != nil {
		return nil, err
	}
	return computeSystem, nil
}

// registerNotification registers the package-wide HCS notification callback
// on this system's primary handle. Must be called BEFORE waitBackground
// starts so notifications are not missed.
func (computeSystem *System) registerNotification(ctx context.Context) error {
	id := registerNotificationContext(computeSystem.id, 0, computeSystem.notify, computeSystem.migrationNotifyCh)
	if err := computecore.HcsSetComputeSystemCallback(
		ctx, computeSystem.handle,
		computecore.HcsEventOptionEnableLiveMigrationEvents,
		uintptr(id), notificationCallback,
	); err != nil {
		unregisterNotificationContext(id)
		return err
	}
	computeSystem.notificationID = id
	return nil
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
	query := string(queryb)

	resultJSON, err := runOperation(ctx, func(op computecore.HcsOperation) error {
		return computecore.HcsEnumerateComputeSystems(ctx, query, op)
	})
	if err != nil {
		return nil, &HcsError{Op: operation, Err: err, Events: eventsFromError(err)}
	}
	if resultJSON == "" {
		return nil, ErrUnexpectedValue
	}
	computeSystems := []schema1.ContainerProperties{}
	if err = json.Unmarshal([]byte(resultJSON), &computeSystems); err != nil {
		return nil, err
	}

	return computeSystems, nil
}

// Start synchronously starts the computeSystem.
func (computeSystem *System) Start(ctx context.Context) (err error) {
	operation := "hcs::System::Start"

	ctx, span := oc.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", computeSystem.id))

	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	// Prevent starting an exited system: we do not recreate waitBlock or
	// rerun waitBackground, so we have no way to be notified of it closing again.
	if computeSystem.handle == 0 {
		return makeSystemError(computeSystem, operation, ErrAlreadyClosed)
	}

	startCtx, cancel := context.WithTimeout(ctx, timeout.SystemStart)
	defer cancel()
	_, callErr := runOperation(startCtx, func(op computecore.HcsOperation) error {
		return computecore.HcsStartComputeSystem(ctx, computeSystem.handle, op, "")
	})
	if callErr != nil {
		return makeSystemError(computeSystem, operation, callErr)
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

	if computeSystem.handle == 0 || computeSystem.stopped() {
		return nil
	}

	err := submitOperation(ctx, func(op computecore.HcsOperation) error {
		return computecore.HcsShutDownComputeSystem(ctx, computeSystem.handle, op, "")
	})
	if err != nil &&
		!errors.Is(err, ErrVmcomputeAlreadyStopped) &&
		!errors.Is(err, ErrComputeSystemDoesNotExist) &&
		!errors.Is(err, ErrVmcomputeOperationPending) {
		return makeSystemError(computeSystem, operation, err)
	}
	return nil
}

// Terminate requests a compute system terminate.
func (computeSystem *System) Terminate(ctx context.Context) error {
	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	operation := "hcs::System::Terminate"

	if computeSystem.handle == 0 || computeSystem.stopped() {
		return nil
	}

	err := submitOperation(ctx, func(op computecore.HcsOperation) error {
		return computecore.HcsTerminateComputeSystem(ctx, computeSystem.handle, op, "")
	})
	if err != nil &&
		!errors.Is(err, ErrVmcomputeAlreadyStopped) &&
		!errors.Is(err, ErrComputeSystemDoesNotExist) &&
		!errors.Is(err, ErrVmcomputeOperationPending) {
		return makeSystemError(computeSystem, operation, err)
	}
	return nil
}

// waitBackground blocks until either the HCS callback delivers
// HcsEventSystemExited (real exit, publish status) or Close fires (release
// without publishing). It then sets `computeSystem.waitError` (if any) and
// unblocks all `Wait` calls.
//
// HCS does not deliver a final notification on HcsCloseComputeSystem — it
// just unregisters the callback — so Close needs its own signal.
// Prematurely closing WaitChannel() causes `hcsExec.waitForContainerExit`
// to Kill the container's running process and report exitCode=-1
// (rendered as 255).
//
// This MUST be called exactly once per `computeSystem.handle` but `Wait` is
// safe to call multiple times.
func (computeSystem *System) waitBackground() {
	operation := "hcs::System::waitBackground"
	ctx, span := oc.StartSpan(context.Background(), operation)
	defer span.End()
	span.AddAttributes(trace.StringAttribute("cid", computeSystem.id))

	var raw json.RawMessage
	var err error
	select {
	case <-computeSystem.waitBlock:
		log.G(ctx).Debug("system waitBackground returning without exit notification (handle closed)")
		return
	case raw = <-computeSystem.notify.exit:
	case abortErr := <-computeSystem.notify.abort:
		err = makeSystemError(computeSystem, operation, abortErr)
	}

	if err == nil && len(raw) > 0 {
		var status struct {
			Status   int32  `json:"Status"`
			ExitType string `json:"ExitType"`
		}
		if uErr := json.Unmarshal(raw, &status); uErr != nil {
			log.G(ctx).WithError(uErr).WithField("exit-data", string(raw)).Warning("failed to parse SystemExitStatus")
		} else if status.ExitType == "UnexpectedExit" {
			log.G(ctx).Debug("unexpected system exit")
			computeSystem.exitError = makeSystemError(computeSystem, operation, ErrVmcomputeUnexpectedExit)
		}
	}
	computeSystem.closedWaitOnce.Do(func() {
		computeSystem.waitError = err
		computeSystem.stopTime = time.Now()
		close(computeSystem.waitBlock)
	})
	oc.SetSpanStatus(span, err)
}

func (computeSystem *System) WaitChannel() <-chan struct{} {
	return computeSystem.waitBlock
}

func (computeSystem *System) WaitError() error {
	return computeSystem.waitError
}

// Wait synchronously waits for the compute system to shutdown or terminate.
// If the compute system has already exited returns the previous error (if any).
func (computeSystem *System) Wait() error {
	return computeSystem.WaitCtx(context.Background())
}

// WaitCtx synchronously waits for the compute system to shutdown or terminate, or the context to be cancelled.
//
// See [System.Wait] for more information.
func (computeSystem *System) WaitCtx(ctx context.Context) error {
	select {
	case <-computeSystem.WaitChannel():
		return computeSystem.WaitError()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// stopped returns true if the compute system stopped.
func (computeSystem *System) stopped() bool {
	select {
	case <-computeSystem.waitBlock:
		return true
	default:
	}
	return false
}

// ExitError returns an error describing the reason the compute system terminated.
func (computeSystem *System) ExitError() error {
	if !computeSystem.stopped() {
		return errors.New("container not exited")
	}
	if computeSystem.waitError != nil {
		return computeSystem.waitError
	}
	return computeSystem.exitError
}

// Properties returns the requested container properties targeting a V1 schema container.
func (computeSystem *System) Properties(ctx context.Context, types ...schema1.PropertyType) (*schema1.ContainerProperties, error) {
	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	operation := "hcs::System::Properties"

	if computeSystem.handle == 0 {
		return nil, makeSystemError(computeSystem, operation, ErrAlreadyClosed)
	}

	queryBytes, err := json.Marshal(schema1.PropertyQuery{PropertyTypes: types})
	if err != nil {
		return nil, makeSystemError(computeSystem, operation, err)
	}
	query := string(queryBytes)

	propertiesJSON, err := runOperation(ctx, func(op computecore.HcsOperation) error {
		return computecore.HcsGetComputeSystemProperties(ctx, computeSystem.handle, op, query)
	})
	if err != nil {
		return nil, makeSystemError(computeSystem, operation, err)
	}

	if propertiesJSON == "" {
		return nil, ErrUnexpectedValue
	}
	properties := &schema1.ContainerProperties{}
	if err := json.Unmarshal([]byte(propertiesJSON), properties); err != nil {
		return nil, makeSystemError(computeSystem, operation, err)
	}

	return properties, nil
}

// queryInProc handles querying for container properties without reaching out to HCS. `props`
// will be updated to contain any data returned from the queries present in `types`. If any properties
// failed to be queried they will be tallied up and returned in as the first return value. Failures on
// query are NOT considered errors; the only failure case for this method is if the containers job object
// cannot be opened.
func (computeSystem *System) queryInProc(
	ctx context.Context,
	props *hcsschema.Properties,
	types []hcsschema.PropertyType,
) ([]hcsschema.PropertyType, error) {
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

	var fallbackQueryTypes []hcsschema.PropertyType
	for _, propType := range types {
		switch propType {
		case hcsschema.PTStatistics:
			// Handle a bad caller asking for the same type twice. No use in re-querying if this is
			// filled in already.
			if props.Statistics == nil {
				props.Statistics, err = computeSystem.statisticsInProc(job)
				if err != nil {
					log.G(ctx).WithError(err).Warn("failed to get statistics in-proc")

					fallbackQueryTypes = append(fallbackQueryTypes, propType)
				}
			}
		default:
			fallbackQueryTypes = append(fallbackQueryTypes, propType)
		}
	}

	return fallbackQueryTypes, nil
}

// statisticsInProc emulates what HCS does to grab statistics for a given container with a small
// change to make grabbing the private working set total much more efficient.
func (computeSystem *System) statisticsInProc(job *jobobject.JobObject) (*hcsschema.Statistics, error) {
	// Start timestamp for these stats before we grab them to match HCS
	timestamp := time.Now()

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
	// working set ourselves and ask for everything else separately. The optimization we can make here is
	// to open the silo ourselves and do the same queries for the rest of the info, as well as calculating
	// the private working set in a more efficient manner by:
	//
	// 1. Find the pids running in the silo
	// 2. Get a process handle for every process (only need PROCESS_QUERY_LIMITED_INFORMATION access)
	// 3. Call NtQueryInformationProcess on each process with the class ProcessVmCounters
	// 4. Tally up the total using the field PrivateWorkingSetSize in VM_COUNTERS_EX2.
	privateWorkingSet, err := job.QueryPrivateWorkingSet()
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

// hcsPropertiesV2Query is a helper to make a HcsGetComputeSystemProperties call using the V2 schema property types.
func (computeSystem *System) hcsPropertiesV2Query(ctx context.Context, types []hcsschema.PropertyType) (*hcsschema.Properties, error) {
	operation := "hcs::System::PropertiesV2"

	if computeSystem.handle == 0 {
		return nil, makeSystemError(computeSystem, operation, ErrAlreadyClosed)
	}

	queryBytes, err := json.Marshal(hcsschema.PropertyQuery{PropertyTypes: types})
	if err != nil {
		return nil, makeSystemError(computeSystem, operation, err)
	}
	query := string(queryBytes)

	propertiesJSON, err := runOperation(ctx, func(op computecore.HcsOperation) error {
		return computecore.HcsGetComputeSystemProperties(ctx, computeSystem.handle, op, query)
	})
	if err != nil {
		return nil, makeSystemError(computeSystem, operation, err)
	}

	if propertiesJSON == "" {
		return nil, ErrUnexpectedValue
	}
	props := &hcsschema.Properties{}
	if err := json.Unmarshal([]byte(propertiesJSON), props); err != nil {
		return nil, makeSystemError(computeSystem, operation, err)
	}

	return props, nil
}

// PropertiesV2 returns the requested compute systems properties targeting a V2 schema compute system.
func (computeSystem *System) PropertiesV2(ctx context.Context, types ...hcsschema.PropertyType) (_ *hcsschema.Properties, err error) {
	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	// Let HCS tally up the total for VM based queries instead of querying ourselves.
	if computeSystem.typ != "container" {
		return computeSystem.hcsPropertiesV2Query(ctx, types)
	}

	// Define a starter Properties struct with the default fields returned from every
	// query. Owner is only returned from Statistics but it's harmless to include.
	properties := &hcsschema.Properties{
		Id:            computeSystem.id,
		SystemType:    computeSystem.typ,
		RuntimeOsType: computeSystem.os,
		Owner:         computeSystem.owner,
	}

	logEntry := log.G(ctx)
	// First lets try and query ourselves without reaching to HCS. If any of the queries fail
	// we'll take note and fallback to querying HCS for any of the failed types.
	fallbackTypes, err := computeSystem.queryInProc(ctx, properties, types)
	if err == nil && len(fallbackTypes) == 0 {
		return properties, nil
	} else if err != nil {
		logEntry = logEntry.WithError(fmt.Errorf("failed to query compute system properties in-proc: %w", err))
		fallbackTypes = types
	}

	logEntry.WithFields(logrus.Fields{
		logfields.ContainerID: computeSystem.id,
		"propertyTypes":       fallbackTypes,
	}).Info("falling back to HCS for property type queries")

	hcsProperties, err := computeSystem.hcsPropertiesV2Query(ctx, fallbackTypes)
	if err != nil {
		return nil, err
	}

	// Now add in anything that we might have successfully queried in process.
	if properties.Statistics != nil {
		hcsProperties.Statistics = properties.Statistics
		hcsProperties.Owner = properties.Owner
	}

	// For future support for querying processlist in-proc as well.
	if properties.ProcessList != nil {
		hcsProperties.ProcessList = properties.ProcessList
	}

	return hcsProperties, nil
}

// PropertiesV3 returns the requested compute system properties using a V2 schema property query.
// Unlike [System.PropertiesV2], this method accepts a full [hcsschema.PropertyQuery] directly,
// giving the caller more control over the query structure. The query is forwarded to HCS as-is
// without any in-proc optimisations such as that is V2.
func (computeSystem *System) PropertiesV3(ctx context.Context, query *hcsschema.PropertyQuery) (_ *hcsschema.Properties, err error) {
	operation := "hcs::System::PropertiesV3"

	ctx, span := oc.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", computeSystem.id))

	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	if computeSystem.handle == 0 {
		return nil, makeSystemError(computeSystem, operation, ErrAlreadyClosed)
	}

	log.G(ctx).WithFields(logrus.Fields{
		logfields.ContainerID: computeSystem.id,
		"propertyTypes":       query.PropertyTypes,
		"propertyQueries":     query.Queries,
	}).Debug("querying compute system properties via PropertiesV3")

	queryBytes, err := json.Marshal(query)
	if err != nil {
		return nil, makeSystemError(computeSystem, operation, err)
	}
	queryStr := string(queryBytes)

	propertiesJSON, err := runOperation(ctx, func(op computecore.HcsOperation) error {
		return computecore.HcsGetComputeSystemProperties(ctx, computeSystem.handle, op, queryStr)
	})
	if err != nil {
		return nil, makeSystemError(computeSystem, operation, err)
	}

	if propertiesJSON == "" {
		return nil, ErrUnexpectedValue
	}

	props := &hcsschema.Properties{}
	if err := json.Unmarshal([]byte(propertiesJSON), props); err != nil {
		return nil, makeSystemError(computeSystem, operation, err)
	}

	return props, nil
}

// Pause pauses the execution of the computeSystem. This feature is not enabled in TP5.
func (computeSystem *System) Pause(ctx context.Context) (err error) {
	operation := "hcs::System::Pause"

	ctx, span := oc.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", computeSystem.id))

	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	if computeSystem.handle == 0 {
		return makeSystemError(computeSystem, operation, ErrAlreadyClosed)
	}

	pauseCtx, cancel := context.WithTimeout(ctx, timeout.SystemPause)
	defer cancel()
	_, callErr := runOperation(pauseCtx, func(op computecore.HcsOperation) error {
		return computecore.HcsPauseComputeSystem(ctx, computeSystem.handle, op, "")
	})
	if callErr != nil {
		return makeSystemError(computeSystem, operation, callErr)
	}
	return nil
}

// Resume resumes the execution of the computeSystem. This feature is not enabled in TP5.
func (computeSystem *System) Resume(ctx context.Context) (err error) {
	operation := "hcs::System::Resume"

	ctx, span := oc.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", computeSystem.id))

	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	if computeSystem.handle == 0 {
		return makeSystemError(computeSystem, operation, ErrAlreadyClosed)
	}

	resumeCtx, cancel := context.WithTimeout(ctx, timeout.SystemResume)
	defer cancel()
	_, callErr := runOperation(resumeCtx, func(op computecore.HcsOperation) error {
		return computecore.HcsResumeComputeSystem(ctx, computeSystem.handle, op, "")
	})
	if callErr != nil {
		return makeSystemError(computeSystem, operation, callErr)
	}
	return nil
}

// Save the compute system
func (computeSystem *System) Save(ctx context.Context, options interface{}) (err error) {
	operation := "hcs::System::Save"

	ctx, span := oc.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", computeSystem.id))

	saveOptions, err := json.Marshal(options)
	if err != nil {
		return err
	}
	saveOptionsStr := string(saveOptions)

	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	if computeSystem.handle == 0 {
		return makeSystemError(computeSystem, operation, ErrAlreadyClosed)
	}

	saveCtx, cancel := context.WithTimeout(ctx, timeout.SystemSave)
	defer cancel()
	_, callErr := runOperation(saveCtx, func(op computecore.HcsOperation) error {
		return computecore.HcsSaveComputeSystem(ctx, computeSystem.handle, op, saveOptionsStr)
	})
	if callErr != nil {
		return makeSystemError(computeSystem, operation, callErr)
	}
	return nil
}

// createProcess launches a process in the compute system and returns its
// handle plus the stdio info HCS produced.
//
// On process-isolated containers (observed on WS2022) HcsCreateProcess can
// complete synchronously and leave the tracking operation in a state where
// the wait spuriously fails (e.g. E_INVALIDARG) even though HCS handed back
// a valid process handle. Recover via HcsGetProcessInfo, but only on wait
// failure — re-fetching after a successful wait races a short-lived process
// exit and surfaces as HCS_E_PROCESS_ALREADY_STOPPED, which would wrongly
// fail the create.
func (computeSystem *System) createProcess(ctx context.Context, operation string, c interface{}) (*Process, *computecore.HcsProcessInformation, error) {
	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	if computeSystem.handle == 0 {
		return nil, nil, makeSystemError(computeSystem, operation, ErrAlreadyClosed)
	}

	configurationb, err := json.Marshal(c)
	if err != nil {
		return nil, nil, makeSystemError(computeSystem, operation, err)
	}
	configuration := string(configurationb)

	// Tag the operation label with the offending command line so error logs
	// identify what HCS rejected.
	switch v := c.(type) {
	case *hcsschema.ProcessParameters:
		operation += ": " + v.CommandLine
	case *schema1.ProcessConfig:
		operation += ": " + v.CommandLine
	}

	var processHandle computecore.HcsProcess
	processInfo, _, createErr := runProcessOperation(ctx, func(op computecore.HcsOperation) error {
		var hErr error
		processHandle, hErr = computecore.HcsCreateProcess(ctx, computeSystem.handle, configuration, op, nil)
		return hErr
	})
	if createErr != nil {
		// No handle means the create itself failed; only a failed wait with
		// a live handle is the recoverable sync-completion case.
		if processHandle == 0 {
			return nil, nil, makeSystemError(computeSystem, operation, createErr)
		}

		log.G(ctx).WithError(createErr).Debug("HcsCreateProcess wait failed; falling back to HcsGetProcessInfo")
		var recoverErr error
		processInfo, _, recoverErr = runProcessOperation(ctx, func(op computecore.HcsOperation) error {
			return computecore.HcsGetProcessInfo(ctx, processHandle, op)
		})
		if recoverErr != nil {
			// Recovery error (e.g. RPC_E_NULL_CONTEXT_HANDLE on a stale handle)
			// hides the real cause; surface the original create-wait error.
			computecore.HcsCloseProcess(ctx, processHandle)
			return nil, nil, makeSystemError(computeSystem, operation, createErr)
		}
	}

	log.G(ctx).WithField("pid", processInfo.ProcessID).Debug("created process pid")
	return newProcess(processHandle, int(processInfo.ProcessID), computeSystem), &processInfo, nil
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
		return nil, makeSystemError(computeSystem, operation, err)
	}
	process.stdin = pipes[0]
	process.stdout = pipes[1]
	process.stderr = pipes[2]
	process.hasCachedStdio = true

	if err = process.registerNotification(ctx); err != nil {
		return nil, makeSystemError(computeSystem, operation, err)
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
		return nil, makeSystemError(computeSystem, operation, ErrAlreadyClosed)
	}

	processHandle, err := computecore.HcsOpenProcess(ctx, computeSystem.handle, uint32(pid), syscall.GENERIC_ALL)
	if err != nil {
		return nil, makeSystemError(computeSystem, operation, err)
	}

	process := newProcess(processHandle, pid, computeSystem)
	defer func() {
		if err != nil {
			_ = process.Close()
		}
	}()
	if err = process.registerNotification(ctx); err != nil {
		return nil, makeSystemError(computeSystem, operation, err)
	}
	go process.waitBackground()

	return process, nil
}

// Close cleans up any state associated with the compute system but does not terminate or wait for it.
func (computeSystem *System) Close() error {
	return computeSystem.CloseCtx(context.Background())
}

// CloseCtx is similar to [System.Close], but accepts a context.
//
// The context is used for all operations, including waits, so timeouts/cancellations may prevent
// proper system cleanup.
func (computeSystem *System) CloseCtx(ctx context.Context) (err error) {
	operation := "hcs::System::Close"
	ctx, span := oc.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", computeSystem.id))

	computeSystem.handleLock.Lock()
	defer computeSystem.handleLock.Unlock()

	// Don't double free this
	if computeSystem.handle == 0 {
		return nil
	}

	// HcsCloseComputeSystem internally unregisters our notification
	// callback and drains in-flight invocations before tearing the handle
	// down.
	computecore.HcsCloseComputeSystem(ctx, computeSystem.handle)
	unregisterNotificationContext(computeSystem.notificationID)
	computeSystem.notificationID = 0

	// Release Wait/WaitChannel callers with ErrAlreadyClosed
	// and unblock waitBackground.
	computeSystem.handle = 0
	computeSystem.closedWaitOnce.Do(func() {
		computeSystem.waitError = ErrAlreadyClosed
		computeSystem.stopTime = time.Now()
		close(computeSystem.waitBlock)
	})

	return nil
}

// Modify the System by sending a request to HCS
func (computeSystem *System) Modify(ctx context.Context, config interface{}) error {
	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	operation := "hcs::System::Modify"

	if computeSystem.handle == 0 {
		return makeSystemError(computeSystem, operation, ErrAlreadyClosed)
	}

	requestBytes, err := json.Marshal(config)
	if err != nil {
		return err
	}

	requestJSON := string(requestBytes)

	_, callErr := runOperation(ctx, func(op computecore.HcsOperation) error {
		return computecore.HcsModifyComputeSystem(ctx, computeSystem.handle, op, requestJSON, 0)
	})
	if callErr != nil {
		return makeSystemError(computeSystem, operation, callErr)
	}
	return nil
}

func (computeSystem *System) StoppedTime() time.Time {
	return computeSystem.stopTime
}

func (computeSystem *System) StartedTime() time.Time {
	return computeSystem.startTime
}
