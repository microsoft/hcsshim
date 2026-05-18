//go:build windows

package computecore

import (
	gcontext "context"
	"syscall"
	"time"
	"unsafe"

	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"

	"github.com/Microsoft/hcsshim/internal/interop"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/timeout"
)

//go:generate go tool github.com/Microsoft/go-winio/tools/mkwinsyscall -output zsyscall_windows.go computecore.go

// Operation management
//sys hcsCreateOperation(context uintptr, callback uintptr) (operation HcsOperation, err error) = computecore.HcsCreateOperation?
//sys hcsCreateOperationWithNotifications(eventTypes uint32, context uintptr, callback uintptr) (operation HcsOperation, err error) = computecore.HcsCreateOperationWithNotifications?
//sys hcsCloseOperation(operation HcsOperation) = computecore.HcsCloseOperation
//sys hcsGetOperationContext(operation HcsOperation) (context uintptr) = computecore.HcsGetOperationContext
//sys hcsSetOperationContext(operation HcsOperation, context uintptr) (hr error) = computecore.HcsSetOperationContext?
//sys hcsGetComputeSystemFromOperation(operation HcsOperation) (computeSystem HcsSystem) = computecore.HcsGetComputeSystemFromOperation
//sys hcsGetProcessFromOperation(operation HcsOperation) (process HcsProcess) = computecore.HcsGetProcessFromOperation
//sys hcsGetOperationType(operation HcsOperation) (operationType int32) = computecore.HcsGetOperationType
//sys hcsGetOperationId(operation HcsOperation) (operationId uint64) = computecore.HcsGetOperationId
//sys hcsGetOperationResult(operation HcsOperation, resultDocument **uint16) (hr error) = computecore.HcsGetOperationResult?
//sys hcsGetOperationResultAndProcessInfo(operation HcsOperation, processInformation *HcsProcessInformation, resultDocument **uint16) (hr error) = computecore.HcsGetOperationResultAndProcessInfo?
//sys hcsAddResourceToOperation(operation HcsOperation, resourceType uint32, uri string, handle syscall.Handle) (hr error) = computecore.HcsAddResourceToOperation?
//sys hcsGetProcessorCompatibilityFromSavedState(runtimeFileName string, processorFeaturesString **uint16) (hr error) = computecore.HcsGetProcessorCompatibilityFromSavedState?
//sys hcsWaitForOperationResult(operation HcsOperation, timeoutMs uint32, resultDocument **uint16) (hr error) = computecore.HcsWaitForOperationResult?
//sys hcsWaitForOperationResultAndProcessInfo(operation HcsOperation, timeoutMs uint32, processInformation *HcsProcessInformation, resultDocument **uint16) (hr error) = computecore.HcsWaitForOperationResultAndProcessInfo?
//sys hcsSetOperationCallback(operation HcsOperation, context uintptr, callback uintptr) (hr error) = computecore.HcsSetOperationCallback?
//sys hcsCancelOperation(operation HcsOperation) (hr error) = computecore.HcsCancelOperation?
//sys hcsGetOperationProperties(operation HcsOperation, options string, resultDocument **uint16) (hr error) = computecore.HcsGetOperationProperties?

