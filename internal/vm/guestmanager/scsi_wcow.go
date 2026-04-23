//go:build windows && wcow

package guestmanager

import (
	"context"
	"fmt"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// AddMappedVirtualDisk maps a virtual disk into a WCOW guest.
func (gm *Guest) AddMappedVirtualDisk(ctx context.Context, settings guestresource.WCOWMappedVirtualDisk) error {
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

// AddMappedVirtualDiskForContainerScratch attaches a scratch disk in the WCOW guest.
func (gm *Guest) AddMappedVirtualDiskForContainerScratch(ctx context.Context, settings guestresource.WCOWMappedVirtualDisk) error {
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

// RemoveMappedVirtualDisk unmaps a virtual disk from the WCOW guest.
func (gm *Guest) RemoveMappedVirtualDisk(ctx context.Context, settings guestresource.WCOWMappedVirtualDisk) error {
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
