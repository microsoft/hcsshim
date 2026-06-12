//go:build windows

package hcs

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/Microsoft/hcsshim/internal/computecore"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"golang.org/x/sys/windows"
)

// ─────────────────────────────────────────────────────────────────────────────
// Test helpers
//
// notificationHandler has the signature (event, ctx uintptr), matching the
// raw HCS syscall callback. The two arguments are built very differently in
// tests:
//
//  1. event — a pointer to an HcsEvent struct that, in production, lives in
//     memory HCS allocated outside the Go heap. To honor the cgo rule that
//     such pointers must not refer to Go-managed memory, allocCEvent
//     allocates the HcsEvent (and any UTF-16 EventData buffer) with
//     LocalAlloc rather than using a Go &HcsEvent{}.
//
//  2. ctx — not a pointer at all, but an opaque integer key into the
//     package-level notificationContexts map. Real Go pointers can't be
//     handed to HCS across the callback boundary, so each registration
//     stores its state (channel, etc.) in that map and gives HCS only the
//     ID. Tests use registerSystemCtx to insert an entry pointing at their
//     channel and pass the returned ID straight into notificationHandler.
// ─────────────────────────────────────────────────────────────────────────────

// allocCEvent returns a uintptr to a LocalAlloc'd HcsEvent of the given type.
// If payload is non-empty it is encoded as UTF-16 into a second LocalAlloc'd
// buffer and wired up as EventData; otherwise EventData is left nil.
func allocCEvent(t *testing.T, eventType computecore.HcsEventType, payload string) uintptr {
	t.Helper()

	evtAddr, err := windows.LocalAlloc(windows.LPTR, uint32(unsafe.Sizeof(computecore.HcsEvent{})))
	if err != nil {
		t.Fatalf("LocalAlloc(event): %v", err)
	}
	t.Cleanup(func() { _, _ = windows.LocalFree(windows.Handle(evtAddr)) })

	e := (*computecore.HcsEvent)(unsafe.Pointer(evtAddr))
	e.Type = eventType

	if payload == "" {
		return evtAddr
	}

	utf16, err := windows.UTF16FromString(payload)
	if err != nil {
		t.Fatalf("UTF16FromString: %v", err)
	}
	// UTF-16 code units are 2 bytes by definition.
	dataAddr, err := windows.LocalAlloc(windows.LPTR, uint32(len(utf16)*2))
	if err != nil {
		t.Fatalf("LocalAlloc(data): %v", err)
	}
	t.Cleanup(func() { _, _ = windows.LocalFree(windows.Handle(dataAddr)) })

	// Copy the UTF-16 sequence (including the trailing NUL from UTF16FromString)
	// into the C buffer.
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(dataAddr)), len(utf16)), utf16)
	e.EventData = (*uint16)(unsafe.Pointer(dataAddr))
	return evtAddr
}

// registerSystemCtx registers a fresh system-style notificationContext that
// forwards GroupLiveMigration events to ch and returns its lookup ID as a
// uintptr ready to pass to notificationHandler. Cleanup is registered on t.
func registerSystemCtx(t *testing.T, ch chan hcsschema.OperationSystemMigrationNotificationInfo) uintptr {
	t.Helper()
	id := registerNotificationContext("test-system", 0, nil, ch)
	t.Cleanup(func() { unregisterNotificationContext(id) })
	return uintptr(id)
}

// expectNotification fails the test unless want is the next queued value on ch.
func expectNotification(t *testing.T, ch <-chan hcsschema.OperationSystemMigrationNotificationInfo, want hcsschema.OperationSystemMigrationNotificationInfo) {
	t.Helper()
	select {
	case got := <-ch:
		// OperationSystemMigrationNotificationInfo contains a json.RawMessage
		// (a []byte) and is therefore not comparable with ==.
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("notification mismatch: got %+v want %+v", got, want)
		}
	default:
		t.Fatal("expected a notification on the channel")
	}
}

