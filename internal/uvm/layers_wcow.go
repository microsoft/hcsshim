//go:build windows

package uvm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	layerutil "github.com/Microsoft/hcsshim/internal/layers/util"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/uvm/scsi"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	"github.com/Microsoft/hcsshim/internal/wcow"
	"github.com/containerd/containerd/api/types"
)

// A manager for handling the layers of a windows UVM
type wcowUVMLayerManager interface {
	// configure takes in the hcs schema document of a VM that is about to boot and sets it up properly by
	// mounting the layers. (if required)
	// The UtilityVM instance is modified to account for newly added SCSI disks/VSMB shares etc.
	Configure(context.Context, *UtilityVM, *hcsschema.ComputeSystem) error
	Close() error
}

type wcowUVMLegacyLayerManager struct {
	roLayers     []string
	scratchLayer string
}

// Close implements wcowUVMLayerManager
func (*wcowUVMLegacyLayerManager) Close() error {
	// legacy layer manager doesn't need any cleanup. SCSI disks & VSMB shares will be automatically
	// removed when the UVM is closed.
	return nil
}

// Configure implements WCOWUVMLayerManager
func (l *wcowUVMLegacyLayerManager) Configure(ctx context.Context, uvm *UtilityVM, doc *hcsschema.ComputeSystem) error {
	if uvm.id == "" || doc.VirtualMachine.Devices.Scsi == nil {
		// UVM struct must be initialized to have a valid ID before calling this method
		panic("UVM uninitialized")
	}

	uvmFolder, err := layerutil.LocateUVMFolder(ctx, l.roLayers)
	if err != nil {
		return fmt.Errorf("failed to locate utility VM folder from layer folders: %s", err)
	}

	// Create sandbox.vhdx in the scratch folder based on the template, granting the correct permissions to it
	scratchPath := filepath.Join(l.scratchLayer, "sandbox.vhdx")
	if _, err := os.Stat(scratchPath); os.IsNotExist(err) {
		if err := wcow.CreateUVMScratch(ctx, uvmFolder, l.scratchLayer, uvm.id); err != nil {
			return fmt.Errorf("failed to create scratch: %s", err)
		}
	} else {
		// Sandbox.vhdx exists, just need to grant vm access to it.
		if err := wclayer.GrantVmAccess(ctx, uvm.id, scratchPath); err != nil {
			return fmt.Errorf("failed to grant vm access to scratch: %w", err)
		}
	}

	doc.VirtualMachine.Devices.Scsi[guestrequest.ScsiControllerGuids[0]].Attachments["0"] = hcsschema.Attachment{

		Path:  scratchPath,
		Type_: "VirtualDisk",
	}

	// Ideally the layer manager should be decoupled from the SCSI management of the UVM and we should
	// expose some method (like recordSCSIMount) on SCSI manager that can be called here.
	uvm.reservedSCSISlots = append(uvm.reservedSCSISlots, scsi.Slot{Controller: 0, LUN: 0})

	// UVM rootfs share is readonly.
	if doc.VirtualMachine.Devices == nil {
		doc.VirtualMachine.Devices = &hcsschema.Devices{}
	}

	vsmbOpts := uvm.DefaultVSMBOptions(true)
	vsmbOpts.TakeBackupPrivilege = true
	doc.VirtualMachine.Devices.VirtualSmb = &hcsschema.VirtualSmb{
		DirectFileMappingInMB: 1024, // Sensible default, but could be a tuning parameter somewhere
		Shares: []hcsschema.VirtualSmbShare{
			{
				Name:    "os",
				Path:    filepath.Join(uvmFolder, `UtilityVM\Files`),
				Options: vsmbOpts,
			},
		},
	}
	return nil
}

// Only one of the `layerFolders` or `rootfs` MUST be provided. If `layerFolders` is
// provided a legacy layer manager will be returned. If `rootfs` is provided a layer manager
// based on the type of mount will be returned
func newWCOWUVMLegacyLayerManager(layerFolders []string, rootfs []*types.Mount) (wcowUVMLayerManager, error) {
	err := layerutil.ValidateRootfsAndLayers(rootfs, layerFolders)
	if err != nil {
		return nil, err
	}

	var roLayers []string
	var scratchLayer string
	if len(layerFolders) > 0 {
		scratchLayer, roLayers = layerFolders[len(layerFolders)-1], layerFolders[:len(layerFolders)-1]
	} else {
		scratchLayer, roLayers, err = layerutil.ParseLegacyRootfsMount(rootfs[0])
		if err != nil {
			return nil, err
		}
	}

	// In non-CRI cases the UVM's scratch VHD will be created in the same directory as that of the
	// container scratch, since both are named "sandbox.vhdx" we create a directory named "vm" and store
	// UVM scratch there. Here we have no way of deciding if this is CRI or non-CRI so we always create
	// the UVM VHD inside the "vm" subdirectory
	// TODO: BUGBUG Remove this. @jhowardmsft
	//       It should be the responsibility of the caller to do the creation and population.
	//       - Update runhcs too (vm.go).
	//       - Remove comment in function header
	//       - Update tests that rely on this current behavior.
	vmPath := filepath.Join(scratchLayer, "vm")
	err = os.MkdirAll(vmPath, 0)
	if err != nil {
		return nil, err
	}

	return &wcowUVMLegacyLayerManager{
		roLayers:     roLayers,
		scratchLayer: vmPath,
	}, nil
}
