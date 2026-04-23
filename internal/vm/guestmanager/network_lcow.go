//go:build windows && lcow

package guestmanager

import (
	"context"
	"fmt"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// AddNetworkInterface adds a network interface to the LCOW guest.
func (gm *Guest) AddNetworkInterface(ctx context.Context, settings *guestresource.LCOWNetworkAdapter) error {
	modifyRequest := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeNetwork,
			RequestType:  guestrequest.RequestTypeAdd,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, modifyRequest.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to add network interface for lcow: %w", err)
	}
	return nil
}

// RemoveNetworkInterface removes a network interface from the LCOW guest.
func (gm *Guest) RemoveNetworkInterface(ctx context.Context, settings *guestresource.LCOWNetworkAdapter) error {
	modifyRequest := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeNetwork,
			RequestType:  guestrequest.RequestTypeRemove,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, modifyRequest.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to remove network interface for lcow: %w", err)
	}
	return nil
}
