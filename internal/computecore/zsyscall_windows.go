// Code generated mksyscall_windows.exe DO NOT EDIT

package computecore

import (
	"syscall"
	"unsafe"

	"github.com/Microsoft/hcsshim/internal/interop"
	"golang.org/x/sys/windows"
)

var _ unsafe.Pointer

// Do the interface allocations only once for common
// Errno values.
const (
	errnoERROR_IO_PENDING = 997
)

var (
	errERROR_IO_PENDING error = syscall.Errno(errnoERROR_IO_PENDING)
)

// errnoErr returns common boxed Errno values, to prevent
// allocations at runtime.
func errnoErr(e syscall.Errno) error {
	switch e {
	case 0:
		return nil
	case errnoERROR_IO_PENDING:
		return errERROR_IO_PENDING
	}
	// TODO: add more here, after collecting data on the common
	// error values see on Windows. (perhaps when running
	// all.bat?)
	return e
}

var (
	modcomputecore = windows.NewLazySystemDLL("computecore.dll")

	procHcsEnumerateComputeSystems              = modcomputecore.NewProc("HcsEnumerateComputeSystems")
	procHcsCreateOperation                      = modcomputecore.NewProc("HcsCreateOperation")
	procHcsCloseOperation                       = modcomputecore.NewProc("HcsCloseOperation")
	procHcsGetOperationContext                  = modcomputecore.NewProc("HcsGetOperationContext")
	procHcsSetOperationContext                  = modcomputecore.NewProc("HcsSetOperationContext")
	procHcsGetComputeSystemFromOperation        = modcomputecore.NewProc("HcsGetComputeSystemFromOperation")
	procHcsGetProcessFromOperation              = modcomputecore.NewProc("HcsGetProcessFromOperation")
	procHcsGetOperationType                     = modcomputecore.NewProc("HcsGetOperationType")
	procHcsGetOperationId                       = modcomputecore.NewProc("HcsGetOperationId")
	procHcsGetOperationResult                   = modcomputecore.NewProc("HcsGetOperationResult")
	procHcsGetOperationResultAndProcessInfo     = modcomputecore.NewProc("HcsGetOperationResultAndProcessInfo")
	procHcsWaitForOperationResult               = modcomputecore.NewProc("HcsWaitForOperationResult")
	procHcsWaitForOperationResultAndProcessInfo = modcomputecore.NewProc("HcsWaitForOperationResultAndProcessInfo")
	procHcsSetOperationCallback                 = modcomputecore.NewProc("HcsSetOperationCallback")
	procHcsCancelOperation                      = modcomputecore.NewProc("HcsCancelOperation")
	procHcsCreateComputeSystem                  = modcomputecore.NewProc("HcsCreateComputeSystem")
	procHcsOpenComputeSystem                    = modcomputecore.NewProc("HcsOpenComputeSystem")
	procHcsCloseComputeSystem                   = modcomputecore.NewProc("HcsCloseComputeSystem")
	procHcsStartComputeSystem                   = modcomputecore.NewProc("HcsStartComputeSystem")
	procHcsShutDownComputeSystem                = modcomputecore.NewProc("HcsShutDownComputeSystem")
	procHcsTerminateComputeSystem               = modcomputecore.NewProc("HcsTerminateComputeSystem")
	procHcsPauseComputeSystem                   = modcomputecore.NewProc("HcsPauseComputeSystem")
	procHcsResumeComputeSystem                  = modcomputecore.NewProc("HcsResumeComputeSystem")
	procHcsSaveComputeSystem                    = modcomputecore.NewProc("HcsSaveComputeSystem")
	procHcsGetComputeSystemProperties           = modcomputecore.NewProc("HcsGetComputeSystemProperties")
	procHcsModifyComputeSystem                  = modcomputecore.NewProc("HcsModifyComputeSystem")
	procHcsSetComputeSystemCallback             = modcomputecore.NewProc("HcsSetComputeSystemCallback")
	procHcsCreateProcess                        = modcomputecore.NewProc("HcsCreateProcess")
	procHcsOpenProcess                          = modcomputecore.NewProc("HcsOpenProcess")
	procHcsCloseProcess                         = modcomputecore.NewProc("HcsCloseProcess")
	procHcsTerminateProcess                     = modcomputecore.NewProc("HcsTerminateProcess")
	procHcsSignalProcess                        = modcomputecore.NewProc("HcsSignalProcess")
	procHcsGetProcessInfo                       = modcomputecore.NewProc("HcsGetProcessInfo")
	procHcsGetProcessProperties                 = modcomputecore.NewProc("HcsGetProcessProperties")
	procHcsModifyProcess                        = modcomputecore.NewProc("HcsModifyProcess")
	procHcsSetProcessCallback                   = modcomputecore.NewProc("HcsSetProcessCallback")
	procHcsGetServiceProperties                 = modcomputecore.NewProc("HcsGetServiceProperties")
	procHcsModifyServiceSettings                = modcomputecore.NewProc("HcsModifyServiceSettings")
	procHcsSubmitWerReport                      = modcomputecore.NewProc("HcsSubmitWerReport")
	procHcsCreateEmptyGuestStateFile            = modcomputecore.NewProc("HcsCreateEmptyGuestStateFile")
	procHcsCreateEmptyRuntimeStateFile          = modcomputecore.NewProc("HcsCreateEmptyRuntimeStateFile")
	procHcsGrantVmAccess                        = modcomputecore.NewProc("HcsGrantVmAccess")
	procHcsRevokeVmAccess                       = modcomputecore.NewProc("HcsRevokeVmAccess")
)

