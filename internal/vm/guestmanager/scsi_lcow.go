//go:build windows

package guestmanager

import (
	"context"
	"fmt"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// LCOWScsiManager exposes mapped virtual disk and SCSI device operations in the LCOW guest.
type LCOWScsiManager interface {
	// AddLCOWMappedVirtualDisk maps a virtual disk into the LCOW guest.
	AddLCOWMappedVirtualDisk(ctx context.Context, settings guestresource.LCOWMappedVirtualDisk) error
	// RemoveLCOWMappedVirtualDisk unmaps a virtual disk from the LCOW guest.
	RemoveLCOWMappedVirtualDisk(ctx context.Context, settings guestresource.LCOWMappedVirtualDisk) error
	// RemoveSCSIDevice removes a SCSI device from the guest.
	RemoveSCSIDevice(ctx context.Context, settings guestresource.SCSIDevice) error
}

var _ LCOWScsiManager = (*Guest)(nil)

// AddLCOWMappedVirtualDisk maps a virtual disk into a LCOW guest.
func (gm *Guest) AddLCOWMappedVirtualDisk(ctx context.Context, settings guestresource.LCOWMappedVirtualDisk) error {
	request := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeMappedVirtualDisk,
			RequestType:  guestrequest.RequestTypeAdd,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, request.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to add LCOW mapped virtual disk: %w", err)
	}
	return nil
}

// RemoveLCOWMappedVirtualDisk unmaps a virtual disk from the LCOW guest.
func (gm *Guest) RemoveLCOWMappedVirtualDisk(ctx context.Context, settings guestresource.LCOWMappedVirtualDisk) error {
	request := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeMappedVirtualDisk,
			RequestType:  guestrequest.RequestTypeRemove,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, request.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to remove LCOW mapped virtual disk: %w", err)
	}
	return nil
}
