//go:build windows

package guestmanager

import (
	"context"
	"fmt"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// LCOWLayersManager exposes combined layer operations in the LCOW guest.
type LCOWLayersManager interface {
	// AddLCOWCombinedLayers adds combined layers to the LCOW guest.
	AddLCOWCombinedLayers(ctx context.Context, settings guestresource.LCOWCombinedLayers) error
	// RemoveLCOWCombinedLayers removes combined layers from the LCOW guest.
	RemoveLCOWCombinedLayers(ctx context.Context, settings guestresource.LCOWCombinedLayers) error
}

var _ LCOWLayersManager = (*Guest)(nil)

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
