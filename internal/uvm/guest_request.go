//go:build windows

package uvm

import (
	"context"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

// GuestRequest send an arbitrary guest request to the UVM.
func (uvm *UtilityVM) GuestRequest(ctx context.Context, guestReq interface{}) error {
	msr, err := hcsschema.NewModifySettingGuestRequest(guestReq)
	if err != nil {
		return err
	}
	return uvm.modify(ctx, &msr)
}
