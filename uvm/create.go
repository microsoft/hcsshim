package uvm

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Microsoft/hcsshim/internal/copyfile"
	"github.com/Microsoft/hcsshim/internal/cpu"
	"github.com/Microsoft/hcsshim/internal/guid"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/mergemaps"
	"github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/Microsoft/hcsshim/internal/uvmfolder"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

// UVMOptions are the set of options passed to Create() to create a utility vm.
type UVMOptions struct {
	Id                      string                  // Identifier for the uvm. Defaults to generated GUID.
	Owner                   string                  // Specifies the owner. Defaults to executable name.
	OperatingSystem         string                  // "windows" or "linux".
	Resources               *specs.WindowsResources // Optional resources for the utility VM. Supports Memory.limit and CPU.Count only currently. // TODO consider extending?
	AdditionHCSDocumentJSON string                  // Optional additional JSON to merge into the HCS document prior

	// WCOW specific parameters
	LayerFolders []string // Set of folders for base layers and sandbox. Ordered from top most read-only through base read-only layer, followed by sandbox

	// LCOW specific parameters
	KirdPath               string // Folder in which kernel and initrd reside. Defaults to \Program Files\Linux Containers
	KernelFile             string // Filename under KirdPath for the kernel. Defaults to bootx64.efi
	InitrdFile             string // Filename under KirdPath for the initrd image. Defaults to initrd.img
	KernelBootOptions      string // Additional boot options for the kernel
	KernelDebugMode        bool   // Configures the kernel in debug mode using sane defaults
	KernelDebugComPortPipe string // If kernel is in debug mode, can override the pipe here. Defaults to `\\.\pipe\vmpipe`
}

