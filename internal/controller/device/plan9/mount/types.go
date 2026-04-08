//go:build windows && lcow

package mount

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// Config describes how a Plan9 share should be mounted inside the guest.
type Config struct {
	// ReadOnly mounts the share read-only.
	ReadOnly bool
}

// Equals reports whether two mount Config values describe the same mount parameters.
func (c Config) Equals(other Config) bool {
	return c.ReadOnly == other.ReadOnly
}

// LinuxGuestPlan9Mounter mounts a Plan9 share inside an LCOW guest.
type LinuxGuestPlan9Mounter interface {
	// AddLCOWMappedDirectory maps a Plan9 share into the LCOW guest.
	AddLCOWMappedDirectory(ctx context.Context, settings guestresource.LCOWMappedDirectory) error
}

// LinuxGuestPlan9Unmounter unmounts a Plan9 share from an LCOW guest.
type LinuxGuestPlan9Unmounter interface {
	// RemoveLCOWMappedDirectory unmaps a Plan9 share from the LCOW guest.
	RemoveLCOWMappedDirectory(ctx context.Context, settings guestresource.LCOWMappedDirectory) error
}
