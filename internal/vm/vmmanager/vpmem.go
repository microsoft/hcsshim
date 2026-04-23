//go:build windows && lcow

package vmmanager

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
)

func (uvm *UtilityVM) AddVPMemDevice(ctx context.Context, id uint32, settings hcsschema.VirtualPMemDevice) error {
	request := &hcsschema.ModifySettingRequest{
		RequestType:  guestrequest.RequestTypeAdd,
		Settings:     settings,
		ResourcePath: fmt.Sprintf(resourcepaths.VPMemControllerResourceFormat, id),
	}
	if err := uvm.cs.Modify(ctx, request); err != nil {
		return fmt.Errorf("failed to add VPMem device %d: %w", id, err)
	}
	return nil
}

func (uvm *UtilityVM) RemoveVPMemDevice(ctx context.Context, id uint32) error {
	request := &hcsschema.ModifySettingRequest{
		RequestType:  guestrequest.RequestTypeRemove,
		ResourcePath: fmt.Sprintf(resourcepaths.VPMemControllerResourceFormat, id),
	}
	if err := uvm.cs.Modify(ctx, request); err != nil {
		return fmt.Errorf("failed to remove VPMem device %d: %w", id, err)
	}
	return nil
}
