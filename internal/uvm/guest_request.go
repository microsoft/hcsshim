package uvm

import (
	"context"

	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
)

// GuestRequest send an arbitrary guest request to the UVM.
func (uvm *UtilityVM) GuestRequest(ctx context.Context, guestReq interface{}) error {
	msr := &hcsschema.ModifySettingRequest{
		GuestRequest: guestReq,
	}
	return uvm.modify(ctx, msr)
}
