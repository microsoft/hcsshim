package remotevm

import (
	"context"

	"github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats"
	"github.com/Microsoft/hcsshim/internal/vm"
)

func (uvm *utilityVM) Stats(ctx context.Context) (*stats.VirtualMachineStatistics, error) {
	return nil, vm.ErrNotSupported
}
