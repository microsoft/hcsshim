//go:build windows

package vmmanager

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
)

// VPMemManager manages adding and removing virtual persistent memory devices for a Utility VM.
type VPMemManager interface {
	// AddVPMemDevice adds a virtual pmem device to the Utility VM.
	AddVPMemDevice(ctx context.Context, id uint32, settings hcsschema.VirtualPMemDevice) error

	// RemoveVPMemDevice removes a virtual pmem device from the Utility VM.
	RemoveVPMemDevice(ctx context.Context, id uint32) error
}

var _ VPMemManager = (*UtilityVM)(nil)

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
