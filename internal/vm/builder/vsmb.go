//go:build windows

package builder

import (
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"

	"github.com/pkg/errors"
)

func (uvmb *UtilityVM) AddVSMB(settings hcsschema.VirtualSmb) {
	uvmb.doc.VirtualMachine.Devices.VirtualSmb = &settings
}

func (uvmb *UtilityVM) AddVSMBShare(share hcsschema.VirtualSmbShare) error {
	if uvmb.doc.VirtualMachine.Devices.VirtualSmb == nil {
		return errors.New("VSMB has not been added")
	}

	uvmb.doc.VirtualMachine.Devices.VirtualSmb.Shares = append(
		uvmb.doc.VirtualMachine.Devices.VirtualSmb.Shares,
		share,
	)
	return nil
}