// Compute system lifecycle
//sys hcsEnumerateComputeSystems(query string, operation HcsOperation) (hr error) = computecore.HcsEnumerateComputeSystems?
//sys hcsEnumerateComputeSystemsInNamespace(idNamespace string, query string, operation HcsOperation) (hr error) = computecore.HcsEnumerateComputeSystemsInNamespace?
//sys hcsCreateComputeSystem(id string, configuration string, operation HcsOperation, securityDescriptor unsafe.Pointer, computeSystem *HcsSystem) (hr error) = computecore.HcsCreateComputeSystem?
//sys hcsCreateComputeSystemInNamespace(idNamespace string, id string, configuration string, operation HcsOperation, options unsafe.Pointer, computeSystem *HcsSystem) (hr error) = computecore.HcsCreateComputeSystemInNamespace?
//sys hcsOpenComputeSystem(id string, requestedAccess uint32, computeSystem *HcsSystem) (hr error) = computecore.HcsOpenComputeSystem?
//sys hcsOpenComputeSystemInNamespace(idNamespace string, id string, requestedAccess uint32, computeSystem *HcsSystem) (hr error) = computecore.HcsOpenComputeSystemInNamespace?
//sys hcsCloseComputeSystem(computeSystem HcsSystem) = computecore.HcsCloseComputeSystem
//sys hcsStartComputeSystem(computeSystem HcsSystem, operation HcsOperation, options string) (hr error) = computecore.HcsStartComputeSystem?
//sys hcsShutDownComputeSystem(computeSystem HcsSystem, operation HcsOperation, options string) (hr error) = computecore.HcsShutDownComputeSystem?
//sys hcsTerminateComputeSystem(computeSystem HcsSystem, operation HcsOperation, options string) (hr error) = computecore.HcsTerminateComputeSystem?
//sys hcsCrashComputeSystem(computeSystem HcsSystem, operation HcsOperation, options string) (hr error) = computecore.HcsCrashComputeSystem?
//sys hcsPauseComputeSystem(computeSystem HcsSystem, operation HcsOperation, options string) (hr error) = computecore.HcsPauseComputeSystem?
//sys hcsResumeComputeSystem(computeSystem HcsSystem, operation HcsOperation, options string) (hr error) = computecore.HcsResumeComputeSystem?
//sys hcsSaveComputeSystem(computeSystem HcsSystem, operation HcsOperation, options string) (hr error) = computecore.HcsSaveComputeSystem?
//sys hcsGetComputeSystemProperties(computeSystem HcsSystem, operation HcsOperation, propertyQuery string) (hr error) = computecore.HcsGetComputeSystemProperties?
//sys hcsModifyComputeSystem(computeSystem HcsSystem, operation HcsOperation, configuration string, identity syscall.Handle) (hr error) = computecore.HcsModifyComputeSystem?
//sys hcsWaitForComputeSystemExit(computeSystem HcsSystem, timeoutMs uint32, result **uint16) (hr error) = computecore.HcsWaitForComputeSystemExit?
//sys hcsSetComputeSystemCallback(computeSystem HcsSystem, callbackOptions uint32, context uintptr, callback uintptr) (hr error) = computecore.HcsSetComputeSystemCallback?

// Live migration
//sys hcsInitializeLiveMigrationOnSource(computeSystem HcsSystem, operation HcsOperation, options string) (hr error) = computecore.HcsInitializeLiveMigrationOnSource?
//sys hcsStartLiveMigrationOnSource(computeSystem HcsSystem, operation HcsOperation, options string) (hr error) = computecore.HcsStartLiveMigrationOnSource?
//sys hcsStartLiveMigrationTransfer(computeSystem HcsSystem, operation HcsOperation, options string) (hr error) = computecore.HcsStartLiveMigrationTransfer?
//sys hcsFinalizeLiveMigration(computeSystem HcsSystem, operation HcsOperation, options string) (hr error) = computecore.HcsFinalizeLiveMigration?

// Process lifecycle
//sys hcsCreateProcess(computeSystem HcsSystem, processParameters string, operation HcsOperation, securityDescriptor unsafe.Pointer, process *HcsProcess) (hr error) = computecore.HcsCreateProcess?
//sys hcsOpenProcess(computeSystem HcsSystem, pid uint32, requestedAccess uint32, process *HcsProcess) (hr error) = computecore.HcsOpenProcess?
//sys hcsCloseProcess(process HcsProcess) = computecore.HcsCloseProcess
//sys hcsTerminateProcess(process HcsProcess, operation HcsOperation, options string) (hr error) = computecore.HcsTerminateProcess?
//sys hcsSignalProcess(process HcsProcess, operation HcsOperation, options string) (hr error) = computecore.HcsSignalProcess?
//sys hcsGetProcessInfo(process HcsProcess, operation HcsOperation) (hr error) = computecore.HcsGetProcessInfo?
//sys hcsGetProcessProperties(process HcsProcess, operation HcsOperation, propertyQuery string) (hr error) = computecore.HcsGetProcessProperties?
//sys hcsModifyProcess(process HcsProcess, operation HcsOperation, settings string) (hr error) = computecore.HcsModifyProcess?
//sys hcsSetProcessCallback(process HcsProcess, callbackOptions uint32, context uintptr, callback uintptr) (hr error) = computecore.HcsSetProcessCallback?
//sys hcsWaitForProcessExit(process HcsProcess, timeoutMs uint32, result **uint16) (hr error) = computecore.HcsWaitForProcessExit?

