//go:build windows && lcow

package guestmanager

import (
	"context"
	"fmt"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// AddMappedVirtualDisk maps a virtual disk into a LCOW guest.
func (gm *Guest) AddMappedVirtualDisk(ctx context.Context, settings guestresource.LCOWMappedVirtualDisk) error {
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

// RemoveMappedVirtualDisk unmaps a virtual disk from the LCOW guest.
func (gm *Guest) RemoveMappedVirtualDisk(ctx context.Context, settings guestresource.LCOWMappedVirtualDisk) error {
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
