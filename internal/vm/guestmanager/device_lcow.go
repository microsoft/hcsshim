//go:build windows

package guestmanager

import (
	"context"
	"fmt"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// LCOWDeviceManager exposes VPCI and VPMem device operations in the LCOW guest.
type LCOWDeviceManager interface {
	// AddVPCIDevice adds a VPCI device to the guest.
	AddVPCIDevice(ctx context.Context, settings guestresource.LCOWMappedVPCIDevice) error
	// AddVPMemDevice adds a VPMem device to the guest.
	AddVPMemDevice(ctx context.Context, settings guestresource.LCOWMappedVPMemDevice) error
	// RemoveVPMemDevice removes a VPMem device from the guest.
	RemoveVPMemDevice(ctx context.Context, settings guestresource.LCOWMappedVPMemDevice) error
}

var _ LCOWDeviceManager = (*Guest)(nil)

// AddVPCIDevice adds a VPCI device in the guest.
func (gm *Guest) AddVPCIDevice(ctx context.Context, settings guestresource.LCOWMappedVPCIDevice) error {
	request := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeVPCIDevice,
			RequestType:  guestrequest.RequestTypeAdd,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, request.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to add VPCI device: %w", err)
	}
	return nil
}

// AddVPMemDevice adds a VPMem device in the guest.
func (gm *Guest) AddVPMemDevice(ctx context.Context, settings guestresource.LCOWMappedVPMemDevice) error {
	request := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeVPMemDevice,
			RequestType:  guestrequest.RequestTypeAdd,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, request.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to add VPMem device: %w", err)
	}
	return nil
}

// RemoveVPMemDevice removes a VPMem device in the guest.
func (gm *Guest) RemoveVPMemDevice(ctx context.Context, settings guestresource.LCOWMappedVPMemDevice) error {
	request := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeVPMemDevice,
			RequestType:  guestrequest.RequestTypeRemove,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, request.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to remove VPMem device: %w", err)
	}
	return nil
}
