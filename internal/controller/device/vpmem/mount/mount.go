//go:build windows

package mount

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// MountConfig describes how a VPMem device should be mounted inside the guest.
type MountConfig struct{}

// Equals reports whether two MountConfig values describe the same mount
// parameters.
func (mc MountConfig) Equals(other MountConfig) bool {
	return true
}

type MountState int

const (
	// The mount has never been mounted.
	MountStateReserved MountState = iota
	// The mount is currently mounted in the guest.
	MountStateMounted
	// The mount was previously mounted and unmounted.
	MountStateUnmounted
)

type LinuxGuestVPMemMounter interface {
	AddLCOWMappedVPMemDevice(ctx context.Context, settings guestresource.LCOWMappedVPMemDevice) error
}

type LinuxGuestVPMemUnmounter interface {
	RemoveLCOWMappedVPMemDevice(ctx context.Context, settings guestresource.LCOWMappedVPMemDevice) error
}

const (
	mountFmtVPMem = "/run/layers/p%d"
)

// Mount defines a mount of a VPMem device inside the guest. It manages the
// lifecycle of the mount inside the guest independent of the lifecycle of the
// device attachment.
//
// All operations on the mount are expected to be ordered by the caller. No
// locking is done at this layer.
type Mount struct {
	deviceNumber uint32
	config       MountConfig

	state     MountState
	refCount  int
	guestPath string
}

// NewReserved creates a new Mount in the reserved state with the provided
// configuration.
func NewReserved(deviceNumber uint32, config MountConfig) *Mount {
	return &Mount{
		deviceNumber: deviceNumber,
		config:       config,
		state:        MountStateReserved,
		refCount:     1,
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

func (m *Mount) MountToGuest(ctx context.Context, guest LinuxGuestVPMemMounter) (string, error) {
	switch m.state {
	case MountStateReserved:
		guestPath := fmt.Sprintf(mountFmtVPMem, m.deviceNumber)
		settings := guestresource.LCOWMappedVPMemDevice{
			DeviceNumber: m.deviceNumber,
			MountPath:    guestPath,
		}
		if err := guest.AddLCOWMappedVPMemDevice(ctx, settings); err != nil {
			// Move to unmounted since we know from reserved there was no
			// guest state.
			m.state = MountStateUnmounted
			return "", fmt.Errorf("add LCOW mapped VPMem device %d: %w", m.deviceNumber, err)
		}
		m.guestPath = guestPath
		m.state = MountStateMounted
		return m.guestPath, nil
	case MountStateMounted:
		return m.guestPath, nil
	case MountStateUnmounted:
		return "", fmt.Errorf("cannot mount an unmounted mount")
	}
	return "", nil
}

func (m *Mount) UnmountFromGuest(ctx context.Context, guest LinuxGuestVPMemUnmounter) error {
	switch m.state {
	case MountStateReserved:
		m.refCount--
		return nil
	case MountStateMounted:
		if m.refCount == 1 {
			settings := guestresource.LCOWMappedVPMemDevice{
				DeviceNumber: m.deviceNumber,
				MountPath:    m.guestPath,
			}
			if err := guest.RemoveLCOWMappedVPMemDevice(ctx, settings); err != nil {
				return fmt.Errorf("remove LCOW mapped VPMem device %d: %w", m.deviceNumber, err)
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
