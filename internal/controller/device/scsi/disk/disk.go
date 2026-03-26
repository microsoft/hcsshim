//go:build windows

package disk

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/controller/device/scsi/mount"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// DiskType identifies the attachment protocol used when adding a disk to the VM's SCSI bus.
type DiskType string

const (
	// DiskTypeVirtualDisk attaches the disk as a virtual hard disk (VHD/VHDX).
	DiskTypeVirtualDisk DiskType = "VirtualDisk"

	// DiskTypePassThru attaches a physical disk directly to the VM with pass-through access.
	DiskTypePassThru DiskType = "PassThru"

	// DiskTypeExtensibleVirtualDisk attaches a disk via an extensible virtual disk (EVD) provider.
	// The hostPath must be in the form evd://<type>/<mountPath>.
	DiskTypeExtensibleVirtualDisk DiskType = "ExtensibleVirtualDisk"
)

// DiskConfig describes the host-side disk to attach to the VM's SCSI bus.
type DiskConfig struct {
	// HostPath is the path on the host to the disk to be attached.
	HostPath string
	// ReadOnly specifies whether the disk should be attached with read-only access.
	ReadOnly bool
	// Type specifies the attachment protocol to use when attaching the disk.
	Type DiskType
	// EVDType is the EVD provider name.
	// Only populated when Type is [DiskTypeExtensibleVirtualDisk].
	EVDType string
}

// equals reports whether two DiskConfig values describe the same attachment parameters.
func (d DiskConfig) Equals(other DiskConfig) bool {
	return d.HostPath == other.HostPath &&
		d.ReadOnly == other.ReadOnly &&
		d.Type == other.Type &&
		d.EVDType == other.EVDType
}

type DiskState int

const (
	// The disk has never been attached.
	DiskStateReserved DiskState = iota
	// The disk is currently attached to the guest.
	DiskStateAttached
	// The disk was previously attached and ejected and must be detached.
	DiskStateEjected
	// The disk was previously attached and detached, this is terminal.
	DiskStateDetached
)

type VMSCSIAdder interface {
	AddSCSIDisk(ctx context.Context, disk hcsschema.Attachment, controller uint, lun uint) error
}

type VMSCSIRemover interface {
	RemoveSCSIDisk(ctx context.Context, controller uint, lun uint) error
}

type LinuxGuestSCSIEjector interface {
	RemoveSCSIDevice(ctx context.Context, settings guestresource.SCSIDevice) error
}

// Disk represents a SCSI disk attached to the VM. It manages the lifecycle of
// the disk attachment as well as the guest mounts on the disk partitions.
//
// All operations on the disk are expected to be ordered by the caller. No
// locking is done at this layer.
type Disk struct {
	controller uint
	lun        uint
	config     DiskConfig

	state DiskState
	// Note that len(mounts) > 0 is the ref count for a disk.
	mounts map[uint64]*mount.Mount
}

// NewReserved creates a new Disk in the reserved state with the provided configuration.
func NewReserved(controller, lun uint, config DiskConfig) *Disk {
	return &Disk{
		controller: controller,
		lun:        lun,
		config:     config,
		state:      DiskStateReserved,
		mounts:     make(map[uint64]*mount.Mount),
	}
}

func (d *Disk) State() DiskState {
	return d.state
}

func (d *Disk) Config() DiskConfig {
	return d.config
}

func (d *Disk) HostPath() string {
	return d.config.HostPath
}

func (d *Disk) AttachToVM(ctx context.Context, vm VMSCSIAdder) error {
	switch d.state {
	case DiskStateReserved:
		// Attach the disk to the VM.
		if err := vm.AddSCSIDisk(ctx, hcsschema.Attachment{
			Path:                      d.config.HostPath,
			Type_:                     string(d.config.Type),
			ReadOnly:                  d.config.ReadOnly,
			ExtensibleVirtualDiskType: d.config.EVDType,
		}, d.controller, d.lun); err != nil {
			// Move to detached since we know from reserved there was no guest
			// state.
			d.state = DiskStateDetached
			return fmt.Errorf("attach disk to VM: %w", err)
		}
		d.state = DiskStateAttached
		return nil
	case DiskStateAttached:
		// Disk is already attached, this is idempotent.
		return nil
	case DiskStateDetached:
		// We don't support re-attaching a detached disk, this is an error.
		return fmt.Errorf("disk already detached")
	}
	return nil
}

