//go:build windows && lcow

package linuxcontainer

import (
	"context"

	plan9Mount "github.com/Microsoft/hcsshim/internal/controller/device/plan9/mount"
	"github.com/Microsoft/hcsshim/internal/controller/device/plan9/share"
	"github.com/Microsoft/hcsshim/internal/controller/device/scsi/disk"
	scsiMount "github.com/Microsoft/hcsshim/internal/controller/device/scsi/mount"
	"github.com/Microsoft/hcsshim/internal/controller/device/vpci"
	"github.com/Microsoft/hcsshim/internal/gcs"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"

	"github.com/Microsoft/go-winio/pkg/guid"
)

// CreateOpts holds additional options for container creation.
type CreateOpts struct {
	IsScratchEncryptionEnabled bool
}

// guest abstracts the UVM guest connection for container lifecycle operations.
type guest interface {
	Capabilities() gcs.GuestDefinedCapabilities
	CreateContainer(ctx context.Context, cid string, config interface{}) (*gcs.Container, error)
	DeleteContainerState(ctx context.Context, cid string) error

	AddLCOWCombinedLayers(ctx context.Context, settings guestresource.LCOWCombinedLayers) error
	RemoveLCOWCombinedLayers(ctx context.Context, settings guestresource.LCOWCombinedLayers) error
}

// scsiController abstracts host-side SCSI disk reservation and guest mapping.
type scsiController interface {
	Reserve(ctx context.Context, diskConfig disk.Config, mountConfig scsiMount.Config) (guid.GUID, error)
	UnmapFromGuest(ctx context.Context, reservation guid.GUID) error
	MapToGuest(ctx context.Context, id guid.GUID) (string, error)
}

// plan9Controller abstracts host-side Plan9 share reservation and guest mapping.
type plan9Controller interface {
	Reserve(ctx context.Context, shareConfig share.Config, mountConfig plan9Mount.Config) (guid.GUID, error)
	UnmapFromGuest(ctx context.Context, reservation guid.GUID) error
	MapToGuest(ctx context.Context, id guid.GUID) (string, error)
}

// vPCIController abstracts host-side virtual PCI device reservation and VM assignment.
type vPCIController interface {
	Reserve(ctx context.Context, device vpci.Device) (guid.GUID, error)
	RemoveFromVM(ctx context.Context, vmBusGUID guid.GUID) error
	AddToVM(ctx context.Context, vmBusGUID guid.GUID) error
}