// Create creates an HCS compute system representing a utility VM.
//
// WCOW Notes:
//   - If the sandbox folder does not exist, it will be created
//   - If the sandbox folder does not contain `sandbox.vhdx` it will be created based on the system template located in the layer folders.
//   - The sandbox is always attached to SCSI 0:0
//
func Create(opts *UVMOptions) (*UtilityVM, error) {
	logrus.Debugf("uvm::Create %+v", opts)

	uvm := &UtilityVM{
		id:              opts.Id,
		owner:           opts.Owner,
		operatingSystem: opts.OperatingSystem,
	}

	uvmFolder := "" // Windows

	if opts.OperatingSystem != "linux" && opts.OperatingSystem != "windows" {
		logrus.Debugf("uvm::Create Unsupported OS")
		return nil, fmt.Errorf("unsupported operating system %q", opts.OperatingSystem)
	}

	// Defaults if omitted by caller.
	if uvm.id == "" {
		uvm.id = guid.New().String()
	}
	if uvm.owner == "" {
		uvm.owner = filepath.Base(os.Args[0])
	}

	attachments := make(map[string]schema2.VirtualMachinesResourcesStorageAttachmentV2)
	scsi := make(map[string]schema2.VirtualMachinesResourcesStorageScsiV2)

	if uvm.operatingSystem == "windows" {
		if len(opts.LayerFolders) < 2 {
			return nil, fmt.Errorf("at least 2 LayerFolders must be supplied")
		}

		var err error
		uvmFolder, err = uvmfolder.LocateUVMFolder(opts.LayerFolders)
		if err != nil {
			return nil, fmt.Errorf("failed to locate utility VM folder from layer folders: %s", err)
		}

		// Create the sandbox in the top-most layer folder, creating the folder if it doesn't already exist.
		sandboxFolder := opts.LayerFolders[len(opts.LayerFolders)-1]
		logrus.Debugf("uvm::createWCOW Sandbox folder: %s", sandboxFolder)

		// Create the directory if it doesn't exist
		if _, err := os.Stat(sandboxFolder); os.IsNotExist(err) {
			logrus.Debugf("uvm::createWCOW Creating folder: %s ", sandboxFolder)
			if err := os.MkdirAll(sandboxFolder, 0777); err != nil {
				return nil, fmt.Errorf("failed to create utility VM sandbox folder: %s", err)
			}
		}

		// Create sandbox.vhdx in the sandbox folder based on the template, granting the correct permissions to it
		if _, err := os.Stat(filepath.Join(sandboxFolder, `sandbox.vhdx`)); os.IsNotExist(err) {
			if err := CreateWCOWSandbox(uvmFolder, sandboxFolder, uvm.id); err != nil {
				return nil, fmt.Errorf("failed to create sandbox: %s", err)
			}
		}

		// We attach the sandbox to SCSI 0:0
		attachments["0"] = schema2.VirtualMachinesResourcesStorageAttachmentV2{
			Path: filepath.Join(sandboxFolder, "sandbox.vhdx"),
			Type: "VirtualDisk",
		}
	} else {
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
			return nil, fmt.Errorf("kernel '%s' not found", filepath.Join(opts.KirdPath, opts.KernelFile))
		}
		if _, err := os.Stat(filepath.Join(opts.KirdPath, opts.InitrdFile)); os.IsNotExist(err) {
			return nil, fmt.Errorf("initrd '%s' not found", filepath.Join(opts.KirdPath, opts.InitrdFile))
		}
	}

	scsi["0"] = schema2.VirtualMachinesResourcesStorageScsiV2{Attachments: attachments}

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

	hcsDocument := &schema2.ComputeSystemV2{
		Owner:         uvm.owner,
		SchemaVersion: schemaversion.SchemaV20(),
		VirtualMachine: &schema2.VirtualMachineV2{
			Chipset: &schema2.VirtualMachinesResourcesChipsetV2{
				UEFI: &schema2.VirtualMachinesResourcesUefiV2{},
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
				SCSI:             scsi,
				VirtualSMBShares: []schema2.VirtualMachinesResourcesStorageVSmbShareV2{schema2.VirtualMachinesResourcesStorageVSmbShareV2{Name: "os"}},
				GuestInterface:   &schema2.VirtualMachinesResourcesGuestInterfaceV2{ConnectToBridge: true},
			},
		},
	}

	if uvm.operatingSystem == "windows" {
		hcsDocument.VirtualMachine.Chipset.UEFI.BootThis = &schema2.VirtualMachinesResourcesUefiBootEntryV2{
			DevicePath: `\EFI\Microsoft\Boot\bootmgfw.efi`,
			DiskNumber: 0,
			UefiDevice: "VMBFS",
		}
		hcsDocument.VirtualMachine.ComputeTopology.Memory.DirectFileMappingMB = 1024 // Sensible default, but could be a tuning parameter somewhere
		hcsDocument.VirtualMachine.Devices.VirtualSMBShares[0].Path = filepath.Join(uvmFolder, `UtilityVM\Files`)
		hcsDocument.VirtualMachine.Devices.VirtualSMBShares[0].Flags = schema2.VsmbFlagReadOnly | schema2.VsmbFlagPseudoOplocks | schema2.VsmbFlagTakeBackupPrivilege | schema2.VsmbFlagCacheIO | schema2.VsmbFlagShareRead
	} else {
		hcsDocument.VirtualMachine.Devices.GuestInterface.BridgeFlags = 3 // TODO: Contants
		hcsDocument.VirtualMachine.Chipset.UEFI.BootThis = &schema2.VirtualMachinesResourcesUefiBootEntryV2{
			DevicePath:   `\` + opts.KernelFile,
			DiskNumber:   0,
			UefiDevice:   "VMBFS",
			OptionalData: `initrd=\` + opts.InitrdFile,
		}
		hcsDocument.VirtualMachine.Devices.VirtualSMBShares[0].Path = opts.KirdPath
		hcsDocument.VirtualMachine.Devices.VirtualSMBShares[0].Flags = schema2.VsmbFlagReadOnly | schema2.VsmbFlagShareRead | schema2.VsmbFlagCacheIO | schema2.VsmbFlagTakeBackupPrivilege // 0x17 (23 dec)
		hcsDocument.VirtualMachine.Devices.VPMem = &schema2.VirtualMachinesResourcesStorageVpmemControllerV2{MaximumCount: maxVPMEM}

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
	}

	fullDoc, err := mergemaps.MergeJSON(hcsDocument, ([]byte)(opts.AdditionHCSDocumentJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to merge additional JSON '%s': %s", opts.AdditionHCSDocumentJSON, err)
	}

	hcsSystem, err := hcs.CreateComputeSystem(uvm.id, fullDoc)
	if err != nil {
		logrus.Debugln("failed to create UVM: ", err)
		return nil, err
	}

	uvm.hcsSystem = hcsSystem
	if uvm.operatingSystem == "windows" {
		uvm.scsiLocations.hostPath[0][0] = attachments["0"].Path
	}
	return uvm, nil

}

// CreateWCOWSandbox is a helper to create a sandbox for a Windows utility VM
// with permissions to the specified VM ID in a specified directory
func CreateWCOWSandbox(imagePath, destDirectory, vmID string) error {
	sourceSandbox := filepath.Join(imagePath, `UtilityVM\SystemTemplate.vhdx`)
	targetSandbox := filepath.Join(destDirectory, "sandbox.vhdx")
	logrus.Debugf("uvm::CreateWCOWSandbox %s from %s", targetSandbox, sourceSandbox)
	if err := copyfile.CopyFile(sourceSandbox, targetSandbox, true); err != nil {
		return err
	}
	if err := wclayer.GrantVmAccess(vmID, targetSandbox); err != nil {
		// TODO: Delete the file?
		return err
	}
	return nil
}
