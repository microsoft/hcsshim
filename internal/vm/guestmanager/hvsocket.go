//go:build windows

package guestmanager

import (
	"context"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/pkg/errors"
)

// updateHvSocketAddress configures the HvSocket address for GCS communication.
func (gm *Guest) updateHvSocketAddress(ctx context.Context, settings *hcsschema.HvSocketAddress) error {
	conSetupReq := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			RequestType:  guestrequest.RequestTypeUpdate,
			ResourceType: guestresource.ResourceTypeHvSocket,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, conSetupReq.GuestRequest)
	if err != nil {
		return errors.Wrap(err, "failed to update hvSocket address")
	}
	return nil
}
