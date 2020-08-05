package oci

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/uvm"
)

// HandleCPUGroupSetup will parse the cpugroup annotations and setup the cpugroup for `vm`
func HandleCPUGroupSetup(ctx context.Context, vm *uvm.UtilityVM, annotations map[string]string) error {
	cpuGroupOpts, err := annotationsToCPUGroupOptions(ctx, annotations)
	if err != nil {
		return err
	}
	if err := vm.ConfigureVMCPUGroup(ctx, cpuGroupOpts); err != nil {
		return err
	}
	return nil
}

// annotationsToCPUGroupOptions parses the related cpugroup annotations and creates the CPUGroupOptions from the values
func annotationsToCPUGroupOptions(ctx context.Context, annotations map[string]string) (*uvm.CPUGroupOptions, error) {
	processorTopology, err := uvm.HostProcessorInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get host processor information: %s", err)
	}
	lpIndices := []uint32{}
	for _, l := range processorTopology.LogicalProcessors {
		lpIndices = append(lpIndices, l.LpIndex)
	}

	opts := &uvm.CPUGroupOptions{
		CreateRandomID:    parseAnnotationsBool(ctx, annotations, annotationCPUGroupCreateRandomID, false),
		ID:                parseAnnotationsString(annotations, annotationCPUGroupID, uvm.CPUGroupNullID),
		LogicalProcessors: parseCommaSeperatedUint32(annotations, annotationCPUGroupLPs, lpIndices),
		Cap:               parseAnnotationsUint32(ctx, annotations, annotationCPUGroupCap, uvm.DefaultCPUGroupCap),
		Priority:          parseAnnotationsUint32(ctx, annotations, annotationCPUGroupPriority, uvm.DefaultCPUGroupPriority),
	}
	return opts, nil
}
