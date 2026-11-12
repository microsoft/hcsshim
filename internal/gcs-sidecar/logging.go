//go:build windows

package bridge

import (
	"fmt"
	"reflect"
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
	streaming         bool
	started           bool
	closed            bool
	pending           []*logrus.Entry
	pendingBytes      int

	wake      chan struct{}
	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

// RuntimeLoggingController coordinates ETW collector notifications with hook
// publication so an enable notification cannot be overwritten by stale state.
type RuntimeLoggingController struct {
	mu      sync.Mutex
	hook    *RuntimeLoggingHook
	enabled bool
}

func NewRuntimeLoggingController() *RuntimeLoggingController {
	return &RuntimeLoggingController{}
}

func (c *RuntimeLoggingController) Attach(hook *RuntimeLoggingHook) {
	c.mu.Lock()
	c.hook = hook
	hook.SetCollectionEnabled(c.enabled)
	c.mu.Unlock()
}

func (c *RuntimeLoggingController) SetCollectionEnabled(enabled bool) {
	c.mu.Lock()
	c.enabled = enabled
	if c.hook != nil {
		c.hook.SetCollectionEnabled(enabled)
	}
	c.mu.Unlock()
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
	h.streaming = false
	if h.readyLocked() {
		if len(h.pending) == 0 {
			h.streaming = true
			h.started = true
		} else {
			h.streaming = false
		}
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
	if h.readyLocked() {
		if len(h.pending) == 0 {
			h.streaming = true
			h.started = true
		} else {
			h.streaming = false
		}
	} else {
		h.streaming = false
	}
	h.mu.Unlock()
	h.signal()
}

func (h *RuntimeLoggingHook) readyLocked() bool {
	return !h.closed && h.policyResolved && h.allowed && h.collectionEnabled
}

func (h *RuntimeLoggingHook) Levels() []logrus.Level {
	return h.next.Levels()
}

func (h *RuntimeLoggingHook) Fire(entry *logrus.Entry) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed || (h.policyResolved && !h.allowed) || (h.started && !h.collectionEnabled) {
		return nil
	}
	if h.readyLocked() && h.streaming {
		return h.next.Fire(entry)
	}

	buffered, size, ok := copyStartupEntry(entry)
	if !ok || len(h.pending) >= maxBufferedStartupEvents ||
		size > maxBufferedStartupBytes-h.pendingBytes {
		return nil
	}
	h.pending = append(h.pending, buffered)
	h.pendingBytes += size
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
			if !h.readyLocked() {
				h.mu.Unlock()
				continue
			}
			entries := h.pending
			h.pending = nil
			h.pendingBytes = 0
			h.started = true
			for _, entry := range entries {
				if !h.readyLocked() {
					break
				}
				_ = h.next.Fire(entry)
			}
			if h.readyLocked() {
				h.streaming = true
				h.started = true
			}
			h.mu.Unlock()
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
	case map[string]interface{}, map[interface{}]interface{}, []interface{}:
		return copyStartupValue(v, maxBytes, 0)
	default:
		serialized := fmt.Sprintf("(Unsupported: %T) %v", value, value)
		return serialized, len(serialized), len(serialized) <= maxBytes
	}
}

func copyStartupValue(value interface{}, maxBytes, depth int) (interface{}, int, bool) {
	if depth >= 8 {
		return nil, 0, false
	}
	switch v := value.(type) {
	case nil:
		return nil, 1, true
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
	case float32:
		return v, 4, true
	case float64:
		return v, 8, true
	case map[string]interface{}:
		if len(v) > maxStartupArrayElements {
			return nil, 0, false
		}
		cloned := make(map[string]interface{}, len(v))
		size := len(v) * 32
		for key, item := range v {
			if len(key) > maxBytes-size {
				return nil, 0, false
			}
			copied, itemSize, ok := copyStartupValue(item, maxBytes-size-len(key), depth+1)
			if !ok || size+len(key)+itemSize > maxBytes {
				return nil, 0, false
			}
			cloned[key] = copied
			size += len(key) + itemSize
		}
		return cloned, size, size <= maxBytes
	case map[interface{}]interface{}:
		if len(v) > maxStartupArrayElements {
			return nil, 0, false
		}
		cloned := make(map[interface{}]interface{}, len(v))
		size := len(v) * 32
		for key, item := range v {
			copiedKey, keySize, ok := copyStartupValue(key, maxBytes-size, depth+1)
			if !ok || copiedKey == nil || !reflect.TypeOf(copiedKey).Comparable() {
				return nil, 0, false
			}
			copiedValue, valueSize, ok := copyStartupValue(item, maxBytes-size-keySize, depth+1)
			if !ok || size+keySize+valueSize > maxBytes {
				return nil, 0, false
			}
			cloned[copiedKey] = copiedValue
			size += keySize + valueSize
		}
		return cloned, size, size <= maxBytes
	case []interface{}:
		if len(v) > maxStartupArrayElements || len(v)*16 > maxBytes {
			return nil, 0, false
		}
		cloned := make([]interface{}, len(v))
		size := len(v) * 16
		for i, item := range v {
			copied, itemSize, ok := copyStartupValue(item, maxBytes-size, depth+1)
			if !ok || size+itemSize > maxBytes {
				return nil, 0, false
			}
			cloned[i] = copied
			size += itemSize
		}
		return cloned, size, size <= maxBytes
	default:
		return nil, 0, false
	}
}