func hcsEnumerateComputeSystems(query string, operation hcsOperation) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(query)
	if hr != nil {
		return
	}
	return _hcsEnumerateComputeSystems(_p0, operation)
}

func _hcsEnumerateComputeSystems(query *uint16, operation hcsOperation) (hr error) {
	if hr = procHcsEnumerateComputeSystems.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsEnumerateComputeSystems.Addr(), 2, uintptr(unsafe.Pointer(query)), uintptr(operation), 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsCreateOperation(context uintptr, callback hcsOperationCompletion) (operation hcsOperation) {
	if err := procHcsCreateOperation.Find(); err != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsCreateOperation.Addr(), 2, uintptr(context), uintptr(callback), 0)
	operation = hcsOperation(r0)
	return
}

func hcsCloseOperation(operation hcsOperation) {
	if err := procHcsCloseOperation.Find(); err != nil {
		return
	}
	syscall.Syscall(procHcsCloseOperation.Addr(), 1, uintptr(operation), 0, 0)
	return
}

func hcsGetOperationContext(operation hcsOperation) (context uintptr) {
	if err := procHcsGetOperationContext.Find(); err != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsGetOperationContext.Addr(), 1, uintptr(operation), 0, 0)
	context = uintptr(r0)
	return
}

func hcsSetOperationContext(operation hcsOperation, context uintptr) (hr error) {
	if hr = procHcsSetOperationContext.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsSetOperationContext.Addr(), 2, uintptr(operation), uintptr(context), 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsGetComputeSystemFromOperation(operation hcsOperation) (computeSystem hcsSystem) {
	if err := procHcsGetComputeSystemFromOperation.Find(); err != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsGetComputeSystemFromOperation.Addr(), 1, uintptr(operation), 0, 0)
	computeSystem = hcsSystem(r0)
	return
}

func hcsGetProcessFromOperation(operation hcsOperation) (process hcsProcess) {
	if err := procHcsGetProcessFromOperation.Find(); err != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsGetProcessFromOperation.Addr(), 1, uintptr(operation), 0, 0)
	process = hcsProcess(r0)
	return
}

func hcsGetOperationType(operation *hcsOperation) (operationType hcsOperationType) {
	if err := procHcsGetOperationType.Find(); err != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsGetOperationType.Addr(), 1, uintptr(unsafe.Pointer(operation)), 0, 0)
	operationType = hcsOperationType(r0)
	return
}

