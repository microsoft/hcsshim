//go:build windows && (lcow || wcow)

package vmmanager

import (
	"context"
	"fmt"

	"github.com/Microsoft/go-winio/pkg/guid"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
)

func (uvm *UtilityVM) AddDevice(ctx context.Context, vmbusGUID guid.GUID, settings hcsschema.VirtualPciDevice) error {
	request := &hcsschema.ModifySettingRequest{
		ResourcePath: fmt.Sprintf(resourcepaths.VirtualPCIResourceFormat, vmbusGUID),
		RequestType:  guestrequest.RequestTypeAdd,
		Settings:     settings,
	}
	if err := uvm.cs.Modify(ctx, request); err != nil {
		return fmt.Errorf("failed to add PCI device %s: %w", vmbusGUID, err)
	}
	return nil
}

func (uvm *UtilityVM) RemoveDevice(ctx context.Context, vmbusGUID guid.GUID) error {
	request := &hcsschema.ModifySettingRequest{
		ResourcePath: fmt.Sprintf(resourcepaths.VirtualPCIResourceFormat, vmbusGUID),
		RequestType:  guestrequest.RequestTypeRemove,
	}
	if err := uvm.cs.Modify(ctx, request); err != nil {
		return fmt.Errorf("failed to remove PCI device %s: %w", vmbusGUID, err)
	}
	return nil
}
