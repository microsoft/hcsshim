//go:build windows

package mount

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

type LinuxGuestSCSIMounter interface {
	AddLCOWMappedVirtualDisk(ctx context.Context, settings guestresource.LCOWMappedVirtualDisk) error
}

type LinuxGuestSCSIUnmounter interface {
	RemoveLCOWMappedVirtualDisk(ctx context.Context, settings guestresource.LCOWMappedVirtualDisk) error
}

const (
	mountFmtLCOW = "/run/mounts/scsi/%d_%d_%d"
)

// Implements the mount operation for LCOW from the expected Reserved state.
//
// It does not transition the state to ensure the exposed mount interface
// handles transitions for all LCOW/WCOW.
func (m *Mount) mountReservedLCOW(ctx context.Context, guest LinuxGuestSCSIMounter) error {
	if m.state != MountStateReserved {
		return fmt.Errorf("unexpected mount state %d, expected reserved", m.state)
	}
	guestPath := fmt.Sprintf(mountFmtLCOW, m.controller, m.lun, m.config.Partition)
	settings := guestresource.LCOWMappedVirtualDisk{
		MountPath:        guestPath,
		Controller:       uint8(m.controller),
		Lun:              uint8(m.lun),
		Partition:        m.config.Partition,
		ReadOnly:         m.config.ReadOnly,
		Encrypted:        m.config.Encrypted,
		Options:          m.config.Options,
		EnsureFilesystem: m.config.EnsureFilesystem,
		Filesystem:       m.config.Filesystem,
		BlockDev:         m.config.BlockDev,
	}
	if err := guest.AddLCOWMappedVirtualDisk(ctx, settings); err != nil {
		return fmt.Errorf("add LCOW mapped virtual disk controller=%d lun=%d: %w", m.controller, m.lun, err)
	}
	m.guestPath = guestPath
	return nil
}

// Implements the unmount operation for LCOW from the expected Mounted state.
//
// It does not transition the state to ensure the exposed mount interface
// handles transitions for all LCOW/WCOW.
func (m *Mount) unmountLCOW(ctx context.Context, guest LinuxGuestSCSIUnmounter) error {
	if m.state != MountStateMounted {
		return fmt.Errorf("unexpected mount state %d, expected mounted", m.state)
	}
	guestPath := fmt.Sprintf(mountFmtLCOW, m.controller, m.lun, m.config.Partition)
	settings := guestresource.LCOWMappedVirtualDisk{
		MountPath:        guestPath,
		Controller:       uint8(m.controller),
		Lun:              uint8(m.lun),
		Partition:        m.config.Partition,
		ReadOnly:         m.config.ReadOnly,
		Encrypted:        m.config.Encrypted,
		Options:          m.config.Options,
		EnsureFilesystem: m.config.EnsureFilesystem,
		Filesystem:       m.config.Filesystem,
		BlockDev:         m.config.BlockDev,
	}
	if err := guest.RemoveLCOWMappedVirtualDisk(ctx, settings); err != nil {
		return fmt.Errorf("remove LCOW mapped virtual disk controller=%d lun=%d: %w", m.controller, m.lun, err)
	}
	return nil
}
