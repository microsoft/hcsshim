//go:build windows

package builder

import (
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

// NumaOptions configures NUMA settings for the Utility VM.
type NumaOptions interface {
	// SetNUMAProcessorsSettings sets the NUMA processor settings for the Utility VM.
	SetNUMAProcessorsSettings(numaProcessors *hcsschema.NumaProcessors)
	// SetNUMASettings sets the NUMA settings for the Utility VM.
	SetNUMASettings(numa *hcsschema.Numa)
}

var _ NumaOptions = (*UtilityVM)(nil)

func (uvmb *UtilityVM) SetNUMAProcessorsSettings(numaProcessors *hcsschema.NumaProcessors) {
	uvmb.doc.VirtualMachine.ComputeTopology.Processor.NumaProcessorsSettings = numaProcessors
}

func (uvmb *UtilityVM) SetNUMASettings(numa *hcsschema.Numa) {
	uvmb.doc.VirtualMachine.ComputeTopology.Numa = numa
}
