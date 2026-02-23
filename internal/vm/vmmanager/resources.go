//go:build windows

package vmmanager

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

type ResourceManager interface {
	// SetCPUGroup assigns the Utility VM to a cpu group.
	SetCPUGroup(ctx context.Context, settings *hcsschema.CpuGroup) error

	// UpdateCPULimits updates the CPU limits for the Utility VM.
	// `limit` is the percentage of CPU cycles that the Utility VM is allowed to use.
	// `weight` is the relative weight of the Utility VM compared to other VMs when CPU cycles are contended.
	// `reservation` is the percentage of CPU cycles that are reserved for the Utility VM.
	// `maximumFrequencyMHz` is the maximum frequency in MHz that the Utility VM can use.
	UpdateCPULimits(ctx context.Context, settings *hcsschema.ProcessorLimits) error

	// UpdateMemory makes a call to the VM's orchestrator to update the VM's size in MB
	UpdateMemory(ctx context.Context, memory uint64) error
}

var _ ResourceManager = (*UtilityVM)(nil)

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
