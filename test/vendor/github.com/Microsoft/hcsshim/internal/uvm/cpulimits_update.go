package uvm

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

// UpdateCPULimits updates the CPU limits of the utility vm
func (uvm *UtilityVM) UpdateCPULimits(ctx context.Context, limits *hcsschema.ProcessorLimits) error {
	req := &hcsschema.ModifySettingRequest{
		ResourcePath: resourcepaths.CPULimitsResourcePath,
		Settings:     limits,
	}

	return uvm.modify(ctx, req)
}
