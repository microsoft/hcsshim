//go:build windows

package disk

import (
	"context"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

// Type identifies the attachment protocol used when adding a disk to the VM's SCSI bus.
type Type string

const (
	// TypeVirtualDisk attaches the disk as a virtual hard disk (VHD/VHDX).
	TypeVirtualDisk Type = "VirtualDisk"

	// TypePassThru attaches a physical disk directly to the VM with pass-through access.
	TypePassThru Type = "PassThru"

	// TypeExtensibleVirtualDisk attaches a disk via an extensible virtual disk (EVD) provider.
	TypeExtensibleVirtualDisk Type = "ExtensibleVirtualDisk"
)

// Config describes the host-side disk to attach to the VM's SCSI bus.
type Config struct {
	// HostPath is the path on the host to the disk to be attached.
	HostPath string
	// ReadOnly specifies whether the disk should be attached with read-only access.
	ReadOnly bool
	// Type specifies the attachment protocol to use when attaching the disk.
	Type Type
	// EVDType is the EVD provider name.
	// Only populated when Type is [TypeExtensibleVirtualDisk].
	EVDType string
}

// Equals reports whether two disk Config values describe the same attachment parameters.
func (d Config) Equals(other Config) bool {
	return d.HostPath == other.HostPath &&
		d.ReadOnly == other.ReadOnly &&
		d.Type == other.Type &&
		d.EVDType == other.EVDType
}

// VMSCSIAdder adds a SCSI disk to a Utility VM's SCSI bus.
type VMSCSIAdder interface {
	// AddSCSIDisk hot-adds a SCSI disk to the Utility VM.
	AddSCSIDisk(ctx context.Context, disk hcsschema.Attachment, controller uint, lun uint) error
}

// VMSCSIRemover removes a SCSI disk from a Utility VM's SCSI bus.
type VMSCSIRemover interface {
	// RemoveSCSIDisk removes a SCSI disk from the Utility VM.
	RemoveSCSIDisk(ctx context.Context, controller uint, lun uint) error
}
