// +build windows

package hcsoci

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/ospath"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/uvm"
	uvmpkg "github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	"github.com/pkg/errors"
)

// ImageLayers contains all the layers for an image.
type ImageLayers struct {
	vm                 *uvm.UtilityVM
	containerRootInUVM string
	layers             []string
}

// Release unmounts all of the layers located in the layers array.
func (layers *ImageLayers) Release(ctx context.Context, all bool) error {
	op := UnmountOperationSCSI
	if layers.vm == nil || all {
		op = UnmountOperationAll
	}
	var crp string
	if layers.vm != nil {
		crp = containerRootfsPath(layers.vm, layers.containerRootInUVM)
	}
	err := UnmountContainerLayers(ctx, layers.layers, crp, layers.vm, op)
	if err != nil {
		return err
	}
	layers.layers = nil
	return nil
}

// MountContainerLayers is a helper for clients to hide all the complexity of layer mounting
// Layer folder are in order: base, [rolayer1..rolayern,] scratch
//
// v1/v2: Argon WCOW: Returns the mount path on the host as a volume GUID.
// v1:    Xenon WCOW: Done internally in HCS, so no point calling doing anything here.
// v2:    Xenon WCOW: Returns a CombinedLayersV2 structure where ContainerRootPath is a folder
//                    inside the utility VM which is a GUID mapping of the scratch folder. Each
//                    of the layers are the VSMB locations where the read-only layers are mounted.
//
// TODO dcantah: Keep better track of the layers that are added, don't simply discard the SCSI, VSMB, etc. resource types gotten inside.
func MountContainerLayers(ctx context.Context, layerFolders []string, guestRoot string, uvm *uvmpkg.UtilityVM) (_ string, err error) {
	log.G(ctx).WithField("layerFolders", layerFolders).Debug("hcsshim::mountContainerLayers")

	if uvm == nil {
		if len(layerFolders) < 2 {
			return "", fmt.Errorf("need at least two layers - base and scratch")
		}
		path := layerFolders[len(layerFolders)-1]
		rest := layerFolders[:len(layerFolders)-1]
		if err := wclayer.ActivateLayer(ctx, path); err != nil {
			return "", err
		}
		defer func() {
			if err != nil {
				wclayer.DeactivateLayer(ctx, path)
			}
		}()

		if err := wclayer.PrepareLayer(ctx, path, rest); err != nil {
			return "", err
		}
		defer func() {
			if err != nil {
				wclayer.UnprepareLayer(ctx, path)
			}
		}()

		mountPath, err := wclayer.GetLayerMountPath(ctx, path)
		if err != nil {
			return "", err
		}
		return mountPath, nil
	}

	// V2 UVM
	log.G(ctx).WithField("os", uvm.OS()).Debug("hcsshim::mountContainerLayers V2 UVM")

	var (
		layersAdded       []string
		lcowUvmLayerPaths []string
	)
	defer func() {
		if err != nil {
			if uvm.OS() == "windows" {
				for _, l := range layersAdded {
					if err := uvm.RemoveVSMB(ctx, l); err != nil {
						log.G(ctx).WithError(err).Warn("failed to remove wcow layer on cleanup")
					}
				}
			} else {
				for _, l := range layersAdded {
					// Assume it was added to vPMEM and fall back to SCSI
					e := uvm.RemoveVPMEM(ctx, l)
					if e == uvmpkg.ErrNotAttached {
						e = uvm.RemoveSCSI(ctx, l)
					}
					if e != nil {
						log.G(ctx).WithError(e).Warn("failed to remove lcow layer on cleanup")
					}
				}
			}
		}
	}()

	for _, layerPath := range layerFolders[:len(layerFolders)-1] {
		log.G(ctx).WithField("layerPath", layerPath).Debug("mounting layer")
		if uvm.OS() == "windows" {
			options := uvm.DefaultVSMBOptions(true)
			options.TakeBackupPrivilege = true
			if _, err := uvm.AddVSMB(ctx, layerPath, options); err != nil {
				return "", fmt.Errorf("failed to add VSMB layer: %s", err)
			}
			layersAdded = append(layersAdded, layerPath)
		} else {
			var (
				layerPath = filepath.Join(layerPath, "layer.vhd")
				uvmPath   string
			)
			uvmPath, err = addLCOWLayer(ctx, uvm, layerPath)
			if err != nil {
				return "", fmt.Errorf("failed to add LCOW layer: %s", err)
			}
			layersAdded = append(layersAdded, layerPath)
			lcowUvmLayerPaths = append(lcowUvmLayerPaths, uvmPath)
		}
	}

	hostPath := filepath.Join(layerFolders[len(layerFolders)-1], "sandbox.vhdx")
	containerScratchPathInUVM := ospath.Join(uvm.OS(), guestRoot)
	log.G(ctx).WithField("hostPath", hostPath).Debug("mounting scratch VHD")
	scsiMount, err := uvm.AddSCSI(ctx, hostPath, containerScratchPathInUVM, false, uvmpkg.VMAccessTypeIndividual)
	if err != nil {
		return "", fmt.Errorf("failed to add SCSI scratch VHD: %s", err)
	}
	containerScratchPathInUVM = scsiMount.UVMPath

	defer func() {
		if err != nil {
			if err := uvm.RemoveSCSI(ctx, hostPath); err != nil {
				log.G(ctx).WithError(err).Warn("failed to remove scratch on cleanup")
			}
		}
	}()

	var rootfs string
	if uvm.OS() == "windows" {
		// 	Load the filter at the C:\s<ID> location calculated above. We pass into this request each of the
		// 	read-only layer folders.
		layers, err := computeV2Layers(ctx, uvm, layersAdded)
		if err != nil {
			return "", err
		}
		err = uvm.CombineLayersWCOW(ctx, layers, containerScratchPathInUVM)
		rootfs = containerScratchPathInUVM
	} else {
		rootfs = ospath.Join(uvm.OS(), guestRoot, rootfsPath)
		err = uvm.CombineLayersLCOW(ctx, lcowUvmLayerPaths, containerScratchPathInUVM, rootfs)
	}
	if err != nil {
		return "", err
	}
	log.G(ctx).Debug("hcsshim::mountContainerLayers Succeeded")
	return rootfs, nil
}

