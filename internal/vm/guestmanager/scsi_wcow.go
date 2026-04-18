//go:build windows && wcow

package guestmanager

import (
	"context"
	"fmt"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// AddWCOWMappedVirtualDisk maps a virtual disk into a WCOW guest.
func (gm *Guest) AddWCOWMappedVirtualDisk(ctx context.Context, settings guestresource.WCOWMappedVirtualDisk) error {
	request := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeMappedVirtualDisk,
			RequestType:  guestrequest.RequestTypeAdd,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, request.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to add WCOW mapped virtual disk: %w", err)
	}
	return nil
}

// AddWCOWMappedVirtualDiskForContainerScratch attaches a scratch disk in the WCOW guest.
func (gm *Guest) AddWCOWMappedVirtualDiskForContainerScratch(ctx context.Context, settings guestresource.WCOWMappedVirtualDisk) error {
	request := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeMappedVirtualDiskForContainerScratch,
			RequestType:  guestrequest.RequestTypeAdd,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, request.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to add WCOW container scratch disk: %w", err)
	}
	return nil
}

// RemoveWCOWMappedVirtualDisk unmaps a virtual disk from the WCOW guest.
func (gm *Guest) RemoveWCOWMappedVirtualDisk(ctx context.Context, settings guestresource.WCOWMappedVirtualDisk) error {
	request := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeMappedVirtualDisk,
			RequestType:  guestrequest.RequestTypeRemove,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, request.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to remove WCOW mapped virtual disk: %w", err)
	}
	return nil
}
