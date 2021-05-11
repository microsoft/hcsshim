package remotevm

import (
	"github.com/Microsoft/hcsshim/internal/vmservice"
)

func (uvmb *utilityVMBuilder) SetLinuxKernelDirectBoot(kernel, initRD, cmd string) error {
	uvmb.config.BootConfig = &vmservice.VMConfig_DirectBoot{
		DirectBoot: &vmservice.DirectBoot{
			KernelPath:    kernel,
			InitrdPath:    initRD,
			KernelCmdline: cmd,
		},
	}
	return nil
}

func (uvmb *utilityVMBuilder) SetUEFIBoot(firmware, devPath, args string) error {
	uvmb.config.BootConfig = &vmservice.VMConfig_Uefi{
		Uefi: &vmservice.UEFI{
			FirmwarePath: firmware,
			DevicePath:   devPath,
			OptionalData: args,
		},
	}
	return nil
}
