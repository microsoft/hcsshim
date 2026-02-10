//go:build windows

package guestmanager

import (
	"context"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/pkg/errors"
)

// LCOWNetworkManager exposes guest network operations.
type LCOWNetworkManager interface {
	// AddLCOWNetworkInterface adds a network interface to the LCOW guest.
	AddLCOWNetworkInterface(ctx context.Context, settings *guestresource.LCOWNetworkAdapter) error
	// RemoveLCOWNetworkInterface removes a network interface from the LCOW guest.
	RemoveLCOWNetworkInterface(ctx context.Context, settings *guestresource.LCOWNetworkAdapter) error
}

var _ LCOWNetworkManager = (*Guest)(nil)

// AddLCOWNetworkInterface adds a LCOW network interface using a typed adapter.
func (gm *Guest) AddLCOWNetworkInterface(ctx context.Context, settings *guestresource.LCOWNetworkAdapter) error {
	modifyRequest := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeNetwork,
			RequestType:  guestrequest.RequestTypeAdd,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, modifyRequest.GuestRequest)
	if err != nil {
		return errors.Wrap(err, "failed to add network interface for lcow")
	}
	return nil
}

// RemoveLCOWNetworkInterface removes a LCOW network interface using a typed adapter.
func (gm *Guest) RemoveLCOWNetworkInterface(ctx context.Context, settings *guestresource.LCOWNetworkAdapter) error {
	modifyRequest := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeNetwork,
			RequestType:  guestrequest.RequestTypeRemove,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, modifyRequest.GuestRequest)
	if err != nil {
		return errors.Wrap(err, "failed to remove network interface for lcow")
	}
	return nil
}