func (d *Disk) DetachFromVM(ctx context.Context, vm VMSCSIRemover, lGuest LinuxGuestSCSIEjector) error {
	if d.state == DiskStateAttached {
		// Ensure for correctness nobody leaked a mount
		if len(d.mounts) != 0 {
			// This disk is still active by some other mounts. Leave it.
			return nil
		}
		// The linux guest needs to have the SCSI ejected. Do that now.
		if lGuest != nil {
			if err := lGuest.RemoveSCSIDevice(ctx, guestresource.SCSIDevice{
				Controller: uint8(d.controller),
				Lun:        uint8(d.lun),
			}); err != nil {
				return fmt.Errorf("remove SCSI device from guest: %w", err)
			}
		}
		// Set it to ejected and continue processing.
		d.state = DiskStateEjected
	}

	switch d.state {
	case DiskStateReserved:
		// Disk is not attached, this is idempotent.
		return nil
	case DiskStateAttached:
		panic(fmt.Errorf("unexpected attached disk state in detach, expected ejected"))
	case DiskStateEjected:
		// The disk is ejected but still attached, attempt to detach it again.
		if err := vm.RemoveSCSIDisk(ctx, d.controller, d.lun); err != nil {
			return fmt.Errorf("detach ejected disk from VM: %w", err)
		}
		d.state = DiskStateDetached
		return nil
	case DiskStateDetached:
		// Disk is already detached, this is idempotent.
		return nil
	}
	return fmt.Errorf("unexpected disk state %d", d.state)
}

func (d *Disk) ReservePartition(ctx context.Context, config mount.MountConfig) (*mount.Mount, error) {
	if d.state != DiskStateReserved && d.state != DiskStateAttached {
		return nil, fmt.Errorf("unexpected disk state %d, expected reserved or attached", d.state)
	}

	// Check if the partition is already reserved.
	if m, ok := d.mounts[config.Partition]; ok {
		if err := m.Reserve(config); err != nil {
			return nil, fmt.Errorf("reserve partition %d: %w", config.Partition, err)
		}
		return m, nil
	}
	// Create a new mount for this partition in the reserved state.
	m := mount.NewReserved(d.controller, d.lun, config)
	d.mounts[config.Partition] = m
	return m, nil
}

func (d *Disk) MountPartitionToGuest(ctx context.Context, partition uint64, linuxGuest mount.LinuxGuestSCSIMounter, windowsGuest mount.WindowsGuestSCSIMounter) (string, error) {
	if d.state != DiskStateAttached {
		return "", fmt.Errorf("unexpected disk state %d, expected attached", d.state)
	}
	if m, ok := d.mounts[partition]; ok {
		return m.MountToGuest(ctx, linuxGuest, windowsGuest)
	}
	return "", fmt.Errorf("partition %d not found on disk", partition)
}

func (d *Disk) UnmountPartitionFromGuest(ctx context.Context, partition uint64, linuxGuest mount.LinuxGuestSCSIUnmounter, windowsGuest mount.WindowsGuestSCSIUnmounter) error {
	if m, ok := d.mounts[partition]; ok {
		if err := m.UnmountFromGuest(ctx, linuxGuest, windowsGuest); err != nil {
			return fmt.Errorf("unmount partition %d from guest: %w", partition, err)
		}
		// This was the last caller of Unmount, remove the partition in the disk
		// mounts.
		if m.State() == mount.MountStateUnmounted {
			delete(d.mounts, partition)
		}
	}
	// Consider a not found mount, a success for retry logic in the caller.
	return nil
}
