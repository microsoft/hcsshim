//go:build windows && lcow

package plan9

import (
	"github.com/Microsoft/hcsshim/internal/controller/device/plan9/mount"
	"github.com/Microsoft/hcsshim/internal/controller/device/plan9/share"
)

// reservation identifies a caller's share and guest mount, guarded by Controller.mu.
type reservation struct {
	// share is the exact host-share variant selected for this reservation.
	share *share.Share

	// mount is the selected guest mount; nil after its reference is released.
	mount *mount.Mount
}

// vmPlan9 combines the VM-side Plan9 add and remove operations.
type vmPlan9 interface {
	share.VMPlan9Adder
	share.VMPlan9Remover
}

// guestPlan9 combines all guest-side Plan9 operations for LCOW guests.
type guestPlan9 interface {
	mount.GuestPlan9Mounter
	mount.GuestPlan9Unmounter
}
