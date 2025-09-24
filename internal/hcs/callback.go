//go:build windows

package hcs

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"syscall"

	"github.com/sirupsen/logrus"

	"github.com/Microsoft/hcsshim/internal/interop"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/vmcompute"
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

type notificationChannel chan error

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

func notificationWatcher(
	notificationType hcsNotification,
	callbackNumber uintptr,
	notificationStatus uintptr,
	notificationData *uint16,
) uintptr {
	ctx, entry := log.SetEntry(context.Background(), logrus.Fields{
		"notification-type": notificationType.String(),
	})

	result := processNotification(ctx, notificationStatus, notificationData)
	if result != nil {
		entry.Data[logrus.ErrorKey] = result
	}

	callbackMapLock.RLock()
	context := callbackMap[callbackNumber]
	callbackMapLock.RUnlock()

	if context == nil {
		entry.WithField("context", callbackNumber).Warn("Received notification for unknown context (callback number)")
		return 0
	}

	entry.Data["system-id"] = context.systemID
	if context.processID != 0 {
		entry.Data[logfields.ProcessID] = context.processID
	}
	entry.Debug("HCS notification")

	if channel, ok := context.channels[notificationType]; ok {
		channel <- result
	}

	return 0
}

// processNotification parses and validates HCS notifications and returns the result as an error.
func processNotification(ctx context.Context, notificationStatus uintptr, notificationData *uint16) (err error) {
	// TODO: merge/unify with [processHcsResult]
	status := int32(notificationStatus)
	if status < 0 {
		err = interop.Win32FromHresult(notificationStatus)
	}

	if notificationData == nil {
		return err
	}

	resultJSON := interop.ConvertAndFreeCoTaskMemString(notificationData)
	result := &hcsResult{}
	if jsonErr := json.Unmarshal([]byte(resultJSON), result); jsonErr != nil {
		log.G(ctx).WithFields(logrus.Fields{
			logfields.JSON:  resultJSON,
			logrus.ErrorKey: err,
		}).Warn("Could not unmarshal HCS result")
		return err
	}

	// the HResult and data payload should have the same error value
	if result.Error < 0 && status != result.Error {
		log.G(ctx).WithFields(logrus.Fields{
			"status": status,
			"data":   result.Error,
		}).Warn("Mismatched status and data HResult values")
	}

	if len(result.ErrorEvents) > 0 {
		return &resultError{
			Err:    err,
			Events: result.ErrorEvents,
		}
	}
	return err
}
