//go:build windows

package guestmanager

import (
	"context"
	"fmt"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// HVSocketManager exposes the hvSocket operations in the Guest.
type HVSocketManager interface {
	UpdateHvSocketAddress(ctx context.Context, settings *hcsschema.HvSocketAddress) error
}

var _ HVSocketManager = (*Guest)(nil)

// UpdateHvSocketAddress updates the Hyper-V socket address settings for the VM.
// These address settings are applied by the GCS every time the VM starts or restores.
func (gm *Guest) UpdateHvSocketAddress(ctx context.Context, settings *hcsschema.HvSocketAddress) error {
	conSetupReq := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			RequestType:  guestrequest.RequestTypeUpdate,
			ResourceType: guestresource.ResourceTypeHvSocket,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, conSetupReq.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to update hvSocket address: %w", err)
	}
	return nil
}
