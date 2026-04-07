//go:build windows && lcow

package disk

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// GuestSCSIEjector issues a guest-side SCSI device removal for LCOW guests.
type GuestSCSIEjector interface {
	// RemoveSCSIDevice removes a SCSI device from the LCOW guest.
	RemoveSCSIDevice(ctx context.Context, settings guestresource.SCSIDevice) error
}

// ejectFromGuest issues an explicit guest-side SCSI device removal for LCOW guests.
// This must be performed before the host removes the disk from the VM bus.
func (d *Disk) ejectFromGuest(ctx context.Context, guest GuestSCSIEjector) error {
	log.G(ctx).Debug("ejecting SCSI device from guest")

	if err := guest.RemoveSCSIDevice(ctx, guestresource.SCSIDevice{
		Controller: uint8(d.controller),
		Lun:        uint8(d.lun),
	}); err != nil {
		return fmt.Errorf("eject SCSI device controller=%d lun=%d from guest: %w", d.controller, d.lun, err)
	}

	return nil
}
