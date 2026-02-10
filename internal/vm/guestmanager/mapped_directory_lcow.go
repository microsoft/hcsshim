//go:build windows

package guestmanager

import (
	"context"
	"fmt"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// LCOWDirectoryManager exposes mapped directory operations in the LCOW guest.
type LCOWDirectoryManager interface {
	// AddLCOWMappedDirectory maps a directory into the LCOW guest.
	AddLCOWMappedDirectory(ctx context.Context, settings guestresource.LCOWMappedDirectory) error
	// RemoveLCOWMappedDirectory unmaps a directory from the LCOW guest.
	RemoveLCOWMappedDirectory(ctx context.Context, settings guestresource.LCOWMappedDirectory) error
}

var _ LCOWDirectoryManager = (*Guest)(nil)

// AddLCOWMappedDirectory maps a directory into LCOW guest.
func (gm *Guest) AddLCOWMappedDirectory(ctx context.Context, settings guestresource.LCOWMappedDirectory) error {
	request := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeMappedDirectory,
			RequestType:  guestrequest.RequestTypeAdd,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, request.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to add mapped directory for lcow: %w", err)
	}
	return nil
}

// RemoveLCOWMappedDirectory unmaps a directory from LCOW guest.
func (gm *Guest) RemoveLCOWMappedDirectory(ctx context.Context, settings guestresource.LCOWMappedDirectory) error {
	request := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeMappedDirectory,
			RequestType:  guestrequest.RequestTypeRemove,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, request.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to remove mapped directory for lcow: %w", err)
	}
	return nil
}