// Service
//sys hcsGetServiceProperties(propertyQuery string, result **uint16) (hr error) = computecore.HcsGetServiceProperties?
//sys hcsModifyServiceSettings(settings string, result **uint16) (hr error) = computecore.HcsModifyServiceSettings?
//sys hcsSubmitWerReport(settings string) (hr error) = computecore.HcsSubmitWerReport?

// File and VM access
//sys hcsCreateEmptyGuestStateFile(guestStateFilePath string) (hr error) = computecore.HcsCreateEmptyGuestStateFile?
//sys hcsCreateEmptyRuntimeStateFile(runtimeStateFilePath string) (hr error) = computecore.HcsCreateEmptyRuntimeStateFile?
//sys hcsGrantVmAccess(vmId string, filePath string) (hr error) = computecore.HcsGrantVmAccess?
//sys hcsRevokeVmAccess(vmId string, filePath string) (hr error) = computecore.HcsRevokeVmAccess?
//sys hcsGrantVmGroupAccess(filePath string) (hr error) = computecore.HcsGrantVmGroupAccess?
//sys hcsRevokeVmGroupAccess(filePath string) (hr error) = computecore.HcsRevokeVmGroupAccess?

// errVmcomputeOperationPending is an error encountered when the operation is being completed asynchronously
const errVmcomputeOperationPending = syscall.Errno(0xC0370103)

func execute(ctx gcontext.Context, timeout time.Duration, f func() error) error {
	now := time.Now()
	if timeout > 0 {
		var cancel gcontext.CancelFunc
		ctx, cancel = gcontext.WithTimeout(ctx, timeout)
		defer cancel()
	}

	deadline, ok := ctx.Deadline()
	trueTimeout := timeout
	if ok {
		trueTimeout = deadline.Sub(now)
		log.G(ctx).WithFields(logrus.Fields{
			logfields.Timeout: trueTimeout,
			"desiredTimeout":  timeout,
		}).Trace("Executing syscall with deadline")
	}

	done := make(chan error, 1)
	go func() {
		done <- f()
	}()
	select {
	case <-ctx.Done():
		if ctx.Err() == gcontext.DeadlineExceeded {
			log.G(ctx).WithField(logfields.Timeout, trueTimeout).
				Warning("Syscall did not complete within operation timeout. This may indicate a platform issue. " +
					"If it appears to be making no forward progress, obtain the stacks and see if there is a syscall " +
					"stuck in the platform API for a significant length of time.")
		}
		return ctx.Err()
	case err := <-done:
		return err
	}
}

// Operation management

func HcsCreateOperation(ctx gcontext.Context, callbackContext uintptr, callback uintptr) (operation HcsOperation, hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsCreateOperation")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()

	return operation, execute(ctx, timeout.SyscallWatcher, func() error {
		var err error
		operation, err = hcsCreateOperation(callbackContext, callback)
		return err
	})
}

func HcsCreateOperationWithNotifications(ctx gcontext.Context, eventTypes uint32, callbackContext uintptr, callback uintptr) (operation HcsOperation, hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsCreateOperationWithNotifications")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()

	return operation, execute(ctx, timeout.SyscallWatcher, func() error {
		var err error
		operation, err = hcsCreateOperationWithNotifications(eventTypes, callbackContext, callback)
		return err
	})
}

func HcsCloseOperation(ctx gcontext.Context, operation HcsOperation) {
	_, span := oc.StartSpan(ctx, "HcsCloseOperation")
	defer span.End()

	hcsCloseOperation(operation)
}

func HcsGetOperationContext(ctx gcontext.Context, operation HcsOperation) uintptr {
	_, span := oc.StartSpan(ctx, "HcsGetOperationContext")
	defer span.End()

	return hcsGetOperationContext(operation)
}

func HcsSetOperationContext(ctx gcontext.Context, operation HcsOperation, callbackContext uintptr) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsSetOperationContext")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsSetOperationContext(operation, callbackContext)
	})
}

func HcsGetComputeSystemFromOperation(ctx gcontext.Context, operation HcsOperation) HcsSystem {
	_, span := oc.StartSpan(ctx, "HcsGetComputeSystemFromOperation")
	defer span.End()

	return hcsGetComputeSystemFromOperation(operation)
}

