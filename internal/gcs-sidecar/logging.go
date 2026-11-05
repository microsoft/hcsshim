//go:build windows

package bridge

import (
	"sync/atomic"

	"github.com/sirupsen/logrus"
)

// RuntimeLoggingHook gates a logging sink independently of Logrus's output
// writer, which does not control hooks. Its initial state denies emission.
type RuntimeLoggingHook struct {
	next    logrus.Hook
	allowed atomic.Bool
}

func NewRuntimeLoggingHook(next logrus.Hook) *RuntimeLoggingHook {
	return &RuntimeLoggingHook{next: next}
}

func (h *RuntimeLoggingHook) SetAllowed(allowed bool) {
	h.allowed.Store(allowed)
}

func (h *RuntimeLoggingHook) Levels() []logrus.Level {
	return h.next.Levels()
}

func (h *RuntimeLoggingHook) Fire(entry *logrus.Entry) error {
	if !h.allowed.Load() {
		return nil
	}
	return h.next.Fire(entry)
}
