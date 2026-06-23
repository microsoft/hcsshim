//go:build windows

package hcsv2

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"

	"github.com/Microsoft/hcsshim/internal/computecore"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/logfields"
)

// migrationNotificationBufferSize is the capacity of a System's live migration
// notification channel. It only needs enough headroom to absorb a short burst
// of HCS-side events between consumer reads; the dispatch in
// notificationHandler is non-blocking and drops on overflow.
const migrationNotificationBufferSize = 16

// HCS V2 callbacks take an opaque void* context. Rather than handing HCS a
// live Go pointer, we register a numeric ID that maps to the real context in
// notificationContexts.
//
// Registrations use HcsEventOptionEnableLiveMigrationEvents.
// The package never attaches a per-operation callback.
var (
	notificationNextID   atomic.Uint64
	notificationContexts sync.Map // uint64 -> *notificationContext

	notificationCallback = syscall.NewCallback(notificationHandler)
)

// notificationState rendezvous a terminal HCS event with waitBackground.
// Exactly one of exit (normal exit + payload) or abort (abnormal termination)
// is signaled. Both channels are buffered(1) and closed after send.
type notificationState struct {
	signalOnce sync.Once
	exit       chan json.RawMessage
	abort      chan error
}

func newNotificationState() *notificationState {
	return &notificationState{
		exit:  make(chan json.RawMessage, 1),
		abort: make(chan error, 1),
	}
}

// signalExit delivers a normal exit payload. First signal wins.
func (s *notificationState) signalExit(raw json.RawMessage) {
	s.signalOnce.Do(func() {
		s.exit <- raw
		close(s.exit)
	})
}

// signalAbort delivers an abnormal-termination error. First signal wins.
func (s *notificationState) signalAbort(err error) {
	s.signalOnce.Do(func() {
		s.abort <- err
		close(s.abort)
	})
}

// notificationContext is the per-handle data resolved from the callback's
// opaque ctx. processID == 0 means the callback belongs to a system handle.
type notificationContext struct {
	systemID    string
	processID   int // 0 for system handle callbacks
	state       *notificationState
	migrationCh chan<- hcsschema.OperationSystemMigrationNotificationInfo
}

// registerNotificationContext returns the ID to pass as the void* context to
// HcsSet{ComputeSystem,Process}Callback. The caller must invoke
// unregisterNotificationContext after the HCS handle is closed (HCS guarantees
// no further callbacks fire past close).
//
// migrationCh may be nil; pass non-nil only for system handles that should
// receive live migration notifications.
func registerNotificationContext(systemID string, processID int, state *notificationState, migrationCh chan<- hcsschema.OperationSystemMigrationNotificationInfo) uint64 {
	id := notificationNextID.Add(1)
	notificationContexts.Store(id, &notificationContext{
		systemID:    systemID,
		processID:   processID,
		state:       state,
		migrationCh: migrationCh,
	})
	return id
}

// unregisterNotificationContext drops the mapping for id. No-op for id == 0.
func unregisterNotificationContext(id uint64) {
	if id != 0 {
		notificationContexts.Delete(id)
	}
}

// notificationHandler is the single syscall callback shared by all HCS system
// and process registrations. It logs the event, signals the owning
// notificationState on terminal exit events, and dispatches live migration
// events to the registered migration channel. The return value is ignored by
// HCS.
func notificationHandler(eventPtr uintptr, ctx uintptr) uintptr {
	if eventPtr == 0 {
		return 0
	}
	e := (*computecore.HcsEvent)(unsafe.Pointer(eventPtr))

	fields := logrus.Fields{"event-type": e.Type.String()}
	var eventData string
	if e.EventData != nil {
		eventData = windows.UTF16PtrToString(e.EventData)
		fields["event-data"] = eventData
	}

	source := "system"
	v, ok := notificationContexts.Load(uint64(ctx))
	if ok {
		nc := v.(*notificationContext)
		if nc.systemID != "" {
			fields[logfields.ContainerID] = nc.systemID
		}
		if nc.processID != 0 {
			fields[logfields.ProcessID] = nc.processID
			source = "process"
		}
		switch e.Type {
		case computecore.HcsEventTypeSystemExited, computecore.HcsEventTypeProcessExited:
			if nc.state != nil {
				nc.state.signalExit(json.RawMessage(eventData))
			}
		case computecore.HcsEventTypeServiceDisconnect:
			if nc.state != nil {
				nc.state.signalAbort(ErrUnexpectedProcessAbort)
			}
		case computecore.HcsEventTypeGroupLiveMigration:
			// Forward to the system's migration channel, if one was
			// registered.
			if nc.migrationCh != nil {
				dispatchMigrationEvent(nc.migrationCh, e.Type, json.RawMessage(eventData))
			}
		}
	}

	logrus.WithFields(fields).Debugf("HCS %s notification", source)
	return 0
}

// dispatchMigrationEvent decodes a GroupLiveMigration EventData payload and
// non-blocking-sends it on ch. An empty payload yields the zero value (HCS
// occasionally delivers LM events with a nil EventData pointer).
func dispatchMigrationEvent(ch chan<- hcsschema.OperationSystemMigrationNotificationInfo, eventType computecore.HcsEventType, eventData json.RawMessage) {
	var info hcsschema.OperationSystemMigrationNotificationInfo
	if len(eventData) > 0 {
		if err := json.Unmarshal(eventData, &info); err != nil {
			logrus.WithFields(logrus.Fields{
				"event-type":    eventType.String(),
				"event-data":    string(eventData),
				logrus.ErrorKey: err,
			}).Warn("failed to unmarshal migration notification payload, dropping event")
			return
		}
	}
	select {
	case ch <- info:
	default:
		logrus.WithField("event-type", eventType.String()).Warn("migration notification channel full, dropping event")
	}
}
