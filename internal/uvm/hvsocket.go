package uvm

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
)

// UpdateHvSocketService calls HCS to update/create the hvsocket service for
// the UVM. Takes in a service ID and the hvsocket service configuration. If there is no
// entry for the service ID already it will be created. The same call on HvSockets side
// handles the Create/Update/Delete cases based on what is passed in. Here is the logic
// for the call.
//
// 1. If the service ID does not currently exist in the service table, it will be created
// with whatever descriptors and state was specified (disabled or not).
// 2. If the service already exists and empty descriptors and Disabled is passed in for the
// service config, the service will be removed.
// 3. Otherwise any combination that is not Disabled && Empty descriptors will just update the
// service.
//
// If the request is crafted with Disabled = True and empty descriptors, then this function
// will behave identically to a call to RemoveHvSocketService. Prefer RemoveHvSocketService for this
// behavior as the relevant fields are set on HCS' side.
func (uvm *UtilityVM) UpdateHvSocketService(ctx context.Context, sid string, doc *hcsschema.HvSocketServiceConfig) error {
	request := &hcsschema.ModifySettingRequest{
		RequestType:  guestrequest.RequestTypeUpdate,
		ResourcePath: fmt.Sprintf(resourcepaths.HvSocketConfigResourceFormat, sid),
		Settings:     doc,
	}
	return uvm.modify(ctx, request)
}

// RemoveHvSocketService will remove an hvsocket service entry if it exists.
func (uvm *UtilityVM) RemoveHvSocketService(ctx context.Context, sid string) error {
	request := &hcsschema.ModifySettingRequest{
		RequestType:  guestrequest.RequestTypeRemove,
		ResourcePath: fmt.Sprintf(resourcepaths.HvSocketConfigResourceFormat, sid),
	}
	return uvm.modify(ctx, request)
}