func addLCOWLayer(ctx context.Context, uvm *uvmpkg.UtilityVM, layerPath string) (uvmPath string, err error) {
	// don't try to add as vpmem when we want additional devices on the uvm to be fully physically backed
	if !uvm.DevicesPhysicallyBacked() {
		// We first try vPMEM and if it is full or the file is too large we
		// fall back to SCSI.
		uvmPath, err = uvm.AddVPMEM(ctx, layerPath)
		if err == nil {
			return uvmPath, err
		} else if err != uvmpkg.ErrNoAvailableLocation && err != uvmpkg.ErrMaxVPMEMLayerSize {
			return "", err
		}
	}

	uvmPath = fmt.Sprintf(lcowGlobalMountPrefix, uvm.UVMMountCounter())
	sm, err := uvm.AddSCSI(ctx, layerPath, uvmPath, true, uvmpkg.VMAccessTypeNoop)
	if err != nil {
		return "", fmt.Errorf("failed to add SCSI layer: %s", err)
	}

	return sm.UVMPath, nil
}

// UnmountOperation is used when calling Unmount() to determine what type of unmount is
// required. In V1 schema, this must be unmountOperationAll. In V2, client can
// be more optimal and only unmount what they need which can be a minor performance
// improvement (eg if you know only one container is running in a utility VM, and
// the UVM is about to be torn down, there's no need to unmount the VSMB shares,
// just SCSI to have a consistent file system).
type UnmountOperation uint

const (
	UnmountOperationSCSI  UnmountOperation = 0x01
	UnmountOperationVSMB                   = 0x02
	UnmountOperationVPMEM                  = 0x04
	UnmountOperationAll                    = UnmountOperationSCSI | UnmountOperationVSMB | UnmountOperationVPMEM
)

