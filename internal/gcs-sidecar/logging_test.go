//go:build windows

package bridge

import (
	"errors"
	"io"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/sirupsen/logrus"
)

type runtimeLogCapture struct {
	count atomic.Int64
	entry atomic.Pointer[logrus.Entry]
	err   error
}

func (h *runtimeLogCapture) Levels() []logrus.Level { return logrus.AllLevels }
func (h *runtimeLogCapture) Fire(entry *logrus.Entry) error {
	h.count.Add(1)
	h.entry.Store(entry)
	return h.err
}

func TestRuntimeLoggingHook(t *testing.T) {
	capture := &runtimeLogCapture{}
	hook := NewRuntimeLoggingHook(capture)
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	logger.AddHook(hook)
	logger.Info("before policy")
	if capture.count.Load() != 0 {
		t.Fatal("hook emitted before authorization")
	}
	if !reflect.DeepEqual(hook.Levels(), capture.Levels()) {
		t.Fatal("hook changed supported levels")
	}
	hook.SetAllowed(true)
	logger.WithField("containerID", "test").Info("allowed")
	if capture.count.Load() != 1 {
		t.Fatal("allowed hook was suppressed by discarded output")
	}
	if got := capture.entry.Load(); got.Message != "allowed" || got.Data["containerID"] != "test" {
		t.Fatalf("event fields not preserved: %+v", got)
	}
	hook.SetAllowed(false)
	logger.Info("denied")
	if capture.count.Load() != 1 {
		t.Fatal("denied hook emitted")
	}

	entry := logrus.NewEntry(logger).WithField("containerID", "test")
	capture.err = errors.New("sink failed")
	hook.SetAllowed(true)
	if err := hook.Fire(entry); !errors.Is(err, capture.err) {
		t.Fatalf("sink error not preserved: %v", err)
	}
	if capture.entry.Load() != entry {
		t.Fatal("hook replaced the log entry")
	}
}

func TestRuntimeLoggingHookConcurrent(t *testing.T) {
	capture := &runtimeLogCapture{}
	hook := NewRuntimeLoggingHook(capture)
	entry := logrus.NewEntry(logrus.New())
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 1000 {
				hook.SetAllowed(i%2 == 0)
				if err := hook.Fire(entry); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	hook.SetAllowed(false)
	before := capture.count.Load()
	_ = hook.Fire(entry)
	if capture.count.Load() != before {
		t.Fatal("hook emitted after denial")
	}
}
