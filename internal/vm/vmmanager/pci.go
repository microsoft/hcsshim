//go:build windows

package vmmanager

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/pkg/errors"
)

// PCIManager manages assiging pci devices to a Utility VM. This is Windows specific at the moment.
type PCIManager interface {
	// AddDevice adds the pci device identified by `instanceID` to the Utility VM.
	// https://docs.microsoft.com/en-us/windows-hardware/drivers/install/instance-ids
	AddDevice(ctx context.Context, vmbusGUID string, function hcsschema.VirtualPciFunction) error

	// RemoveDevice removes the pci device identified by `vmbusGUID` from the Utility VM.
	RemoveDevice(ctx context.Context, vmbusGUID string) error
}

var _ PCIManager = (*UtilityVM)(nil)

func (uvm *UtilityVM) AddDevice(ctx context.Context, vmbusGUID string, function hcsschema.VirtualPciFunction) error {
	var propagateAffinity *bool
	T := true
	if osversion.Get().Build >= osversion.V25H1Server {
		propagateAffinity = &T
	}
	request := &hcsschema.ModifySettingRequest{
		ResourcePath: fmt.Sprintf(resourcepaths.VirtualPCIResourceFormat, vmbusGUID),
		RequestType:  guestrequest.RequestTypeAdd,
		Settings: hcsschema.VirtualPciDevice{
			Functions: []hcsschema.VirtualPciFunction{
				function,
			},
			PropagateNumaAffinity: propagateAffinity,
		},
	}
	if err := uvm.cs.Modify(ctx, request); err != nil {
		return errors.Wrapf(err, "failed to add PCI device %s", vmbusGUID)
	}
	return nil
}

func (uvm *UtilityVM) RemoveDevice(ctx context.Context, vmbusGUID string) error {
	request := &hcsschema.ModifySettingRequest{
		ResourcePath: fmt.Sprintf(resourcepaths.VirtualPCIResourceFormat, vmbusGUID),
		RequestType:  guestrequest.RequestTypeRemove,
	}
	if err := uvm.cs.Modify(ctx, request); err != nil {
		return errors.Wrapf(err, "failed to remove PCI device %s", vmbusGUID)
	}
	return nil
}
