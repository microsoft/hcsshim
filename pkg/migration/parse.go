//go:build windows

package migration

import (
	"encoding/json"
	"fmt"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// InitializeOptionsFromProto converts a protobuf [InitializeOptions] to the
// HCS schema [hcsschema.MigrationInitializeOptions].
func InitializeOptionsFromProto(p *InitializeOptions) (*hcsschema.MigrationInitializeOptions, error) {
	if p == nil {
		return nil, nil
	}

	memoryTransport, err := memoryTransportFromProto(p.MemoryTransport)
	if err != nil {
		return nil, fmt.Errorf("convert memory transport: %w", err)
	}
	return &hcsschema.MigrationInitializeOptions{
		MemoryTransport:                  memoryTransport,
		MemoryTransferThrottleParams:     throttleParamsFromProto(p.MemoryTransferThrottleParams),
		CompressionSettings:              compressionSettingsFromProto(p.CompressionSettings),
		ChecksumVerification:             p.ChecksumVerification,
		PerfTracingEnabled:               p.PerfTracingEnabled,
		CancelIfBlackoutThresholdExceeds: p.CancelIfBlackoutThresholdExceeds,
		PrepareMemoryTransferMode:        p.PrepareMemoryTransferMode,
	}, nil
}

// memoryTransportFromProto converts a protobuf [MemoryTransport] enum value to its HCS [hcsschema.MigrationMemoryTransport] equivalent.
// It returns an error for any value other than TCP, since HCS requires a valid memory transport to start migration.
func memoryTransportFromProto(t MemoryTransport) (hcsschema.MigrationMemoryTransport, error) {
	switch t {
	case MemoryTransport_MEMORY_TRANSPORT_TCP:
		return hcsschema.MigrationMemoryTransportTCP, nil
	default:
		return "", fmt.Errorf("unsupported memory transport %q", t)
	}
}

// throttleParamsFromProto converts a protobuf [MemoryTransferThrottleParams] to its HCS [hcsschema.MemoryMigrationTransferThrottleParams] equivalent.
func throttleParamsFromProto(p *MemoryTransferThrottleParams) *hcsschema.MemoryMigrationTransferThrottleParams {
	if p == nil {
		return nil
	}
	s := &hcsschema.MemoryMigrationTransferThrottleParams{
		SkipThrottling:                              p.SkipThrottling,
		ThrottlingScale:                             p.ThrottlingScale,
		TargetNumberOfBrownoutTransferPasses:        p.TargetNumberOfBrownoutTransferPasses,
		StartingBrownoutPassNumberForThrottling:     p.StartingBrownoutPassNumberForThrottling,
		MaximumNumberOfBrownoutTransferPasses:       p.MaximumNumberOfBrownoutTransferPasses,
		TargetBlackoutTransferTime:                  p.TargetBlackoutTransferTime,
		BlackoutTimeThresholdForCancellingMigration: p.BlackoutTimeThresholdForCancellingMigration,
	}
	if p.MinimumThrottlePercentage != nil {
		v := uint8(*p.MinimumThrottlePercentage)
		s.MinimumThrottlePercentage = &v
	}
	return s
}

// compressionSettingsFromProto converts a protobuf [CompressionSettings] to its HCS [hcsschema.MigrationCompressionSettings] equivalent.
func compressionSettingsFromProto(p *CompressionSettings) *hcsschema.MigrationCompressionSettings {
	if p == nil {
		return nil
	}
	return &hcsschema.MigrationCompressionSettings{
		ThrottleWorkerCount: p.ThrottleWorkerCount,
	}
}

// ToOrigin maps an HCS migration origin to its wire form, falling back to the
// controller-known origin when HCS leaves the field empty.
func ToOrigin(origin, fallback hcsschema.MigrationOrigin) Origin {
	if origin == "" {
		origin = fallback
	}

	switch origin {
	case hcsschema.MigrationOriginSource:
		return Origin_ORIGIN_SOURCE
	case hcsschema.MigrationOriginDestination:
		return Origin_ORIGIN_DESTINATION
	}

	return Origin_ORIGIN_UNSPECIFIED
}

// ToPhase maps an HCS migration event to its wire-form phase, returning
// PHASE_UNSPECIFIED for an unrecognized event.
func ToPhase(event hcsschema.MigrationEvent) Phase {
	switch event {
	case hcsschema.MigrationEventSetupDone:
		return Phase_PHASE_SETUP_DONE
	case hcsschema.MigrationEventTransferInProgress:
		return Phase_PHASE_TRANSFER_IN_PROGRESS
	case hcsschema.MigrationEventBlackoutStarted:
		return Phase_PHASE_BLACKOUT_STARTED
	case hcsschema.MigrationEventOfflineDone:
		return Phase_PHASE_OFFLINE_DONE
	case hcsschema.MigrationEventBlackoutExited:
		return Phase_PHASE_BLACKOUT_EXITED
	case hcsschema.MigrationEventMigrationDone:
		return Phase_PHASE_DONE
	case hcsschema.MigrationEventMigrationRecoveryDone:
		return Phase_PHASE_RECOVERY_DONE
	case hcsschema.MigrationEventMigrationFailed:
		return Phase_PHASE_FAILED
	}

	return Phase_PHASE_UNSPECIFIED
}

// ToPhaseState maps an HCS migration result to its wire-form state.
func ToPhaseState(result hcsschema.MigrationResult, phase Phase) PhaseState {
	switch result {
	case hcsschema.MigrationResultSuccess:
		return PhaseState_PHASE_STATE_SUCCESS
	case hcsschema.MigrationResultMigrationCancelled:
		return PhaseState_PHASE_STATE_CANCELLED
	case hcsschema.MigrationResultGuestInitiatedCancellation:
		return PhaseState_PHASE_STATE_GUEST_INITIATED_CANCELLATION
	case hcsschema.MigrationResultSourceMigrationFailed:
		return PhaseState_PHASE_STATE_SOURCE_FAILED
	case hcsschema.MigrationResultDestinationMigrationFailed:
		return PhaseState_PHASE_STATE_DESTINATION_FAILED
	case hcsschema.MigrationResultMigrationRecoveryFailed:
		return PhaseState_PHASE_STATE_RECOVERY_FAILED
	}

	// No HCS result: progress phases imply forward progress (failures arrive
	// as PHASE_FAILED), so default to SUCCESS; terminal phases stay UNSPECIFIED
	// so callers can tell "HCS did not say" from a real outcome.
	switch phase {
	case Phase_PHASE_SETUP_DONE,
		Phase_PHASE_TRANSFER_IN_PROGRESS,
		Phase_PHASE_BLACKOUT_STARTED,
		Phase_PHASE_OFFLINE_DONE,
		Phase_PHASE_BLACKOUT_EXITED:
		return PhaseState_PHASE_STATE_SUCCESS
	}

	return PhaseState_PHASE_STATE_UNSPECIFIED
}

// ToNotification converts an HCS migration event into its wire-form notification.
func ToNotification(info hcsschema.OperationSystemMigrationNotificationInfo, fallbackOrigin hcsschema.MigrationOrigin) *Notification {
	phase := ToPhase(info.Event)
	notification := &Notification{
		Origin: ToOrigin(info.Origin, fallbackOrigin),
		Phase:  phase,
		State:  ToPhaseState(info.Result, phase),
	}

	if info.Event == hcsschema.MigrationEventBlackoutExited && len(info.AdditionalDetails) > 0 {
		// On unmarshal failure we drop PhaseDetails rather than the whole
		// notification; the core phase/state info is still useful.
		var details hcsschema.BlackoutExitedEventDetails
		if err := json.Unmarshal(info.AdditionalDetails, &details); err == nil {
			notification.PhaseDetails = &Notification_BlackoutExited{
				BlackoutExited: &BlackoutExitedEventDetails{
					BlackoutDurationMilliseconds: details.BlackoutDurationMilliseconds,
					BlackoutStopTimestamp:        timestamppb.New(details.BlackoutStopTimestamp),
				},
			}
		}
	}

	return notification
}