func hcsGetOperationId(operation hcsOperation) (operationId uint64) {
	if err := procHcsGetOperationId.Find(); err != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsGetOperationId.Addr(), 1, uintptr(operation), 0, 0)
	operationId = uint64(r0)
	return
}

func hcsGetOperationResult(operation hcsOperation, resultDocument **uint16) (hr error) {
	if hr = procHcsGetOperationResult.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsGetOperationResult.Addr(), 2, uintptr(operation), uintptr(unsafe.Pointer(resultDocument)), 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsGetOperationResultAndProcessInfo(operation hcsOperation, processInformation *hcsProcessInformation, resultDocument **uint16) (hr error) {
	if hr = procHcsGetOperationResultAndProcessInfo.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsGetOperationResultAndProcessInfo.Addr(), 3, uintptr(operation), uintptr(unsafe.Pointer(processInformation)), uintptr(unsafe.Pointer(resultDocument)))
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsWaitForOperationResult(operation hcsOperation, timeoutMs uint32, resultDocument **uint16) (hr error) {
	if hr = procHcsWaitForOperationResult.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsWaitForOperationResult.Addr(), 3, uintptr(operation), uintptr(timeoutMs), uintptr(unsafe.Pointer(resultDocument)))
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsWaitForOperationResultAndProcessInfo(operation hcsOperation, timeoutMs uint32, processInformation *hcsProcessInformation, resultDocument **uint16) (hr error) {
	if hr = procHcsWaitForOperationResultAndProcessInfo.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall6(procHcsWaitForOperationResultAndProcessInfo.Addr(), 4, uintptr(operation), uintptr(timeoutMs), uintptr(unsafe.Pointer(processInformation)), uintptr(unsafe.Pointer(resultDocument)), 0, 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsSetOperationCallback(operation hcsOperation, context uintptr, callback hcsOperationCompletion) (hr error) {
	if hr = procHcsSetOperationCallback.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsSetOperationCallback.Addr(), 3, uintptr(operation), uintptr(context), uintptr(callback))
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsCancelOperation(operation hcsOperation) (hr error) {
	if hr = procHcsCancelOperation.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsCancelOperation.Addr(), 1, uintptr(operation), 0, 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsCreateComputeSystem(id string, configuration string, operation hcsOperation, securityDescriptor uintptr, computeSystem *hcsSystem) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(id)
	if hr != nil {
		return
	}
	var _p1 *uint16
	_p1, hr = syscall.UTF16PtrFromString(configuration)
	if hr != nil {
		return
	}
	return _hcsCreateComputeSystem(_p0, _p1, operation, securityDescriptor, computeSystem)
}

func _hcsCreateComputeSystem(id *uint16, configuration *uint16, operation hcsOperation, securityDescriptor uintptr, computeSystem *hcsSystem) (hr error) {
	if hr = procHcsCreateComputeSystem.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall6(procHcsCreateComputeSystem.Addr(), 5, uintptr(unsafe.Pointer(id)), uintptr(unsafe.Pointer(configuration)), uintptr(operation), uintptr(securityDescriptor), uintptr(unsafe.Pointer(computeSystem)), 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsOpenComputeSystem(id string, requestedAccess uint32, computeSystem *hcsSystem) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(id)
	if hr != nil {
		return
	}
	return _hcsOpenComputeSystem(_p0, requestedAccess, computeSystem)
}

func _hcsOpenComputeSystem(id *uint16, requestedAccess uint32, computeSystem *hcsSystem) (hr error) {
	if hr = procHcsOpenComputeSystem.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsOpenComputeSystem.Addr(), 3, uintptr(unsafe.Pointer(id)), uintptr(requestedAccess), uintptr(unsafe.Pointer(computeSystem)))
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsCloseComputeSystem(computeSystem hcsSystem) (hr error) {
	if hr = procHcsCloseComputeSystem.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsCloseComputeSystem.Addr(), 1, uintptr(computeSystem), 0, 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsStartComputeSystem(computeSystem hcsSystem, operation hcsOperation, options string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(options)
	if hr != nil {
		return
	}
	return _hcsStartComputeSystem(computeSystem, operation, _p0)
}

func _hcsStartComputeSystem(computeSystem hcsSystem, operation hcsOperation, options *uint16) (hr error) {
	if hr = procHcsStartComputeSystem.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsStartComputeSystem.Addr(), 3, uintptr(computeSystem), uintptr(operation), uintptr(unsafe.Pointer(options)))
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsShutDownComputeSystem(computeSystem hcsSystem, operation hcsOperation, options string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(options)
	if hr != nil {
		return
	}
	return _hcsShutDownComputeSystem(computeSystem, operation, _p0)
}

func _hcsShutDownComputeSystem(computeSystem hcsSystem, operation hcsOperation, options *uint16) (hr error) {
	if hr = procHcsShutDownComputeSystem.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsShutDownComputeSystem.Addr(), 3, uintptr(computeSystem), uintptr(operation), uintptr(unsafe.Pointer(options)))
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsTerminateComputeSystem(computeSystem hcsSystem, operation hcsOperation, options string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(options)
	if hr != nil {
		return
	}
	return _hcsTerminateComputeSystem(computeSystem, operation, _p0)
}

func _hcsTerminateComputeSystem(computeSystem hcsSystem, operation hcsOperation, options *uint16) (hr error) {
	if hr = procHcsTerminateComputeSystem.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsTerminateComputeSystem.Addr(), 3, uintptr(computeSystem), uintptr(operation), uintptr(unsafe.Pointer(options)))
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsPauseComputeSystem(computeSystem hcsSystem, operation hcsOperation, options string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(options)
	if hr != nil {
		return
	}
	return _hcsPauseComputeSystem(computeSystem, operation, _p0)
}

func _hcsPauseComputeSystem(computeSystem hcsSystem, operation hcsOperation, options *uint16) (hr error) {
	if hr = procHcsPauseComputeSystem.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsPauseComputeSystem.Addr(), 3, uintptr(computeSystem), uintptr(operation), uintptr(unsafe.Pointer(options)))
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsResumeComputeSystem(computeSystem hcsSystem, operation hcsOperation, options string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(options)
	if hr != nil {
		return
	}
	return _hcsResumeComputeSystem(computeSystem, operation, _p0)
}

func _hcsResumeComputeSystem(computeSystem hcsSystem, operation hcsOperation, options *uint16) (hr error) {
	if hr = procHcsResumeComputeSystem.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsResumeComputeSystem.Addr(), 3, uintptr(computeSystem), uintptr(operation), uintptr(unsafe.Pointer(options)))
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsSaveComputeSystem(computeSystem hcsSystem, operation hcsOperation, options string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(options)
	if hr != nil {
		return
	}
	return _hcsSaveComputeSystem(computeSystem, operation, _p0)
}

func _hcsSaveComputeSystem(computeSystem hcsSystem, operation hcsOperation, options *uint16) (hr error) {
	if hr = procHcsSaveComputeSystem.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsSaveComputeSystem.Addr(), 3, uintptr(computeSystem), uintptr(operation), uintptr(unsafe.Pointer(options)))
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsGetComputeSystemProperties(computeSystem hcsSystem, operation hcsOperation, propertyQuery string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(propertyQuery)
	if hr != nil {
		return
	}
	return _hcsGetComputeSystemProperties(computeSystem, operation, _p0)
}

func _hcsGetComputeSystemProperties(computeSystem hcsSystem, operation hcsOperation, propertyQuery *uint16) (hr error) {
	if hr = procHcsGetComputeSystemProperties.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsGetComputeSystemProperties.Addr(), 3, uintptr(computeSystem), uintptr(operation), uintptr(unsafe.Pointer(propertyQuery)))
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsModifyComputeSystem(computeSystem hcsSystem, operation hcsOperation, configuration string, identity syscall.Handle) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(configuration)
	if hr != nil {
		return
	}
	return _hcsModifyComputeSystem(computeSystem, operation, _p0, identity)
}

func _hcsModifyComputeSystem(computeSystem hcsSystem, operation hcsOperation, configuration *uint16, identity syscall.Handle) (hr error) {
	if hr = procHcsModifyComputeSystem.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall6(procHcsModifyComputeSystem.Addr(), 4, uintptr(computeSystem), uintptr(operation), uintptr(unsafe.Pointer(configuration)), uintptr(identity), 0, 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsSetComputeSystemCallback(computeSystem hcsSystem, callbackOptions hcsEventOptions, context uintptr, callback hcsEventCallback) (hr error) {
	if hr = procHcsSetComputeSystemCallback.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall6(procHcsSetComputeSystemCallback.Addr(), 4, uintptr(computeSystem), uintptr(callbackOptions), uintptr(context), uintptr(callback), 0, 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsCreateProcess(computeSystem hcsSystem, processParameters string, operation hcsOperation, securityDescriptor uintptr, process *hcsProcess) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(processParameters)
	if hr != nil {
		return
	}
	return _hcsCreateProcess(computeSystem, _p0, operation, securityDescriptor, process)
}

func _hcsCreateProcess(computeSystem hcsSystem, processParameters *uint16, operation hcsOperation, securityDescriptor uintptr, process *hcsProcess) (hr error) {
	if hr = procHcsCreateProcess.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall6(procHcsCreateProcess.Addr(), 5, uintptr(computeSystem), uintptr(unsafe.Pointer(processParameters)), uintptr(operation), uintptr(securityDescriptor), uintptr(unsafe.Pointer(process)), 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsOpenProcess(computeSystem hcsSystem, processId uint32, requestedAccess uint32, process *hcsProcess) (hr error) {
	if hr = procHcsOpenProcess.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall6(procHcsOpenProcess.Addr(), 4, uintptr(computeSystem), uintptr(processId), uintptr(requestedAccess), uintptr(unsafe.Pointer(process)), 0, 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsCloseProcess(process hcsProcess) (hr error) {
	if hr = procHcsCloseProcess.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsCloseProcess.Addr(), 1, uintptr(process), 0, 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsTerminateProcess(process hcsProcess, operation hcsOperation, options string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(options)
	if hr != nil {
		return
	}
	return _hcsTerminateProcess(process, operation, _p0)
}

func _hcsTerminateProcess(process hcsProcess, operation hcsOperation, options *uint16) (hr error) {
	if hr = procHcsTerminateProcess.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsTerminateProcess.Addr(), 3, uintptr(process), uintptr(operation), uintptr(unsafe.Pointer(options)))
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsSignalProcess(process hcsProcess, operation hcsOperation, options string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(options)
	if hr != nil {
		return
	}
	return _hcsSignalProcess(process, operation, _p0)
}

func _hcsSignalProcess(process hcsProcess, operation hcsOperation, options *uint16) (hr error) {
	if hr = procHcsSignalProcess.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsSignalProcess.Addr(), 3, uintptr(process), uintptr(operation), uintptr(unsafe.Pointer(options)))
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsGetProcessInfo(process hcsProcess, operation hcsOperation) (hr error) {
	if hr = procHcsGetProcessInfo.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsGetProcessInfo.Addr(), 2, uintptr(process), uintptr(operation), 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsGetProcessProperties(process hcsProcess, operation hcsOperation, propertyQuery string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(propertyQuery)
	if hr != nil {
		return
	}
	return _hcsGetProcessProperties(process, operation, _p0)
}

func _hcsGetProcessProperties(process hcsProcess, operation hcsOperation, propertyQuery *uint16) (hr error) {
	if hr = procHcsGetProcessProperties.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsGetProcessProperties.Addr(), 3, uintptr(process), uintptr(operation), uintptr(unsafe.Pointer(propertyQuery)))
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsModifyProcess(process hcsProcess, operation hcsOperation, settings string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(settings)
	if hr != nil {
		return
	}
	return _hcsModifyProcess(process, operation, _p0)
}

func _hcsModifyProcess(process hcsProcess, operation hcsOperation, settings *uint16) (hr error) {
	if hr = procHcsModifyProcess.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsModifyProcess.Addr(), 3, uintptr(process), uintptr(operation), uintptr(unsafe.Pointer(settings)))
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsSetProcessCallback(process hcsProcess, callbackOptions hcsEventOptions, context uintptr, callback hcsEventCallback) (hr error) {
	if hr = procHcsSetProcessCallback.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall6(procHcsSetProcessCallback.Addr(), 4, uintptr(process), uintptr(callbackOptions), uintptr(context), uintptr(callback), 0, 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsGetServiceProperties(propertyQuery string, result **uint16) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(propertyQuery)
	if hr != nil {
		return
	}
	return _hcsGetServiceProperties(_p0, result)
}

func _hcsGetServiceProperties(propertyQuery *uint16, result **uint16) (hr error) {
	if hr = procHcsGetServiceProperties.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsGetServiceProperties.Addr(), 2, uintptr(unsafe.Pointer(propertyQuery)), uintptr(unsafe.Pointer(result)), 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsModifyServiceSettings(settings string, result **uint16) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(settings)
	if hr != nil {
		return
	}
	return _hcsModifyServiceSettings(_p0, result)
}

func _hcsModifyServiceSettings(settings *uint16, result **uint16) (hr error) {
	if hr = procHcsModifyServiceSettings.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsModifyServiceSettings.Addr(), 2, uintptr(unsafe.Pointer(settings)), uintptr(unsafe.Pointer(result)), 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsSubmitWerReport(settings string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(settings)
	if hr != nil {
		return
	}
	return _hcsSubmitWerReport(_p0)
}

func _hcsSubmitWerReport(settings *uint16) (hr error) {
	if hr = procHcsSubmitWerReport.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsSubmitWerReport.Addr(), 1, uintptr(unsafe.Pointer(settings)), 0, 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsCreateEmptyGuestStateFile(guestStateFilePath string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(guestStateFilePath)
	if hr != nil {
		return
	}
	return _hcsCreateEmptyGuestStateFile(_p0)
}

func _hcsCreateEmptyGuestStateFile(guestStateFilePath *uint16) (hr error) {
	if hr = procHcsCreateEmptyGuestStateFile.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsCreateEmptyGuestStateFile.Addr(), 1, uintptr(unsafe.Pointer(guestStateFilePath)), 0, 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsCreateEmptyRuntimeStateFile(runtimeStateFilePath string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(runtimeStateFilePath)
	if hr != nil {
		return
	}
	return _hcsCreateEmptyRuntimeStateFile(_p0)
}

func _hcsCreateEmptyRuntimeStateFile(runtimeStateFilePath *uint16) (hr error) {
	if hr = procHcsCreateEmptyRuntimeStateFile.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsCreateEmptyRuntimeStateFile.Addr(), 1, uintptr(unsafe.Pointer(runtimeStateFilePath)), 0, 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsGrantVmAccess(vmId string, filePath string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(vmId)
	if hr != nil {
		return
	}
	var _p1 *uint16
	_p1, hr = syscall.UTF16PtrFromString(filePath)
	if hr != nil {
		return
	}
	return _hcsGrantVmAccess(_p0, _p1)
}

func _hcsGrantVmAccess(vmId *uint16, filePath *uint16) (hr error) {
	if hr = procHcsGrantVmAccess.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsGrantVmAccess.Addr(), 2, uintptr(unsafe.Pointer(vmId)), uintptr(unsafe.Pointer(filePath)), 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}

func hcsRevokeVmAccess(vmId string, filePath string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(vmId)
	if hr != nil {
		return
	}
	var _p1 *uint16
	_p1, hr = syscall.UTF16PtrFromString(filePath)
	if hr != nil {
		return
	}
	return _hcsRevokeVmAccess(_p0, _p1)
}

func _hcsRevokeVmAccess(vmId *uint16, filePath *uint16) (hr error) {
	if hr = procHcsRevokeVmAccess.Find(); hr != nil {
		return
	}
	r0, _, _ := syscall.Syscall(procHcsRevokeVmAccess.Addr(), 2, uintptr(unsafe.Pointer(vmId)), uintptr(unsafe.Pointer(filePath)), 0)
	if int32(r0) < 0 {
		hr = interop.Win32FromHresult(r0)
	}
	return
}
