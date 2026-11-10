//go:build windows

package bridge

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	maxBufferedStartupEvents = 128
	maxBufferedStartupBytes  = 256 * 1024
	maxStartupEventBytes     = 16 * 1024
	maxStartupFields         = 32
	maxStartupArrayElements  = 256
)

// RuntimeLoggingHook holds pre-policy startup events until both the policy and
// an ETW collection session permit forwarding.
type RuntimeLoggingHook struct {
	next logrus.Hook

	mu                sync.Mutex
	allowed           bool
	policyResolved    bool
	collectionEnabled bool
	closed            bool
	pending           []*logrus.Entry
	pendingBytes      int

	wake      chan struct{}
	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

func NewRuntimeLoggingHook(next logrus.Hook) *RuntimeLoggingHook {
	h := &RuntimeLoggingHook{
		next: next,
		wake: make(chan struct{}, 1),
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	go h.forward()
	return h
}

// SetAllowed publishes the runtime-logging decision from the validated guest
// policy. A denial permanently discards pending startup events.
func (h *RuntimeLoggingHook) SetAllowed(allowed bool) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.policyResolved = true
	h.allowed = allowed
	if !allowed {
		h.pending = nil
		h.pendingBytes = 0
	}
	h.mu.Unlock()
	h.signal()
}

// SetCollectionEnabled is called when ETW reports collector enable/disable.
func (h *RuntimeLoggingHook) SetCollectionEnabled(enabled bool) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.collectionEnabled = enabled
	h.mu.Unlock()
	h.signal()
}

func (h *RuntimeLoggingHook) Levels() []logrus.Level {
	return h.next.Levels()
}

func (h *RuntimeLoggingHook) Fire(entry *logrus.Entry) error {
	h.mu.Lock()
	if h.closed || (h.policyResolved && (!h.allowed || !h.collectionEnabled)) {
		h.mu.Unlock()
		return nil
	}

	buffered, size, ok := copyStartupEntry(entry)
	if !ok || len(h.pending) >= maxBufferedStartupEvents ||
		size > maxBufferedStartupBytes-h.pendingBytes {
		h.mu.Unlock()
		return nil
	}
	h.pending = append(h.pending, buffered)
	h.pendingBytes += size
	h.mu.Unlock()
	h.signal()
	return nil
}

func (h *RuntimeLoggingHook) signal() {
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

func (h *RuntimeLoggingHook) forward() {
	defer close(h.done)
	for {
		select {
		case <-h.stop:
			return
		case <-h.wake:
			h.mu.Lock()
			if h.closed || !h.policyResolved || !h.allowed || !h.collectionEnabled {
				h.mu.Unlock()
				continue
			}
			entries := h.pending
			h.pending = nil
			h.pendingBytes = 0
			h.mu.Unlock()

			for _, entry := range entries {
				h.mu.Lock()
				allowed := !h.closed && h.policyResolved && h.allowed && h.collectionEnabled
				h.mu.Unlock()
				if !allowed {
					break
				}
				_ = h.next.Fire(entry)
			}
		}
	}
}

// Close discards queued events and waits for any in-flight ETW write before the
// provider is unregistered.
func (h *RuntimeLoggingHook) Close() {
	h.closeOnce.Do(func() {
		h.mu.Lock()
		h.closed = true
		h.pending = nil
		h.pendingBytes = 0
		h.mu.Unlock()
		close(h.stop)
		<-h.done
	})
}

func copyStartupEntry(entry *logrus.Entry) (*logrus.Entry, int, bool) {
	if len(entry.Message) > maxStartupEventBytes {
		return nil, 0, false
	}
	data := make(logrus.Fields, len(entry.Data))
	size := len(entry.Message)
	for key, value := range entry.Data {
		if len(data) == maxStartupFields || size+len(key)+32 > maxStartupEventBytes {
			return nil, 0, false
		}
		remaining := maxStartupEventBytes - size - len(key) - 32
		copied, valueSize, ok := copyStartupField(value, remaining)
		if !ok || size+len(key)+valueSize+32 > maxStartupEventBytes {
			return nil, 0, false
		}
		data[key] = copied
		size += len(key) + valueSize + 32
	}
	if size > maxStartupEventBytes {
		return nil, 0, false
	}

	copied := &logrus.Entry{
		Logger:  entry.Logger,
		Data:    data,
		Time:    entry.Time,
		Level:   entry.Level,
		Message: entry.Message,
	}
	return copied, size, true
}

func copyStartupField(value interface{}, maxBytes int) (interface{}, int, bool) {
	switch v := value.(type) {
	case string:
		return v, len(v), len(v) <= maxBytes
	case []byte:
		if len(v) > maxBytes {
			return nil, 0, false
		}
		return append([]byte(nil), v...), len(v), true
	case bool:
		return v, 1, true
	case int:
		return v, 8, true
	case int8:
		return v, 1, true
	case int16:
		return v, 2, true
	case int32:
		return v, 4, true
	case int64:
		return v, 8, true
	case uint:
		return v, 8, true
	case uint8:
		return v, 1, true
	case uint16:
		return v, 2, true
	case uint32:
		return v, 4, true
	case uint64:
		return v, 8, true
	case uintptr:
		return v, 8, true
	case float32:
		return v, 4, true
	case float64:
		return v, 8, true
	case time.Time:
		return v, 16, true
	case time.Duration:
		return v, 8, true
	case error:
		message := v.Error()
		return message, len(message), len(message) <= maxBytes
	case []string:
		if len(v) > maxStartupArrayElements {
			return nil, 0, false
		}
		size := len(v) * 16
		if size > maxBytes {
			return nil, 0, false
		}
		for _, item := range v {
			if len(item) > maxBytes-size {
				return nil, 0, false
			}
			size += len(item)
		}
		copied := append([]string(nil), v...)
		return copied, size, true
	default:
		return nil, 0, false
	}
}
