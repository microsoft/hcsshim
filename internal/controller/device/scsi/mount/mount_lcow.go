//go:build windows && lcow

package mount

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// mountFmt is the guest path template for SCSI partition mounts on LCOW.
// The path encodes the controller index, LUN, and partition index so that each
// combination gets a unique, stable mount point. Example:
//
//	/run/mounts/scsi/<controller>_<lun>_<partition>
const (
	mountFmt = "/run/mounts/scsi/%d_%d_%d"
)

// GuestSCSIMounter mounts a virtual disk partition inside an LCOW guest.
type GuestSCSIMounter interface {
	// AddLCOWMappedVirtualDisk maps a virtual disk partition into the LCOW guest.
	AddLCOWMappedVirtualDisk(ctx context.Context, settings guestresource.LCOWMappedVirtualDisk) error
}

// GuestSCSIUnmounter unmounts a virtual disk partition from an LCOW guest.
type GuestSCSIUnmounter interface {
	// RemoveLCOWMappedVirtualDisk unmaps a virtual disk partition from the LCOW guest.
	RemoveLCOWMappedVirtualDisk(ctx context.Context, settings guestresource.LCOWMappedVirtualDisk) error
}

// Config describes how a partition of a SCSI disk should be mounted inside
// an LCOW guest.
type Config struct {
	// Partition is the target partition index (1-based) on a partitioned
	// device. Zero means the whole disk.
	Partition uint64
	// ReadOnly mounts the partition read-only.
	ReadOnly bool
	// Encrypted encrypts the device with dm-crypt.
	Encrypted bool
	// Options are mount flags or data passed to the guest mount call.
	Options []string
	// EnsureFilesystem formats the partition as Filesystem if not already
	// formatted.
	EnsureFilesystem bool
	// Filesystem is the target filesystem type.
	Filesystem string
	// BlockDev mounts the device as a block device instead of a filesystem.
	BlockDev bool
}

// Equals reports whether two mount Config values describe the same mount parameters.
// Options are compared in a case-insensitive and order-insensitive manner.
func (c Config) Equals(other Config) bool {
	cmpFoldCase := func(a, b string) int {
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	}

	return c.ReadOnly == other.ReadOnly &&
		c.Encrypted == other.Encrypted &&
		c.EnsureFilesystem == other.EnsureFilesystem &&
		c.Filesystem == other.Filesystem &&
		c.BlockDev == other.BlockDev &&
		slices.EqualFunc(
			slices.SortedFunc(slices.Values(c.Options), cmpFoldCase),
			slices.SortedFunc(slices.Values(other.Options), cmpFoldCase),
			strings.EqualFold,
		)
}

// mountReserved issues the LCOW guest mount for a partition in the
// [StateReserved] state. It does not transition the state; that is handled
// by the caller in [Mount.MountToGuest].
func (m *Mount) mountReserved(ctx context.Context, guest GuestSCSIMounter) error {
	if m.state != StateReserved {
		return fmt.Errorf("unexpected mount state %s, expected reserved", m.state)
	}

	// Build the stable guest path from the controller, LUN, and partition index.
	guestPath := fmt.Sprintf(mountFmt, m.controller, m.lun, m.config.Partition)
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

// unmount issues the LCOW guest unmount for a partition in the
// [StateMounted] state. It does not transition the state; that is handled
// by the caller in [Mount.UnmountFromGuest].
func (m *Mount) unmount(ctx context.Context, guest GuestSCSIUnmounter) error {
	if m.state != StateMounted {
		return fmt.Errorf("unexpected mount state %s, expected mounted", m.state)
	}

	// Generate the predictable guest path.
	guestPath := fmt.Sprintf(mountFmt, m.controller, m.lun, m.config.Partition)
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
