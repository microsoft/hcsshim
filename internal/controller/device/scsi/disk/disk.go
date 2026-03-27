//go:build windows

package disk

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/Microsoft/hcsshim/internal/controller/device/scsi/mount"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// Disk represents a SCSI disk attached to a Hyper-V VM. It tracks the
// attachment lifecycle and delegates per-partition mount management to [mount.Mount].
//
// All operations on a [Disk] are expected to be ordered by the caller.
// No locking is performed at this layer.
type Disk struct {
	// controller and lun are the hardware address of this disk on the VM's SCSI bus.
	controller uint
	lun        uint

	// config is the immutable host-side disk configuration supplied at construction.
	config Config

	// state tracks the current lifecycle position of this attachment.
	state State

	// mounts maps a partition index to its guest mount.
	// len(mounts) > 0 serves as the ref count for the disk.
	mounts map[uint64]*mount.Mount
}

// NewReserved creates a new [Disk] in the [StateReserved] state with the
// provided controller, LUN, and host-side disk configuration.
func NewReserved(controller, lun uint, config Config) *Disk {
	return &Disk{
		controller: controller,
		lun:        lun,
		config:     config,
		state:      StateReserved,
		mounts:     make(map[uint64]*mount.Mount),
	}
}

// State returns the current lifecycle state of the disk.
func (d *Disk) State() State {
	return d.state
}

// Config returns the host-side disk configuration.
func (d *Disk) Config() Config {
	return d.config
}

// HostPath returns the host-side path of the disk image.
func (d *Disk) HostPath() string {
	return d.config.HostPath
}

// AttachToVM adds the disk to the VM's SCSI bus. It is idempotent for an
// already-attached disk; on failure the disk is moved into detached state and
// a new [Disk] must be created to retry.
func (d *Disk) AttachToVM(ctx context.Context, vm VMSCSIAdder) error {

	// Drive the state machine.
	switch d.state {
	case StateReserved:
		log.G(ctx).WithFields(logrus.Fields{
			logfields.Controller: d.controller,
			logfields.LUN:        d.lun,
			logfields.HostPath:   d.config.HostPath,
			logfields.DiskType:   d.config.Type,
		}).Debug("attaching SCSI disk to VM")

		// Attempt to hot-add the disk to the VM SCSI bus.
		if err := vm.AddSCSIDisk(ctx, hcsschema.Attachment{
			Path:                      d.config.HostPath,
			Type_:                     string(d.config.Type),
			ReadOnly:                  d.config.ReadOnly,
			ExtensibleVirtualDiskType: d.config.EVDType,
		}, d.controller, d.lun); err != nil {
			// Since the disk was never attached, move directly to the terminal
			// Detached state. No guest state was established, so there is nothing
			// to clean up.
			d.state = StateDetached
			return fmt.Errorf("attach SCSI disk controller=%d lun=%d to VM: %w", d.controller, d.lun, err)
		}

		// Move to attached state after a successful attach.
		d.state = StateAttached
		log.G(ctx).Debug("SCSI disk attached to VM")
		return nil

	case StateAttached:
		// Already attached — no-op.
		return nil

	case StateDetached:
		// Re-attaching a detached disk is not supported.
		return fmt.Errorf("disk already attached at controller=%d lun=%d", d.controller, d.lun)
	default:
	}

	return nil
}

