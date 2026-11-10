//go:build windows

package bridge

import (
	"io"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

type runtimeLogCapture struct {
	count   atomic.Int64
	entries chan *logrus.Entry
}

func (h *runtimeLogCapture) Levels() []logrus.Level { return logrus.AllLevels }
func (h *runtimeLogCapture) Fire(entry *logrus.Entry) error {
	h.count.Add(1)
	h.entries <- entry
	return nil
}

func newTestRuntimeLoggingHook(next logrus.Hook) *RuntimeLoggingHook {
	hook := NewRuntimeLoggingHook(next)
	return hook
}

func receiveLogEntry(t *testing.T, entries <-chan *logrus.Entry) *logrus.Entry {
	t.Helper()
	select {
	case entry := <-entries:
		return entry
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ETW hook")
		return nil
	}
}

func TestRuntimeLoggingHookFlushesAfterPolicyAndCollector(t *testing.T) {
	capture := &runtimeLogCapture{entries: make(chan *logrus.Entry, 8)}
	hook := newTestRuntimeLoggingHook(capture)
	t.Cleanup(hook.Close)
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	logger.AddHook(hook)

	logger.WithField("containerID", "startup").Info("sidecar initialized")
	if capture.count.Load() != 0 {
		t.Fatal("startup event emitted before policy validation")
	}

	hook.SetAllowed(true)
	logger.Info("policy installed")
	if capture.count.Load() != 0 {
		t.Fatal("event emitted before an ETW collector was enabled")
	}

	hook.SetCollectionEnabled(true)
	logger.WithField("containerID", "ready").Info("forwarding active")

	first := receiveLogEntry(t, capture.entries)
	second := receiveLogEntry(t, capture.entries)
	if got := []string{first.Message, second.Message}; !reflect.DeepEqual(got, []string{
		"sidecar initialized", "forwarding active",
	}) {
		t.Fatalf("forwarded messages = %v", got)
	}
	if first.Data["containerID"] != "startup" || second.Data["containerID"] != "ready" {
		t.Fatal("structured startup or live fields were not preserved")
	}

	hook.SetCollectionEnabled(false)
	logger.Info("collector stopped")
	if capture.count.Load() != 2 {
		t.Fatal("event emitted while ETW collection was disabled")
	}
}

func TestRuntimeLoggingHookCollectorCanPrecedePolicy(t *testing.T) {
	capture := &runtimeLogCapture{entries: make(chan *logrus.Entry, 2)}
	hook := newTestRuntimeLoggingHook(capture)
	t.Cleanup(hook.Close)
	entry := logrus.NewEntry(logrus.New())
	entry.Message = "captured before policy"
	if err := hook.Fire(entry); err != nil {
		t.Fatal(err)
	}
	hook.SetCollectionEnabled(true)
	if capture.count.Load() != 0 {
		t.Fatal("event emitted before policy validation")
	}
	hook.SetAllowed(true)
	if got := receiveLogEntry(t, capture.entries); got.Message != entry.Message {
		t.Fatalf("forwarded %q, want %q", got.Message, entry.Message)
	}
}

func TestRuntimeLoggingHookPolicyDenialDropsStartupEvents(t *testing.T) {
	capture := &runtimeLogCapture{entries: make(chan *logrus.Entry, 4)}
	hook := newTestRuntimeLoggingHook(capture)
	t.Cleanup(hook.Close)
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	logger.AddHook(hook)

	logger.Info("must be discarded")
	hook.SetCollectionEnabled(true)
	hook.SetAllowed(false)
	logger.Info("also denied")
	if capture.count.Load() != 0 {
		t.Fatal("policy-denied startup events were emitted")
	}
	hook.mu.Lock()
	defer hook.mu.Unlock()
	if len(hook.pending) != 0 {
		t.Fatal("policy denial retained startup events")
	}
}

func TestRuntimeLoggingHookDropsOversizedStartupEntry(t *testing.T) {
	capture := &runtimeLogCapture{entries: make(chan *logrus.Entry, 1)}
	hook := newTestRuntimeLoggingHook(capture)
	t.Cleanup(hook.Close)
	entry := logrus.NewEntry(logrus.New())
	entry.Message = "oversized"
	entry.Data = logrus.Fields{"payload": make([]byte, maxStartupEventBytes+1)}
	if err := hook.Fire(entry); err != nil {
		t.Fatal(err)
	}
	hook.SetAllowed(true)
	hook.SetCollectionEnabled(true)
	if capture.count.Load() != 0 {
		t.Fatal("oversized startup entry was buffered")
	}
	hook.mu.Lock()
	defer hook.mu.Unlock()
	if len(hook.pending) != 0 {
		t.Fatal("oversized startup entry was retained")
	}
}

func TestRuntimeLoggingHookDropsOversizedStartupFieldArray(t *testing.T) {
	capture := &runtimeLogCapture{entries: make(chan *logrus.Entry, 1)}
	hook := newTestRuntimeLoggingHook(capture)
	t.Cleanup(hook.Close)
	entry := logrus.NewEntry(logrus.New())
	entry.Data = logrus.Fields{"values": make([]string, maxStartupArrayElements+1)}
	if err := hook.Fire(entry); err != nil {
		t.Fatal(err)
	}
	hook.SetAllowed(true)
	hook.SetCollectionEnabled(true)
	if capture.count.Load() != 0 {
		t.Fatal("startup event with an oversized field array was buffered")
	}
	hook.mu.Lock()
	defer hook.mu.Unlock()
	if len(hook.pending) != 0 {
		t.Fatal("startup event with oversized field array was retained")
	}
}

func TestRuntimeLoggingHookBoundsStartupBufferBytes(t *testing.T) {
	capture := &runtimeLogCapture{entries: make(chan *logrus.Entry, 20)}
	hook := newTestRuntimeLoggingHook(capture)
	t.Cleanup(hook.Close)
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	logger.AddHook(hook)
	message := strings.Repeat("x", maxStartupEventBytes-32)
	for range 20 {
		logger.Info(message)
	}
	hook.SetAllowed(true)
	hook.SetCollectionEnabled(true)
	for range maxBufferedStartupBytes / maxStartupEventBytes {
		receiveLogEntry(t, capture.entries)
	}
	if got := capture.count.Load(); got != maxBufferedStartupBytes/maxStartupEventBytes {
		t.Fatalf("forwarded %d events, want byte-budget limit %d", got, maxBufferedStartupBytes/maxStartupEventBytes)
	}
}

func TestRuntimeLoggingHookBoundsStartupBuffer(t *testing.T) {
	capture := &runtimeLogCapture{entries: make(chan *logrus.Entry, maxBufferedStartupEvents+1)}
	hook := newTestRuntimeLoggingHook(capture)
	t.Cleanup(hook.Close)
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	logger.AddHook(hook)

	for i := 0; i < maxBufferedStartupEvents+1; i++ {
		logger.WithField("index", i).Info("startup")
	}
	hook.SetAllowed(true)
	hook.SetCollectionEnabled(true)
	for range maxBufferedStartupEvents {
		receiveLogEntry(t, capture.entries)
	}
	if capture.count.Load() != maxBufferedStartupEvents {
		t.Fatalf("forwarded %d events, want bounded count %d", capture.count.Load(), maxBufferedStartupEvents)
	}
}

func TestRuntimeLoggingHookConcurrentStateChanges(t *testing.T) {
	capture := &runtimeLogCapture{entries: make(chan *logrus.Entry, 10000)}
	hook := newTestRuntimeLoggingHook(capture)
	t.Cleanup(hook.Close)
	entry := logrus.NewEntry(logrus.New())
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 1000 {
				hook.SetAllowed(i%2 == 0)
				hook.SetCollectionEnabled(i%3 == 0)
				_ = hook.Fire(entry)
			}
		}()
	}
	wg.Wait()
	hook.Close()
}
