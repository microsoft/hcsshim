//go:build windows

package guestmanager

import (
	"context"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"

	"github.com/pkg/errors"
)

// RemoveSCSIDevice removes a SCSI device in the guest.
func (gm *Guest) RemoveSCSIDevice(ctx context.Context, settings guestresource.SCSIDevice) error {
	request := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeSCSIDevice,
			RequestType:  guestrequest.RequestTypeRemove,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, request.GuestRequest)
	if err != nil {
		return errors.Wrap(err, "failed to remove SCSI device")
	}
	return nil
}
