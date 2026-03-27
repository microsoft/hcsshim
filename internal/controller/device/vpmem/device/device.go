//go:build windows

package device

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/controller/device/vpmem/mount"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

// DeviceConfig describes the host-side VPMem device to attach to the VM.
type DeviceConfig struct {
	// HostPath is the path on the host to the VHD to be attached.
	HostPath string
	// ReadOnly specifies whether the device should be attached with read-only access.
	ReadOnly bool
	// ImageFormat specifies the image format of the VHD (e.g. "Vhd1").
	ImageFormat string
}

// Equals reports whether two DeviceConfig values describe the same attachment parameters.
func (d DeviceConfig) Equals(other DeviceConfig) bool {
	return d.HostPath == other.HostPath &&
		d.ReadOnly == other.ReadOnly &&
		d.ImageFormat == other.ImageFormat
}

type DeviceState int

const (
	// The device has never been attached.
	DeviceStateReserved DeviceState = iota
	// The device is currently attached to the guest.
	DeviceStateAttached
	// The device was previously attached and detached, this is terminal.
	DeviceStateDetached
)

type VMVPMemAdder interface {
	AddVPMemDevice(ctx context.Context, device hcsschema.VirtualPMemDevice, deviceNumber uint32) error
}

type VMVPMemRemover interface {
	RemoveVPMemDevice(ctx context.Context, deviceNumber uint32) error
}

// Device represents a VPMem device attached to the VM. It manages the lifecycle
// of the device attachment as well as the guest mount on the device.
//
// All operations on the device are expected to be ordered by the caller. No
// locking is done at this layer.
type Device struct {
	deviceNumber uint32
	config       DeviceConfig

	state DeviceState
	mount *mount.Mount
}

// NewReserved creates a new Device in the reserved state with the provided
// configuration.
func NewReserved(deviceNumber uint32, config DeviceConfig) *Device {
	return &Device{
		deviceNumber: deviceNumber,
		config:       config,
		state:        DeviceStateReserved,
	}
}

func (d *Device) State() DeviceState {
	return d.state
}

func (d *Device) Config() DeviceConfig {
	return d.config
}

func (d *Device) HostPath() string {
	return d.config.HostPath
}

func (d *Device) AttachToVM(ctx context.Context, vm VMVPMemAdder) error {
	switch d.state {
	case DeviceStateReserved:
		if err := vm.AddVPMemDevice(ctx, hcsschema.VirtualPMemDevice{
			HostPath:    d.config.HostPath,
			ReadOnly:    d.config.ReadOnly,
			ImageFormat: d.config.ImageFormat,
		}, d.deviceNumber); err != nil {
			// Move to detached since we know from reserved there was no guest
			// state.
			d.state = DeviceStateDetached
			return fmt.Errorf("attach VPMem device to VM: %w", err)
		}
		d.state = DeviceStateAttached
		return nil
	case DeviceStateAttached:
		return nil
	case DeviceStateDetached:
		return fmt.Errorf("device already detached")
	}
	return nil
}

func (d *Device) DetachFromVM(ctx context.Context, vm VMVPMemRemover) error {
	switch d.state {
	case DeviceStateReserved:
		return nil
	case DeviceStateAttached:
		// Ensure for correctness nobody leaked a mount.
		if d.mount != nil {
			// This device is still active by a mount. Leave it.
			return nil
		}
		if err := vm.RemoveVPMemDevice(ctx, d.deviceNumber); err != nil {
			return fmt.Errorf("detach VPMem device from VM: %w", err)
		}
		d.state = DeviceStateDetached
		return nil
	case DeviceStateDetached:
		return nil
	}
	return fmt.Errorf("unexpected device state %d", d.state)
}

func (d *Device) ReserveMount(ctx context.Context, config mount.MountConfig) (*mount.Mount, error) {
	if d.state != DeviceStateReserved && d.state != DeviceStateAttached {
		return nil, fmt.Errorf("unexpected device state %d, expected reserved or attached", d.state)
	}

	if d.mount != nil {
		if err := d.mount.Reserve(config); err != nil {
			return nil, fmt.Errorf("reserve mount: %w", err)
		}
		return d.mount, nil
	}
	m := mount.NewReserved(d.deviceNumber, config)
	d.mount = m
	return m, nil
}

func (d *Device) MountToGuest(ctx context.Context, guest mount.LinuxGuestVPMemMounter) (string, error) {
	if d.state != DeviceStateAttached {
		return "", fmt.Errorf("unexpected device state %d, expected attached", d.state)
	}
	if d.mount == nil {
		return "", fmt.Errorf("no mount reserved on device")
	}
	return d.mount.MountToGuest(ctx, guest)
}

func (d *Device) UnmountFromGuest(ctx context.Context, guest mount.LinuxGuestVPMemUnmounter) error {
	if d.mount == nil {
		// Consider a missing mount a success for retry logic in the caller.
		return nil
	}
	if err := d.mount.UnmountFromGuest(ctx, guest); err != nil {
		return fmt.Errorf("unmount from guest: %w", err)
	}
	if d.mount.State() == mount.MountStateUnmounted {
		d.mount = nil
	}
	return nil
}
