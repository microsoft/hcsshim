// Windows Syscall layer for computecore.dll introduced in Windows RS5 1809 for
// managing containers via the HCS (Host Compute Service)

package computecore

import (
	"syscall"
)

//go:generate go run ../../mksyscall_windows.go -output zsyscall_windows.go computecore.go

//sys hcsEnumerateComputeSystems(query string, operation hcsOperation) (hr error) = computecore.HcsEnumerateComputeSystems?
//sys hcsCreateOperation(context uintptr, callback hcsOperationCompletion) (operation hcsOperation) = computecore.HcsCreateOperation?
//sys hcsCloseOperation(operation hcsOperation) = computecore.HcsCloseOperation?
//sys hcsGetOperationContext(operation hcsOperation) (context uintptr) = computecore.HcsGetOperationContext?
//sys hcsSetOperationContext(operation hcsOperation, context uintptr) (hr error) = computecore.HcsSetOperationContext?
//sys hcsGetComputeSystemFromOperation(operation hcsOperation) (computeSystem hcsSystem) = computecore.HcsGetComputeSystemFromOperation?
//sys hcsGetProcessFromOperation(operation hcsOperation) (process hcsProcess) = computecore.HcsGetProcessFromOperation?
//sys hcsGetOperationType(operation *hcsOperation) (operationType hcsOperationType) = computecore.HcsGetOperationType?
//sys hcsGetOperationId(operation hcsOperation) (operationId uint64) = computecore.HcsGetOperationId?
//sys hcsGetOperationResult(operation hcsOperation, resultDocument **uint16) (hr error) = computecore.HcsGetOperationResult?
//sys hcsGetOperationResultAndProcessInfo(operation hcsOperation, processInformation *hcsProcessInformation, resultDocument **uint16) (hr error) = computecore.HcsGetOperationResultAndProcessInfo?
//sys hcsWaitForOperationResult(operation hcsOperation, timeoutMs uint32, resultDocument **uint16) (hr error) = computecore.HcsWaitForOperationResult?
//sys hcsWaitForOperationResultAndProcessInfo(operation hcsOperation, timeoutMs uint32, processInformation *hcsProcessInformation, resultDocument **uint16) (hr error) = computecore.HcsWaitForOperationResultAndProcessInfo?
//sys hcsSetOperationCallback(operation hcsOperation, context uintptr, callback hcsOperationCompletion) (hr error) = computecore.HcsSetOperationCallback?
//sys hcsCancelOperation(operation hcsOperation) (hr error) = computecore.HcsCancelOperation?
//sys hcsCreateComputeSystem(id string, configuration string, operation hcsOperation, securityDescriptor uintptr, computeSystem *hcsSystem) (hr error) = computecore.HcsCreateComputeSystem?
//sys hcsOpenComputeSystem(id string, requestedAccess uint32, computeSystem *hcsSystem) (hr error) = computecore.HcsOpenComputeSystem?
//sys hcsCloseComputeSystem(computeSystem hcsSystem) (hr error) = computecore.HcsCloseComputeSystem?
//sys hcsStartComputeSystem(computeSystem hcsSystem, operation hcsOperation, options string) (hr error) = computecore.HcsStartComputeSystem?
//sys hcsShutDownComputeSystem(computeSystem hcsSystem, operation hcsOperation, options string) (hr error) = computecore.HcsShutDownComputeSystem?
//sys hcsTerminateComputeSystem(computeSystem hcsSystem, operation hcsOperation, options string) (hr error) = computecore.HcsTerminateComputeSystem?
//sys hcsPauseComputeSystem(computeSystem hcsSystem, operation hcsOperation, options string) (hr error) = computecore.HcsPauseComputeSystem?
//sys hcsResumeComputeSystem(computeSystem hcsSystem, operation hcsOperation, options string) (hr error) = computecore.HcsResumeComputeSystem?
//sys hcsSaveComputeSystem(computeSystem hcsSystem, operation hcsOperation, options string) (hr error) = computecore.HcsSaveComputeSystem?
//sys hcsGetComputeSystemProperties(computeSystem hcsSystem, operation hcsOperation, propertyQuery string) (hr error) = computecore.HcsGetComputeSystemProperties?
//sys hcsModifyComputeSystem(computeSystem hcsSystem, operation hcsOperation, configuration string, identity syscall.Handle) (hr error) = computecore.HcsModifyComputeSystem?
//sys hcsSetComputeSystemCallback(computeSystem hcsSystem, callbackOptions hcsEventOptions, context uintptr, callback hcsEventCallback) (hr error) = computecore.HcsSetComputeSystemCallback?
//sys hcsCreateProcess(computeSystem hcsSystem, processParameters string, operation hcsOperation, securityDescriptor uintptr, process *hcsProcess) (hr error) = computecore.HcsCreateProcess?
//sys hcsOpenProcess(computeSystem hcsSystem, processId uint32, requestedAccess uint32, process *hcsProcess) (hr error) = computecore.HcsOpenProcess?
//sys hcsCloseProcess(process hcsProcess) (hr error) = computecore.HcsCloseProcess?
//sys hcsTerminateProcess(process hcsProcess, operation hcsOperation, options string) (hr error) = computecore.HcsTerminateProcess?
//sys hcsSignalProcess(process hcsProcess, operation hcsOperation, options string) (hr error) = computecore.HcsSignalProcess?
//sys hcsGetProcessInfo(process hcsProcess, operation hcsOperation) (hr error) = computecore.HcsGetProcessInfo?
//sys hcsGetProcessProperties(process hcsProcess, operation hcsOperation, propertyQuery string) (hr error) = computecore.HcsGetProcessProperties?
//sys hcsModifyProcess(process hcsProcess, operation hcsOperation, settings string) (hr error) = computecore.HcsModifyProcess?
//sys hcsSetProcessCallback(process hcsProcess, callbackOptions hcsEventOptions, context uintptr, callback hcsEventCallback) (hr error) = computecore.HcsSetProcessCallback?
//sys hcsGetServiceProperties(propertyQuery string, result **uint16) (hr error) = computecore.HcsGetServiceProperties?
//sys hcsModifyServiceSettings(settings string, result **uint16) (hr error) = computecore.HcsModifyServiceSettings?
//sys hcsSubmitWerReport(settings string) (hr error) = computecore.HcsSubmitWerReport?
//sys hcsCreateEmptyGuestStateFile(guestStateFilePath string) (hr error) = computecore.HcsCreateEmptyGuestStateFile?
//sys hcsCreateEmptyRuntimeStateFile(runtimeStateFilePath string) (hr error) = computecore.HcsCreateEmptyRuntimeStateFile?
//sys hcsGrantVmAccess(vmId string, filePath string) (hr error) = computecore.HcsGrantVmAccess?
//sys hcsRevokeVmAccess(vmId string, filePath string) (hr error) = computecore.HcsRevokeVmAccess?

