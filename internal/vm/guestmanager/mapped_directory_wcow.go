//go:build windows

package guestmanager

import (
	"context"
	"fmt"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// WCOWDirectoryManager exposes mapped directory operations in the WCOW guest.
type WCOWDirectoryManager interface {
	// AddMappedDirectory maps a directory into the WCOW guest.
	AddMappedDirectory(ctx context.Context, settings *hcsschema.MappedDirectory) error
}

var _ WCOWDirectoryManager = (*Guest)(nil)

// AddMappedDirectory maps a directory into the guest.
func (gm *Guest) AddMappedDirectory(ctx context.Context, settings *hcsschema.MappedDirectory) error {
	request := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeMappedDirectory,
			RequestType:  guestrequest.RequestTypeAdd,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, request.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to add mapped directory: %w", err)
	}
	return nil
}
