//go:build windows && (lcow || wcow)

package vmmanager

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

func (uvm *UtilityVM) SetCPUGroup(ctx context.Context, settings *hcsschema.CpuGroup) error {
	modification := &hcsschema.ModifySettingRequest{
		ResourcePath: resourcepaths.CPUGroupResourcePath,
		Settings:     settings,
	}
	if err := uvm.cs.Modify(ctx, modification); err != nil {
		return fmt.Errorf("failed to modify CPU Group: %w", err)
	}
	return nil
}

func (uvm *UtilityVM) UpdateCPULimits(ctx context.Context, settings *hcsschema.ProcessorLimits) error {
	modification := &hcsschema.ModifySettingRequest{
		ResourcePath: resourcepaths.CPULimitsResourcePath,
		Settings:     settings,
	}
	if err := uvm.cs.Modify(ctx, modification); err != nil {
		return fmt.Errorf("failed to modify CPU Limits: %w", err)
	}
	return nil
}

func (uvm *UtilityVM) UpdateMemory(ctx context.Context, memory uint64) error {
	modification := &hcsschema.ModifySettingRequest{
		ResourcePath: resourcepaths.MemoryResourcePath,
		Settings:     memory,
	}
	if err := uvm.cs.Modify(ctx, modification); err != nil {
		return fmt.Errorf("failed to modify memory: %w", err)
	}
	return nil
}