// Handle to a compute system
type hcsSystem syscall.Handle

// Handle to a process running in a compute system
type hcsProcess syscall.Handle

// Handle to an operation on a compute system
type hcsOperation syscall.Handle

// Type of an operation. These correspond to the functions that invoke the operation.
type hcsOperationType int32

const (
	hcsOperationTypeNone                 hcsOperationType = -1
	hcsOperationTypeEnumerate                             = 0
	hcsOperationTypeCreate                                = 1
	hcsOperationTypeStart                                 = 2
	hcsOperationTypeShutdown                              = 3
	hcsOperationTypePause                                 = 4
	hcsOperationTypeResume                                = 5
	hcsOperationTypeSave                                  = 6
	hcsOperationTypeTerminate                             = 7
	hcsOperationTypeModify                                = 8
	hcsOperationTypeGetProperties                         = 9
	hcsOperationTypeCreateProcess                         = 10
	hcsOperationTypeSignalProcess                         = 11
	hcsOperationTypeGetProcessInfo                        = 12
	hcsOperationTypeGetProcessProperties                  = 13
	hcsOperationTypeModifyProcess                         = 14
)

// hcsOperationCompletion is a callback `(operation hcsOperation, context uintptr)`.
type hcsOperationCompletion syscall.Handle

// Events indicated to callbacks registered by HcsRegisterComputeSystemCallback or
// HcsRegisterProcessCallback (since Windows 1809).
type hcsEventType int32

const (
	hcsEventTypeInvalid hcsEventType = 0x00000000

	// Events for HCS_SYSTEM handles

	hcsEventTypeSystemExited                      = 0x00000001
	hcsEventTypeSystemCrashInitiated              = 0x00000002
	hcsEventTypeSystemCrashReport                 = 0x00000003
	hcsEventTypeSystemRdpEnhancedModeStateChanged = 0x00000004
	hcsEventTypeSystemSiloJobCreated              = 0x00000005
	hcsEventTypeSystemGuestConnectionClosed       = 0x00000006

	// Events for HCS_PROCESS handles

	hcsEventTypeProcessExited = 0x00010000

	// Common Events

	hcsEventTypeOperationCallback = 0x01000000
	hcsEventTypeServiceDisconnect = 0x02000000
)

// Provides information about an event that occurred on a compute system or process.
type hcsEvent struct {
	// Type of Event
	Type hcsEventType

	// EventData provides additional data for the event.
	EventData string

	// Operation is a Handle to a completed operation (if Type is HcsEventOperationCallback).
	Operation hcsOperation
}

// Options for an event callback registration
type hcsEventOptions int32

const (
	hcsEventOptionNone                     hcsEventOptions = 0x00000000
	hcsEventOptionEnableOperationCallbacks                 = 0x00000001
)

// hcsEventCallback is a callback `(event *hcsEvent, context uintptr)`.
type hcsEventCallback syscall.Handle

// Struct containing information about a process created by HcsStartProcessInComputeSystem
type hcsProcessInformation struct {
	ProcessId uint32 // Identifier of the created process
	Reserved  uint32
	StdInput  syscall.Handle // If created, standard input handle of the process
	StdOutput syscall.Handle // If created, standard output handle of the process
	StdError  syscall.Handle // If created, standard error handle of the process
}
