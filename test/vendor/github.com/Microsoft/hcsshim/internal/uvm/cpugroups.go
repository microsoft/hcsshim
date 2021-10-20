package uvm

import (
	"context"
	"errors"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/cpugroup"
	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/osversion"
)

var errCPUGroupCreateNotSupported = fmt.Errorf("cpu group assignment on create requires a build of %d or higher", osversion.V21H1)

// ReleaseCPUGroup unsets the cpugroup from the VM
func (uvm *UtilityVM) ReleaseCPUGroup(ctx context.Context) error {
	if err := uvm.unsetCPUGroup(ctx); err != nil {
		return fmt.Errorf("failed to remove VM %s from cpugroup", uvm.ID())
	}
	return nil
}

// SetCPUGroup setups up the cpugroup for the VM with the requested id
func (uvm *UtilityVM) SetCPUGroup(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("must specify an ID to use when configuring a VM's cpugroup")
	}
	return uvm.setCPUGroup(ctx, id)
}

// setCPUGroup sets the VM's cpugroup
func (uvm *UtilityVM) setCPUGroup(ctx context.Context, id string) error {
	req := &hcsschema.ModifySettingRequest{
		ResourcePath: resourcepaths.CPUGroupResourcePath,
		Settings: &hcsschema.CpuGroup{
			Id: id,
		},
	}
	if err := uvm.modify(ctx, req); err != nil {
		return err
	}
	return nil
}

// unsetCPUGroup sets the VM's cpugroup to the null group ID
// set groupID to 00000000-0000-0000-0000-000000000000 to remove the VM from a cpugroup
//
// Since a VM must be moved to the null group before potentially being added to a different
// cpugroup, that means there may be a segment of time that the VM's cpu usage runs unrestricted.
func (uvm *UtilityVM) unsetCPUGroup(ctx context.Context) error {
	return uvm.setCPUGroup(ctx, cpugroup.NullGroupID)
}