func HcsGetProcessFromOperation(ctx gcontext.Context, operation HcsOperation) HcsProcess {
	_, span := oc.StartSpan(ctx, "HcsGetProcessFromOperation")
	defer span.End()

	return hcsGetProcessFromOperation(operation)
}

func HcsGetOperationType(ctx gcontext.Context, operation HcsOperation) int32 {
	_, span := oc.StartSpan(ctx, "HcsGetOperationType")
	defer span.End()

	return hcsGetOperationType(operation)
}

func HcsGetOperationID(ctx gcontext.Context, operation HcsOperation) uint64 {
	_, span := oc.StartSpan(ctx, "HcsGetOperationId")
	defer span.End()

	return hcsGetOperationId(operation)
}

func HcsGetOperationResult(ctx gcontext.Context, operation HcsOperation) (resultDocument string, hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsGetOperationResult")
	defer span.End()
	defer func() {
		if resultDocument != "" {
			span.AddAttributes(trace.StringAttribute("resultDocument", resultDocument))
		}
		oc.SetSpanStatus(span, hr)
	}()

	return resultDocument, execute(ctx, timeout.SyscallWatcher, func() error {
		var resultDocumentp *uint16
		err := hcsGetOperationResult(operation, &resultDocumentp)
		if resultDocumentp != nil {
			resultDocument = interop.ConvertAndFreeCoTaskMemString(resultDocumentp)
		}
		return err
	})
}

func HcsGetOperationResultAndProcessInfo(ctx gcontext.Context, operation HcsOperation) (processInformation HcsProcessInformation, resultDocument string, hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsGetOperationResultAndProcessInfo")
	defer span.End()
	defer func() {
		if resultDocument != "" {
			span.AddAttributes(trace.StringAttribute("resultDocument", resultDocument))
		}
		oc.SetSpanStatus(span, hr)
	}()

	return processInformation, resultDocument, execute(ctx, timeout.SyscallWatcher, func() error {
		var resultDocumentp *uint16
		err := hcsGetOperationResultAndProcessInfo(operation, &processInformation, &resultDocumentp)
		if resultDocumentp != nil {
			resultDocument = interop.ConvertAndFreeCoTaskMemString(resultDocumentp)
		}
		return err
	})
}

func HcsAddResourceToOperation(ctx gcontext.Context, operation HcsOperation, resourceType uint32, uri string, handle syscall.Handle) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsAddResourceToOperation")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.StringAttribute("uri", uri))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsAddResourceToOperation(operation, resourceType, uri, handle)
	})
}

func HcsGetProcessorCompatibilityFromSavedState(ctx gcontext.Context, runtimeFileName string) (processorFeaturesString string, hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsGetProcessorCompatibilityFromSavedState")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.StringAttribute("runtimeFileName", runtimeFileName))

	return processorFeaturesString, execute(ctx, timeout.SyscallWatcher, func() error {
		var processorFeaturesStringp *uint16
		err := hcsGetProcessorCompatibilityFromSavedState(runtimeFileName, &processorFeaturesStringp)
		if processorFeaturesStringp != nil {
			processorFeaturesString = interop.ConvertAndFreeCoTaskMemString(processorFeaturesStringp)
		}
		return err
	})
}

func HcsWaitForOperationResult(ctx gcontext.Context, operation HcsOperation, timeoutMs uint32) (resultDocument string, hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsWaitForOperationResult")
	defer span.End()
	defer func() {
		if resultDocument != "" {
			span.AddAttributes(trace.StringAttribute("resultDocument", resultDocument))
		}
		oc.SetSpanStatus(span, hr)
	}()

	return resultDocument, execute(ctx, timeout.SyscallWatcher, func() error {
		var resultDocumentp *uint16
		err := hcsWaitForOperationResult(operation, timeoutMs, &resultDocumentp)
		if resultDocumentp != nil {
			resultDocument = interop.ConvertAndFreeCoTaskMemString(resultDocumentp)
		}
		return err
	})
}

