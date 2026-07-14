//go:build windows

package computecore

import (
	"fmt"
	"syscall"
)

// HcsSystem is the handle associated with a created compute system.
type HcsSystem syscall.Handle

// HcsProcess is the handle associated with a created process in a compute system.
type HcsProcess syscall.Handle

// HcsOperation is the handle associated with an operation on a compute system.
type HcsOperation syscall.Handle

// HcsCallback is the handle associated with the function to call when events occur.
type HcsCallback syscall.Handle

// HcsProcessInformation is the structure used when creating or getting process info.
type HcsProcessInformation struct {
	ProcessID uint32
	_         uint32 // reserved padding
	StdInput  syscall.Handle
	StdOutput syscall.Handle
	StdError  syscall.Handle
}

// HcsResourceType specifies the type of resource to add to an operation.
const (
	HcsResourceTypeNone      uint32 = 0
	HcsResourceTypeFile      uint32 = 1
	HcsResourceTypeJob       uint32 = 2
	HcsResourceTypeComObject uint32 = 3
	HcsResourceTypeSocket    uint32 = 4
)

// HcsEventType represents the type of event received from HCS.
type HcsEventType uint32

const (
	HcsEventTypeInvalid                           HcsEventType = 0x00000000
	HcsEventTypeSystemExited                      HcsEventType = 0x00000001
	HcsEventTypeSystemCrashInitiated              HcsEventType = 0x00000002
	HcsEventTypeSystemCrashReport                 HcsEventType = 0x00000003
	HcsEventTypeSystemRdpEnhancedModeStateChanged HcsEventType = 0x00000004
	HcsEventTypeSystemSiloJobCreated              HcsEventType = 0x00000005
	HcsEventTypeSystemGuestConnectionClosed       HcsEventType = 0x00000006
	HcsEventTypeProcessExited                     HcsEventType = 0x00010000
	HcsEventTypeOperationCallback                 HcsEventType = 0x01000000
	HcsEventTypeServiceDisconnect                 HcsEventType = 0x02000000
	HcsEventTypeGroupVMLifecycle                  HcsEventType = 0x80000002
	HcsEventTypeGroupLiveMigration                HcsEventType = 0x80000003
	HcsEventTypeGroupOperationInfo                HcsEventType = 0xC0000001
)

func (t HcsEventType) String() string {
	switch t {
	case HcsEventTypeInvalid:
		return "Invalid"
	case HcsEventTypeSystemExited:
		return "SystemExited"
	case HcsEventTypeSystemCrashInitiated:
		return "SystemCrashInitiated"
	case HcsEventTypeSystemCrashReport:
		return "SystemCrashReport"
	case HcsEventTypeSystemRdpEnhancedModeStateChanged:
		return "SystemRdpEnhancedModeStateChanged"
	case HcsEventTypeSystemSiloJobCreated:
		return "SystemSiloJobCreated"
	case HcsEventTypeSystemGuestConnectionClosed:
		return "SystemGuestConnectionClosed"
	case HcsEventTypeProcessExited:
		return "ProcessExited"
	case HcsEventTypeOperationCallback:
		return "OperationCallback"
	case HcsEventTypeServiceDisconnect:
		return "ServiceDisconnect"
	case HcsEventTypeGroupVMLifecycle:
		return "GroupVmLifecycle"
	case HcsEventTypeGroupLiveMigration:
		return "GroupLiveMigration"
	case HcsEventTypeGroupOperationInfo:
		return "GroupOperationInfo"
	default:
		return fmt.Sprintf("Unknown: 0x%08X", uint32(t))
	}
}

// HcsEventOptions controls which event groups are enabled for a callback.
const (
	HcsEventOptionNone                      uint32 = 0
	HcsEventOptionEnableOperationCallbacks  uint32 = 1
	HcsEventOptionEnableLiveMigrationEvents uint32 = 4
)

// HcsEvent is the event structure passed to HCS_EVENT_CALLBACK.
type HcsEvent struct {
	Type      HcsEventType
	EventData *uint16
}
