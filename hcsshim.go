// Shim for the Host Compute Service (HCS) to manage Windows Server
// containers and Hyper-V containers.

package hcsshim

import (
	"syscall"

	"github.com/Microsoft/hcsshim/internal/hcserror"
	"github.com/Microsoft/hcsshim/internal/interop"
	"github.com/sirupsen/logrus"
)

//go:generate go run mksyscall_windows.go -output zhcsshim.go hcsshim.go

//sys SetCurrentThreadCompartmentId(compartmentId uint32) (hr error) = iphlpapi.SetCurrentThreadCompartmentId

//sys hcsEnumerateComputeSystems(query string, computeSystems **uint16, result **uint16) (hr error) = vmcompute.HcsEnumerateComputeSystems?
//sys hcsCreateComputeSystem(id string, configuration string, identity syscall.Handle, computeSystem *hcsSystem, result **uint16) (hr error) = vmcompute.HcsCreateComputeSystem?
//sys hcsOpenComputeSystem(id string, computeSystem *hcsSystem, result **uint16) (hr error) = vmcompute.HcsOpenComputeSystem?
//sys hcsCloseComputeSystem(computeSystem hcsSystem) (hr error) = vmcompute.HcsCloseComputeSystem?
//sys hcsStartComputeSystem(computeSystem hcsSystem, options string, result **uint16) (hr error) = vmcompute.HcsStartComputeSystem?
//sys hcsShutdownComputeSystem(computeSystem hcsSystem, options string, result **uint16) (hr error) = vmcompute.HcsShutdownComputeSystem?
//sys hcsTerminateComputeSystem(computeSystem hcsSystem, options string, result **uint16) (hr error) = vmcompute.HcsTerminateComputeSystem?
//sys hcsPauseComputeSystem(computeSystem hcsSystem, options string, result **uint16) (hr error) = vmcompute.HcsPauseComputeSystem?
//sys hcsResumeComputeSystem(computeSystem hcsSystem, options string, result **uint16) (hr error) = vmcompute.HcsResumeComputeSystem?
//sys hcsGetComputeSystemProperties(computeSystem hcsSystem, propertyQuery string, properties **uint16, result **uint16) (hr error) = vmcompute.HcsGetComputeSystemProperties?
//sys hcsModifyComputeSystem(computeSystem hcsSystem, configuration string, result **uint16) (hr error) = vmcompute.HcsModifyComputeSystem?
//sys hcsRegisterComputeSystemCallback(computeSystem hcsSystem, callback uintptr, context uintptr, callbackHandle *hcsCallback) (hr error) = vmcompute.HcsRegisterComputeSystemCallback?
//sys hcsUnregisterComputeSystemCallback(callbackHandle hcsCallback) (hr error) = vmcompute.HcsUnregisterComputeSystemCallback?

//sys hcsCreateProcess(computeSystem hcsSystem, processParameters string, processInformation *hcsProcessInformation, process *hcsProcess, result **uint16) (hr error) = vmcompute.HcsCreateProcess?
//sys hcsOpenProcess(computeSystem hcsSystem, pid uint32, process *hcsProcess, result **uint16) (hr error) = vmcompute.HcsOpenProcess?
//sys hcsCloseProcess(process hcsProcess) (hr error) = vmcompute.HcsCloseProcess?
//sys hcsTerminateProcess(process hcsProcess, result **uint16) (hr error) = vmcompute.HcsTerminateProcess?
//sys hcsGetProcessInfo(process hcsProcess, processInformation *hcsProcessInformation, result **uint16) (hr error) = vmcompute.HcsGetProcessInfo?
//sys hcsGetProcessProperties(process hcsProcess, processProperties **uint16, result **uint16) (hr error) = vmcompute.HcsGetProcessProperties?
//sys hcsModifyProcess(process hcsProcess, settings string, result **uint16) (hr error) = vmcompute.HcsModifyProcess?
//sys hcsGetServiceProperties(propertyQuery string, properties **uint16, result **uint16) (hr error) = vmcompute.HcsGetServiceProperties?
//sys hcsRegisterProcessCallback(process hcsProcess, callback uintptr, context uintptr, callbackHandle *hcsCallback) (hr error) = vmcompute.HcsRegisterProcessCallback?
//sys hcsUnregisterProcessCallback(callbackHandle hcsCallback) (hr error) = vmcompute.HcsUnregisterProcessCallback?

//sys hcsModifyServiceSettings(settings string, result **uint16) (hr error) = vmcompute.HcsModifyServiceSettings?

//sys _hnsCall(method string, path string, object string, response **uint16) (hr error) = vmcompute.HNSCall?

const (
	// Specific user-visible exit codes
	WaitErrExecFailed = 32767

	ERROR_GEN_FAILURE          = hcserror.ERROR_GEN_FAILURE
	ERROR_SHUTDOWN_IN_PROGRESS = syscall.Errno(1115)
	WSAEINVAL                  = syscall.Errno(10022)

	// Timeout on wait calls
	TimeoutInfinite = 0xFFFFFFFF
)

type HcsError = hcserror.HcsError

type hcsSystem syscall.Handle
type hcsProcess syscall.Handle
type hcsCallback syscall.Handle

type hcsProcessInformation struct {
	ProcessId uint32
	Reserved  uint32
	StdInput  syscall.Handle
	StdOutput syscall.Handle
	StdError  syscall.Handle
}

func processHcsResult(err error, resultp *uint16) error {
	if resultp != nil {
		result := interop.ConvertAndFreeCoTaskMemString(resultp)
		logrus.Debugf("Result: %s", result)
	}
	return err
}
