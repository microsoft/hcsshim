//go:build windows

package mount

import (
	"context"
	"slices"

	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// Config describes how a partition of a SCSI disk should be mounted inside
// the guest.
type Config struct {
	// Partition is the target partition index (1-based) on a partitioned
	// device. Zero means the whole disk.
	//
	// WCOW only supports whole disk. LCOW supports both whole disk and
	// partition mounts if the disk has multiple.
	Partition uint64
	// ReadOnly mounts the partition read-only.
	ReadOnly bool
	// Encrypted encrypts the device with dm-crypt.
	//
	// Only supported for LCOW.
	Encrypted bool
	// Options are mount flags or data passed to the guest mount call.
	//
	// Only supported for LCOW.
	Options []string
	// EnsureFilesystem formats the partition as Filesystem if not already
	// formatted.
	//
	// Only supported for LCOW.
	EnsureFilesystem bool
	// Filesystem is the target filesystem type.
	//
	// Only supported for LCOW.
	Filesystem string
	// BlockDev mounts the device as a block device instead of a filesystem.
	//
	// Only supported for LCOW.
	BlockDev bool
	// FormatWithRefs formats the disk using ReFS.
	//
	// Only supported for WCOW scratch disks.
	FormatWithRefs bool
}

// Equals reports whether two mount Config values describe the same mount parameters.
func (c Config) Equals(other Config) bool {
	return c.ReadOnly == other.ReadOnly &&
		c.Encrypted == other.Encrypted &&
		c.EnsureFilesystem == other.EnsureFilesystem &&
		c.Filesystem == other.Filesystem &&
		c.BlockDev == other.BlockDev &&
		c.FormatWithRefs == other.FormatWithRefs &&
		slices.Equal(c.Options, other.Options)
}

// LinuxGuestSCSIMounter mounts a virtual disk partition inside an LCOW guest.
type LinuxGuestSCSIMounter interface {
	// AddLCOWMappedVirtualDisk maps a virtual disk partition into the LCOW guest.
	AddLCOWMappedVirtualDisk(ctx context.Context, settings guestresource.LCOWMappedVirtualDisk) error
}

// LinuxGuestSCSIUnmounter unmounts a virtual disk partition from an LCOW guest.
type LinuxGuestSCSIUnmounter interface {
	// RemoveLCOWMappedVirtualDisk unmaps a virtual disk partition from the LCOW guest.
	RemoveLCOWMappedVirtualDisk(ctx context.Context, settings guestresource.LCOWMappedVirtualDisk) error
}

// WindowsGuestSCSIMounter mounts a virtual disk partition inside a WCOW guest.
type WindowsGuestSCSIMounter interface {
	// AddWCOWMappedVirtualDisk maps a virtual disk into the WCOW guest.
	AddWCOWMappedVirtualDisk(ctx context.Context, settings guestresource.WCOWMappedVirtualDisk) error
	// AddWCOWMappedVirtualDiskForContainerScratch maps a virtual disk as a
	// container scratch disk into the WCOW guest.
	AddWCOWMappedVirtualDiskForContainerScratch(ctx context.Context, settings guestresource.WCOWMappedVirtualDisk) error
}

// WindowsGuestSCSIUnmounter unmounts a virtual disk partition from a WCOW guest.
type WindowsGuestSCSIUnmounter interface {
	// RemoveWCOWMappedVirtualDisk unmaps a virtual disk from the WCOW guest.
	RemoveWCOWMappedVirtualDisk(ctx context.Context, settings guestresource.WCOWMappedVirtualDisk) error
}