// expectNoNotification fails the test if a notification is queued on ch.
func expectNoNotification(t *testing.T, ch <-chan hcsschema.OperationSystemMigrationNotificationInfo) {
	t.Helper()
	select {
	case got := <-ch:
		t.Fatalf("did not expect a notification, got %+v", got)
	default:
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Nil / unknown context guards
// ─────────────────────────────────────────────────────────────────────────────

// TestNotificationHandler_LM_NilOrUnknownArgs verifies that the handler is a
// no-op (returns 0, sends nothing on the channel) when the event pointer is
// zero or the context ID does not resolve to a registered entry.
func TestNotificationHandler_LM_NilOrUnknownArgs(t *testing.T) {
	ch := make(chan hcsschema.OperationSystemMigrationNotificationInfo, 1)
	ctx := registerSystemCtx(t, ch)

	cases := []struct {
		name       string
		event, ctx uintptr
	}{
		{"BothZero", 0, 0},
		{"EventZero", 0, ctx},
		// A non-zero but never-registered ID must miss the lookup
		// silently rather than dispatch or panic.
		{"UnknownCtx", allocCEvent(t, computecore.HcsEventTypeGroupLiveMigration, `{"Event":"SetupDone"}`), ^uintptr(0)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if ret := notificationHandler(tc.event, tc.ctx); ret != 0 {
				t.Fatalf("expected 0, got %d", ret)
			}
		})
	}
	expectNoNotification(t, ch)
}

// ─────────────────────────────────────────────────────────────────────────────
// Payload decoding
// ─────────────────────────────────────────────────────────────────────────────

// TestNotificationHandler_LM_Payloads verifies that real-world HCS
// GroupLiveMigration JSON payloads — including a nil EventData pointer — are
// decoded and forwarded on the notification channel.
func TestNotificationHandler_LM_Payloads(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    hcsschema.OperationSystemMigrationNotificationInfo
	}{
		{
			name: "NilEventData",
			// payload "" => EventData pointer is nil; want is the zero value.
		},
		{
			name:    "SetupDone",
			payload: `{"Event":"SetupDone"}`,
			want:    hcsschema.OperationSystemMigrationNotificationInfo{Event: hcsschema.MigrationEventSetupDone},
		},
		{
			name:    "BlackoutStarted",
			payload: `{"Event":"BlackoutStarted"}`,
			want:    hcsschema.OperationSystemMigrationNotificationInfo{Event: hcsschema.MigrationEventBlackoutStarted},
		},
		{
			name:    "OfflineDoneSuccess",
			payload: `{"Event":"OfflineDone","Result":"Success"}`,
			want: hcsschema.OperationSystemMigrationNotificationInfo{
				Event:  hcsschema.MigrationEventOfflineDone,
				Result: hcsschema.MigrationResultSuccess,
			},
		},
		{
			name:    "MigrationDoneSuccess",
			payload: `{"Event":"MigrationDone","Result":"Success"}`,
			want: hcsschema.OperationSystemMigrationNotificationInfo{
				Event:  hcsschema.MigrationEventMigrationDone,
				Result: hcsschema.MigrationResultSuccess,
			},
		},
		{
			name:    "WithOrigin",
			payload: `{"Origin":"Source","Event":"MigrationDone","Result":"Success"}`,
			want: hcsschema.OperationSystemMigrationNotificationInfo{
				Origin: hcsschema.MigrationOriginSource,
				Event:  hcsschema.MigrationEventMigrationDone,
				Result: hcsschema.MigrationResultSuccess,
			},
		},
		{
			// AdditionalDetails is modeled as the HCS schema `Any` type and
			// stored as json.RawMessage so callers can decode it into the
			// concrete struct based on Event. Verify the raw bytes are
			// preserved verbatim through the decode/forward path.
			name:    "BlackoutExitedWithAdditionalDetails",
			payload: `{"Event":"BlackoutExited","Result":"Success","AdditionalDetails":{"BlackoutDurationMilliseconds":1234,"BlackoutStopTimestamp":"2026-04-23T12:34:56Z"}}`,
			want: hcsschema.OperationSystemMigrationNotificationInfo{
				Event:             hcsschema.MigrationEventBlackoutExited,
				Result:            hcsschema.MigrationResultSuccess,
				AdditionalDetails: json.RawMessage(`{"BlackoutDurationMilliseconds":1234,"BlackoutStopTimestamp":"2026-04-23T12:34:56Z"}`),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ch := make(chan hcsschema.OperationSystemMigrationNotificationInfo, 1)
			ctx := registerSystemCtx(t, ch)
			evt := allocCEvent(t, computecore.HcsEventTypeGroupLiveMigration, tc.payload)

			if ret := notificationHandler(evt, ctx); ret != 0 {
				t.Fatalf("expected 0, got %d", ret)
			}
			expectNotification(t, ch, tc.want)
		})
	}
}

// TestNotificationHandler_LM_InvalidJSONDropped verifies that an unparseable
// EventData payload is logged and dropped without sending.
func TestNotificationHandler_LM_InvalidJSONDropped(t *testing.T) {
	ch := make(chan hcsschema.OperationSystemMigrationNotificationInfo, 1)
	ctx := registerSystemCtx(t, ch)
	evt := allocCEvent(t, computecore.HcsEventTypeGroupLiveMigration, "not-json")

	if ret := notificationHandler(evt, ctx); ret != 0 {
		t.Fatalf("expected 0, got %d", ret)
	}
	expectNoNotification(t, ch)
}

