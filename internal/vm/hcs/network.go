package hcs

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
)

func (uvm *utilityVM) AddNIC(ctx context.Context, nicID, endpointID, macAddr string) error {
	request := hcsschema.ModifySettingRequest{
		RequestType:  guestrequest.RequestTypeAdd,
		ResourcePath: fmt.Sprintf(resourcepaths.NetworkResourceFormat, nicID),
		Settings: hcsschema.NetworkAdapter{
			EndpointId: endpointID,
			MacAddress: macAddr,
		},
	}
	return uvm.cs.Modify(ctx, request)
}

func (uvm *utilityVM) RemoveNIC(ctx context.Context, nicID, endpointID, macAddr string) error {
	request := hcsschema.ModifySettingRequest{
		RequestType:  guestrequest.RequestTypeRemove,
		ResourcePath: fmt.Sprintf(resourcepaths.NetworkResourceFormat, nicID),
		Settings: hcsschema.NetworkAdapter{
			EndpointId: endpointID,
			MacAddress: macAddr,
		},
	}
	return uvm.cs.Modify(ctx, request)
}