func HcsWaitForOperationResultAndProcessInfo(ctx gcontext.Context, operation HcsOperation, timeoutMs uint32) (processInformation HcsProcessInformation, resultDocument string, hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsWaitForOperationResultAndProcessInfo")
	defer span.End()
	defer func() {
		if resultDocument != "" {
			span.AddAttributes(trace.StringAttribute("resultDocument", resultDocument))
		}
		oc.SetSpanStatus(span, hr)
	}()

	return processInformation, resultDocument, execute(ctx, timeout.SyscallWatcher, func() error {
		var resultDocumentp *uint16
		err := hcsWaitForOperationResultAndProcessInfo(operation, timeoutMs, &processInformation, &resultDocumentp)
		if resultDocumentp != nil {
			resultDocument = interop.ConvertAndFreeCoTaskMemString(resultDocumentp)
		}
		return err
	})
}

func HcsSetOperationCallback(ctx gcontext.Context, operation HcsOperation, callbackContext uintptr, callback uintptr) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsSetOperationCallback")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsSetOperationCallback(operation, callbackContext, callback)
	})
}

func HcsCancelOperation(ctx gcontext.Context, operation HcsOperation) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsCancelOperation")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsCancelOperation(operation)
	})
}

func HcsGetOperationProperties(ctx gcontext.Context, operation HcsOperation, options string) (resultDocument string, hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsGetOperationProperties")
	defer span.End()
	defer func() {
		if resultDocument != "" {
			span.AddAttributes(trace.StringAttribute("resultDocument", resultDocument))
		}
		oc.SetSpanStatus(span, hr)
	}()
	span.AddAttributes(trace.StringAttribute("options", options))

	return resultDocument, execute(ctx, timeout.SyscallWatcher, func() error {
		var resultDocumentp *uint16
		err := hcsGetOperationProperties(operation, options, &resultDocumentp)
		if resultDocumentp != nil {
			resultDocument = interop.ConvertAndFreeCoTaskMemString(resultDocumentp)
		}
		return err
	})
}

// Compute system lifecycle

func HcsEnumerateComputeSystems(ctx gcontext.Context, query string, operation HcsOperation) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsEnumerateComputeSystems")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.StringAttribute("query", query))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsEnumerateComputeSystems(query, operation)
	})
}

func HcsEnumerateComputeSystemsInNamespace(ctx gcontext.Context, idNamespace string, query string, operation HcsOperation) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsEnumerateComputeSystemsInNamespace")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(
		trace.StringAttribute("idNamespace", idNamespace),
		trace.StringAttribute("query", query))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsEnumerateComputeSystemsInNamespace(idNamespace, query, operation)
	})
}

func HcsCreateComputeSystem(ctx gcontext.Context, id string, configuration string, operation HcsOperation, securityDescriptor unsafe.Pointer) (computeSystem HcsSystem, hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsCreateComputeSystem")
	defer span.End()
	defer func() {
		if hr != errVmcomputeOperationPending { //nolint:errorlint // explicitly returned
			oc.SetSpanStatus(span, hr)
		}
	}()
	span.AddAttributes(
		trace.StringAttribute("id", id),
		trace.StringAttribute("configuration", configuration))

	return computeSystem, execute(ctx, timeout.SystemCreate, func() error {
		return hcsCreateComputeSystem(id, configuration, operation, securityDescriptor, &computeSystem)
	})
}

func HcsCreateComputeSystemInNamespace(ctx gcontext.Context, idNamespace string, id string, configuration string, operation HcsOperation, options unsafe.Pointer) (computeSystem HcsSystem, hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsCreateComputeSystemInNamespace")
	defer span.End()
	defer func() {
		if hr != errVmcomputeOperationPending { //nolint:errorlint // explicitly returned
			oc.SetSpanStatus(span, hr)
		}
	}()
	span.AddAttributes(
		trace.StringAttribute("idNamespace", idNamespace),
		trace.StringAttribute("id", id),
		trace.StringAttribute("configuration", configuration))

	return computeSystem, execute(ctx, timeout.SystemCreate, func() error {
		return hcsCreateComputeSystemInNamespace(idNamespace, id, configuration, operation, options, &computeSystem)
	})
}

func HcsOpenComputeSystem(ctx gcontext.Context, id string, requestedAccess uint32) (computeSystem HcsSystem, hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsOpenComputeSystem")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()

	return computeSystem, execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsOpenComputeSystem(id, requestedAccess, &computeSystem)
	})
}

