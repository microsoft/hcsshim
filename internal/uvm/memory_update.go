//go:build windows

package uvm

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/memory"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"
)

const bytesPerPage = 4096

// UpdateMemory makes a call to the VM's orchestrator to update the VM's size in MB
// Internally, HCS will get the number of pages this corresponds to and attempt to assign
// pages to numa nodes evenly
func (uvm *UtilityVM) UpdateMemory(ctx context.Context, sizeInBytes uint64) error {
	requestedSizeInMB := sizeInBytes / memory.MiB
	actual := vmutils.NormalizeMemorySize(ctx, uvm.id, requestedSizeInMB)
	req := &hcsschema.ModifySettingRequest{
		ResourcePath: resourcepaths.MemoryResourcePath,
		Settings:     actual,
	}
	return uvm.modify(ctx, req)
}

// GetAssignedMemoryInBytes returns the amount of assigned memory for the UVM in bytes
func (uvm *UtilityVM) GetAssignedMemoryInBytes(ctx context.Context) (uint64, error) {
	props, err := uvm.hcsSystem.PropertiesV2(ctx, hcsschema.PTMemory)
	if err != nil {
		return 0, err
	}
	if props.Memory == nil {
		return 0, fmt.Errorf("no memory properties returned for system %s", uvm.id)
	}
	if props.Memory.VirtualMachineMemory == nil {
		return 0, fmt.Errorf("no virtual memory properties returned for system %s", uvm.id)
	}
	pages := props.Memory.VirtualMachineMemory.AssignedMemory
	if pages == 0 {
		return 0, fmt.Errorf("assigned memory returned should not be 0 for system %s", uvm.id)
	}
	memInBytes := pages * bytesPerPage
	return memInBytes, nil
}
