//go:build windows

package vmmanager

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
)

// Plan9Manager manages adding plan 9 shares to a Utility VM.
type Plan9Manager interface {
	// AddPlan9 adds a plan 9 share to a running Utility VM.
	AddPlan9(ctx context.Context, settings hcsschema.Plan9Share) error

	// RemovePlan9 removes a plan 9 share from a running Utility VM.
	RemovePlan9(ctx context.Context, settings hcsschema.Plan9Share) error
}

var _ Plan9Manager = (*UtilityVM)(nil)

func (uvm *UtilityVM) AddPlan9(ctx context.Context, settings hcsschema.Plan9Share) error {
	modification := &hcsschema.ModifySettingRequest{
		RequestType:  guestrequest.RequestTypeAdd,
		Settings:     settings,
		ResourcePath: resourcepaths.Plan9ShareResourcePath,
	}
	if err := uvm.cs.Modify(ctx, modification); err != nil {
		return fmt.Errorf("failed to add Plan9 share %s: %w", settings.Name, err)
	}
	return nil
}

func (uvm *UtilityVM) RemovePlan9(ctx context.Context, settings hcsschema.Plan9Share) error {
	modification := &hcsschema.ModifySettingRequest{
		RequestType:  guestrequest.RequestTypeRemove,
		Settings:     settings,
		ResourcePath: resourcepaths.Plan9ShareResourcePath,
	}
	if err := uvm.cs.Modify(ctx, modification); err != nil {
		return fmt.Errorf("failed to remove Plan9 share %s: %w", settings.Name, err)
	}
	return nil
}
