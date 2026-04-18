//go:build windows

package vmmanager

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
)

func (uvm *UtilityVM) AddVSMB(ctx context.Context, settings hcsschema.VirtualSmbShare) error {
	modification := &hcsschema.ModifySettingRequest{
		RequestType:  guestrequest.RequestTypeAdd,
		Settings:     settings,
		ResourcePath: resourcepaths.VSMBShareResourcePath,
	}
	if err := uvm.cs.Modify(ctx, modification); err != nil {
		return fmt.Errorf("failed to add VSMB share %s: %w", settings.Name, err)
	}
	return nil
}

func (uvm *UtilityVM) RemoveVSMB(ctx context.Context, settings hcsschema.VirtualSmbShare) error {
	modification := &hcsschema.ModifySettingRequest{
		RequestType:  guestrequest.RequestTypeRemove,
		Settings:     settings,
		ResourcePath: resourcepaths.VSMBShareResourcePath,
	}
	if err := uvm.cs.Modify(ctx, modification); err != nil {
		return fmt.Errorf("failed to remove VSMB share %s: %w", settings.Name, err)
	}
	return nil
}
