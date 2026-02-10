//go:build windows

package guestmanager

import (
	"context"
	"fmt"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// CIMsManager exposes guest WCOW block CIM operations.
type CIMsManager interface {
	// AddWCOWBlockCIMs adds WCOW block CIM mounts in the guest.
	AddWCOWBlockCIMs(ctx context.Context, settings *guestresource.CWCOWBlockCIMMounts) error
	// RemoveWCOWBlockCIMs removes WCOW block CIM mounts from the guest.
	RemoveWCOWBlockCIMs(ctx context.Context, settings *guestresource.CWCOWBlockCIMMounts) error
}

var _ CIMsManager = (*Guest)(nil)

// AddWCOWBlockCIMs adds WCOW block CIM mounts in the guest.
func (gm *Guest) AddWCOWBlockCIMs(ctx context.Context, settings *guestresource.CWCOWBlockCIMMounts) error {
	request := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeWCOWBlockCims,
			RequestType:  guestrequest.RequestTypeAdd,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, request.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to add WCOW block CIMs: %w", err)
	}

	return nil
}

// RemoveWCOWBlockCIMs removes WCOW block CIM mounts in the guest.
func (gm *Guest) RemoveWCOWBlockCIMs(ctx context.Context, settings *guestresource.CWCOWBlockCIMMounts) error {
	request := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeWCOWBlockCims,
			RequestType:  guestrequest.RequestTypeRemove,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, request.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to remove WCOW block CIMs: %w", err)
	}

	return nil
}
