//go:build windows

package mount

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
)

// Mount represents a single partition mount inside a Hyper-V guest VM. It
// tracks the mount lifecycle and supports reference counting so multiple
// callers can share the same physical guest mount.
//
// All operations on a [Mount] are expected to be ordered by the caller.
// No locking is performed at this layer.
type Mount struct {
	// controller and lun are the hardware address of the parent disk on the VM's SCSI bus.
	controller uint
	lun        uint

	// config is the immutable guest-side mount configuration supplied at construction.
	config Config

	// state tracks the current lifecycle position of this mount.
	state State

	// refCount is the number of active callers sharing this mount.
	// The guest unmount is issued only when it drops to zero.
	refCount int

	// guestPath is the auto-generated path inside the guest where the
	// partition is mounted. Valid only in [StateMounted].
	guestPath string
}

// NewReserved creates a new [Mount] in the [StateReserved] state with the
// provided controller, LUN, and guest-side mount configuration.
func NewReserved(controller, lun uint, config Config) *Mount {
	return &Mount{
		controller: controller,
		lun:        lun,
		config:     config,
		state:      StateReserved,
		refCount:   1,
	}
}

// State returns the current lifecycle state of the mount.
func (m *Mount) State() State {
	return m.state
}

// GuestPath returns the path inside the guest where the partition is mounted.
// The path is only valid once the mount is in [StateMounted].
func (m *Mount) GuestPath() string {
	return m.guestPath
}

// Reserve increments the reference count on this mount, allowing an additional
// caller to share the same guest path.
func (m *Mount) Reserve(config Config) error {
	if !m.config.Equals(config) {
		return fmt.Errorf("cannot reserve mount with different config")
	}
	if m.state != StateReserved && m.state != StateMounted {
		return fmt.Errorf("cannot reserve mount in state %s", m.state)
	}
	m.refCount++
	return nil
}

// MountToGuest issues the guest-side mount operation and returns the guest
// path.
func (m *Mount) MountToGuest(ctx context.Context, linuxGuest LinuxGuestSCSIMounter, windowsGuest WindowsGuestSCSIMounter) (string, error) {
	ctx, _ = log.WithContext(ctx, logrus.WithFields(logrus.Fields{
		logfields.Controller: m.controller,
		logfields.LUN:        m.lun,
		logfields.Partition:  m.config.Partition,
	}))

	// Drive the mount state machine.
	switch m.state {
	case StateReserved:
		log.G(ctx).Debug("mounting partition in guest")

		// Issue the platform-specific guest mount. On failure, advance to
		// StateUnmounted since no guest state was established from Reserved.
		if linuxGuest != nil {
			if err := m.mountReservedLCOW(ctx, linuxGuest); err != nil {
				// Move to unmounted since we know from reserved there was no
				// guest state.
				m.state = StateUnmounted
				return "", err
			}
		} else if windowsGuest != nil {
			if err := m.mountReservedWCOW(ctx, windowsGuest); err != nil {
				// Move to unmounted since we know from reserved there was no
				// guest state.
				m.state = StateUnmounted
				return "", err
			}
		} else {
			return "", fmt.Errorf("both linuxGuest and windowsGuest cannot be nil")
		}
		m.state = StateMounted
		// Note we don't increment the ref count here as the caller of
		// MountToGuest is responsible for calling it once per reservation, so
		// we know the ref count should be 1 at this point.

		log.G(ctx).WithField(logfields.UVMPath, m.guestPath).Debug("successfully mounted partition in guest")
		return m.guestPath, nil

	case StateMounted:
		// Already mounted — the caller holds a reservation so return the
		// existing guest path directly.
		return m.guestPath, nil

	case StateUnmounted:
		return "", fmt.Errorf("cannot mount a partition in state %s", m.state)
	}
	return "", nil
}

// UnmountFromGuest decrements the reference count and, when it reaches zero,
// issues the guest-side unmount.
func (m *Mount) UnmountFromGuest(ctx context.Context, linuxGuest LinuxGuestSCSIUnmounter, windowsGuest WindowsGuestSCSIUnmounter) error {
	ctx, _ = log.WithContext(ctx, logrus.WithFields(logrus.Fields{
		logfields.Controller: m.controller,
		logfields.LUN:        m.lun,
		logfields.Partition:  m.config.Partition,
	}))

	// Drive the state machine.
	switch m.state {
	case StateReserved:
		// No guest work to do, just decrement the ref count and if it hits zero we are done.
		m.refCount--
		return nil

	case StateMounted:
		if m.refCount == 1 {
			log.G(ctx).Debug("unmounting partition from guest")

			// Last reference — issue the physical guest unmount.
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
			m.state = StateUnmounted
			log.G(ctx).Debug("partition unmounted from guest")
		}
		m.refCount--
		return nil

	case StateUnmounted:
		return fmt.Errorf("cannot unmount a partition in state %s", m.state)
	}
	return nil
}
