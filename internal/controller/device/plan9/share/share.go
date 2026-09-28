//go:build windows && lcow

package share

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/Microsoft/hcsshim/internal/controller/device/plan9/mount"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"
)

// Share represents a Plan9 share attached to a Hyper-V VM. It tracks the
// share lifecycle and delegates guest mount management to [mount.Mount].
//
// All operations on a [Share] are expected to be ordered by the caller.
// No locking is performed at this layer.
type Share struct {
	// name is the HCS-level identifier for this share.
	name string

	// config is the immutable host-side share configuration supplied at construction.
	config Config

	// state tracks the current lifecycle position of this share.
	state State

	// mounts holds one reference-counted guest mount per configuration.
	mounts map[mount.Config]*mount.Mount
}

// NewReserved creates a new [Share] in the [StateReserved] state with the
// provided name and host-side share configuration.
func NewReserved(name string, config Config) *Share {
	return &Share{
		name:   name,
		config: config,
		state:  StateReserved,
		mounts: make(map[mount.Config]*mount.Mount),
	}
}

// State returns the current lifecycle state of the share.
func (s *Share) State() State {
	return s.state
}

// Config returns the host-side share configuration.
func (s *Share) Config() Config {
	return s.config
}

// Name returns the HCS-level share name.
func (s *Share) Name() string {
	return s.name
}

// HostPath returns the host-side path of the share.
func (s *Share) HostPath() string {
	return s.config.HostPath
}

// AddToVM adds the share to the VM's Plan9 provider. It is idempotent for an
// already-added share; on failure the share is moved into invalid state so
// that outstanding mount reservations can be drained before the share is
// fully removed.
func (s *Share) AddToVM(ctx context.Context, vm VMPlan9Adder) error {

	// Drive the state machine.
	switch s.state {
	case StateReserved:
		log.G(ctx).WithFields(logrus.Fields{
			logfields.HostPath: s.config.HostPath,
			"shareName":        s.name,
		}).Debug("adding Plan9 share to VM")

		// Build the HCS flags from the share config.
		flags := hcsschema.Plan9ShareFlagsLinuxMetadata
		if s.config.ReadOnly {
			flags |= hcsschema.Plan9ShareFlagsReadOnly
		}
		if s.config.Restrict {
			flags |= hcsschema.Plan9ShareFlagsRestrictFileAccess
		}

		// Attempt to add the share to the VM.
		if err := vm.AddPlan9(ctx, hcsschema.Plan9Share{
			Name:         s.name,
			AccessName:   s.name,
			Path:         s.config.HostPath,
			Port:         vmutils.Plan9Port,
			Flags:        flags,
			AllowedFiles: s.config.AllowedNames,
		}); err != nil {
			// The share was never added to the VM. Transition to Invalid so
			// that outstanding mount reservations can still be drained by
			// callers via UnmountFromGuest before the share is fully removed.
			s.state = StateInvalid
			return fmt.Errorf("add Plan9 share %s to VM: %w", s.name, err)
		}

		// Move to added state after a successful add.
		s.state = StateAdded
		log.G(ctx).Debug("Plan9 share added to VM")
		return nil

	case StateAdded:
		// Already added — no-op.
		return nil

	case StateInvalid:
		// A previous add attempt failed. The caller must drain all mount
		// reservations via UnmountFromGuest and then call RemoveFromVM to
		// transition to StateRemoved.
		return fmt.Errorf("share %s is in invalid state; drain mounts and remove", s.name)

	case StateRemoved:
		// Re-adding a removed share is not supported.
		return fmt.Errorf("share %s already removed", s.name)
	default:
		return fmt.Errorf("share %s in unknown state %d", s.name, s.state)
	}
}

