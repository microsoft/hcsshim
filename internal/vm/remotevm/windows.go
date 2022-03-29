//go:build windows

package remotevm

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/cpugroup"
	"github.com/Microsoft/hcsshim/internal/vmservice"
)

func (uvmb *utilityVMBuilder) SetCPUGroup(ctx context.Context, id string) error {
	if uvmb.config.WindowsOptions == nil {
		uvmb.config.WindowsOptions = &vmservice.WindowsOptions{}
	}
	// Map from guid to the underlying hypervisor ID.
	conf, err := cpugroup.GetCPUGroupConfig(ctx, id)
	if err != nil {
		return err
	}
	uvmb.config.WindowsOptions.CpuGroupID = conf.HypervisorGroupId
	return nil
}
