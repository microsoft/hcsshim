package uvm

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Microsoft/hcsshim/internal/copyfile"
	"github.com/Microsoft/hcsshim/internal/cpu"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/Microsoft/hcsshim/internal/uvmfolder"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	"github.com/sirupsen/logrus"
)

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

func (uvm *UtilityVM) createWCOW(opts *UVMOptions) error {
	logrus.Debugf("uvm::createWCOW Creating utility VM id=%s", uvm.id)

	if len(opts.LayerFolders) < 2 {
		return fmt.Errorf("at least 2 LayerFolders must be supplied")
	}

	uvmFolder, err := uvmfolder.LocateUVMFolder(opts.LayerFolders)
	if err != nil {
		return fmt.Errorf("failed to locate utility VM folder from layer folders: %s", err)
	}

	// Create the sandbox in the top-most layer folder, creating the folder if it doesn't already exist.
	sandboxFolder := opts.LayerFolders[len(opts.LayerFolders)-1]
	logrus.Debugf("uvm::createWCOW Sandbox folder: %s", sandboxFolder)

	// Create the directory if it doesn't exist
	if _, err := os.Stat(sandboxFolder); os.IsNotExist(err) {
		logrus.Debugf("uvm::createWCOW Creating folder: %s ", sandboxFolder)
		if err := os.MkdirAll(sandboxFolder, 0777); err != nil {
			return fmt.Errorf("failed to create utility VM sandbox folder: %s", err)
		}
	}

	// Create sandbox.vhdx in the sandbox folder based on the template, granting the correct permissions to it
	if _, err := os.Stat(filepath.Join(sandboxFolder, `sandbox.vhdx`)); os.IsNotExist(err) {
		if err := CreateWCOWSandbox(uvmFolder, sandboxFolder, uvm.id); err != nil {
			return fmt.Errorf("failed to create sandbox: %s", err)
		}
	}

	// We attach the sandbox to SCSI 0:0
	attachments := make(map[string]schema2.VirtualMachinesResourcesStorageAttachmentV2)
	attachments["0"] = schema2.VirtualMachinesResourcesStorageAttachmentV2{
		Path: filepath.Join(sandboxFolder, "sandbox.vhdx"),
		Type: "VirtualDisk",
	}
	scsi := make(map[string]schema2.VirtualMachinesResourcesStorageScsiV2)
	scsi["0"] = schema2.VirtualMachinesResourcesStorageScsiV2{Attachments: attachments}

	// Resources
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
				UEFI: &schema2.VirtualMachinesResourcesUefiV2{
					BootThis: &schema2.VirtualMachinesResourcesUefiBootEntryV2{
						DevicePath: `\EFI\Microsoft\Boot\bootmgfw.efi`,
						DiskNumber: 0,
						UefiDevice: "VMBFS",
					},
				},
			},
			ComputeTopology: &schema2.VirtualMachinesResourcesComputeTopologyV2{
				Memory: &schema2.VirtualMachinesResourcesComputeMemoryV2{
					Backing:             "Virtual",
					Startup:             memory,
					DirectFileMappingMB: 1024, // Sensible default, but could be a tuning parameter somewhere
				},
				Processor: &schema2.VirtualMachinesResourcesComputeProcessorV2{
					Count: processors,
				},
			},

			Devices: &schema2.VirtualMachinesDevicesV2{
				// Add networking here.... TODO
				SCSI: scsi,
				VirtualSMBShares: []schema2.VirtualMachinesResourcesStorageVSmbShareV2{schema2.VirtualMachinesResourcesStorageVSmbShareV2{
					Flags: schema2.VsmbFlagReadOnly | schema2.VsmbFlagPseudoOplocks | schema2.VsmbFlagTakeBackupPrivilege | schema2.VsmbFlagCacheIO | schema2.VsmbFlagShareRead,
					Name:  "os",
					Path:  filepath.Join(uvmFolder, `UtilityVM\Files`),
				}},
				GuestInterface: &schema2.VirtualMachinesResourcesGuestInterfaceV2{ConnectToBridge: true},
			},
		},
	}

	hcsSystem, err := hcs.CreateComputeSystem(uvm.id, hcsDocument, opts.AdditionHCSDocumentJSON)
	if err != nil {
		logrus.Debugln("failed to create UVM: ", err)
		return err
	}

	uvm.hcsSystem = hcsSystem
	uvm.scsiLocations.hostPath[0][0] = attachments["0"].Path
	return nil

}