func HcsOpenComputeSystemInNamespace(ctx gcontext.Context, idNamespace string, id string, requestedAccess uint32) (computeSystem HcsSystem, hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsOpenComputeSystemInNamespace")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.StringAttribute("idNamespace", idNamespace))

	return computeSystem, execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsOpenComputeSystemInNamespace(idNamespace, id, requestedAccess, &computeSystem)
	})
}

func HcsCloseComputeSystem(ctx gcontext.Context, computeSystem HcsSystem) {
	_, span := oc.StartSpan(ctx, "HcsCloseComputeSystem")
	defer span.End()

	hcsCloseComputeSystem(computeSystem)
}

func HcsStartComputeSystem(ctx gcontext.Context, computeSystem HcsSystem, operation HcsOperation, options string) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsStartComputeSystem")
	defer span.End()
	defer func() {
		if hr != errVmcomputeOperationPending { //nolint:errorlint // explicitly returned
			oc.SetSpanStatus(span, hr)
		}
	}()
	span.AddAttributes(trace.StringAttribute("options", options))

	return execute(ctx, timeout.SystemStart, func() error {
		return hcsStartComputeSystem(computeSystem, operation, options)
	})
}

func HcsShutDownComputeSystem(ctx gcontext.Context, computeSystem HcsSystem, operation HcsOperation, options string) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsShutDownComputeSystem")
	defer span.End()
	defer func() {
		if hr != errVmcomputeOperationPending { //nolint:errorlint // explicitly returned
			oc.SetSpanStatus(span, hr)
		}
	}()
	span.AddAttributes(trace.StringAttribute("options", options))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsShutDownComputeSystem(computeSystem, operation, options)
	})
}

func HcsTerminateComputeSystem(ctx gcontext.Context, computeSystem HcsSystem, operation HcsOperation, options string) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsTerminateComputeSystem")
	defer span.End()
	defer func() {
		if hr != errVmcomputeOperationPending { //nolint:errorlint // explicitly returned
			oc.SetSpanStatus(span, hr)
		}
	}()
	span.AddAttributes(trace.StringAttribute("options", options))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsTerminateComputeSystem(computeSystem, operation, options)
	})
}

func HcsCrashComputeSystem(ctx gcontext.Context, computeSystem HcsSystem, operation HcsOperation, options string) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsCrashComputeSystem")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.StringAttribute("options", options))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsCrashComputeSystem(computeSystem, operation, options)
	})
}

func HcsPauseComputeSystem(ctx gcontext.Context, computeSystem HcsSystem, operation HcsOperation, options string) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsPauseComputeSystem")
	defer span.End()
	defer func() {
		if hr != errVmcomputeOperationPending { //nolint:errorlint // explicitly returned
			oc.SetSpanStatus(span, hr)
		}
	}()
	span.AddAttributes(trace.StringAttribute("options", options))

	return execute(ctx, timeout.SystemPause, func() error {
		return hcsPauseComputeSystem(computeSystem, operation, options)
	})
}

func HcsResumeComputeSystem(ctx gcontext.Context, computeSystem HcsSystem, operation HcsOperation, options string) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsResumeComputeSystem")
	defer span.End()
	defer func() {
		if hr != errVmcomputeOperationPending { //nolint:errorlint // explicitly returned
			oc.SetSpanStatus(span, hr)
		}
	}()
	span.AddAttributes(trace.StringAttribute("options", options))

	return execute(ctx, timeout.SystemResume, func() error {
		return hcsResumeComputeSystem(computeSystem, operation, options)
	})
}

func HcsSaveComputeSystem(ctx gcontext.Context, computeSystem HcsSystem, operation HcsOperation, options string) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsSaveComputeSystem")
	defer span.End()
	defer func() {
		if hr != errVmcomputeOperationPending { //nolint:errorlint // explicitly returned
			oc.SetSpanStatus(span, hr)
		}
	}()

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsSaveComputeSystem(computeSystem, operation, options)
	})
}

func HcsGetComputeSystemProperties(ctx gcontext.Context, computeSystem HcsSystem, operation HcsOperation, propertyQuery string) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsGetComputeSystemProperties")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.StringAttribute("propertyQuery", propertyQuery))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsGetComputeSystemProperties(computeSystem, operation, propertyQuery)
	})
}

func HcsModifyComputeSystem(ctx gcontext.Context, computeSystem HcsSystem, operation HcsOperation, configuration string, identity syscall.Handle) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsModifyComputeSystem")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.StringAttribute("configuration", configuration))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsModifyComputeSystem(computeSystem, operation, configuration, identity)
	})
}

