//go:build windows && !wcow && !lcow

package mount

import (
	"context"
	"fmt"
)

// Config is a stub for unsupported platform builds.
//
// The shared mount.go code (build tag: windows) references m.config.Partition
// for logging and calls m.config.Equals() for reservation validation, so
// both the Partition field and the Equals method must exist here for
// compilation even though no real mount operation is ever performed.
type Config struct {
	Partition uint64
}

// Equals is required by the shared mount.go Reserve method.
// See the comment on [Config] for why this stub exists.
func (c Config) Equals(other Config) bool {
	return true
}

// GuestSCSIMounter is not supported for unsupported guests.
// This is a stub with no method available for use.
type GuestSCSIMounter interface{}

// GuestSCSIUnmounter is not supported for unsupported guests.
// This is a stub with no method available for use.
type GuestSCSIUnmounter interface{}

// mountReserved is not supported for unsupported guests.
func (m *Mount) mountReserved(_ context.Context, _ GuestSCSIMounter) error {
	return fmt.Errorf("unsupported guest")
}

// unmountPartition is not supported for unsupported guests.
func (m *Mount) unmountPartition(_ context.Context, _ GuestSCSIUnmounter) error {
	return fmt.Errorf("unsupported guest")
}