// UnmountContainerLayers is a helper for clients to hide all the complexity of layer unmounting
func UnmountContainerLayers(ctx context.Context, layerFolders []string, containerRootPath string, uvm *uvmpkg.UtilityVM, op UnmountOperation) error {
	log.G(ctx).WithField("layerFolders", layerFolders).Debug("hcsshim::unmountContainerLayers")
	if uvm == nil {
		// Must be an argon - folders are mounted on the host
		if op != UnmountOperationAll {
			return fmt.Errorf("only operation supported for host-mounted folders is unmountOperationAll")
		}
		if len(layerFolders) < 1 {
			return fmt.Errorf("need at least one layer for Unmount")
		}
		path := layerFolders[len(layerFolders)-1]
		if err := wclayer.UnprepareLayer(ctx, path); err != nil {
			return err
		}
		return wclayer.DeactivateLayer(ctx, path)
	}

	// V2 Xenon

	// Base+Scratch as a minimum. This is different to v1 which only requires the scratch
	if len(layerFolders) < 2 {
		return fmt.Errorf("at least two layers are required for unmount")
	}

	var retError error

	// Always remove the combined layers as they are part of scsi/vsmb/vpmem
	// removals.
	if err := uvm.RemoveCombinedLayers(ctx, containerRootPath); err != nil {
		log.G(ctx).WithError(err).Warn("failed guest request to remove combined layers")
		retError = err
	}

	// Unload the SCSI scratch path
	if (op & UnmountOperationSCSI) == UnmountOperationSCSI {
		hostScratchFile := filepath.Join(layerFolders[len(layerFolders)-1], "sandbox.vhdx")
		if err := uvm.RemoveSCSI(ctx, hostScratchFile); err != nil {
			log.G(ctx).WithError(err).Warn("failed to remove scratch")
			if retError == nil {
				retError = err
			} else {
				retError = errors.Wrapf(retError, err.Error())
			}
		}
	}

	// Remove each of the read-only layers from VSMB. These's are ref-counted and
	// only removed once the count drops to zero. This allows multiple containers
	// to share layers.
	if uvm.OS() == "windows" && (op&UnmountOperationVSMB) == UnmountOperationVSMB {
		for _, layerPath := range layerFolders[:len(layerFolders)-1] {
			if e := uvm.RemoveVSMB(ctx, layerPath); e != nil {
				log.G(ctx).WithError(e).Warn("remove VSMB failed")
				if retError == nil {
					retError = e
				} else {
					retError = errors.Wrapf(retError, e.Error())
				}
			}
		}
	}

	// Remove each of the read-only layers from VPMEM (or SCSI). These's are ref-counted
	// and only removed once the count drops to zero. This allows multiple containers to
	// share layers. Note that SCSI is used on large layers.
	if uvm.OS() == "linux" && (op&UnmountOperationVPMEM) == UnmountOperationVPMEM {
		for _, layerPath := range layerFolders[:len(layerFolders)-1] {
			hostPath := filepath.Join(layerPath, "layer.vhd")

			// Assume it was added to vPMEM and fall back to SCSI
			e := uvm.RemoveVPMEM(ctx, hostPath)
			if e == uvmpkg.ErrNotAttached {
				e = uvm.RemoveSCSI(ctx, hostPath)
			}
			if e != nil {
				log.G(ctx).WithError(e).Warn("remove layer failed")
				if retError == nil {
					retError = e
				} else {
					retError = errors.Wrapf(retError, e.Error())
				}
			}
		}
	}

	return retError
}

func computeV2Layers(ctx context.Context, vm *uvm.UtilityVM, paths []string) (layers []hcsschema.Layer, err error) {
	for _, path := range paths {
		uvmPath, err := vm.GetVSMBUvmPath(ctx, path)
		if err != nil {
			return nil, err
		}
		layerID, err := wclayer.LayerID(ctx, path)
		if err != nil {
			return nil, err
		}
		layers = append(layers, hcsschema.Layer{Id: layerID.String(), Path: uvmPath})
	}
	return layers, nil
}
