package uvm

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Microsoft/hcsshim/cpu"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/sirupsen/logrus"
)

func (uvm *UtilityVM) createWCOW(opts *UVMOptions) error {
	logrus.Debugf("uvm::createWCOW Creating utility VM id=%s", uvm.id)

	if len(uvm.LayerFolders) < 2 {
		return fmt.Errorf("at least 2 LayerFolders must be supplied")
	}

	uvmFolder, err := LocateWCOWUVMFolderFromLayerFolders(uvm.LayerFolders)
	if err != nil {
		return fmt.Errorf("failed to locate utility VM folder from layer folders: %s", err)
	}

	// Create the sandbox in the top-most layer folder, creating the folder if it doesn't already exist.
	sandboxFolder := uvm.LayerFolders[len(uvm.LayerFolders)-1]
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
		if err := CreateWCOWUVMSandbox(uvmFolder, sandboxFolder, uvm.Id); err != nil {
			return fmt.Errorf("failed to create sandbox: %s", err)
		}
	}

	// We attach the sandbox to SCSI 0:0
	attachments := make(map[string]hcsschemav2.VirtualMachinesResourcesStorageAttachmentV2)
	attachments["0"] = hcsschemav2.VirtualMachinesResourcesStorageAttachmentV2{
		Path: filepath.Join(sandboxFolder, "sandbox.vhdx"),
		Type: "VirtualDisk",
	}
	scsi := make(map[string]hcsschemav2.VirtualMachinesResourcesStorageScsiV2)
	scsi["0"] = hcsschemav2.VirtualMachinesResourcesStorageScsiV2{Attachments: attachments}

	// Resources
	memory := int32(1024)
	processors := int32(2)
	if numCPU() == 1 {
		processors = 1
	}
	if uvm.Resources != nil {
		if uvm.Resources.Memory != nil && uvm.Resources.Memory.Limit != nil {
			memory = int32(*uvm.Resources.Memory.Limit / 1024 / 1024) // OCI spec is in bytes. HCS takes MB
		}
		if uvm.Resources.CPU != nil && uvm.Resources.CPU.Count != nil {
			processors = int32(*uvm.Resources.CPU.Count)
		}
	}

	hcsDocument := &hcsschemav2.ComputeSystemV2{
		Owner:         uvm.Owner,
		SchemaVersion: schemaversion.SchemaV20(),
		VirtualMachine: &hcsschemav2.VirtualMachineV2{
			Chipset: &hcsschemav2.VirtualMachinesResourcesChipsetV2{
				UEFI: &hcsschemav2.VirtualMachinesResourcesUefiV2{
					BootThis: &hcsschemav2.VirtualMachinesResourcesUefiBootEntryV2{
						DevicePath: `\EFI\Microsoft\Boot\bootmgfw.efi`,
						DiskNumber: 0,
						UefiDevice: "VMBFS",
					},
				},
			},
			ComputeTopology: &hcsschemav2.VirtualMachinesResourcesComputeTopologyV2{
				Memory: &hcsschemav2.VirtualMachinesResourcesComputeMemoryV2{
					Backing:             "Virtual",
					Startup:             memory,
					DirectFileMappingMB: 1024, // Sensible default, but could be a tuning parameter somewhere
				},
				Processor: &hcsschemav2.VirtualMachinesResourcesComputeProcessorV2{
					Count: processors,
				},
			},

			Devices: &hcsschemav2.VirtualMachinesDevicesV2{
				// Add networking here.... TODO
				SCSI: scsi,
				VirtualSMBShares: []hcsschemav2.VirtualMachinesResourcesStorageVSmbShareV2{hcsschemav2.VirtualMachinesResourcesStorageVSmbShareV2{
					Flags: hcsschemav2.VsmbFlagReadOnly | hcsschemav2.VsmbFlagPseudoOplocks | hcsschemav2.VsmbFlagTakeBackupPrivilege | hcsschemav2.VsmbFlagCacheIO | hcsschemav2.VsmbFlagShareRead,
					Name:  "os",
					Path:  filepath.Join(uvmFolder, `UtilityVM\Files`),
				}},
				GuestInterface: &hcsschemav2.VirtualMachinesResourcesGuestInterfaceV2{ConnectToBridge: true},
			},
		},
	}

	hcsDocumentB, err := json.Marshal(hcsDocument)
	if err != nil {
		return err
	}
	if err := uvm.createHCSComputeSystem(string(hcsDocumentB)); err != nil {
		logrus.Debugln("failed to create UVM: ", err)
		return err
	}

	uvm.scsiLocations.hostPath[0][0] = attachments["0"].Path
	return nil

}
