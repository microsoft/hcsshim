package uvm

import (
	"context"
	"errors"
)

func (uvm *UtilityVM) DeleteContainerState(ctx context.Context, cid string) error {
	if !uvm.DeleteContainerStateSupported() {
		return errors.New("uvm guest connection does not support deleteContainerState")
	}

	return uvm.gc.DeleteContainerState(ctx, cid)
}
