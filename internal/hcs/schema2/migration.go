package hcsschema

// MigrationInitializeOptions is a set of options for the migration workflow.
type MigrationInitializeOptions struct {
	// Origin is the side of migration the workflow is performed on.
	Origin MigrationOrigin `json:"Origin,omitempty"`
	// MemoryTransport specifies the settings for memory transfer during migration. On source, this
	// setting is required when migration is started. On destination, this setting is required when
	// migration is initiated.
	MemoryTransport MigrationMemoryTransport `json:"MemoryTransport,omitempty"`
	// MemoryTransferThrottleParams specifies settings for throttling during memory transfer.
	MemoryTransferThrottleParams *MemoryMigrationTransferThrottleParams `json:"MemoryTransferThrottleParams,omitempty"`
	// CompressionSettings specifies additional settings when compression is enabled.
	CompressionSettings *MigrationCompressionSettings `json:"CompressionSettings,omitempty"`
	// ChecksumVerification enables memory checksum verification.
	ChecksumVerification bool `json:"ChecksumVerification,omitempty"`
	// PerfTracingEnabled enables performance tracing during migration.
	PerfTracingEnabled bool `json:"PerfTracingEnabled,omitempty"`
	// CancelIfBlackoutThresholdExceeds cancels the operation if the blackout threshold is exceeded.
	CancelIfBlackoutThresholdExceeds bool `json:"CancelIfBlackoutThresholdExceeds,omitempty"`
	// PrepareMemoryTransferMode extends timeout for cross-version live migration.
	PrepareMemoryTransferMode bool `json:"PrepareMemoryTransferMode,omitempty"`
	// CompatibilityData is the compatibility information required for the destination VM.
	CompatibilityData *CompatibilityInfo `json:"CompatibilityData,omitempty"`
}

// MigrationFinalizedOptions is a set of additional options used for HcsLiveMigrationFinalization.
type MigrationFinalizedOptions struct {
	// Origin is the side of migration the workflow is performed on.
	Origin MigrationOrigin `json:"Origin,omitempty"`
	// FinalizedOperation is the final state transition for the VM as part of concluding the LM workflow.
	FinalizedOperation MigrationFinalOperation `json:"FinalizedOperation,omitempty"`
}

// MigrationStartOptions specifies options for starting a migration.
type MigrationStartOptions struct {
	// NetworkSettings specifies network settings for the socket provided.
	NetworkSettings *MigrationNetworkSettings `json:"NetworkSettings,omitempty"`
}

// MigrationTransferOptions specifies options for the migration transfer phase.
type MigrationTransferOptions struct {
	// Origin is the side of migration the workflow is performed on.
	Origin MigrationOrigin `json:"Origin,omitempty"`
}

// StartOptions specifies options for starting a compute system.
type StartOptions struct {
	// DestinationMigrationOptions specifies settings to use when starting a migration on the destination side.
	DestinationMigrationOptions *MigrationStartOptions `json:"DestinationMigrationOptions,omitempty"`
}

// MigrationOrigin indicates where migration is initiated from.
type MigrationOrigin string

const (
	// MigrationOriginSource indicates the source side of migration.
	MigrationOriginSource MigrationOrigin = "Source"
	// MigrationOriginDestination indicates the destination side of migration.
	MigrationOriginDestination MigrationOrigin = "Destination"
)

// MigrationMemoryTransport is the transport protocol used for memory transfer during migration.
type MigrationMemoryTransport string

const (
	// MigrationMemoryTransportTCP indicates the VM memory is copied over a TCP/IP connection.
	MigrationMemoryTransportTCP MigrationMemoryTransport = "TCP"
)

// MemoryMigrationTransferThrottleParams specifies settings for migration memory transfer throttling.
type MemoryMigrationTransferThrottleParams struct {
	// SkipThrottling indicates whether throttling should be skipped.
	SkipThrottling *bool `json:"SkipThrottling,omitempty"`
	// ThrottlingScale is the scale of the throttling as a percentage (1-100).
	ThrottlingScale *float64 `json:"ThrottlingScale,omitempty"`
	// MinimumThrottlePercentage is the minimum percentage to which memory transfer can be throttled.
	MinimumThrottlePercentage *uint8 `json:"MinimumThrottlePercentage,omitempty"`
	// TargetNumberOfBrownoutTransferPasses is the number of passes targeted before the VM enters blackout.
	TargetNumberOfBrownoutTransferPasses *uint32 `json:"TargetNumberOfBrownoutTransferPasses,omitempty"`
	// StartingBrownoutPassNumberForThrottling is the transfer pass where throttling begins.
	StartingBrownoutPassNumberForThrottling *uint32 `json:"StartingBrownoutPassNumberForThrottling,omitempty"`
	// MaximumNumberOfBrownoutTransferPasses is the maximum number of passes before forcing blackout.
	MaximumNumberOfBrownoutTransferPasses *uint32 `json:"MaximumNumberOfBrownoutTransferPasses,omitempty"`
	// TargetBlackoutTransferTime is the expected duration for blackout transfer time.
	TargetBlackoutTransferTime *uint32 `json:"TargetBlackoutTransferTime,omitempty"`
	// BlackoutTimeThresholdForCancellingMigration is the blackout duration threshold for cancelling migration.
	BlackoutTimeThresholdForCancellingMigration *uint32 `json:"BlackoutTimeThresholdForCancellingMigration,omitempty"`
}

// MigrationCompressionSettings specifies compression settings for migration.
type MigrationCompressionSettings struct {
	// ThrottleWorkerCount is the [de]compression thread count. Values higher than what the host
	// and VM configuration can support will be adjusted. The value should be non-zero.
	ThrottleWorkerCount *uint32 `json:"ThrottleWorkerCount,omitempty"`
}

// CompatibilityInfo is opaque VM compatibility data, primarily used in migration.
type CompatibilityInfo struct {
	// Data is the raw compatibility information.
	Data []byte `json:"Data,omitempty"`
}

// MigrationFinalOperation is the final operation performed on the compute system to finalize the live migration workflow.
type MigrationFinalOperation string

const (
	// MigrationFinalOperationResume resumes the VM.
	MigrationFinalOperationResume MigrationFinalOperation = "Resume"
	// MigrationFinalOperationStop stops the VM.
	MigrationFinalOperationStop MigrationFinalOperation = "Stop"
)

// MigrationNetworkSettings specifies the transport protocol for network connection provided by client.
type MigrationNetworkSettings struct {
	// SessionID is the session ID associated with the socket connection between source and destination.
	SessionID uint32 `json:"SessionId,omitempty"`
}
