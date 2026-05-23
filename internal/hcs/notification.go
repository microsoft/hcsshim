//go:build windows

package hcs

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

// notificationState is the rendezvous between the HCS callback thread and
// waitBackground. waitBackground must NOT poll the HCS handle directly: that
// races with Close and can fault.
//
// HCS does not synthesize a final exit notification when HcsCloseProcess /
// HcsCloseComputeSystem unregisters the callback, so a single "wait
// finished" channel cannot distinguish a real exit from a Close. We expose
// two channels and let waitBackground select on them:
//
//   - exited: closed by signalExit on a terminal {System,Process}Exited
//     event; raw holds the event's JSON payload.
//   - closed: closed by signalClosed from Close; nothing further will
//     arrive, so waitBackground must return without publishing a
//     synthetic exit (which would surface to consumers as exit_code=255).
type notificationState struct {
	exitOnce  sync.Once
	closeOnce sync.Once
	exited    chan struct{}
	closed    chan struct{}
	raw       json.RawMessage
}

func newNotificationState() *notificationState {
	return &notificationState{
		exited: make(chan struct{}),
		closed: make(chan struct{}),
	}
}

// signalExit records a real exit event and unblocks waitBackground. Safe to
// call multiple times; only the first wins.
func (s *notificationState) signalExit(raw json.RawMessage) {
	s.exitOnce.Do(func() {
		s.raw = raw
		close(s.exited)
	})
}

// signalClosed unblocks waitBackground without recording an exit. Call from
// Close when no terminal event was observed. Safe to call multiple times.
func (s *notificationState) signalClosed() {
	s.closeOnce.Do(func() { close(s.closed) })
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
		// Only terminal events count as a real exit; do NOT signal
		// closed here.
		case computecore.HcsEventTypeSystemExited, computecore.HcsEventTypeProcessExited:
			if nc.state != nil {
				nc.state.signalExit(json.RawMessage(eventData))
			}
		case computecore.HcsEventTypeGroupLiveMigration:
			// Forward to the system's migration channel, if one was
			// registered. Decoding failures and a full channel are
			// both logged-and-dropped: the HCS callback thread must
			// never block, and a malformed payload can't be acted on.
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
