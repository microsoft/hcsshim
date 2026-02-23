//go:build windows

package vmmanager

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
)

// PCIManager manages assiging pci devices to a Utility VM. This is Windows specific at the moment.
type PCIManager interface {
	// AddDevice adds a pci device identified by `vmbusGUID` to the Utility VM with the provided settings.
	AddDevice(ctx context.Context, vmbusGUID string, settings hcsschema.VirtualPciDevice) error

	// RemoveDevice removes the pci device identified by `vmbusGUID` from the Utility VM.
	RemoveDevice(ctx context.Context, vmbusGUID string) error
}

var _ PCIManager = (*UtilityVM)(nil)

func (uvm *UtilityVM) AddDevice(ctx context.Context, vmbusGUID string, settings hcsschema.VirtualPciDevice) error {
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

func (uvm *UtilityVM) RemoveDevice(ctx context.Context, vmbusGUID string) error {
	request := &hcsschema.ModifySettingRequest{
		ResourcePath: fmt.Sprintf(resourcepaths.VirtualPCIResourceFormat, vmbusGUID),
		RequestType:  guestrequest.RequestTypeRemove,
	}
	if err := uvm.cs.Modify(ctx, request); err != nil {
		return fmt.Errorf("failed to remove PCI device %s: %w", vmbusGUID, err)
	}
	return nil
}