func HcsWaitForComputeSystemExit(ctx gcontext.Context, computeSystem HcsSystem, timeoutMs uint32) (result string, hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsWaitForComputeSystemExit")
	defer span.End()
	defer func() {
		if result != "" {
			span.AddAttributes(trace.StringAttribute("result", result))
		}
		oc.SetSpanStatus(span, hr)
	}()

	return result, execute(ctx, timeout.SyscallWatcher, func() error {
		var resultp *uint16
		err := hcsWaitForComputeSystemExit(computeSystem, timeoutMs, &resultp)
		if resultp != nil {
			result = interop.ConvertAndFreeCoTaskMemString(resultp)
		}
		return err
	})
}

func HcsSetComputeSystemCallback(ctx gcontext.Context, computeSystem HcsSystem, callbackOptions uint32, callbackContext uintptr, callback uintptr) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsSetComputeSystemCallback")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsSetComputeSystemCallback(computeSystem, callbackOptions, callbackContext, callback)
	})
}

// Live migration

func HcsInitializeLiveMigrationOnSource(ctx gcontext.Context, computeSystem HcsSystem, operation HcsOperation, options string) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsInitializeLiveMigrationOnSource")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.StringAttribute("options", options))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsInitializeLiveMigrationOnSource(computeSystem, operation, options)
	})
}

func HcsStartLiveMigrationOnSource(ctx gcontext.Context, computeSystem HcsSystem, operation HcsOperation, options string) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsStartLiveMigrationOnSource")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.StringAttribute("options", options))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsStartLiveMigrationOnSource(computeSystem, operation, options)
	})
}

func HcsStartLiveMigrationTransfer(ctx gcontext.Context, computeSystem HcsSystem, operation HcsOperation, options string) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsStartLiveMigrationTransfer")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.StringAttribute("options", options))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsStartLiveMigrationTransfer(computeSystem, operation, options)
	})
}

func HcsFinalizeLiveMigration(ctx gcontext.Context, computeSystem HcsSystem, operation HcsOperation, options string) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsFinalizeLiveMigration")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.StringAttribute("options", options))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsFinalizeLiveMigration(computeSystem, operation, options)
	})
}

// Process lifecycle

func HcsCreateProcess(ctx gcontext.Context, computeSystem HcsSystem, processParameters string, operation HcsOperation, securityDescriptor unsafe.Pointer) (process HcsProcess, hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsCreateProcess")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	if span.IsRecordingEvents() {
		if s, err := log.ScrubProcessParameters(processParameters); err == nil {
			span.AddAttributes(trace.StringAttribute("processParameters", s))
		}
	}

	return process, execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsCreateProcess(computeSystem, processParameters, operation, securityDescriptor, &process)
	})
}

func HcsOpenProcess(ctx gcontext.Context, computeSystem HcsSystem, pid uint32, requestedAccess uint32) (process HcsProcess, hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsOpenProcess")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.Int64Attribute("pid", int64(pid)))

	return process, execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsOpenProcess(computeSystem, pid, requestedAccess, &process)
	})
}

func HcsCloseProcess(ctx gcontext.Context, process HcsProcess) {
	_, span := oc.StartSpan(ctx, "HcsCloseProcess")
	defer span.End()

	hcsCloseProcess(process)
}

func HcsTerminateProcess(ctx gcontext.Context, process HcsProcess, operation HcsOperation, options string) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsTerminateProcess")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsTerminateProcess(process, operation, options)
	})
}

func HcsSignalProcess(ctx gcontext.Context, process HcsProcess, operation HcsOperation, options string) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsSignalProcess")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.StringAttribute("options", options))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsSignalProcess(process, operation, options)
	})
}

func HcsGetProcessInfo(ctx gcontext.Context, process HcsProcess, operation HcsOperation) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsGetProcessInfo")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsGetProcessInfo(process, operation)
	})
}

func HcsGetProcessProperties(ctx gcontext.Context, process HcsProcess, operation HcsOperation, propertyQuery string) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsGetProcessProperties")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsGetProcessProperties(process, operation, propertyQuery)
	})
}