// DetachFromVM ejects the disk from the guest and removes it from the VM's SCSI
// bus. It is idempotent for a disk that was never attached or is already detached;
// a failed removal is retriable by calling DetachFromVM again.
func (d *Disk) DetachFromVM(ctx context.Context, vm VMSCSIRemover, linuxGuest LinuxGuestSCSIEjector) error {
	ctx, _ = log.WithContext(ctx, logrus.WithFields(logrus.Fields{
		logfields.Controller: d.controller,
		logfields.LUN:        d.lun,
	}))

	// Eject the disk from guest if we need that.
	if d.state == StateAttached {
		// If the disk still has active mounts it is in use; skip detach.
		if len(d.mounts) != 0 {
			// This disk is still active by some other mounts. Leave it.
			return nil
		}

		// LCOW guests require an explicit SCSI device removal before the host
		// removes the disk from the VM bus. For WCOW, Windows handles hot-unplug
		// automatically, so linuxGuest will be nil.
		if linuxGuest != nil {
			log.G(ctx).Debug("ejecting SCSI device from guest")

			if err := linuxGuest.RemoveSCSIDevice(ctx, guestresource.SCSIDevice{
				Controller: uint8(d.controller),
				Lun:        uint8(d.lun),
			}); err != nil {
				return fmt.Errorf("eject SCSI device controller=%d lun=%d from guest: %w", d.controller, d.lun, err)
			}
		}

		// Advance to Ejected before attempting VM removal so that a removal
		// failure leaves the disk in a retriable position without re-ejecting.
		d.state = StateEjected
		log.G(ctx).Debug("SCSI device ejected from guest")
	}

	// Drive the state machine for VM disk detachment.
	switch d.state {
	case StateReserved:
		// Disk was never attached — no-op.

	case StateAttached:
		// Unreachable: the block above always advances past StateAttached.
		return fmt.Errorf("unexpected disk state %s in detach path, expected ejected", d.state)

	case StateEjected:
		log.G(ctx).Debug("removing SCSI disk from VM")

		// Guest has released the device; remove it from the VM SCSI bus.
		if err := vm.RemoveSCSIDisk(ctx, d.controller, d.lun); err != nil {
			// Leave the disk in StateEjected so the caller can retry this step.
			return fmt.Errorf("remove ejected SCSI disk controller=%d lun=%d from VM: %w", d.controller, d.lun, err)
		}
		d.state = StateDetached
		log.G(ctx).Debug("SCSI disk removed from VM")

	case StateDetached:
		// Already fully detached — no-op.
	}

	return nil
}

// ReservePartition reserves a slot for a guest mount on the given partition.
// If a mount already exists for that partition, it increments the reference
// count after verifying the config matches.
func (d *Disk) ReservePartition(ctx context.Context, config mount.Config) (*mount.Mount, error) {
	if d.state != StateReserved && d.state != StateAttached {
		return nil, fmt.Errorf("cannot reserve partition on disk in state %s", d.state)
	}

	ctx, _ = log.WithContext(ctx, logrus.WithFields(logrus.Fields{
		logfields.Controller: d.controller,
		logfields.LUN:        d.lun,
		logfields.Partition:  config.Partition,
	}))

	// If a mount already exists for this partition, bump its ref count.
	if existingMount, ok := d.mounts[config.Partition]; ok {
		if err := existingMount.Reserve(config); err != nil {
			return nil, fmt.Errorf("reserve partition %d: %w", config.Partition, err)
		}

		log.G(ctx).Trace("existing mount found for partition, incrementing ref count")
		return existingMount, nil
	}

	// No existing mount for this partition — create one in the reserved state.
	newMount := mount.NewReserved(d.controller, d.lun, config)
	d.mounts[config.Partition] = newMount

	log.G(ctx).Trace("reserved new mount for partition")
	return newMount, nil
}

// MountPartitionToGuest mounts the partition inside the guest,
// returning the auto-generated guest path. The partition must first be reserved
// via [Disk.ReservePartition].
func (d *Disk) MountPartitionToGuest(ctx context.Context, partition uint64, linuxGuest mount.LinuxGuestSCSIMounter, windowsGuest mount.WindowsGuestSCSIMounter) (string, error) {
	if d.state != StateAttached {
		return "", fmt.Errorf("cannot mount partition on disk in state %s, expected attached", d.state)
	}

	// Look up the pre-reserved mount for this partition.
	existingMnt, ok := d.mounts[partition]
	if !ok {
		return "", fmt.Errorf("partition %d not reserved on disk controller=%d lun=%d", partition, d.controller, d.lun)
	}
	return existingMnt.MountToGuest(ctx, linuxGuest, windowsGuest)
}

// UnmountPartitionFromGuest unmounts the partition at the given index from the
// guest. When the mount's reference count reaches zero, and it transitions to
// the unmounted state, its entry is removed from the disk so a subsequent
// [Disk.DetachFromVM] call sees no active mounts.
func (d *Disk) UnmountPartitionFromGuest(ctx context.Context, partition uint64, linuxGuest mount.LinuxGuestSCSIUnmounter, windowsGuest mount.WindowsGuestSCSIUnmounter) error {
	existingMount, ok := d.mounts[partition]
	if !ok {
		// No mount found — treat as a no-op to support retry by callers.
		return nil
	}

	if err := existingMount.UnmountFromGuest(ctx, linuxGuest, windowsGuest); err != nil {
		return fmt.Errorf("unmount partition %d from guest: %w", partition, err)
	}

	// If the mount reached the terminal unmounted state, remove it from the disk
	// so that len(mounts) correctly reflects the active consumer count.
	if existingMount.State() == mount.StateUnmounted {
		delete(d.mounts, partition)
	}
	return nil
}
