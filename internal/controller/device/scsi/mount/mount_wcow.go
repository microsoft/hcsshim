//go:build windows && wcow

package mount

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// mountFmt is the guest path template for SCSI partition mounts on WCOW.
// The path encodes the controller index, LUN, and partition index so that each
// combination gets a unique, stable mount point. Example:
//
//	C:/mounts/scsi/<controller>_<lun>_<partition>
const (
	mountFmt = "C:\\mounts\\scsi\\%d_%d_%d"
)

// GuestSCSIMounter mounts a virtual disk partition inside an WCOW guest.
type GuestSCSIMounter interface {
	// AddWCOWMappedVirtualDisk maps a virtual disk into the WCOW guest.
	AddWCOWMappedVirtualDisk(ctx context.Context, settings guestresource.WCOWMappedVirtualDisk) error
	// AddWCOWMappedVirtualDiskForContainerScratch maps a virtual disk as a
	// container scratch disk into the WCOW guest.
	AddWCOWMappedVirtualDiskForContainerScratch(ctx context.Context, settings guestresource.WCOWMappedVirtualDisk) error
}

// GuestSCSIUnmounter unmounts a virtual disk partition from an LCOW guest.
type GuestSCSIUnmounter interface {
	// RemoveWCOWMappedVirtualDisk unmaps a virtual disk from the WCOW guest.
	RemoveWCOWMappedVirtualDisk(ctx context.Context, settings guestresource.WCOWMappedVirtualDisk) error
}

// Config describes how a partition of a SCSI disk should be mounted inside
// a WCOW guest.
type Config struct {
	// Partition is the target partition index (1-based) on a partitioned
	// device. Zero means the whole disk.
	Partition uint64
	// ReadOnly mounts the partition read-only.
	ReadOnly bool
	// FormatWithRefs formats the disk using ReFS.
	FormatWithRefs bool
}

// Equals reports whether two mount Config values describe the same mount parameters.
func (c Config) Equals(other Config) bool {
	return c.ReadOnly == other.ReadOnly &&
		c.FormatWithRefs == other.FormatWithRefs
}

// mountReserved issues the WCOW guest mount for a partition in the
// [StateReserved] state. It does not transition the state which is handled
// by the caller in [Mount.MountToGuest].
func (m *Mount) mountReserved(ctx context.Context, guest GuestSCSIMounter) error {
	if m.state != StateReserved {
		return fmt.Errorf("unexpected mount state %s, expected reserved", m.state)
	}

	// Generate a predictable guest path.
	guestPath := fmt.Sprintf(mountFmt, m.controller, m.lun, m.config.Partition)
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

// unmountPartition issues the WCOW guest unmount for a partition in the
// [StateMounted] state. It does not transition the state; that is handled
// by the caller in [Mount.UnmountFromGuest].
func (m *Mount) unmountPartition(ctx context.Context, guest GuestSCSIUnmounter) error {
	if m.state != StateMounted {
		return fmt.Errorf("unexpected mount state %s, expected mounted", m.state)
	}

	// Build the predictable guest path.
	guestPath := fmt.Sprintf(mountFmt, m.controller, m.lun, m.config.Partition)
	settings := guestresource.WCOWMappedVirtualDisk{
		ContainerPath: guestPath,
		Lun:           int32(m.lun),
	}
	if err := guest.RemoveWCOWMappedVirtualDisk(ctx, settings); err != nil {
		return fmt.Errorf("remove WCOW mapped virtual disk controller=%d lun=%d: %w", m.controller, m.lun, err)
	}
	return nil
}
