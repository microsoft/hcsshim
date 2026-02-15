//go:build windows

package vmmanager

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/pkg/errors"
)

// NetworkManager manages adding and removing network adapters for a Utility VM.
type NetworkManager interface {
	// AddNIC adds a network adapter to the Utility VM. `nicID` should be a string representation of a
	// Windows GUID.
	AddNIC(ctx context.Context, nicID string, settings *hcsschema.NetworkAdapter) error

	// RemoveNIC removes a network adapter from the Utility VM. `nicID` should be a string representation of a
	// Windows GUID.
	RemoveNIC(ctx context.Context, nicID string, settings *hcsschema.NetworkAdapter) error

	// UpdateNIC updates the configuration of a network adapter on the Utility VM.
	UpdateNIC(ctx context.Context, nicID string, settings *hcsschema.NetworkAdapter) error
}

var _ NetworkManager = (*UtilityVM)(nil)

func (uvm *UtilityVM) AddNIC(ctx context.Context, nicID string, settings *hcsschema.NetworkAdapter) error {
	request := hcsschema.ModifySettingRequest{
		RequestType:  guestrequest.RequestTypeAdd,
		ResourcePath: fmt.Sprintf(resourcepaths.NetworkResourceFormat, nicID),
		Settings:     settings,
	}
	if err := uvm.cs.Modify(ctx, request); err != nil {
		return errors.Wrapf(err, "failed to add NIC %s", nicID)
	}
	return nil
}

func (uvm *UtilityVM) RemoveNIC(ctx context.Context, nicID string, settings *hcsschema.NetworkAdapter) error {
	request := hcsschema.ModifySettingRequest{
		RequestType:  guestrequest.RequestTypeRemove,
		ResourcePath: fmt.Sprintf(resourcepaths.NetworkResourceFormat, nicID),
		Settings:     settings,
	}
	if err := uvm.cs.Modify(ctx, request); err != nil {
		return errors.Wrapf(err, "failed to remove NIC %s", nicID)
	}
	return nil
}

func (uvm *UtilityVM) UpdateNIC(ctx context.Context, nicID string, settings *hcsschema.NetworkAdapter) error {
	if settings == nil {
		return errors.New("network adapter settings cannot be nil")
	}
	req := &hcsschema.ModifySettingRequest{
		RequestType:  guestrequest.RequestTypeUpdate,
		ResourcePath: fmt.Sprintf(resourcepaths.NetworkResourceFormat, nicID),
		Settings:     settings,
	}
	if err := uvm.cs.Modify(ctx, req); err != nil {
		return errors.Wrapf(err, "failed to update NIC %s", nicID)
	}
	return nil
}
