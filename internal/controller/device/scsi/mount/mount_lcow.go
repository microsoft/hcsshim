//go:build windows

package mount

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// mountFmtLCOW is the guest path template for SCSI partition mounts on LCOW.
// The path encodes the controller index, LUN, and partition index so that each
// combination gets a unique, stable mount point. Example:
//
//	/run/mounts/scsi/<controller>_<lun>_<partition>
const (
	mountFmtLCOW = "/run/mounts/scsi/%d_%d_%d"
)

// mountReservedLCOW issues the LCOW guest mount for a partition in the
// [StateReserved] state. It does not transition the state; that is handled
// by the caller in [Mount.MountToGuest].
func (m *Mount) mountReservedLCOW(ctx context.Context, guest LinuxGuestSCSIMounter) error {
	if m.state != StateReserved {
		return fmt.Errorf("unexpected mount state %s, expected reserved", m.state)
	}

	// Build the stable guest path from the controller, LUN, and partition index.
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

// unmountLCOW issues the LCOW guest unmount for a partition in the
// [StateMounted] state. It does not transition the state; that is handled
// by the caller in [Mount.UnmountFromGuest].
func (m *Mount) unmountLCOW(ctx context.Context, guest LinuxGuestSCSIUnmounter) error {
	if m.state != StateMounted {
		return fmt.Errorf("unexpected mount state %s, expected mounted", m.state)
	}

	// Generate the predictable guest path.
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
