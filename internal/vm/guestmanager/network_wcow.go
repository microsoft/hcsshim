//go:build windows && wcow

package guestmanager

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/hcn"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// AddNetworkNamespace adds a network namespace in the guest.
func (gm *Guest) AddNetworkNamespace(ctx context.Context, settings *hcn.HostComputeNamespace) error {
	modifyRequest := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeNetworkNamespace,
			RequestType:  guestrequest.RequestTypeAdd,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, modifyRequest.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to add network namespace: %w", err)
	}
	return nil
}

// RemoveNetworkNamespace removes a network namespace in the guest.
func (gm *Guest) RemoveNetworkNamespace(ctx context.Context, settings *hcn.HostComputeNamespace) error {
	modifyRequest := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeNetworkNamespace,
			RequestType:  guestrequest.RequestTypeRemove,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, modifyRequest.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to remove network namespace: %w", err)
	}
	return nil
}

// AddNetworkInterface adds a network interface to the WCOW guest.
func (gm *Guest) AddNetworkInterface(ctx context.Context, adapterID string, requestType guestrequest.RequestType, settings *hcn.HostComputeEndpoint) error {
	modifyRequest := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeNetwork,
			RequestType:  guestrequest.RequestTypeAdd,
			Settings: guestrequest.NetworkModifyRequest{
				AdapterId:   adapterID,
				RequestType: requestType,
				Settings:    settings, // endpoint configuration
			},
		},
	}

	err := gm.modify(ctx, modifyRequest.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to add network interface: %w", err)
	}
	return nil
}

// RemoveNetworkInterface removes a network interface from the WCOW guest.
func (gm *Guest) RemoveNetworkInterface(ctx context.Context, adapterID string, requestType guestrequest.RequestType, settings *hcn.HostComputeEndpoint) error {
	modifyRequest := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeNetwork,
			RequestType:  guestrequest.RequestTypeRemove,
			Settings: guestrequest.NetworkModifyRequest{
				AdapterId:   adapterID,
				RequestType: requestType,
				Settings:    settings, // endpoint configuration
			},
		},
	}

	err := gm.modify(ctx, modifyRequest.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to remove network interface: %w", err)
	}
	return nil
}
