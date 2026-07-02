//go:build windows

package migration

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func ptr[T any](v T) *T { return &v }

// TestInitializeOptionsFromProto verifies the wire init options are converted to
// the HCS options: nil and unsupported transports are handled, nested params are
// copied (or left nil), and an out-of-range throttle percentage wraps to uint8.
func TestInitializeOptionsFromProto(t *testing.T) {
	tests := []struct {
		name    string
		in      *InitializeOptions
		want    *hcsschema.MigrationInitializeOptions
		wantErr bool
	}{
		{
			name: "nil input returns nil",
		},
		{
			name:    "unsupported transport returns error",
			in:      &InitializeOptions{MemoryTransport: MemoryTransport_MEMORY_TRANSPORT_UNSPECIFIED},
			wantErr: true,
		},
		{
			name: "nil nested params are preserved as nil",
			in: &InitializeOptions{
				MemoryTransport:      MemoryTransport_MEMORY_TRANSPORT_TCP,
				ChecksumVerification: true,
			},
			want: &hcsschema.MigrationInitializeOptions{
				MemoryTransport:      hcsschema.MigrationMemoryTransportTCP,
				ChecksumVerification: true,
			},
		},
		{
			name: "full conversion with nested params",
			in: &InitializeOptions{
				MemoryTransport: MemoryTransport_MEMORY_TRANSPORT_TCP,
				MemoryTransferThrottleParams: &MemoryTransferThrottleParams{
					SkipThrottling:                              ptr(true),
					ThrottlingScale:                             ptr(42.5),
					MinimumThrottlePercentage:                   ptr(uint32(50)),
					TargetNumberOfBrownoutTransferPasses:        ptr(uint32(3)),
					StartingBrownoutPassNumberForThrottling:     ptr(uint32(1)),
					MaximumNumberOfBrownoutTransferPasses:       ptr(uint32(7)),
					TargetBlackoutTransferTime:                  ptr(uint32(100)),
					BlackoutTimeThresholdForCancellingMigration: ptr(uint32(200)),
				},
				CompressionSettings:              &CompressionSettings{ThrottleWorkerCount: ptr(uint32(4))},
				ChecksumVerification:             true,
				PerfTracingEnabled:               true,
				CancelIfBlackoutThresholdExceeds: true,
				PrepareMemoryTransferMode:        true,
			},
			want: &hcsschema.MigrationInitializeOptions{
				MemoryTransport: hcsschema.MigrationMemoryTransportTCP,
				MemoryTransferThrottleParams: &hcsschema.MemoryMigrationTransferThrottleParams{
					SkipThrottling:                              ptr(true),
					ThrottlingScale:                             ptr(42.5),
					MinimumThrottlePercentage:                   ptr(uint8(50)),
					TargetNumberOfBrownoutTransferPasses:        ptr(uint32(3)),
					StartingBrownoutPassNumberForThrottling:     ptr(uint32(1)),
					MaximumNumberOfBrownoutTransferPasses:       ptr(uint32(7)),
					TargetBlackoutTransferTime:                  ptr(uint32(100)),
					BlackoutTimeThresholdForCancellingMigration: ptr(uint32(200)),
				},
				CompressionSettings:              &hcsschema.MigrationCompressionSettings{ThrottleWorkerCount: ptr(uint32(4))},
				ChecksumVerification:             true,
				PerfTracingEnabled:               true,
				CancelIfBlackoutThresholdExceeds: true,
				PrepareMemoryTransferMode:        true,
			},
		},
		{
			// A throttle percentage above 255 wraps when narrowed to uint8.
			name: "minimum throttle percentage truncates to uint8",
			in: &InitializeOptions{
				MemoryTransport:              MemoryTransport_MEMORY_TRANSPORT_TCP,
				MemoryTransferThrottleParams: &MemoryTransferThrottleParams{MinimumThrottlePercentage: ptr(uint32(300))},
			},
			want: &hcsschema.MigrationInitializeOptions{
				MemoryTransport:              hcsschema.MigrationMemoryTransportTCP,
				MemoryTransferThrottleParams: &hcsschema.MemoryMigrationTransferThrottleParams{MinimumThrottlePercentage: ptr(uint8(44))},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := InitializeOptionsFromProto(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestToOrigin verifies a caller sees the migration side mapped to the wire
// origin, with the controller-known fallback used only when HCS omits the origin
// and unknown origins reported as unspecified.
func TestToOrigin(t *testing.T) {
	tests := []struct {
		name     string
		origin   hcsschema.MigrationOrigin
		fallback hcsschema.MigrationOrigin
		want     Origin
	}{
		{name: "empty falls back to source", fallback: hcsschema.MigrationOriginSource, want: Origin_ORIGIN_SOURCE},
		{name: "empty falls back to destination", fallback: hcsschema.MigrationOriginDestination, want: Origin_ORIGIN_DESTINATION},
		{name: "empty with empty fallback is unspecified", want: Origin_ORIGIN_UNSPECIFIED},
		{name: "source ignores fallback", origin: hcsschema.MigrationOriginSource, fallback: hcsschema.MigrationOriginDestination, want: Origin_ORIGIN_SOURCE},
		{name: "destination", origin: hcsschema.MigrationOriginDestination, want: Origin_ORIGIN_DESTINATION},
		{name: "unknown non-empty origin ignores fallback", origin: hcsschema.MigrationOrigin("Bogus"), fallback: hcsschema.MigrationOriginSource, want: Origin_ORIGIN_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToOrigin(tt.origin, tt.fallback); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// TestToPhase verifies every HCS migration event maps to its wire phase and that
// unknown or empty events fall back to the unspecified phase.
func TestToPhase(t *testing.T) {
	tests := []struct {
		event hcsschema.MigrationEvent
		want  Phase
	}{
		{hcsschema.MigrationEventSetupDone, Phase_PHASE_SETUP_DONE},
		{hcsschema.MigrationEventTransferInProgress, Phase_PHASE_TRANSFER_IN_PROGRESS},
		{hcsschema.MigrationEventBlackoutStarted, Phase_PHASE_BLACKOUT_STARTED},
		{hcsschema.MigrationEventOfflineDone, Phase_PHASE_OFFLINE_DONE},
		{hcsschema.MigrationEventBlackoutExited, Phase_PHASE_BLACKOUT_EXITED},
		{hcsschema.MigrationEventMigrationDone, Phase_PHASE_DONE},
		{hcsschema.MigrationEventMigrationRecoveryDone, Phase_PHASE_RECOVERY_DONE},
		{hcsschema.MigrationEventMigrationFailed, Phase_PHASE_FAILED},
		{hcsschema.MigrationEventUnknown, Phase_PHASE_UNSPECIFIED},
		{hcsschema.MigrationEvent(""), Phase_PHASE_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(string(tt.event), func(t *testing.T) {
			if got := ToPhase(tt.event); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// TestToPhaseState verifies an HCS result takes precedence when mapping to the
// wire state, and that with no result the phase decides the outcome: progress
// phases default to success while terminal/unspecified phases stay unspecified.
func TestToPhaseState(t *testing.T) {
	tests := []struct {
		name   string
		result hcsschema.MigrationResult
		phase  Phase
		want   PhaseState
	}{
		{name: "success", result: hcsschema.MigrationResultSuccess, want: PhaseState_PHASE_STATE_SUCCESS},
		{name: "cancelled", result: hcsschema.MigrationResultMigrationCancelled, want: PhaseState_PHASE_STATE_CANCELLED},
		{name: "guest cancellation", result: hcsschema.MigrationResultGuestInitiatedCancellation, want: PhaseState_PHASE_STATE_GUEST_INITIATED_CANCELLATION},
		{name: "source failed", result: hcsschema.MigrationResultSourceMigrationFailed, want: PhaseState_PHASE_STATE_SOURCE_FAILED},
		{name: "destination failed", result: hcsschema.MigrationResultDestinationMigrationFailed, want: PhaseState_PHASE_STATE_DESTINATION_FAILED},
		{name: "recovery failed", result: hcsschema.MigrationResultMigrationRecoveryFailed, want: PhaseState_PHASE_STATE_RECOVERY_FAILED},
		{name: "result wins over phase", result: hcsschema.MigrationResultSuccess, phase: Phase_PHASE_FAILED, want: PhaseState_PHASE_STATE_SUCCESS},
		{name: "no result progress phase defaults to success", phase: Phase_PHASE_TRANSFER_IN_PROGRESS, want: PhaseState_PHASE_STATE_SUCCESS},
		{name: "no result terminal phase is unspecified", phase: Phase_PHASE_DONE, want: PhaseState_PHASE_STATE_UNSPECIFIED},
		{name: "no result unspecified phase is unspecified", phase: Phase_PHASE_UNSPECIFIED, want: PhaseState_PHASE_STATE_UNSPECIFIED},
		{name: "invalid result falls through to progress phase", result: hcsschema.MigrationResultInvalid, phase: Phase_PHASE_SETUP_DONE, want: PhaseState_PHASE_STATE_SUCCESS},
		{name: "invalid result falls through to terminal phase", result: hcsschema.MigrationResultInvalid, phase: Phase_PHASE_FAILED, want: PhaseState_PHASE_STATE_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToPhaseState(tt.result, tt.phase); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// TestToNotification verifies an HCS notification is converted to its wire form:
// origin/phase/state are mapped (with fallback origin), and blackout-exited
// details are attached only on a valid payload and dropped otherwise.
func TestToNotification(t *testing.T) {
	stopTime := time.Unix(1700000000, 0).UTC()
	blackoutDetails, err := json.Marshal(hcsschema.BlackoutExitedEventDetails{
		BlackoutDurationMilliseconds: 1234,
		BlackoutStopTimestamp:        stopTime,
	})
	if err != nil {
		t.Fatalf("marshal details: %v", err)
	}

	tests := []struct {
		name     string
		info     hcsschema.OperationSystemMigrationNotificationInfo
		fallback hcsschema.MigrationOrigin
		want     *Notification
	}{
		{
			name: "maps origin phase and state",
			info: hcsschema.OperationSystemMigrationNotificationInfo{
				Origin: hcsschema.MigrationOriginSource,
				Event:  hcsschema.MigrationEventMigrationFailed,
				Result: hcsschema.MigrationResultSourceMigrationFailed,
			},
			want: &Notification{
				Origin: Origin_ORIGIN_SOURCE,
				Phase:  Phase_PHASE_FAILED,
				State:  PhaseState_PHASE_STATE_SOURCE_FAILED,
			},
		},
		{
			name:     "empty origin uses fallback",
			info:     hcsschema.OperationSystemMigrationNotificationInfo{Event: hcsschema.MigrationEventSetupDone},
			fallback: hcsschema.MigrationOriginDestination,
			want: &Notification{
				Origin: Origin_ORIGIN_DESTINATION,
				Phase:  Phase_PHASE_SETUP_DONE,
				State:  PhaseState_PHASE_STATE_SUCCESS,
			},
		},
		{
			name: "blackout exited with valid details",
			info: hcsschema.OperationSystemMigrationNotificationInfo{
				Origin:            hcsschema.MigrationOriginSource,
				Event:             hcsschema.MigrationEventBlackoutExited,
				AdditionalDetails: blackoutDetails,
			},
			want: &Notification{
				Origin: Origin_ORIGIN_SOURCE,
				Phase:  Phase_PHASE_BLACKOUT_EXITED,
				State:  PhaseState_PHASE_STATE_SUCCESS,
				PhaseDetails: &Notification_BlackoutExited{
					BlackoutExited: &BlackoutExitedEventDetails{
						BlackoutDurationMilliseconds: 1234,
						BlackoutStopTimestamp:        timestamppb.New(stopTime),
					},
				},
			},
		},
		{
			name: "blackout exited with invalid details drops phase details",
			info: hcsschema.OperationSystemMigrationNotificationInfo{
				Origin:            hcsschema.MigrationOriginSource,
				Event:             hcsschema.MigrationEventBlackoutExited,
				AdditionalDetails: json.RawMessage("{invalid"),
			},
			want: &Notification{
				Origin: Origin_ORIGIN_SOURCE,
				Phase:  Phase_PHASE_BLACKOUT_EXITED,
				State:  PhaseState_PHASE_STATE_SUCCESS,
			},
		},
		{
			name: "blackout exited without details has no phase details",
			info: hcsschema.OperationSystemMigrationNotificationInfo{
				Event: hcsschema.MigrationEventBlackoutExited,
			},
			want: &Notification{
				Phase: Phase_PHASE_BLACKOUT_EXITED,
				State: PhaseState_PHASE_STATE_SUCCESS,
			},
		},
		{
			name: "details ignored for non-blackout event",
			info: hcsschema.OperationSystemMigrationNotificationInfo{
				Event:             hcsschema.MigrationEventSetupDone,
				AdditionalDetails: blackoutDetails,
			},
			want: &Notification{
				Phase: Phase_PHASE_SETUP_DONE,
				State: PhaseState_PHASE_STATE_SUCCESS,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToNotification(tt.info, tt.fallback)
			if !proto.Equal(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
