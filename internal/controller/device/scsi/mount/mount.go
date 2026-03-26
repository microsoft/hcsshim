//go:build windows

package mount

import (
	"context"
	"fmt"
	"slices"
)

// MountConfig describes how a partition of a SCSI disk should be mounted inside
// the guest.
type MountConfig struct {
	// Partition is the target partition index (1-based) on a partitioned
	// device. Zero means the whole disk.
	//
	// WCOW only supports whole disk. LCOW supports both whole disk and
	// partition mounts if the disk has multiple.
	Partition uint64
	// ReadOnly mounts the disk read-only.
	ReadOnly bool
	// Encrypted encrypts the device with dm-crypt.
	//
	// Only supported for LCOW.
	Encrypted bool
	// Options are mount flags or data passed to the guest mount call.
	//
	// Only supported for LCOW.
	Options []string
	// EnsureFilesystem formats the mount as Filesystem if not already
	// formatted.
	//
	// Only supported for LCOW.
	EnsureFilesystem bool
	// Filesystem is the target filesystem type.
	//
	// Only supported for LCOW.
	Filesystem string
	// BlockDev mounts the device as a block device.
	//
	// Only supported for LCOW.
	BlockDev bool
	// FormatWithRefs formats the disk using refs.
	//
	// Only supported for WCOW scratch disks.
	FormatWithRefs bool
}

// equals reports whether two MountConfig values describe the same mount parameters.
func (mc MountConfig) Equals(other MountConfig) bool {
	return mc.ReadOnly == other.ReadOnly &&
		mc.Encrypted == other.Encrypted &&
		mc.EnsureFilesystem == other.EnsureFilesystem &&
		mc.Filesystem == other.Filesystem &&
		mc.BlockDev == other.BlockDev &&
		mc.FormatWithRefs == other.FormatWithRefs &&
		slices.Equal(mc.Options, other.Options)
}

type MountState int

const (
	// The mount has never been mounted.
	MountStateReserved MountState = iota
	// The mount is current mounted in the guest.
	MountStateMounted
	// The mount was previously mounted and unmounted.
	MountStateUnmounted
)

// Defines a mount of a partition of a SCSI disk inside the guest. It manages
// the lifecycle of the mount inside the guest independent of the lifecycle of
// the disk attachment.
//
// All operations on the mount are expected to be ordered by the caller. No
// locking is done at this layer.
type Mount struct {
	controller uint
	lun        uint
	config     MountConfig

	state     MountState
	refCount  int
	guestPath string
}

// NewReserved creates a new Mount in the reserved state with the provided configuration.
func NewReserved(controller, lun uint, config MountConfig) *Mount {
	return &Mount{
		controller: controller,
		lun:        lun,
		config:     config,
		state:      MountStateReserved,
		refCount:   1,
	}
}

func (m *Mount) State() MountState {
	return m.state
}

func (m *Mount) GuestPath() string {
	return m.guestPath
}

func (m *Mount) Reserve(config MountConfig) error {
	if !m.config.Equals(config) {
		return fmt.Errorf("cannot reserve ref on mount with different config")
	}
	if m.state != MountStateReserved && m.state != MountStateMounted {
		return fmt.Errorf("cannot reserve ref on mount in state %d", m.state)
	}
	m.refCount++
	return nil
}

func (m *Mount) MountToGuest(ctx context.Context, linuxGuest LinuxGuestSCSIMounter, windowsGuest WindowsGuestSCSIMounter) (string, error) {
	switch m.state {
	// If the mount is reserved, we need to actually mount it in the guest.
	case MountStateReserved:
		if linuxGuest != nil {
			if err := m.mountReservedLCOW(ctx, linuxGuest); err != nil {
				// Move to unmounted since we know from reserved there was no
				// guest state.
				m.state = MountStateUnmounted
				return "", err
			}
		} else if windowsGuest != nil {
			if err := m.mountReservedWCOW(ctx, windowsGuest); err != nil {
				// Move to unmounted since we know from reserved there was no
				// guest state.
				m.state = MountStateUnmounted
				return "", err
			}
		} else {
			panic(fmt.Errorf("both linuxGuest and windowsGuest cannot be nil"))
		}
		m.state = MountStateMounted
		// Note we don't increment the ref count here as the caller of
		// MountToGuest is responsible for calling it once per reservation, so
		// we know the ref count should be 1 at this point.
		return m.guestPath, nil
	case MountStateMounted:
		// The mount is already mounted, and the caller has a reservation so do nothing, its ready.
		return m.guestPath, nil
	case MountStateUnmounted:
		return "", fmt.Errorf("cannot mount an unmounted mount")
	}
	return "", nil
}

func (m *Mount) UnmountFromGuest(ctx context.Context, linuxGuest LinuxGuestSCSIUnmounter, windowsGuest WindowsGuestSCSIUnmounter) error {
	switch m.state {
	case MountStateReserved:
		// No guest work to do, just decrement the ref count and if it hits zero we are done.
		m.refCount--
		return nil
	case MountStateMounted:
		if m.refCount == 1 {
			if linuxGuest != nil {
				if err := m.unmountLCOW(ctx, linuxGuest); err != nil {
					return err
				}
			} else if windowsGuest != nil {
				if err := m.unmountWCOW(ctx, windowsGuest); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("both linuxGuest and windowsGuest cannot be nil")
			}
			m.state = MountStateUnmounted
		}
		m.refCount--
		return nil
	case MountStateUnmounted:
		return fmt.Errorf("cannot unmount an unmounted mount")
	}
	return nil
}
