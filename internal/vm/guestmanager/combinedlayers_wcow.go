//go:build windows && wcow

package guestmanager

import (
	"context"
	"fmt"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// AddCombinedLayers adds WCOW combined layers in the guest.
func (gm *Guest) AddCombinedLayers(ctx context.Context, settings guestresource.WCOWCombinedLayers) error {
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

// AddConfidentialCombinedLayers adds combined layers in the CWCOW guest.
func (gm *Guest) AddConfidentialCombinedLayers(ctx context.Context, settings guestresource.CWCOWCombinedLayers) error {
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

// RemoveCombinedLayers removes WCOW combined layers in the guest.
func (gm *Guest) RemoveCombinedLayers(ctx context.Context, settings guestresource.WCOWCombinedLayers) error {
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

// RemoveConfidentialCombinedLayers removes combined layers in CWCOW guest.
func (gm *Guest) RemoveConfidentialCombinedLayers(ctx context.Context, settings guestresource.CWCOWCombinedLayers) error {
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
