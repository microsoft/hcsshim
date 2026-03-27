//go:build windows

package mount

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// mountFmtWCOW is the guest path template for SCSI partition mounts on WCOW.
// The path encodes the controller index, LUN, and partition index so that each
// combination gets a unique, stable mount point. Example:
//
//	C:/mounts/scsi/<controller>_<lun>_<partition>
const (
	mountFmtWCOW = "C:\\mounts\\scsi\\%d_%d_%d"
)

// mountReservedWCOW issues the WCOW guest mount for a partition in the
// [StateReserved] state. It does not transition the state which is handled
// by the caller in [Mount.MountToGuest].
func (m *Mount) mountReservedWCOW(ctx context.Context, guest WindowsGuestSCSIMounter) error {
	if m.state != StateReserved {
		return fmt.Errorf("unexpected mount state %s, expected reserved", m.state)
	}

	// Generate a predictable guest path.
	guestPath := fmt.Sprintf(mountFmtWCOW, m.controller, m.lun, m.config.Partition)
	settings := guestresource.WCOWMappedVirtualDisk{
		ContainerPath: guestPath,
		Lun:           int32(m.lun),
	}

	// FormatWithRefs disks use a separate scratch path to enable ReFS formatting.
	if m.config.FormatWithRefs {
		if err := guest.AddWCOWMappedVirtualDiskForContainerScratch(ctx, settings); err != nil {
			return fmt.Errorf("add WCOW mapped virtual disk for container scratch controller=%d lun=%d: %w", m.controller, m.lun, err)
		}
	} else {
		if err := guest.AddWCOWMappedVirtualDisk(ctx, settings); err != nil {
			return fmt.Errorf("add WCOW mapped virtual disk controller=%d lun=%d: %w", m.controller, m.lun, err)
		}
	}
	m.guestPath = guestPath
	return nil
}

// unmountWCOW issues the WCOW guest unmount for a partition in the
// [StateMounted] state. It does not transition the state; that is handled
// by the caller in [Mount.UnmountFromGuest].
func (m *Mount) unmountWCOW(ctx context.Context, guest WindowsGuestSCSIUnmounter) error {
	if m.state != StateMounted {
		return fmt.Errorf("unexpected mount state %s, expected mounted", m.state)
	}

	// Build the predictable guest path.
	guestPath := fmt.Sprintf(mountFmtWCOW, m.controller, m.lun, m.config.Partition)
	settings := guestresource.WCOWMappedVirtualDisk{
		ContainerPath: guestPath,
		Lun:           int32(m.lun),
	}
	if err := guest.RemoveWCOWMappedVirtualDisk(ctx, settings); err != nil {
		return fmt.Errorf("remove WCOW mapped virtual disk controller=%d lun=%d: %w", m.controller, m.lun, err)
	}
	return nil
}