// TestNotificationHandler_LM_AdditionalDetailsDecodes verifies that the raw
// JSON captured in AdditionalDetails for a BlackoutExited event can be
// decoded by the consumer into the concrete BlackoutExitedEventDetails
// struct. This is the contract that motivates modeling AdditionalDetails as
// json.RawMessage rather than a typed *interface{}.
func TestNotificationHandler_LM_AdditionalDetailsDecodes(t *testing.T) {
	ch := make(chan hcsschema.OperationSystemMigrationNotificationInfo, 1)
	ctx := registerSystemCtx(t, ch)
	evt := allocCEvent(t, computecore.HcsEventTypeGroupLiveMigration,
		`{"Event":"BlackoutExited","Result":"Success","AdditionalDetails":{"BlackoutDurationMilliseconds":1234,"BlackoutStopTimestamp":"2026-04-23T12:34:56Z"}}`)

	if ret := notificationHandler(evt, ctx); ret != 0 {
		t.Fatalf("expected 0, got %d", ret)
	}

	var got hcsschema.OperationSystemMigrationNotificationInfo
	select {
	case got = <-ch:
	default:
		t.Fatal("expected a notification on the channel")
	}

	if got.Event != hcsschema.MigrationEventBlackoutExited {
		t.Fatalf("unexpected event: %q", got.Event)
	}
	if len(got.AdditionalDetails) == 0 {
		t.Fatal("expected AdditionalDetails to be populated")
	}

	var details hcsschema.BlackoutExitedEventDetails
	if err := json.Unmarshal(got.AdditionalDetails, &details); err != nil {
		t.Fatalf("decode AdditionalDetails: %v", err)
	}

	wantTS, err := time.Parse(time.RFC3339, "2026-04-23T12:34:56Z")
	if err != nil {
		t.Fatalf("parse want timestamp: %v", err)
	}
	want := hcsschema.BlackoutExitedEventDetails{
		BlackoutDurationMilliseconds: 1234,
		BlackoutStopTimestamp:        wantTS,
	}
	if !details.BlackoutStopTimestamp.Equal(want.BlackoutStopTimestamp) ||
		details.BlackoutDurationMilliseconds != want.BlackoutDurationMilliseconds {
		t.Fatalf("decoded details mismatch: got %+v want %+v", details, want)
	}
}

// TestNotificationHandler_LM_AdditionalDetailsAbsent verifies that a
// payload without an AdditionalDetails field results in a nil
// json.RawMessage on the forwarded notification.
func TestNotificationHandler_LM_AdditionalDetailsAbsent(t *testing.T) {
	ch := make(chan hcsschema.OperationSystemMigrationNotificationInfo, 1)
	ctx := registerSystemCtx(t, ch)
	evt := allocCEvent(t, computecore.HcsEventTypeGroupLiveMigration, `{"Event":"SetupDone"}`)

	if ret := notificationHandler(evt, ctx); ret != 0 {
		t.Fatalf("expected 0, got %d", ret)
	}

	got := <-ch
	if got.AdditionalDetails != nil {
		t.Fatalf("expected nil AdditionalDetails, got %q", string(got.AdditionalDetails))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Backpressure
// ─────────────────────────────────────────────────────────────────────────────

// TestNotificationHandler_LM_FullChannelDropsEvent verifies that when the
// notification channel is full the handler drops the new event rather than
// blocking the HCS callback thread.
func TestNotificationHandler_LM_FullChannelDropsEvent(t *testing.T) {
	ch := make(chan hcsschema.OperationSystemMigrationNotificationInfo, 1)
	ctx := registerSystemCtx(t, ch)

	// Pre-fill the channel so the next send would block.
	prefill := hcsschema.OperationSystemMigrationNotificationInfo{Event: hcsschema.MigrationEventSetupDone}
	ch <- prefill

	evt := allocCEvent(t, computecore.HcsEventTypeGroupLiveMigration, `{"Event":"MigrationDone"}`)

	if ret := notificationHandler(evt, ctx); ret != 0 {
		t.Fatalf("expected 0, got %d", ret)
	}

	// The original prefill must still be the only entry (new event dropped).
	if got := <-ch; !reflect.DeepEqual(got, prefill) {
		t.Fatalf("expected prefill to remain, got %+v", got)
	}
	expectNoNotification(t, ch)
}

// ─────────────────────────────────────────────────────────────────────────────
// Event-type routing
// ─────────────────────────────────────────────────────────────────────────────

// TestNotificationHandler_NonLMEvent_NotDispatched verifies that a
// non-GroupLiveMigration event does not land on the migration channel even
// when a channel is registered. This guards the dispatch switch in
// notificationHandler.
func TestNotificationHandler_NonLMEvent_NotDispatched(t *testing.T) {
	ch := make(chan hcsschema.OperationSystemMigrationNotificationInfo, 1)
	ctx := registerSystemCtx(t, ch)

	// SystemExited is a terminal exit event; without a notificationState
	// registered (nil above) it must not panic and must not send anything
	// onto the migration channel.
	evt := allocCEvent(t, computecore.HcsEventTypeSystemExited, `{"Status":0}`)

	if ret := notificationHandler(evt, ctx); ret != 0 {
		t.Fatalf("expected 0, got %d", ret)
	}
	expectNoNotification(t, ch)
}
