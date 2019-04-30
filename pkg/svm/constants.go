package svm

import "github.com/Microsoft/hcsshim/internal/lcow"

// Mode defines the lifetime model of service VMs.
type Mode uint8

const (
	// globalID is the ID used for the global service VM when instance is in global mode.
	globalID = "_lcow_global_svm_"

	// ModeUnique is the mode where multiple service VMs are being managed.
	ModeUnique Mode = 1

	// ModeGlobal is the mode where a single "global" service VM is being
	// used for all operations.
	ModeGlobal Mode = 2

	// DefaultScratchSizeGB is the size of the default LCOW scratch disk in GB
	DefaultScratchSizeGB = lcow.DefaultScratchSizeGB
)
