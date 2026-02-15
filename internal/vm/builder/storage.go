//go:build windows

package builder

import (
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

// StorageQoSOptions configures storage QoS settings for the Utility VM.
type StorageQoSOptions interface {
	// SetStorageQoS sets storage related options for the Utility VM
	SetStorageQoS(options *hcsschema.StorageQoS)
}

var _ StorageQoSOptions = (*UtilityVM)(nil)

func (uvmb *UtilityVM) SetStorageQoS(options *hcsschema.StorageQoS) {
	if options == nil {
		return
	}

	if uvmb.doc.VirtualMachine.StorageQoS == nil {
		uvmb.doc.VirtualMachine.StorageQoS = &hcsschema.StorageQoS{}
	}

	uvmb.doc.VirtualMachine.StorageQoS.BandwidthMaximum = options.BandwidthMaximum
	uvmb.doc.VirtualMachine.StorageQoS.IopsMaximum = options.IopsMaximum
}
