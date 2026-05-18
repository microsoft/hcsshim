//go:build windows && wcow

package disk

import "context"

// GuestSCSIEjector is a no-op for WCOW guests, as they do not need to be
// notified of SCSI device removals.
type GuestSCSIEjector interface{}

// ejectFromGuest is a no-op for WCOW guests, as they do not require explicit
// guest-side SCSI device removal before the host removes the disk from the VM bus.
func (d *Disk) ejectFromGuest(_ context.Context, _ GuestSCSIEjector) error {
	return nil
}
