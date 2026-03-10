//go:build windows

package uvm

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/cmd"
)

func (uvm *UtilityVM) DumpStacks(ctx context.Context) (string, error) {
	if uvm.gc == nil || !uvm.guestCaps.IsDumpStacksSupported() {
		return "", nil
	}

	return uvm.gc.DumpStacks(ctx)
}

func (uvm *UtilityVM) ExecInUVM(ctx context.Context, req *cmd.CmdProcessRequest) (int, error) {
	if uvm.gc == nil {
		return 0, errNotSupported
	}

	return cmd.ExecInUvm(ctx, uvm.gc, req)
}
