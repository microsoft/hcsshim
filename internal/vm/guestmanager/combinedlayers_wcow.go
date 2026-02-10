//go:build windows

package guestmanager

import (
	"context"
	"fmt"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// WCOWLayersManager exposes combined layer operations in the WCOW guest.
type WCOWLayersManager interface {
	// AddWCOWCombinedLayers adds combined layers to the WCOW guest.
	AddWCOWCombinedLayers(ctx context.Context, settings guestresource.WCOWCombinedLayers) error
	// AddCWCOWCombinedLayers adds combined layers to the CWCOW guest.
	AddCWCOWCombinedLayers(ctx context.Context, settings guestresource.CWCOWCombinedLayers) error
	// RemoveWCOWCombinedLayers removes combined layers from the WCOW guest.
	RemoveWCOWCombinedLayers(ctx context.Context, settings guestresource.WCOWCombinedLayers) error
	// RemoveCWCOWCombinedLayers removes combined layers from the CWCOW guest.
	RemoveCWCOWCombinedLayers(ctx context.Context, settings guestresource.CWCOWCombinedLayers) error
}

var _ WCOWLayersManager = (*Guest)(nil)

// AddWCOWCombinedLayers adds WCOW combined layers in the guest.
func (gm *Guest) AddWCOWCombinedLayers(ctx context.Context, settings guestresource.WCOWCombinedLayers) error {
	modifyRequest := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeCombinedLayers,
			RequestType:  guestrequest.RequestTypeAdd,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, modifyRequest.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to add WCOW combined layers: %w", err)
	}
	return nil
}

// AddCWCOWCombinedLayers adds combined layers in the CWCOW guest.
func (gm *Guest) AddCWCOWCombinedLayers(ctx context.Context, settings guestresource.CWCOWCombinedLayers) error {
	modifyRequest := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeCWCOWCombinedLayers,
			RequestType:  guestrequest.RequestTypeAdd,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, modifyRequest.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to add CWCOW combined layers: %w", err)
	}
	return nil
}

// RemoveWCOWCombinedLayers removes WCOW combined layers in the guest.
func (gm *Guest) RemoveWCOWCombinedLayers(ctx context.Context, settings guestresource.WCOWCombinedLayers) error {
	modifyRequest := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeCombinedLayers,
			RequestType:  guestrequest.RequestTypeRemove,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, modifyRequest.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to remove WCOW combined layers: %w", err)
	}
	return nil
}

// RemoveCWCOWCombinedLayers removes combined layers in CWCOW guest.
func (gm *Guest) RemoveCWCOWCombinedLayers(ctx context.Context, settings guestresource.CWCOWCombinedLayers) error {
	modifyRequest := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeCWCOWCombinedLayers,
			RequestType:  guestrequest.RequestTypeRemove,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, modifyRequest.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to remove CWCOW combined layers: %w", err)
	}
	return nil
}
