//go:build windows

package hcs

import (
	"syscall"
	"testing"
	"unsafe"
)

// TestNotificationWatcher_DeliversDataAndError verifies that notificationWatcher
// routes both the error and the raw notificationData payload to the channel that
// the watcher goroutine reads. This is the plumbing that lets hcsshim observe the
// SystemExitStatus JSON carried by an HcsEventSystemExited notification — it's
// how ExitType=Reboot reaches the shim-side reboot handler in container-reboot-v2.
func TestNotificationWatcher_DeliversDataAndError(t *testing.T) {
	const callbackNumber uintptr = 0xdeadbeef
	ctx := &notificationWatcherContext{
		channels: newSystemChannels(),
		systemID: "TestNotificationWatcher_DeliversDataAndError",
	}
	callbackMapLock.Lock()
	callbackMap[callbackNumber] = ctx
	callbackMapLock.Unlock()
	t.Cleanup(func() {
		callbackMapLock.Lock()
		delete(callbackMap, callbackNumber)
		callbackMapLock.Unlock()
	})

	wantData := `{"Status":0,"ExitType":"Reboot"}`
	u16, err := syscall.UTF16FromString(wantData)
	if err != nil {
		t.Fatalf("UTF16FromString: %v", err)
	}
	ptr := (*uint16)(unsafe.Pointer(&u16[0]))

	notificationWatcher(hcsNotificationSystemExited, callbackNumber, 0, ptr)

	select {
	case p, ok := <-ctx.channels[hcsNotificationSystemExited]:
		if !ok {
			t.Fatal("channel closed before payload delivered")
		}
		if p.err != nil {
			t.Fatalf("unexpected err: %v", p.err)
		}
		if p.data != wantData {
			t.Fatalf("payload data = %q, want %q", p.data, wantData)
		}
	default:
		t.Fatal("no payload delivered on channel")
	}
}

// TestNotificationWatcher_NilDataYieldsEmptyString covers the common case of a
// notification without event data (anything other than HcsEventSystemExited).
// The watcher must tolerate notificationData==nil and deliver payload.data == "".
func TestNotificationWatcher_NilDataYieldsEmptyString(t *testing.T) {
	const callbackNumber uintptr = 0xdeadbef0
	ctx := &notificationWatcherContext{
		channels: newSystemChannels(),
		systemID: "TestNotificationWatcher_NilDataYieldsEmptyString",
	}
	callbackMapLock.Lock()
	callbackMap[callbackNumber] = ctx
	callbackMapLock.Unlock()
	t.Cleanup(func() {
		callbackMapLock.Lock()
		delete(callbackMap, callbackNumber)
		callbackMapLock.Unlock()
	})

	notificationWatcher(hcsNotificationSystemStartCompleted, callbackNumber, 0, nil)

	select {
	case p, ok := <-ctx.channels[hcsNotificationSystemStartCompleted]:
		if !ok {
			t.Fatal("channel closed before payload delivered")
		}
		if p.err != nil {
			t.Fatalf("unexpected err: %v", p.err)
		}
		if p.data != "" {
			t.Fatalf("payload data = %q, want empty", p.data)
		}
	default:
		t.Fatal("no payload delivered on channel")
	}
}
