//go:build windows

package hcs

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

func (uvm *utilityVM) AddDevice(ctx context.Context, instanceID, vmbusGUID string) error {
	request, err := hcsschema.NewModifySettingRequest(
		fmt.Sprintf(resourcepaths.VirtualPCIResourceFormat, vmbusGUID),
		hcsschema.ModifyRequestType_ADD,
		hcsschema.VirtualPCIDevice{
			Functions: []hcsschema.VirtualPCIFunction{
				{
					DeviceInstancePath: instanceID,
				},
			},
		},
		nil, // guestRequest
	)
	if err != nil {
		return err
	}
	return uvm.cs.Modify(ctx, request)
}

func (uvm *utilityVM) RemoveDevice(ctx context.Context, instanceID, vmbusGUID string) error {
	request, err := hcsschema.NewModifySettingRequest(
		fmt.Sprintf(resourcepaths.VirtualPCIResourceFormat, vmbusGUID),
		hcsschema.ModifyRequestType_REMOVE,
		nil,
		nil, // guestRequest
	)
	if err != nil {
		return err
	}
	return uvm.cs.Modify(ctx, request)
}
