package uvm

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Microsoft/hcsshim/internal/cpu"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/sirupsen/logrus"
)

func (uvm *UtilityVM) createLCOW(opts *UVMOptions) error {
	logrus.Debugf("uvm::createLCOW id=%s", uvm.id)

	if opts.KirdPath == "" {
		opts.KirdPath = filepath.Join(os.Getenv("ProgramFiles"), "Linux Containers")
	}
	if opts.KernelFile == "" {
		opts.KernelFile = "bootx64.efi"
	}
	if opts.InitrdFile == "" {
		opts.InitrdFile = "initrd.img"
	}
	if opts.KernelDebugComPortPipe == "" {
		opts.KernelDebugComPortPipe = `\\.\pipe\vmpipe`
	}
	if _, err := os.Stat(filepath.Join(opts.KirdPath, opts.KernelFile)); os.IsNotExist(err) {
		return fmt.Errorf("kernel '%s' not found", filepath.Join(opts.KirdPath, opts.KernelFile))
	}
	if _, err := os.Stat(filepath.Join(opts.KirdPath, opts.InitrdFile)); os.IsNotExist(err) {
		return fmt.Errorf("initrd '%s' not found", filepath.Join(opts.KirdPath, opts.InitrdFile))
	}

	memory := int32(1024)
	processors := int32(2)
	if cpu.NumCPU() == 1 {
		processors = 1
	}
	if opts.Resources != nil {
		if opts.Resources.Memory != nil && opts.Resources.Memory.Limit != nil {
			memory = int32(*opts.Resources.Memory.Limit / 1024 / 1024) // OCI spec is in bytes. HCS takes MB
		}
		if opts.Resources.CPU != nil && opts.Resources.CPU.Count != nil {
			processors = int32(*opts.Resources.CPU.Count)
		}
	}

	scsi := make(map[string]schema2.VirtualMachinesResourcesStorageScsiV2)
	scsi["0"] = schema2.VirtualMachinesResourcesStorageScsiV2{Attachments: make(map[string]schema2.VirtualMachinesResourcesStorageAttachmentV2)}
	hcsDocument := &schema2.ComputeSystemV2{
		Owner:         uvm.owner,
		SchemaVersion: schemaversion.SchemaV20(),
		VirtualMachine: &schema2.VirtualMachineV2{
			Chipset: &schema2.VirtualMachinesResourcesChipsetV2{
				UEFI: &schema2.VirtualMachinesResourcesUefiV2{
					BootThis: &schema2.VirtualMachinesResourcesUefiBootEntryV2{
						DevicePath:   `\` + opts.KernelFile,
						DiskNumber:   0,
						UefiDevice:   "VMBFS",
						OptionalData: `initrd=\` + opts.InitrdFile,
					},
				},
			},
			ComputeTopology: &schema2.VirtualMachinesResourcesComputeTopologyV2{
				Memory: &schema2.VirtualMachinesResourcesComputeMemoryV2{
					Backing: "Virtual",
					Startup: memory,
				},
				Processor: &schema2.VirtualMachinesResourcesComputeProcessorV2{
					Count: processors,
				},
			},

			Devices: &schema2.VirtualMachinesDevicesV2{
				// Add networking here.... TODO
				VPMem: &schema2.VirtualMachinesResourcesStorageVpmemControllerV2{
					MaximumCount: 128, // TODO: Consider making this flexible. Effectively the number of unique read-only layers available in the UVM. LCOW max is 128 in the platform.
				},
				SCSI: scsi,
				VirtualSMBShares: []schema2.VirtualMachinesResourcesStorageVSmbShareV2{schema2.VirtualMachinesResourcesStorageVSmbShareV2{
					Flags: schema2.VsmbFlagReadOnly | schema2.VsmbFlagShareRead | schema2.VsmbFlagCacheIO | schema2.VsmbFlagTakeBackupPrivilege, // 0x17 (23 dec)
					Name:  "os",
					Path:  opts.KirdPath,
				}},
				GuestInterface: &schema2.VirtualMachinesResourcesGuestInterfaceV2{
					ConnectToBridge: true,
					BridgeFlags:     3, // TODO What are these??
				},
			},
		},
	}

	if opts.KernelDebugMode {
		hcsDocument.VirtualMachine.Chipset.UEFI.BootThis.OptionalData += " console=ttyS0,115200"
		hcsDocument.VirtualMachine.Devices.COMPorts = &schema2.VirtualMachinesResourcesComPortsV2{Port1: opts.KernelDebugComPortPipe}
		hcsDocument.VirtualMachine.Devices.Keyboard = &schema2.VirtualMachinesResourcesKeyboardV2{}
		hcsDocument.VirtualMachine.Devices.Mouse = &schema2.VirtualMachinesResourcesMouseV2{}
		hcsDocument.VirtualMachine.Devices.Rdp = &schema2.VirtualMachinesResourcesRdpV2{}
		hcsDocument.VirtualMachine.Devices.VideoMonitor = &schema2.VirtualMachinesResourcesVideoMonitorV2{}
	}

	if opts.KernelBootOptions != "" {
		hcsDocument.VirtualMachine.Chipset.UEFI.BootThis.OptionalData = hcsDocument.VirtualMachine.Chipset.UEFI.BootThis.OptionalData + fmt.Sprintf(" %s", opts.KernelBootOptions)
	}

	hcsSystem, err := hcs.CreateComputeSystem(uvm.id, hcsDocument, opts.AdditionHCSDocumentJSON)
	if err != nil {
		logrus.Debugln("failed to create UVM: ", err)
		return err
	}

	uvm.hcsSystem = hcsSystem
	return nil
}
