package uvm

import (
	"context"
)

func (uvm *UtilityVM) DumpStacks(ctx context.Context) (string, error) {
	if uvm.gc == nil || !uvm.guestCaps.DumpStacksSupported {
		return "", nil
	}

	return uvm.gc.DumpStacks(ctx)
}
