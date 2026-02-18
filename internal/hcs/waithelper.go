//go:build windows

package hcs

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
)

func processAsyncHcsResult(
	ctx context.Context,
	err error,
	resultJSON string,
	callbackNum callbackNumber,
	expectedNotification hcsNotification,
	timeout *time.Duration,
) ([]ErrorEvent, error) {
	if IsPending(err) {
		return waitForNotification(ctx, callbackNum, expectedNotification, timeout)
	}

	return processHcsResult(ctx, resultJSON), err
}

func waitForNotification(
	ctx context.Context,
	callbackNum callbackNumber,
	expectedNotification hcsNotification,
	timeout *time.Duration,
) ([]ErrorEvent, error) {
	entry := log.G(ctx).WithFields(logrus.Fields{
		logfields.CallbackNumber: callbackNum,
		"notification-type":      expectedNotification.String(),
	})

	callbackMapLock.RLock()
	callbackCtx := callbackMap[callbackNum]
	callbackMapLock.RUnlock()

	if callbackCtx == nil {
		entry.Error("failed to waitForNotification: callbackNumber does not exist in callbackMap")
		return nil, ErrHandleClose
	}
	channels := callbackCtx.channels

	expectedChannel := channels[expectedNotification]
	if expectedChannel == nil {
		entry.Error("unknown notification type in waitForNotification")
		return nil, ErrInvalidNotificationType
	}

	var c <-chan time.Time
	if timeout != nil {
		timer := time.NewTimer(*timeout)
		c = timer.C
		defer timer.Stop()
	}

	select {
	case err, ok := <-expectedChannel:
		if !ok {
			return nil, ErrHandleClose
		}
		return getEvents(err)
	case err, ok := <-channels[hcsNotificationSystemExited]:
		if !ok {
			return nil, ErrHandleClose
		}
		// If the expected notification is hcsNotificationSystemExited which of the two selects
		// chosen is random. Return the raw error if hcsNotificationSystemExited is expected
		if channels[hcsNotificationSystemExited] == expectedChannel {
			return getEvents(err)
		}
		return nil, ErrUnexpectedContainerExit
	case _, ok := <-channels[hcsNotificationServiceDisconnect]:
		if !ok {
			return nil, ErrHandleClose
		}
		// hcsNotificationServiceDisconnect should never be an expected notification
		// it does not need the same handling as hcsNotificationSystemExited
		return nil, ErrUnexpectedProcessAbort
	case <-c:
		return nil, ErrTimeout
	}
}
