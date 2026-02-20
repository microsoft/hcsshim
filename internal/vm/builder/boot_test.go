//go:build windows

package builder

import (
	"testing"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

func TestBootConfig(t *testing.T) {
	b, cs := newBuilder(t)
	var mboot BootOptions = b

	bootEntry := &hcsschema.UefiBootEntry{
		DevicePath:    "path",
		DeviceType:    "VmbFs",
		VmbFsRootPath: "root",
		OptionalData:  "args",
	}
	mboot.SetUEFIBoot(bootEntry)

	if cs.VirtualMachine.Chipset.Uefi == nil || cs.VirtualMachine.Chipset.Uefi.BootThis == nil {
		t.Fatal("UEFI boot not applied")
	}

	got := cs.VirtualMachine.Chipset.Uefi.BootThis
	if got.DevicePath != "path" {
		t.Fatalf("UEFI DevicePath = %q, want %q", got.DevicePath, "path")
	}
	if got.DeviceType != "VmbFs" {
		t.Fatalf("UEFI DeviceType = %q, want %q", got.DeviceType, "VmbFs")
	}
	if got.VmbFsRootPath != "root" {
		t.Fatalf("UEFI VmbFsRootPath = %q, want %q", got.VmbFsRootPath, "root")
	}
	if got.OptionalData != "args" {
		t.Fatalf("UEFI OptionalData = %q, want %q", got.OptionalData, "args")
	}

	linuxBoot := &hcsschema.LinuxKernelDirect{
		KernelFilePath: "kernel",
		InitRdPath:     "initrd",
		KernelCmdLine:  "cmd",
	}
	err := mboot.SetLinuxKernelDirectBoot(linuxBoot)
	if err != nil {
		t.Fatalf("SetLinuxKernelDirectBoot error = %v", err)
	}
	if cs.VirtualMachine.Chipset.LinuxKernelDirect == nil {
		t.Fatal("LinuxKernelDirect not applied")
	}
	lkd := cs.VirtualMachine.Chipset.LinuxKernelDirect
	if lkd.KernelFilePath != "kernel" {
		t.Fatalf("LinuxKernelDirect KernelFilePath = %q, want %q", lkd.KernelFilePath, "kernel")
	}
	if lkd.InitRdPath != "initrd" {
		t.Fatalf("LinuxKernelDirect InitRdPath = %q, want %q", lkd.InitRdPath, "initrd")
	}
	if lkd.KernelCmdLine != "cmd" {
		t.Fatalf("LinuxKernelDirect KernelCmdLine = %q, want %q", lkd.KernelCmdLine, "cmd")
	}
}
