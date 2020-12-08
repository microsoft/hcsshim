package uvm

import (
	"context"
)

func (uvm *UtilityVM) UpdateContainer(ctx context.Context, cid string, resources interface{}) error {
	if uvm.gc == nil || !uvm.guestCaps.UpdateContainerSupported {
		return nil
	}
	return uvm.gc.UpdateContainer(ctx, cid, resources)
}
