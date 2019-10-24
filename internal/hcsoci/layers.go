// +build windows

package hcsoci

import (
	"context"
	"fmt"
	"path"
	"path/filepath"

	"github.com/Microsoft/hcsshim/internal/guestrequest"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/ospath"
	"github.com/Microsoft/hcsshim/internal/requesttype"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/uvm"
	uvmpkg "github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const scratchPath = "scratch"

// mountContainerLayers is a helper for clients to hide all the complexity of layer mounting
// Layer folder are in order: base, [rolayer1..rolayern,] scratch
//
// v1/v2: Argon WCOW: Returns the mount path on the host as a volume GUID.
// v1:    Xenon WCOW: Done internally in HCS, so no point calling doing anything here.
// v2:    Xenon WCOW: Returns a CombinedLayersV2 structure where ContainerRootPath is a folder
//                    inside the utility VM which is a GUID mapping of the scratch folder. Each
//                    of the layers are the VSMB locations where the read-only layers are mounted.
//
func MountContainerLayers(ctx context.Context, layerFolders []string, guestRoot string, uvm *uvmpkg.UtilityVM) (_ interface{}, err error) {
	log.G(ctx).WithField("layerFolders", layerFolders).Debug("hcsshim::mountContainerLayers")

	if uvm == nil {
		if len(layerFolders) < 2 {
			return nil, fmt.Errorf("need at least two layers - base and scratch")
		}
		path := layerFolders[len(layerFolders)-1]
		rest := layerFolders[:len(layerFolders)-1]
		log.G(ctx).WithField("path", path).Debug("hcsshim::mountContainerLayers ActivateLayer")
		if err := wclayer.ActivateLayer(path); err != nil {
			return nil, err
		}
		defer func() {
			if err != nil {
				if err := wclayer.DeactivateLayer(path); err != nil {
					log.G(ctx).WithFields(logrus.Fields{
						logrus.ErrorKey: err,
						"path":          path,
					}).Warn("failed to DeactivateLayer on cleanup")
				}
			}
		}()

		log.G(ctx).WithFields(logrus.Fields{
			"path": path,
			"rest": rest,
		}).Debug("hcsshim::mountContainerLayers PrepareLayer")
		if err := wclayer.PrepareLayer(path, rest); err != nil {
			return nil, err
		}
		defer func() {
			if err != nil {
				if err := wclayer.UnprepareLayer(path); err != nil {
					log.G(ctx).WithFields(logrus.Fields{
						logrus.ErrorKey: err,
						"path":          path,
					}).Warn("failed to UnprepareLayer on cleanup")
				}
			}
		}()

		mountPath, err := wclayer.GetLayerMountPath(path)
		if err != nil {
			return nil, err
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
		if uvm.OS() == "windows" {
			options := &hcsschema.VirtualSmbShareOptions{
				ReadOnly:            true,
				PseudoOplocks:       true,
				TakeBackupPrivilege: true,
				CacheIo:             true,
				ShareRead:           true,
			}
			err = uvm.AddVSMB(ctx, layerPath, "", options)
			if err == nil {
				layersAdded = append(layersAdded, layerPath)
			}
		} else {
			var (
				layerPath = filepath.Join(layerPath, "layer.vhd")
				uvmPath   string
			)

			// We first try vPMEM and if it is full or the file is too large we
			// fall back to SCSI.
			uvmPath, err = uvm.AddVPMEM(ctx, layerPath)
			if err == uvmpkg.ErrNoAvailableLocation || err == uvmpkg.ErrMaxVPMEMLayerSize {
				uvmPath, err = uvm.AddSCSILayer(ctx, layerPath)
			}
			if err == nil {
				layersAdded = append(layersAdded, layerPath)
				lcowUvmLayerPaths = append(lcowUvmLayerPaths, uvmPath)
			}
		}
		if err != nil {
			return nil, err
		}
	}

	// Add the scratch at an unused SCSI location. The container path inside the
	// utility VM will be C:\<ID>.
	hostPath := filepath.Join(layerFolders[len(layerFolders)-1], "sandbox.vhdx")

	// BUGBUG Rename guestRoot better.
	containerScratchPathInUVM := ospath.Join(uvm.OS(), guestRoot, scratchPath)
	_, _, err = uvm.AddSCSI(ctx, hostPath, containerScratchPathInUVM, false)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			if err := uvm.RemoveSCSI(ctx, hostPath); err != nil {
				log.G(ctx).WithError(err).Warn("failed to remove scratch on cleanup")
			}
		}
	}()

	if uvm.OS() == "windows" {
		// 	Load the filter at the C:\s<ID> location calculated above. We pass into this request each of the
		// 	read-only layer folders.
		layers, err := computeV2Layers(ctx, uvm, layersAdded)
		if err != nil {
			return nil, err
		}
		guestRequest := guestrequest.CombinedLayers{
			ContainerRootPath: containerScratchPathInUVM,
			Layers:            layers,
		}
		combinedLayersModification := &hcsschema.ModifySettingRequest{
			GuestRequest: guestrequest.GuestRequest{
				Settings:     guestRequest,
				ResourceType: guestrequest.ResourceTypeCombinedLayers,
				RequestType:  requesttype.Add,
			},
		}
		if err := uvm.Modify(ctx, combinedLayersModification); err != nil {
			return nil, err
		}
		log.G(ctx).Debug("hcsshim::mountContainerLayers Succeeded")
		return guestRequest, nil
	}

	// This is the LCOW layout inside the utilityVM. NNN is the container "number"
	// which increments for each container created in a utility VM.
	//
	// /run/gcs/c/NNN/config.json
	// /run/gcs/c/NNN/rootfs
	// /run/gcs/c/NNN/scratch/upper
	// /run/gcs/c/NNN/scratch/work
	//
	// /dev/sda on /tmp/scratch type ext4 (rw,relatime,block_validity,delalloc,barrier,user_xattr,acl)
	// /dev/pmem0 on /tmp/v0 type ext4 (ro,relatime,block_validity,delalloc,norecovery,barrier,dax,user_xattr,acl)
	// /dev/sdb on /run/gcs/c/NNN/scratch type ext4 (rw,relatime,block_validity,delalloc,barrier,user_xattr,acl)
	// overlay on /run/gcs/c/NNN/rootfs type overlay (rw,relatime,lowerdir=/tmp/v0,upperdir=/run/gcs/c/NNN/scratch/upper,workdir=/run/gcs/c/NNN/scratch/work)
	//
	// Where /dev/sda      is the scratch for utility VM itself
	//       /dev/pmemX    are read-only layers for containers
	//       /dev/sd(b...) are scratch spaces for each container

	layers := []hcsschema.Layer{}
	for _, l := range lcowUvmLayerPaths {
		layers = append(layers, hcsschema.Layer{Path: l})
	}
	guestRequest := guestrequest.CombinedLayers{
		ContainerRootPath: path.Join(guestRoot, rootfsPath),
		Layers:            layers,
		ScratchPath:       containerScratchPathInUVM,
	}
	combinedLayersModification := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.GuestRequest{
			ResourceType: guestrequest.ResourceTypeCombinedLayers,
			RequestType:  requesttype.Add,
			Settings:     guestRequest,
		},
	}
	if err := uvm.Modify(ctx, combinedLayersModification); err != nil {
		return nil, err
	}
	log.G(ctx).Debug("hcsshim::mountContainerLayers Succeeded")
	return guestRequest, nil

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
func UnmountContainerLayers(ctx context.Context, layerFolders []string, guestRoot string, uvm *uvmpkg.UtilityVM, op UnmountOperation) error {
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
		log.G(ctx).WithField("path", path).Debug("hcsshim::Unmount UnprepareLayer")
		if err := wclayer.UnprepareLayer(path); err != nil {
			return err
		}
		// TODO Should we try this anyway?
		log.G(ctx).WithField("path", path).Debug("hcsshim::unmountContainerLayers DeactivateLayer")
		return wclayer.DeactivateLayer(path)
	}

	// V2 Xenon

	// Base+Scratch as a minimum. This is different to v1 which only requires the scratch
	if len(layerFolders) < 2 {
		return fmt.Errorf("at least two layers are required for unmount")
	}

	var retError error

	// Unload the storage filter followed by the SCSI scratch
	if (op & UnmountOperationSCSI) == UnmountOperationSCSI {
		containerRoofFSPathInUVM := ospath.Join(uvm.OS(), guestRoot, rootfsPath)
		log.G(ctx).WithField("rootPath", containerRoofFSPathInUVM).Debug("hcsshim::unmountContainerLayers CombinedLayers")
		combinedLayersModification := &hcsschema.ModifySettingRequest{
			GuestRequest: guestrequest.GuestRequest{
				ResourceType: guestrequest.ResourceTypeCombinedLayers,
				RequestType:  requesttype.Remove,
				Settings:     guestrequest.CombinedLayers{ContainerRootPath: containerRoofFSPathInUVM},
			},
		}
		if err := uvm.Modify(ctx, combinedLayersModification); err != nil {
			log.G(ctx).WithError(err).Error("failed guest request to remove combined layers")
		}

		// Hot remove the scratch from the SCSI controller
		hostScratchFile := filepath.Join(layerFolders[len(layerFolders)-1], "sandbox.vhdx")
		containerScratchPathInUVM := ospath.Join(uvm.OS(), guestRoot, scratchPath)
		log.G(ctx).WithFields(logrus.Fields{
			"scratchPath": containerScratchPathInUVM,
			"scratchFile": hostScratchFile,
		}).Debug("hcsshim::unmountContainerLayers SCSI")
		if err := uvm.RemoveSCSI(ctx, hostScratchFile); err != nil {
			e := fmt.Errorf("failed to remove SCSI %s: %s", hostScratchFile, err)
			log.G(ctx).WithError(e).Error("failed to remove SCSI")
			if retError == nil {
				retError = e
			} else {
				retError = errors.Wrapf(retError, e.Error())
			}
		}
	}

	// Remove each of the read-only layers from VSMB. These's are ref-counted and
	// only removed once the count drops to zero. This allows multiple containers
	// to share layers.
	if uvm.OS() == "windows" && len(layerFolders) > 1 && (op&UnmountOperationVSMB) == UnmountOperationVSMB {
		for _, layerPath := range layerFolders[:len(layerFolders)-1] {
			if e := uvm.RemoveVSMB(ctx, layerPath); e != nil {
				log.G(ctx).WithError(e).Debug("remove VSMB failed")
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
	if uvm.OS() == "linux" && len(layerFolders) > 1 && (op&UnmountOperationVPMEM) == UnmountOperationVPMEM {
		for _, layerPath := range layerFolders[:len(layerFolders)-1] {
			hostPath := filepath.Join(layerPath, "layer.vhd")

			// Assume it was added to vPMEM and fall back to SCSI
			e := uvm.RemoveVPMEM(ctx, hostPath)
			if e == uvmpkg.ErrNotAttached {
				e = uvm.RemoveSCSI(ctx, hostPath)
			}
			if e != nil {
				log.G(ctx).WithError(e).Debug("remove layer failed")
				if retError == nil {
					retError = e
				} else {
					retError = errors.Wrapf(retError, e.Error())
				}
			}
		}
	}

	// TODO (possibly) Consider deleting the container directory in the utility VM

	return retError
}

func computeV2Layers(ctx context.Context, vm *uvm.UtilityVM, paths []string) (layers []hcsschema.Layer, err error) {
	for _, path := range paths {
		uvmPath, err := vm.GetVSMBUvmPath(ctx, path)
		if err != nil {
			return nil, err
		}
		layerID, err := wclayer.LayerID(path)
		if err != nil {
			return nil, err
		}
		layers = append(layers, hcsschema.Layer{Id: layerID.String(), Path: uvmPath})
	}
	return layers, nil
}
