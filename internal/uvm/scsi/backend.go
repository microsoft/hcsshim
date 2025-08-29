//go:build windows

package scsi

import (
	"context"
	"errors"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/gcs"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// The concrete types here (not the HostBackend/GuestBackend interfaces) would be a good option
// to move out to another package eventually. There is no real reason for them to live in
// the scsi package, and it could cause cyclical dependencies in the future.

// HostBackend provides the host-side operations needed to manage SCSI, such as attach/detach.
type HostBackend interface {
	attacher
}

// GuestBackend provides the guest-side operations needed to manage SCSI, such as mount/unmount
// and unplug.
type GuestBackend interface {
	mounter
	unplugger
}

// attacher provides the low-level operations for attaching a SCSI device to a VM.
type attacher interface {
	attach(ctx context.Context, controller, lun uint, config *attachConfig) error
	detach(ctx context.Context, controller, lun uint) error
}

// mounter provides the low-level operations for mounting a SCSI device inside the guest OS.
type mounter interface {
	mount(ctx context.Context, controller, lun uint, path string, config *mountConfig) error
	unmount(ctx context.Context, controller, lun uint, path string, config *mountConfig) error
}

// unplugger provides the low-level operations for cleanly removing a SCSI device inside the guest OS.
type unplugger interface {
	unplug(ctx context.Context, controller, lun uint) error
}

var _ attacher = &hcsHostBackend{}

type hcsHostBackend struct {
	system *hcs.System
}

// NewHCSHostBackend provides a [HostBackend] using a [hcs.System].
func NewHCSHostBackend(system *hcs.System) HostBackend {
	return &hcsHostBackend{system}
}

func (hhb *hcsHostBackend) attach(ctx context.Context, controller, lun uint, config *attachConfig) error {
	req := &hcsschema.ModifySettingRequest{
		RequestType: guestrequest.RequestTypeAdd,
		Settings: hcsschema.Attachment{
			Path:                      config.path,
			Type_:                     config.typ,
			ReadOnly:                  config.readOnly,
			ExtensibleVirtualDiskType: config.evdType,
		},
		ResourcePath: fmt.Sprintf(resourcepaths.SCSIResourceFormat, guestrequest.ScsiControllerGuids[controller], lun),
	}
	return hhb.system.Modify(ctx, req)
}

func (hhb *hcsHostBackend) detach(ctx context.Context, controller, lun uint) error {
	req := &hcsschema.ModifySettingRequest{
		RequestType:  guestrequest.RequestTypeRemove,
		ResourcePath: fmt.Sprintf(resourcepaths.SCSIResourceFormat, guestrequest.ScsiControllerGuids[controller], lun),
	}
	return hhb.system.Modify(ctx, req)
}

var _ mounter = &bridgeGuestBackend{}
var _ unplugger = &bridgeGuestBackend{}

type bridgeGuestBackend struct {
	gc     *gcs.GuestConnection
	osType string
}

// NewBridgeGuestBackend provides a [GuestBackend] using a [gcs.GuestConnection].
//
// osType should be either "windows" or "linux".
func NewBridgeGuestBackend(gc *gcs.GuestConnection, osType string) GuestBackend {
	return &bridgeGuestBackend{gc, osType}
}

func (bgb *bridgeGuestBackend) mount(ctx context.Context, controller, lun uint, path string, config *mountConfig) error {
	req, err := mountRequest(controller, lun, path, config, bgb.osType)
	if err != nil {
		return err
	}
	return bgb.gc.Modify(ctx, req)
}

func (bgb *bridgeGuestBackend) unmount(ctx context.Context, controller, lun uint, path string, config *mountConfig) error {
	req, err := unmountRequest(controller, lun, path, config, bgb.osType)
	if err != nil {
		return err
	}
	return bgb.gc.Modify(ctx, req)
}

func (bgb *bridgeGuestBackend) unplug(ctx context.Context, controller, lun uint) error {
	req, err := unplugRequest(controller, lun, bgb.osType)
	if err != nil {
		return err
	}
	if req.RequestType == "" {
		return nil
	}
	return bgb.gc.Modify(ctx, req)
}

var _ mounter = &hcsGuestBackend{}
var _ unplugger = &hcsGuestBackend{}

type hcsGuestBackend struct {
	system *hcs.System
	osType string
}

// NewHCSGuestBackend provides a [GuestBackend] using a [hcs.System].
//
// osType should be either "windows" or "linux".
func NewHCSGuestBackend(system *hcs.System, osType string) GuestBackend {
	return &hcsGuestBackend{system, osType}
}

func (hgb *hcsGuestBackend) mount(ctx context.Context, controller, lun uint, path string, config *mountConfig) error {
	req, err := mountRequest(controller, lun, path, config, hgb.osType)
	if err != nil {
		return err
	}
	return hgb.system.Modify(ctx, &hcsschema.ModifySettingRequest{GuestRequest: req})
}

func (hgb *hcsGuestBackend) unmount(ctx context.Context, controller, lun uint, path string, config *mountConfig) error {
	req, err := unmountRequest(controller, lun, path, config, hgb.osType)
	if err != nil {
		return err
	}
	return hgb.system.Modify(ctx, &hcsschema.ModifySettingRequest{GuestRequest: req})
}

func (hgb *hcsGuestBackend) unplug(ctx context.Context, controller, lun uint) error {
	req, err := unplugRequest(controller, lun, hgb.osType)
	if err != nil {
		return err
	}
	if req.RequestType == "" {
		return nil
	}
	return hgb.system.Modify(ctx, &hcsschema.ModifySettingRequest{GuestRequest: req})
}

func mountRequest(controller, lun uint, path string, config *mountConfig, osType string) (guestrequest.ModificationRequest, error) {
	req := guestrequest.ModificationRequest{
		ResourceType: guestresource.ResourceTypeMappedVirtualDisk,
		RequestType:  guestrequest.RequestTypeAdd,
	}
	// This option is set only for cwcow scratch disk mount requests
	// where we need to format the disk with refs.
	// For refs the scratch disk size should > 30 GB.
	if config.formatWithRefs {
		req.ResourceType = guestresource.ResourceTypeMappedVirtualDiskForContainerScratch
	}

	switch osType {
	case "windows":
		// We don't check config.readOnly here, as that will still result in the overall attachment being read-only.
		if controller != 0 {
			return guestrequest.ModificationRequest{}, errors.New("WCOW only supports SCSI controller 0")
		}
		if config.encrypted || len(config.options) != 0 ||
			config.ensureFilesystem || config.filesystem != "" || config.partition != 0 {
			return guestrequest.ModificationRequest{},
				errors.New("WCOW does not support encrypted, verity, guest options, partitions, specifying mount filesystem, or ensuring filesystem on mounts")
		}
		req.Settings = guestresource.WCOWMappedVirtualDisk{
			ContainerPath: path,
			Lun:           int32(lun),
		}

	case "linux":
		req.Settings = guestresource.LCOWMappedVirtualDisk{
			MountPath:        path,
			Controller:       uint8(controller),
			Lun:              uint8(lun),
			Partition:        config.partition,
			ReadOnly:         config.readOnly,
			Encrypted:        config.encrypted,
			Options:          config.options,
			EnsureFilesystem: config.ensureFilesystem,
			Filesystem:       config.filesystem,
			BlockDev:         config.blockDev,
		}
	default:
		return guestrequest.ModificationRequest{}, fmt.Errorf("unsupported os type: %s", osType)
	}
	return req, nil
}

func unmountRequest(controller, lun uint, path string, config *mountConfig, osType string) (guestrequest.ModificationRequest, error) {
	req := guestrequest.ModificationRequest{
		ResourceType: guestresource.ResourceTypeMappedVirtualDisk,
		RequestType:  guestrequest.RequestTypeRemove,
	}
	switch osType {
	case "windows":
		req.Settings = guestresource.WCOWMappedVirtualDisk{
			ContainerPath: path,
			Lun:           int32(lun),
		}
	case "linux":
		req.Settings = guestresource.LCOWMappedVirtualDisk{
			MountPath:  path,
			ReadOnly:   config.readOnly,
			Lun:        uint8(lun),
			Partition:  config.partition,
			Controller: uint8(controller),
			BlockDev:   config.blockDev,
		}
	default:
		return guestrequest.ModificationRequest{}, fmt.Errorf("unsupported os type: %s", osType)
	}
	return req, nil
}

func unplugRequest(controller, lun uint, osType string) (guestrequest.ModificationRequest, error) {
	var req guestrequest.ModificationRequest
	switch osType {
	case "windows":
		// Windows doesn't support an unplug operation, so treat as no-op.
	case "linux":
		req = guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeSCSIDevice,
			RequestType:  guestrequest.RequestTypeRemove,
			Settings: guestresource.SCSIDevice{
				Controller: uint8(controller),
				Lun:        uint8(lun),
			},
		}
	default:
		return guestrequest.ModificationRequest{}, fmt.Errorf("unsupported os type: %s", osType)
	}
	return req, nil
}
