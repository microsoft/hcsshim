//go:build windows

package hcs

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

func (uvm *utilityVM) AddNIC(ctx context.Context, nicID, endpointID, macAddr string) error {
	request, err := hcsschema.NewModifySettingRequest(
		fmt.Sprintf(resourcepaths.NetworkResourceFormat, nicID),
		hcsschema.ModifyRequestType_ADD,
		hcsschema.NetworkAdapter{
			EndpointID: endpointID,
			MacAddress: macAddr,
		},
		nil, // guestRequest
	)
	if err != nil {
		return err
	}
	return uvm.cs.Modify(ctx, request)
}

func (uvm *utilityVM) RemoveNIC(ctx context.Context, nicID, endpointID, macAddr string) error {
	request, err := hcsschema.NewModifySettingRequest(
		fmt.Sprintf(resourcepaths.NetworkResourceFormat, nicID),
		hcsschema.ModifyRequestType_REMOVE,
		hcsschema.NetworkAdapter{
			EndpointID: endpointID,
			MacAddress: macAddr,
		},
		nil, // guestRequest
	)
	if err != nil {
		return err
	}
	return uvm.cs.Modify(ctx, request)
}
