package uvm

import (
	"context"
	"fmt"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/guestrequest"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/requesttype"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
)

// VPCIDevice represents a vpci device. Holds its guid and a handle to the uvm it
// belongs to.
type VPCIDevice struct {
	vm *UtilityVM
	ID string
}

// Release frees the resources of the corresponding vpci device
func (vpci *VPCIDevice) Release(ctx context.Context) error {
	if err := vpci.vm.RemoveDevice(ctx, vpci.ID); err != nil {
		log.G(ctx).WithError(err).Warn("failed to remove VPCI device")
		return err
	}
	return nil
}

// AssignDevice assigns a new vpci device to the uvm
func (uvm *UtilityVM) AssignDevice(ctx context.Context, device hcsschema.VirtualPciDevice) (*VPCIDevice, error) {
	guid, err := guid.NewV4()
	if err != nil {
		return nil, err
	}
	id := guid.String()

	uvm.m.Lock()
	defer uvm.m.Unlock()
	if err := uvm.modify(ctx, &hcsschema.ModifySettingRequest{
		ResourcePath: fmt.Sprintf(virtualPciResourceFormat, id),
		RequestType:  requesttype.Add,
		Settings:     device,
		GuestRequest: guestrequest.GuestRequest{
			ResourceType: guestrequest.ResourceTypeVPCIDevice,
			RequestType:  requesttype.Add,
			Settings: guestrequest.LCOWMappedVPCIDevice{
				VMBusGUID: id,
			},
		},
	}); err != nil {
		return nil, err
	}
	return &VPCIDevice{
		vm: uvm,
		ID: id,
	}, nil
}

// RemoveDevice removes a vpci device from the uvm
func (uvm *UtilityVM) RemoveDevice(ctx context.Context, id string) error {
	uvm.m.Lock()
	defer uvm.m.Unlock()
	return uvm.modify(ctx, &hcsschema.ModifySettingRequest{
		ResourcePath: fmt.Sprintf(virtualPciResourceFormat, id),
		RequestType:  requesttype.Remove,
	})
}
