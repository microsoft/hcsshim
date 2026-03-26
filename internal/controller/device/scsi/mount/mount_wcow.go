//go:build windows

package mount

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

type WindowsGuestSCSIMounter interface {
	AddWCOWMappedVirtualDisk(ctx context.Context, settings guestresource.WCOWMappedVirtualDisk) error
	AddWCOWMappedVirtualDiskForContainerScratch(ctx context.Context, settings guestresource.WCOWMappedVirtualDisk) error
}

type WindowsGuestSCSIUnmounter interface {
	RemoveWCOWMappedVirtualDisk(ctx context.Context, settings guestresource.WCOWMappedVirtualDisk) error
}

const (
	mountFmtWCOW = "C:\\mounts\\scsi\\%d_%d_%d"
)

// Implements the mount operation for WCOW from the expected Reserved state.
//
// It does not transition the state to ensure the exposed mount interface
// handles transitions for all LCOW/WCOW.
func (m *Mount) mountReservedWCOW(ctx context.Context, guest WindowsGuestSCSIMounter) error {
	if m.state != MountStateReserved {
		return fmt.Errorf("unexpected mount state %d, expected reserved", m.state)
	}
	guestPath := fmt.Sprintf(mountFmtWCOW, m.controller, m.lun, m.config.Partition)
	settings := guestresource.WCOWMappedVirtualDisk{
		ContainerPath: guestPath,
		Lun:           int32(m.lun),
	}

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

// Implements the unmount operation for WCOW from the expected Mounted state.
//
// It does not transition the state to ensure the exposed mount interface
// handles transitions for all LCOW/WCOW.
func (m *Mount) unmountWCOW(ctx context.Context, guest WindowsGuestSCSIUnmounter) error {
	if m.state != MountStateMounted {
		return fmt.Errorf("unexpected mount state %d, expected mounted", m.state)
	}
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
