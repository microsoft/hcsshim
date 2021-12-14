package hcs

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
)

func (uvm *utilityVM) AddDevice(ctx context.Context, instanceID, vmbusGUID string) error {
	request := &hcsschema.ModifySettingRequest{
		ResourcePath: fmt.Sprintf(resourcepaths.VirtualPCIResourceFormat, vmbusGUID),
		RequestType:  guestrequest.RequestTypeAdd,
		Settings: hcsschema.VirtualPciDevice{
			Functions: []hcsschema.VirtualPciFunction{
				{
					DeviceInstancePath: instanceID,
				},
			},
		},
	}
	return uvm.cs.Modify(ctx, request)
}

func (uvm *utilityVM) RemoveDevice(ctx context.Context, instanceID, vmbusGUID string) error {
	request := &hcsschema.ModifySettingRequest{
		ResourcePath: fmt.Sprintf(resourcepaths.VirtualPCIResourceFormat, vmbusGUID),
		RequestType:  guestrequest.RequestTypeRemove,
	}
	return uvm.cs.Modify(ctx, request)
}
