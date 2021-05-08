package hcs

import (
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/pkg/errors"
)

func (uvmb *utilityVMBuilder) SetUEFIBoot(dir string, path string, args string) error {
	uvmb.doc.VirtualMachine.Chipset.Uefi = &hcsschema.Uefi{
		BootThis: &hcsschema.UefiBootEntry{
			DevicePath:    path,
			DeviceType:    "VmbFs",
			VmbFsRootPath: dir,
			OptionalData:  args,
		},
	}
	return nil
}

func (uvmb *utilityVMBuilder) SetLinuxKernelDirectBoot(kernel string, initRD string, cmd string) error {
	if osversion.Get().Build < 18286 {
		return errors.New("Linux kernel direct boot requires at least Windows version 18286")
	}
	uvmb.doc.VirtualMachine.Chipset.LinuxKernelDirect = &hcsschema.LinuxKernelDirect{
		KernelFilePath: kernel,
		InitRdPath:     initRD,
		KernelCmdLine:  cmd,
	}
	return nil
}
