//go:build windows && lcow

package guestmanager

import (
	"context"
	"fmt"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// AddLCOWCombinedLayers adds LCOW combined layers in the guest.
func (gm *Guest) AddLCOWCombinedLayers(ctx context.Context, settings guestresource.LCOWCombinedLayers) error {
	modifyRequest := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeCombinedLayers,
			RequestType:  guestrequest.RequestTypeAdd,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, modifyRequest.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to add LCOW combined layers: %w", err)
	}
	return nil
}

// RemoveLCOWCombinedLayers removes LCOW combined layers in the guest.
func (gm *Guest) RemoveLCOWCombinedLayers(ctx context.Context, settings guestresource.LCOWCombinedLayers) error {
	modifyRequest := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeCombinedLayers,
			RequestType:  guestrequest.RequestTypeRemove,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, modifyRequest.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to remove LCOW combined layers: %w", err)
	}
	return nil
}
