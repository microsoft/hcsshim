//go:build windows

package remotevm

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/vmservice"
)

func (uvmb *utilityVMBuilder) SetProcessorCount(ctx context.Context, count uint32) error {
	if uvmb.config.ProcessorConfig == nil {
		uvmb.config.ProcessorConfig = &vmservice.ProcessorConfig{}
	}
	uvmb.config.ProcessorConfig.ProcessorCount = count
	return nil
}
