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
// The handler under test reads its arguments as raw uintptrs that originate
// outside the Go heap (HCS hands them to us via a syscall callback). To
// faithfully exercise that contract — and the cgo pointer-passing rules it
// implies — the helpers below allocate the HcsEvent, the UTF-16 EventData
// buffer, and the channel context out of process heap memory via LocalAlloc.
// All allocations are bound to the test's lifetime through t.Cleanup, so the
// individual tests stay free of teardown bookkeeping.
// ─────────────────────────────────────────────────────────────────────────────

// allocCEvent returns a uintptr to a LocalAlloc'd HcsEvent. If payload is
// non-empty it is encoded as UTF-16 into a second LocalAlloc'd buffer and
// wired up as EventData; otherwise EventData is left nil.
func allocCEvent(t *testing.T, payload string) uintptr {
	t.Helper()

	evtAddr, err := windows.LocalAlloc(windows.LPTR, uint32(unsafe.Sizeof(computecore.HcsEvent{})))
	if err != nil {
		t.Fatalf("LocalAlloc(event): %v", err)
	}
	t.Cleanup(func() { _, _ = windows.LocalFree(windows.Handle(evtAddr)) })

	e := (*computecore.HcsEvent)(unsafe.Pointer(evtAddr))
	e.Type = computecore.HcsEventTypeGroupLiveMigration

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

// allocCChanCtx stores ch in a LocalAlloc'd buffer and returns its address,
// so the handler reads the chan header out of C memory rather than the Go heap
// (matching how HCS delivers the registered callback context).
func allocCChanCtx(t *testing.T, ch chan hcsschema.OperationSystemMigrationNotificationInfo) uintptr {
	t.Helper()
	addr, err := windows.LocalAlloc(windows.LPTR, uint32(unsafe.Sizeof(ch)))
	if err != nil {
		t.Fatalf("LocalAlloc(ctx): %v", err)
	}
	t.Cleanup(func() { _, _ = windows.LocalFree(windows.Handle(addr)) })

	*(*chan hcsschema.OperationSystemMigrationNotificationInfo)(unsafe.Pointer(addr)) = ch
	return addr
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
// Nil-argument guards
// ─────────────────────────────────────────────────────────────────────────────

// TestMigrationCallbackHandler_NilArgs verifies that the handler is a no-op
// (returns 0, sends nothing on the channel) when either argument is zero.
func TestMigrationCallbackHandler_NilArgs(t *testing.T) {
	ch := make(chan hcsschema.OperationSystemMigrationNotificationInfo, 1)

	cases := []struct {
		name       string
		event, ctx uintptr
	}{
		{"BothZero", 0, 0},
		{"EventZero", 0, allocCChanCtx(t, ch)},
		{"CtxZero", allocCEvent(t, ""), 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if ret := migrationCallbackHandler(tc.event, tc.ctx); ret != 0 {
				t.Fatalf("expected 0, got %d", ret)
			}
		})
	}
	expectNoNotification(t, ch)
}

// ─────────────────────────────────────────────────────────────────────────────
// Payload decoding
// ─────────────────────────────────────────────────────────────────────────────

// TestMigrationCallbackHandler_Payloads verifies that real-world HCS
// GroupLiveMigration JSON payloads — including a nil EventData pointer — are
// decoded and forwarded on the notification channel.
func TestMigrationCallbackHandler_Payloads(t *testing.T) {
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
			evt := allocCEvent(t, tc.payload)
			ctx := allocCChanCtx(t, ch)

			if ret := migrationCallbackHandler(evt, ctx); ret != 0 {
				t.Fatalf("expected 0, got %d", ret)
			}
			expectNotification(t, ch, tc.want)
		})
	}
}

// TestMigrationCallbackHandler_InvalidJSONDropped verifies that an
// unparseable EventData payload is logged and dropped without sending.
func TestMigrationCallbackHandler_InvalidJSONDropped(t *testing.T) {
	ch := make(chan hcsschema.OperationSystemMigrationNotificationInfo, 1)
	evt := allocCEvent(t, "not-json")
	ctx := allocCChanCtx(t, ch)

	if ret := migrationCallbackHandler(evt, ctx); ret != 0 {
		t.Fatalf("expected 0, got %d", ret)
	}
	expectNoNotification(t, ch)
}

// TestMigrationCallbackHandler_AdditionalDetailsDecodes verifies that the
// raw JSON captured in AdditionalDetails for a BlackoutExited event can be
// decoded by the consumer into the concrete BlackoutExitedEventDetails struct.
// This is the contract that motivates modeling AdditionalDetails as
// json.RawMessage rather than a typed *interface{}.
func TestMigrationCallbackHandler_AdditionalDetailsDecodes(t *testing.T) {
	ch := make(chan hcsschema.OperationSystemMigrationNotificationInfo, 1)
	evt := allocCEvent(t, `{"Event":"BlackoutExited","Result":"Success","AdditionalDetails":{"BlackoutDurationMilliseconds":1234,"BlackoutStopTimestamp":"2026-04-23T12:34:56Z"}}`)
	ctx := allocCChanCtx(t, ch)

	if ret := migrationCallbackHandler(evt, ctx); ret != 0 {
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

// TestMigrationCallbackHandler_AdditionalDetailsAbsent verifies that a
// payload without an AdditionalDetails field results in a nil
// json.RawMessage on the forwarded notification.
func TestMigrationCallbackHandler_AdditionalDetailsAbsent(t *testing.T) {
	ch := make(chan hcsschema.OperationSystemMigrationNotificationInfo, 1)
	evt := allocCEvent(t, `{"Event":"SetupDone"}`)
	ctx := allocCChanCtx(t, ch)

	if ret := migrationCallbackHandler(evt, ctx); ret != 0 {
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

// TestMigrationCallbackHandler_FullChannelDropsEvent verifies that when the
// notification channel is full the handler drops the new event rather than
// blocking the HCS callback thread.
func TestMigrationCallbackHandler_FullChannelDropsEvent(t *testing.T) {
	ch := make(chan hcsschema.OperationSystemMigrationNotificationInfo, 1)

	// Pre-fill the channel so the next send would block.
	prefill := hcsschema.OperationSystemMigrationNotificationInfo{Event: hcsschema.MigrationEventSetupDone}
	ch <- prefill

	evt := allocCEvent(t, `{"Event":"MigrationDone"}`)
	ctx := allocCChanCtx(t, ch)

	if ret := migrationCallbackHandler(evt, ctx); ret != 0 {
		t.Fatalf("expected 0, got %d", ret)
	}

	// The original prefill must still be the only entry (new event dropped).
	if got := <-ch; !reflect.DeepEqual(got, prefill) {
		t.Fatalf("expected prefill to remain, got %+v", got)
	}
	expectNoNotification(t, ch)
}
