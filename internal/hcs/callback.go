//go:build windows

package hcs

import (
	"fmt"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"github.com/Microsoft/hcsshim/internal/interop"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/vmcompute"
	"github.com/sirupsen/logrus"
)

var (
	nextCallback    uintptr
	callbackMap     = map[uintptr]*notificationWatcherContext{}
	callbackMapLock = sync.RWMutex{}

	notificationWatcherCallback = syscall.NewCallback(notificationWatcher)

	// Notifications for HCS_SYSTEM handles
	hcsNotificationSystemExited                      hcsNotification = 0x00000001
	hcsNotificationSystemCreateCompleted             hcsNotification = 0x00000002
	hcsNotificationSystemStartCompleted              hcsNotification = 0x00000003
	hcsNotificationSystemPauseCompleted              hcsNotification = 0x00000004
	hcsNotificationSystemResumeCompleted             hcsNotification = 0x00000005
	hcsNotificationSystemCrashReport                 hcsNotification = 0x00000006
	hcsNotificationSystemSiloJobCreated              hcsNotification = 0x00000007
	hcsNotificationSystemSaveCompleted               hcsNotification = 0x00000008
	hcsNotificationSystemRdpEnhancedModeStateChanged hcsNotification = 0x00000009
	hcsNotificationSystemShutdownFailed              hcsNotification = 0x0000000A
	hcsNotificationSystemGetPropertiesCompleted      hcsNotification = 0x0000000B
	hcsNotificationSystemModifyCompleted             hcsNotification = 0x0000000C
	hcsNotificationSystemCrashInitiated              hcsNotification = 0x0000000D
	hcsNotificationSystemGuestConnectionClosed       hcsNotification = 0x0000000E

	// Notifications for HCS_PROCESS handles
	hcsNotificationProcessExited hcsNotification = 0x00010000

	// Common notifications
	hcsNotificationInvalid           hcsNotification = 0x00000000
	hcsNotificationServiceDisconnect hcsNotification = 0x01000000
)

type hcsNotification uint32

func (hn hcsNotification) String() string {
	switch hn {
	case hcsNotificationSystemExited:
		return "SystemExited"
	case hcsNotificationSystemCreateCompleted:
		return "SystemCreateCompleted"
	case hcsNotificationSystemStartCompleted:
		return "SystemStartCompleted"
	case hcsNotificationSystemPauseCompleted:
		return "SystemPauseCompleted"
	case hcsNotificationSystemResumeCompleted:
		return "SystemResumeCompleted"
	case hcsNotificationSystemCrashReport:
		return "SystemCrashReport"
	case hcsNotificationSystemSiloJobCreated:
		return "SystemSiloJobCreated"
	case hcsNotificationSystemSaveCompleted:
		return "SystemSaveCompleted"
	case hcsNotificationSystemRdpEnhancedModeStateChanged:
		return "SystemRdpEnhancedModeStateChanged"
	case hcsNotificationSystemShutdownFailed:
		return "SystemShutdownFailed"
	case hcsNotificationSystemGetPropertiesCompleted:
		return "SystemGetPropertiesCompleted"
	case hcsNotificationSystemModifyCompleted:
		return "SystemModifyCompleted"
	case hcsNotificationSystemCrashInitiated:
		return "SystemCrashInitiated"
	case hcsNotificationSystemGuestConnectionClosed:
		return "SystemGuestConnectionClosed"
	case hcsNotificationProcessExited:
		return "ProcessExited"
	case hcsNotificationInvalid:
		return "Invalid"
	case hcsNotificationServiceDisconnect:
		return "ServiceDisconnect"
	default:
		return fmt.Sprintf("Unknown: %d", hn)
	}
}

// notificationPayload carries both the error code and the raw EventData
// string that accompanied the HCS notification. Prior to container-reboot-v2
// the channel was just `chan error`, which silently discarded the
// notificationData pointer — so hcsshim couldn't observe the
// SystemExitStatus JSON (and therefore couldn't see ExitType=Reboot).
// Callers that only care about err can ignore data.
type notificationPayload struct {
	err  error
	data string
}

type notificationChannel chan notificationPayload

type notificationWatcherContext struct {
	channels notificationChannels
	handle   vmcompute.HcsCallback

	systemID  string
	processID int
}

type notificationChannels map[hcsNotification]notificationChannel

func newSystemChannels() notificationChannels {
	channels := make(notificationChannels)
	for _, notif := range []hcsNotification{
		hcsNotificationServiceDisconnect,
		hcsNotificationSystemExited,
		hcsNotificationSystemCreateCompleted,
		hcsNotificationSystemStartCompleted,
		hcsNotificationSystemPauseCompleted,
		hcsNotificationSystemResumeCompleted,
		hcsNotificationSystemSaveCompleted,
	} {
		channels[notif] = make(notificationChannel, 1)
	}
	return channels
}

func newProcessChannels() notificationChannels {
	channels := make(notificationChannels)
	for _, notif := range []hcsNotification{
		hcsNotificationServiceDisconnect,
		hcsNotificationProcessExited,
	} {
		channels[notif] = make(notificationChannel, 1)
	}
	return channels
}

func closeChannels(channels notificationChannels) {
	for _, c := range channels {
		close(c)
	}
}

func notificationWatcher(notificationType hcsNotification, callbackNumber uintptr, notificationStatus uintptr, notificationData *uint16) uintptr {
	var payload notificationPayload
	if int32(notificationStatus) < 0 {
		payload.err = interop.Win32FromHresult(notificationStatus)
	}
	if notificationData != nil {
		payload.data = utf16PtrToString(notificationData)
	}

	callbackMapLock.RLock()
	context := callbackMap[callbackNumber]
	callbackMapLock.RUnlock()

	if context == nil {
		return 0
	}

	log := logrus.WithFields(logrus.Fields{
		"notification-type": notificationType.String(),
		"system-id":         context.systemID,
	})
	if context.processID != 0 {
		log.Data[logfields.ProcessID] = context.processID
	}
	log.Debug("HCS notification")

	if channel, ok := context.channels[notificationType]; ok {
		channel <- payload
	}

	return 0
}

// utf16PtrToString materializes a null-terminated UTF-16 pointer (as the
// Win32 HCS callback gives us) into a Go string. Returns "" on nil input.
// Walks the pointer two bytes at a time until it hits NUL; the caller owns
// neither the pointer nor its backing memory so we must copy immediately.
func utf16PtrToString(p *uint16) string {
	if p == nil {
		return ""
	}
	var units []uint16
	for addr := uintptr(unsafe.Pointer(p)); ; addr += 2 {
		c := *(*uint16)(unsafe.Pointer(addr))
		if c == 0 {
			break
		}
		units = append(units, c)
	}
	return string(utf16.Decode(units))
}
