//go:build windows

package builder

import (
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/osversion"

	"github.com/pkg/errors"
)

// BootOptions configures boot settings for the Utility VM.
type BootOptions interface {
	// SetUEFIBoot sets UEFI configurations for booting a Utility VM.
	SetUEFIBoot(bootEntry *hcsschema.UefiBootEntry)
	// SetLinuxKernelDirectBoot sets Linux direct boot configurations for booting a Utility VM.
	SetLinuxKernelDirectBoot(options *hcsschema.LinuxKernelDirect) error
}

var _ BootOptions = (*UtilityVM)(nil)

func (uvmb *UtilityVM) SetUEFIBoot(bootEntry *hcsschema.UefiBootEntry) {
	uvmb.doc.VirtualMachine.Chipset.Uefi = &hcsschema.Uefi{
		BootThis: bootEntry,
	}
}

func (uvmb *UtilityVM) SetLinuxKernelDirectBoot(options *hcsschema.LinuxKernelDirect) error {
	if osversion.Get().Build < 18286 {
		return errors.New("Linux kernel direct boot requires at least Windows version 18286")
	}
	uvmb.doc.VirtualMachine.Chipset.LinuxKernelDirect = options
	return nil
}
