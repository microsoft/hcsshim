//go:build windows

package builder

import (
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

func (uvmb *UtilityVM) AddVSMBShare(share hcsschema.VirtualSmbShare) error {
	if uvmb.doc.VirtualMachine.Devices.VirtualSmb == nil {
		uvmb.doc.VirtualMachine.Devices.VirtualSmb = &hcsschema.VirtualSmb{
			DirectFileMappingInMB: 1024, // Sensible default, but could be a tuning parameter somewhere
		}
	}

	uvmb.doc.VirtualMachine.Devices.VirtualSmb.Shares = append(
		uvmb.doc.VirtualMachine.Devices.VirtualSmb.Shares,
		share,
	)
	return nil
}