// RemoveFromVM removes the share from the VM. It is idempotent for a share
// that was never added or is already removed; a failed removal is retriable
// by calling RemoveFromVM again.
func (s *Share) RemoveFromVM(ctx context.Context, vm VMPlan9Remover) error {
	ctx, _ = log.WithContext(ctx, logrus.WithField("shareName", s.name))

	switch s.state {
	case StateReserved, StateAdded, StateInvalid:
		// Each state may still have guest mount reservations. Keep the share until
		// all are drained so releasing one caller cannot invalidate another.
		// Reserved and Invalid were never added to the VM; Added continues below
		// to remove the VM resource after the final mount is released.
		if len(s.mounts) != 0 {
			return nil
		}

		// Only remove from the VM if the share was actually added.
		if s.state == StateAdded {
			log.G(ctx).Debug("removing Plan9 share from VM")

			if err := vm.RemovePlan9(ctx, hcsschema.Plan9Share{
				Name:       s.name,
				AccessName: s.name,
				Port:       vmutils.Plan9Port,
			}); err != nil {
				// Leave the share in StateAdded so the caller can retry.
				return fmt.Errorf("remove Plan9 share %s from VM: %w", s.name, err)
			}
		}

		s.state = StateRemoved
		log.G(ctx).Debug("Plan9 share removed from VM")

	case StateRemoved:
		// Already fully removed — no-op.
		// Controller needs to remove ref from its map.
	}

	return nil
}

// ReserveMount reserves a guest mount on this share. If a mount with the same
// config already exists, it increments the reference count; otherwise, it
// creates a new mount for the requested config.
func (s *Share) ReserveMount(ctx context.Context, config mount.Config) (*mount.Mount, error) {
	if s.state != StateReserved && s.state != StateAdded {
		return nil, fmt.Errorf("cannot reserve mount on share in state %s", s.state)
	}

	ctx, _ = log.WithContext(ctx, logrus.WithField("shareName", s.name))

	// Reuse an existing mount only when the guest configuration matches.
	if existing := s.mounts[config]; existing != nil {
		if err := existing.Reserve(config); err != nil {
			return nil, fmt.Errorf("reserve mount on share %s: %w", s.name, err)
		}

		log.G(ctx).Trace("existing mount found for share, incrementing ref count")
		return existing, nil
	}

	// No matching mount exists, so create one for this guest configuration.
	newMount := mount.NewReserved(s.name, config)
	s.mounts[config] = newMount

	log.G(ctx).Trace("reserved new mount for share")
	return newMount, nil
}

// MountToGuest mounts the selected guest mount, returning its guest path.
// The mount must first be reserved via [Share.ReserveMount].
func (s *Share) MountToGuest(ctx context.Context, guest mount.GuestPlan9Mounter, m *mount.Mount) (string, error) {
	if s.state != StateAdded {
		return "", fmt.Errorf("cannot mount share in state %s, expected added", s.state)
	}

	// Verify that the selected mount is still registered with this share.
	if m == nil || s.mounts[m.Config()] != m {
		return "", fmt.Errorf("mount not reserved on share %s", s.name)
	}
	return m.MountToGuest(ctx, guest)
}

// UnmountFromGuest releases the selected guest mount. When its reference count
// reaches zero and it transitions to the unmounted state, that mount entry is
// removed from the share. Other configured mounts remain active, and
// [Share.RemoveFromVM] sees no active mounts only after all entries are removed.
func (s *Share) UnmountFromGuest(ctx context.Context, guest mount.GuestPlan9Unmounter, m *mount.Mount) error {
	if m == nil || m.State() == mount.StateUnmounted {
		// Already released; keep teardown retryable.
		return nil
	}

	// Verify ownership before changing the mount's reference count.
	if s.mounts[m.Config()] != m {
		return fmt.Errorf("mount not reserved on share %s", s.name)
	}

	// Release this mount's reference without affecting other guest configurations.
	if err := m.UnmountFromGuest(ctx, guest); err != nil {
		return fmt.Errorf("unmount share %s from guest: %w", s.name, err)
	}

	// Drop only this mount; other configurations keep the share alive.
	if m.State() == mount.StateUnmounted {
		delete(s.mounts, m.Config())
	}
	return nil
}