func HcsModifyProcess(ctx gcontext.Context, process HcsProcess, operation HcsOperation, settings string) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsModifyProcess")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.StringAttribute("settings", settings))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsModifyProcess(process, operation, settings)
	})
}

func HcsSetProcessCallback(ctx gcontext.Context, process HcsProcess, callbackOptions uint32, callbackContext uintptr, callback uintptr) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsSetProcessCallback")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsSetProcessCallback(process, callbackOptions, callbackContext, callback)
	})
}

func HcsWaitForProcessExit(ctx gcontext.Context, process HcsProcess, timeoutMs uint32) (result string, hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsWaitForProcessExit")
	defer span.End()
	defer func() {
		if result != "" {
			span.AddAttributes(trace.StringAttribute("result", result))
		}
		oc.SetSpanStatus(span, hr)
	}()

	return result, execute(ctx, timeout.SyscallWatcher, func() error {
		var resultp *uint16
		err := hcsWaitForProcessExit(process, timeoutMs, &resultp)
		if resultp != nil {
			result = interop.ConvertAndFreeCoTaskMemString(resultp)
		}
		return err
	})
}

// Service

func HcsGetServiceProperties(ctx gcontext.Context, propertyQuery string) (result string, hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsGetServiceProperties")
	defer span.End()
	defer func() {
		if result != "" {
			span.AddAttributes(trace.StringAttribute("result", result))
		}
		oc.SetSpanStatus(span, hr)
	}()
	span.AddAttributes(trace.StringAttribute("propertyQuery", propertyQuery))

	return result, execute(ctx, timeout.SyscallWatcher, func() error {
		var resultp *uint16
		err := hcsGetServiceProperties(propertyQuery, &resultp)
		if resultp != nil {
			result = interop.ConvertAndFreeCoTaskMemString(resultp)
		}
		return err
	})
}

func HcsModifyServiceSettings(ctx gcontext.Context, settings string) (result string, hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsModifyServiceSettings")
	defer span.End()
	defer func() {
		if result != "" {
			span.AddAttributes(trace.StringAttribute("result", result))
		}
		oc.SetSpanStatus(span, hr)
	}()
	span.AddAttributes(trace.StringAttribute("settings", settings))

	return result, execute(ctx, timeout.SyscallWatcher, func() error {
		var resultp *uint16
		err := hcsModifyServiceSettings(settings, &resultp)
		if resultp != nil {
			result = interop.ConvertAndFreeCoTaskMemString(resultp)
		}
		return err
	})
}

func HcsSubmitWerReport(ctx gcontext.Context, settings string) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsSubmitWerReport")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.StringAttribute("settings", settings))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsSubmitWerReport(settings)
	})
}

// File and VM access

func HcsCreateEmptyGuestStateFile(ctx gcontext.Context, guestStateFilePath string) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsCreateEmptyGuestStateFile")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.StringAttribute("guestStateFilePath", guestStateFilePath))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsCreateEmptyGuestStateFile(guestStateFilePath)
	})
}

func HcsCreateEmptyRuntimeStateFile(ctx gcontext.Context, runtimeStateFilePath string) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsCreateEmptyRuntimeStateFile")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.StringAttribute("runtimeStateFilePath", runtimeStateFilePath))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsCreateEmptyRuntimeStateFile(runtimeStateFilePath)
	})
}

func HcsGrantVMAccess(ctx gcontext.Context, vmID string, filePath string) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsGrantVmAccess")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(
		trace.StringAttribute("vmID", vmID),
		trace.StringAttribute("filePath", filePath))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsGrantVmAccess(vmID, filePath)
	})
}

func HcsRevokeVMAccess(ctx gcontext.Context, vmID string, filePath string) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsRevokeVmAccess")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(
		trace.StringAttribute("vmID", vmID),
		trace.StringAttribute("filePath", filePath))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsRevokeVmAccess(vmID, filePath)
	})
}

func HcsGrantVMGroupAccess(ctx gcontext.Context, filePath string) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsGrantVmGroupAccess")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.StringAttribute("filePath", filePath))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsGrantVmGroupAccess(filePath)
	})
}

func HcsRevokeVMGroupAccess(ctx gcontext.Context, filePath string) (hr error) {
	ctx, span := oc.StartSpan(ctx, "HcsRevokeVmGroupAccess")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.StringAttribute("filePath", filePath))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsRevokeVmGroupAccess(filePath)
	})
}
