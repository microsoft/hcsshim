//go:build windows

package plan9

import (
	"github.com/Microsoft/hcsshim/internal/controller/device/plan9/mount"
	"github.com/Microsoft/hcsshim/internal/controller/device/plan9/share"
)

// reservation links a caller-supplied reservation ID to a Plan9 share host
// path. Access must be guarded by Controller.mu.
type reservation struct {
	// hostPath is the key into Controller.sharesByHostPath for the share.
	hostPath string

	// name is the share name corresponding to the hostPath.
	name string
}

// vmPlan9 combines the VM-side Plan9 add and remove operations.
type vmPlan9 interface {
	share.VMPlan9Adder
	share.VMPlan9Remover
}

// guestPlan9 combines all guest-side Plan9 operations for LCOW guests.
type guestPlan9 interface {
	mount.LinuxGuestPlan9Mounter
	mount.LinuxGuestPlan9Unmounter
}
