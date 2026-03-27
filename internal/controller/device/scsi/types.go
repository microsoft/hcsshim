//go:build windows

package scsi

import (
	"github.com/Microsoft/hcsshim/internal/controller/device/scsi/disk"
	"github.com/Microsoft/hcsshim/internal/controller/device/scsi/mount"
)

// numLUNsPerController is the maximum number of LUNs per controller, fixed by Hyper-V.
const numLUNsPerController = 64

// reservation links a caller-supplied reservation ID to a SCSI slot and
// partition index. Access must be guarded by Controller.mu.
type reservation struct {
	// controllerSlot is the index into Controller.controllerSlots for the disk.
	controllerSlot int
	// partition is the partition index on the disk for this reservation.
	partition uint64
}

// VMSCSIOps combines the VM-side SCSI add and remove operations.
type VMSCSIOps interface {
	disk.VMSCSIAdder
	disk.VMSCSIRemover
}

// LinuxGuestSCSIOps combines all guest-side SCSI operations for LCOW guests.
type LinuxGuestSCSIOps interface {
	mount.LinuxGuestSCSIMounter
	mount.LinuxGuestSCSIUnmounter
	disk.LinuxGuestSCSIEjector
}

// WindowsGuestSCSIOps combines all guest-side SCSI operations for WCOW guests.
type WindowsGuestSCSIOps interface {
	mount.WindowsGuestSCSIMounter
	mount.WindowsGuestSCSIUnmounter
}
