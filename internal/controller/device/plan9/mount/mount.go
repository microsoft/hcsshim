//go:build windows

package mount

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"
)

// GuestPathFmt is the guest path template for Plan9 mounts on LCOW.
// The path encodes the share name so that each share gets a unique,
// stable mount point. Example:
//
//	/run/mounts/plan9/<shareName>
const GuestPathFmt = "/run/mounts/plan9/%s"

// Mount represents a single Plan9 share mount inside a Hyper-V guest VM. It
// tracks the mount lifecycle and supports reference counting so multiple
// callers can share the same physical guest mount.
//
// All operations on a [Mount] are expected to be ordered by the caller.
// No locking is performed at this layer.
type Mount struct {
	// shareName is the HCS-level identifier for the parent share.
	shareName string

	// config is the immutable guest-side mount configuration supplied at construction.
	config Config

	// state tracks the current lifecycle position of this mount.
	state State

	// refCount is the number of active callers sharing this mount.
	// The guest unmount is issued only when it drops to zero.
	refCount int

	// guestPath is the auto-generated path inside the guest where the
	// share is mounted. Valid only in [StateMounted].
	guestPath string
}

// NewReserved creates a new [Mount] in the [StateReserved] state with the
// provided share name and guest-side mount configuration.
func NewReserved(shareName string, config Config) *Mount {
	return &Mount{
		shareName: shareName,
		config:    config,
		state:     StateReserved,
		refCount:  1,
		guestPath: fmt.Sprintf(GuestPathFmt, shareName),
	}
}

// State returns the current lifecycle state of the mount.
func (m *Mount) State() State {
	return m.state
}

// GuestPath returns the path inside the guest where the share is mounted.
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
func (m *Mount) MountToGuest(ctx context.Context, guest LinuxGuestPlan9Mounter) (string, error) {
	ctx, _ = log.WithContext(ctx, logrus.WithField("shareName", m.shareName))

	// Drive the mount state machine.
	switch m.state {
	case StateReserved:
		log.G(ctx).Debug("mounting Plan9 share in guest")

		// Issue the guest mount via the GCS mapped directory API.
		if err := guest.AddLCOWMappedDirectory(ctx, guestresource.LCOWMappedDirectory{
			MountPath: m.guestPath,
			ShareName: m.shareName,
			Port:      vmutils.Plan9Port,
			ReadOnly:  m.config.ReadOnly,
		}); err != nil {
			// Move to unmounted since no guest state was established from Reserved.
			m.state = StateUnmounted
			return "", fmt.Errorf("add LCOW mapped directory share=%s: %w", m.shareName, err)
		}

		m.state = StateMounted
		// Note we don't increment the ref count here as the caller of
		// MountToGuest is responsible for calling it once per reservation.

		log.G(ctx).WithField(logfields.UVMPath, m.guestPath).Debug("Plan9 share mounted in guest")
		return m.guestPath, nil

	case StateMounted:
		// Already mounted — the caller holds a reservation so return the
		// existing guest path directly.
		return m.guestPath, nil

	case StateUnmounted:
		return "", fmt.Errorf("cannot mount a share in state %s", m.state)
	}
	return "", nil
}

// UnmountFromGuest decrements the reference count and, when it reaches zero,
// issues the guest-side unmount.
func (m *Mount) UnmountFromGuest(ctx context.Context, guest LinuxGuestPlan9Unmounter) error {
	ctx, _ = log.WithContext(ctx, logrus.WithField("shareName", m.shareName))

	// Drive the state machine.
	switch m.state {
	case StateReserved:
		// No guest work to do, just decrement the ref count and if it hits zero we are done.
		m.refCount--
		// Once we hit the last entry, we can transition to unmounted so that
		// caller can understand the terminal state of this mount.
		if m.refCount == 0 {
			m.state = StateUnmounted
		}
		return nil

	case StateMounted:
		if m.refCount == 1 {
			log.G(ctx).Debug("unmounting Plan9 share from guest")

			// Last reference — issue the physical guest unmount.
			if err := guest.RemoveLCOWMappedDirectory(ctx, guestresource.LCOWMappedDirectory{
				MountPath: m.guestPath,
				ShareName: m.shareName,
				Port:      vmutils.Plan9Port,
				ReadOnly:  m.config.ReadOnly,
			}); err != nil {
				return fmt.Errorf("remove LCOW mapped directory share=%s: %w", m.shareName, err)
			}

			m.state = StateUnmounted
			log.G(ctx).Debug("Plan9 share unmounted from guest")
		}
		m.refCount--
		return nil

	case StateUnmounted:
		// Already in the terminal state — nothing to do. This can happen when
		// MountToGuest failed (it transitions StateReserved → StateUnmounted),
		// and the caller subsequently calls UnmapFromGuest to clean up.
	}
	return nil
}
