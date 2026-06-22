//go:build windows

package migration

import (
	"fmt"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
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
