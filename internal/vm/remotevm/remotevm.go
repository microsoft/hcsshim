//go:build windows

package remotevm

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/Microsoft/hcsshim/internal/jobobject"
	"github.com/Microsoft/hcsshim/internal/vm"
	"github.com/Microsoft/hcsshim/internal/vmservice"
)

var _ vm.UVM = &utilityVM{}

type utilityVM struct {
	id           string
	waitError    error
	job          *jobobject.JobObject
	config       *vmservice.VMConfig
	client       vmservice.VMService
	capabilities *vmservice.CapabilitiesVMResponse
}

var vmSupportedResourceToVMService = map[vm.Resource]vmservice.CapabilitiesVMResponse_Resource{
	vm.VPMem:     vmservice.CapabilitiesVMResponse_Vpmem,
	vm.SCSI:      vmservice.CapabilitiesVMResponse_Scsi,
	vm.PCI:       vmservice.CapabilitiesVMResponse_Vpci,
	vm.Plan9:     vmservice.CapabilitiesVMResponse_Plan9,
	vm.Network:   vmservice.CapabilitiesVMResponse_VMNic,
	vm.Memory:    vmservice.CapabilitiesVMResponse_Memory,
	vm.Processor: vmservice.CapabilitiesVMResponse_Processor,
}

func (uvm *utilityVM) ID() string {
	return uvm.id
}

func (uvm *utilityVM) Start(ctx context.Context) error {
	// The expectation is the VM should be in a paused state after creation.
	if _, err := uvm.client.ResumeVM(ctx, &emptypb.Empty{}); err != nil {
		return fmt.Errorf("failed to start remote VM: %w", err)
	}
	return nil
}

func (uvm *utilityVM) Stop(ctx context.Context) error {
	if _, err := uvm.client.TeardownVM(ctx, &emptypb.Empty{}); err != nil {
		return fmt.Errorf("failed to stop remote VM: %w", err)
	}
	return nil
}

func (uvm *utilityVM) Wait() error {
	_, err := uvm.client.WaitVM(context.Background(), &emptypb.Empty{})
	if err != nil {
		uvm.waitError = err
		return fmt.Errorf("failed to wait on remote VM: %w", err)
	}
	return nil
}

func (uvm *utilityVM) Pause(ctx context.Context) error {
	if _, err := uvm.client.PauseVM(ctx, &emptypb.Empty{}); err != nil {
		return fmt.Errorf("failed to pause remote VM: %w", err)
	}
	return nil
}

func (uvm *utilityVM) Resume(ctx context.Context) error {
	if _, err := uvm.client.ResumeVM(ctx, &emptypb.Empty{}); err != nil {
		return fmt.Errorf("failed to resume remote VM: %w", err)
	}
	return nil
}

func (uvm *utilityVM) Save(ctx context.Context) error {
	return vm.ErrNotSupported
}

func (uvm *utilityVM) Supported(resource vm.Resource, operation vm.ResourceOperation) bool {
	var foundResource *vmservice.CapabilitiesVMResponse_SupportedResource
	for _, supportedResource := range uvm.capabilities.SupportedResources {
		if vmSupportedResourceToVMService[resource] == supportedResource.Resource {
			foundResource = supportedResource
		}
	}

	supported := false
	switch operation {
	case vm.Add:
		supported = foundResource.Add
	case vm.Remove:
		supported = foundResource.Remove
	case vm.Update:
		supported = foundResource.Update
	default:
		return false
	}

	return supported
}

func (uvm *utilityVM) ExitError() error {
	return uvm.waitError
}
