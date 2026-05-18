//go:build windows && (lcow || wcow)

package vmmanager

import (
	"context"
	"errors"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
)

func (uvm *UtilityVM) AddNIC(ctx context.Context, nicID string, settings *hcsschema.NetworkAdapter) error {
	request := hcsschema.ModifySettingRequest{
		RequestType:  guestrequest.RequestTypeAdd,
		ResourcePath: fmt.Sprintf(resourcepaths.NetworkResourceFormat, nicID),
		Settings:     settings,
	}
	if err := uvm.cs.Modify(ctx, request); err != nil {
		return fmt.Errorf("failed to add NIC %s: %w", nicID, err)
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
		return fmt.Errorf("failed to remove NIC %s: %w", nicID, err)
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
		return fmt.Errorf("failed to update NIC %s: %w", nicID, err)
	}
	return nil
}
