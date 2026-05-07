//go:build windows

package hcs

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/vmcompute"
)

// fireNotificationForTest simulates an HCS notification callback for a given
// callback number. Used to drive async completion paths in tests.
func fireNotificationForTest(callbackNumber uintptr, notification hcsNotification, result error) {
	callbackMapLock.RLock()
	ctx := callbackMap[callbackNumber]
	callbackMapLock.RUnlock()
	if ctx == nil {
		return
	}
	if ch, ok := ctx.channels[notification]; ok {
		ch <- result
	}
}

func newTestSystemWithHandle(id string, handle uintptr) *System {
	s := newSystem(id)
	s.handle = vmcompute.HcsSystem(handle)
	return s
}

func registerCallbackForTest(s *System) error {
	return s.registerCallback(context.Background())
}

func startWaitBackgroundForTest(s *System) {
	go s.waitBackground()
}
