//go:build windows
// +build windows

package layers

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"

	"github.com/Microsoft/hcsshim/internal/guestpath"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/hcserror"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/ospath"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/internal/wclayer"
)

type LayerMount struct {
	HostPath  string
	GuestPath string
}

// ImageLayers contains all the layers for an image.
type ImageLayers struct {
	vm                 *uvm.UtilityVM
	containerRootInUVM string
	volumeMountPath    string
	layers             []*LayerMount
	// In some instances we may want to avoid cleaning up the image layers, such as when tearing
	// down a sandbox container since the UVM will be torn down shortly after and the resources
	// can be cleaned up on the host.
	skipCleanup bool
}

func NewImageLayers(vm *uvm.UtilityVM, containerRootInUVM string, layers []*LayerMount, volumeMountPath string, skipCleanup bool) *ImageLayers {
	return &ImageLayers{
		vm:                 vm,
		containerRootInUVM: containerRootInUVM,
		layers:             layers,
		volumeMountPath:    volumeMountPath,
		skipCleanup:        skipCleanup,
	}
}

// Release unmounts all of the layers located in the layers array.
func (layers *ImageLayers) Release(ctx context.Context, all bool) error {
	if layers.skipCleanup && layers.vm != nil {
		return nil
	}
	op := UnmountOperationSCSI
	if layers.vm == nil || all {
		op = UnmountOperationAll
	}
	var crp string
	if layers.vm != nil {
		crp = containerRootfsPath(layers.vm, layers.containerRootInUVM)
	}
	err := UnmountContainerLayers(ctx, layers.layers, crp, layers.volumeMountPath, layers.vm, op)
	if err != nil {
		return err
	}
	layers.layers = nil
	return nil
}

// MountLCOWLayers is a helper for clients to hide all the complexity of layer mounting for LCOW
// Layer folder are in order: base, [rolayer1..rolayern,] scratch
// Returns the path at which the `rootfs` of the container can be accessed. Also, returns the path inside the
// UVM at which container scratch directory is located. Usually, this path is the path at which the container
// scratch VHD is mounted. However, in case of scratch sharing this is a directory under the UVM scratch.
func MountLCOWLayers(ctx context.Context, containerID string, layerFolders []string, guestRoot, volumeMountPath string, vm *uvm.UtilityVM) (_, _ string, _ []*LayerMount, err error) {
	if vm.OS() != "linux" {
		return "", "", nil, errors.New("MountLCOWLayers should only be called for LCOW")
	}

	// V2 UVM
	log.G(ctx).WithField("os", vm.OS()).Debug("hcsshim::MountLCOWLayers V2 UVM")

	var (
		layersAdded       []*LayerMount
		lcowUvmLayerPaths []string
	)
	defer func() {
		if err != nil {
			for _, l := range layersAdded {
				if err := removeLCOWLayer(ctx, vm, l); err != nil {
					log.G(ctx).WithError(err).Warn("failed to remove lcow layer on cleanup")
				}
			}
		}
	}()

	for _, layerPath := range layerFolders[:len(layerFolders)-1] {
		log.G(ctx).WithField("layerPath", layerPath).Debug("mounting layer")
		if path.Ext(layerPath) != ".vhd" && path.Ext(layerPath) != ".vhdx" {
			layerPath = filepath.Join(layerPath, "layer.vhd")
		}
		layerMount, err := addLCOWLayer(ctx, vm, layerPath)
		if err != nil {
			return "", "", nil, fmt.Errorf("failed to add LCOW layer: %s", err)
		}
		layersAdded = append(layersAdded, layerMount)
		lcowUvmLayerPaths = append(lcowUvmLayerPaths, layerMount.GuestPath)
	}

	containerScratchPathInUVM := ospath.Join(vm.OS(), guestRoot)
	hostPath, err := getScratchVHDHostPath(layerFolders)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to get scratch VHD path in layer folders: %s", err)
	}
	log.G(ctx).WithField("hostPath", hostPath).Debug("mounting scratch VHD")

	var options []string
	scsiMount, err := vm.AddSCSI(
		ctx,
		hostPath,
		containerScratchPathInUVM,
		false,
		vm.ScratchEncryptionEnabled(),
		options,
		uvm.VMAccessTypeIndividual,
	)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to add SCSI scratch VHD: %s", err)
	}

	// handles the case where we want to share a scratch disk for multiple containers instead
	// of mounting a new one. Pass a unique value for `ScratchPath` to avoid container upper and
	// work directories colliding in the UVM.
	if scsiMount.RefCount() > 1 {
		scratchFmt := fmt.Sprintf("container_%s", filepath.Base(containerScratchPathInUVM))
		containerScratchPathInUVM = ospath.Join("linux", scsiMount.UVMPath, scratchFmt)
	}

	defer func() {
		if err != nil {
			if err := vm.RemoveSCSIMount(ctx, scsiMount.HostPath, scsiMount.UVMPath); err != nil {
				log.G(ctx).WithError(err).Warn("failed to remove scratch on cleanup")
			}
		}
	}()

	// add the scratch to the resulting layer mounts
	scratchLayerMount := &LayerMount{
		HostPath:  scsiMount.HostPath,
		GuestPath: scsiMount.UVMPath,
	}
	layersAdded = append(layersAdded, scratchLayerMount)

	// combine the layers
	rootfs := ospath.Join(vm.OS(), guestRoot, guestpath.RootfsPath)
	err = vm.CombineLayersLCOW(ctx, containerID, lcowUvmLayerPaths, containerScratchPathInUVM, rootfs)
	if err != nil {
		return "", "", nil, err
	}
	log.G(ctx).Debug("hcsshim::MountLCOWLayers Succeeded")
	return rootfs, containerScratchPathInUVM, layersAdded, nil
}

// MountWCOWLayers is a helper for clients to hide all the complexity of layer mounting for WCOW.
// Layer folder are in order: base, [rolayer1..rolayern,] scratch
//
//	v1/v2: Argon WCOW: Returns the mount path on the host as a volume GUID.
//	v1:    Xenon WCOW: Done internally in HCS, so no point calling doing anything here.
//	v2:    Xenon WCOW: Returns a CombinedLayersV2 structure where ContainerRootPath is a folder
//	inside the utility VM which is a GUID mapping of the scratch folder. Each of the layers are
//	the VSMB locations where the read-only layers are mounted.
//
//	Job container: Returns the mount path on the host as a volume guid, with the volume mounted on
//	the host at `volumeMountPath`.
func MountWCOWLayers(ctx context.Context, containerID string, layerFolders []string, guestRoot, volumeMountPath string, vm *uvm.UtilityVM) (_ string, _ []*LayerMount, err error) {
	var (
		layersAdded    []*LayerMount
		layerHostPaths []string
	)

	if vm == nil {
		if len(layerFolders) < 2 {
			return "", nil, errors.New("need at least two layers - base and scratch")
		}
		path := layerFolders[len(layerFolders)-1]
		rest := layerFolders[:len(layerFolders)-1]
		// Simple retry loop to handle some behavior on RS5. Loopback VHDs used to be mounted in a different manner on RS5 (ws2019) which led to some
		// very odd cases where things would succeed when they shouldn't have, or we'd simply timeout if an operation took too long. Many
		// parallel invocations of this code path and stressing the machine seem to bring out the issues, but all of the possible failure paths
		// that bring about the errors we have observed aren't known.
		//
		// On 19h1+ this *shouldn't* be needed, but the logic is to break if everything succeeded so this is harmless and shouldn't need a version check.
		var lErr error
		for i := 0; i < 5; i++ {
			lErr = func() (err error) {
				if err := wclayer.ActivateLayer(ctx, path); err != nil {
					return err
				}

				defer func() {
					if err != nil {
						_ = wclayer.DeactivateLayer(ctx, path)
					}
				}()

				return wclayer.PrepareLayer(ctx, path, rest)
			}()

			if lErr != nil {
				// Common errors seen from the RS5 behavior mentioned above is ERROR_NOT_READY and ERROR_DEVICE_NOT_CONNECTED. The former occurs when HCS
				// tries to grab the volume path of the disk but it doesn't succeed, usually because the disk isn't actually mounted. DEVICE_NOT_CONNECTED
				// has been observed after launching multiple containers in parallel on a machine under high load. This has also been observed to be a trigger
				// for ERROR_NOT_READY as well.
				if hcserr, ok := lErr.(*hcserror.HcsError); ok {
					if hcserr.Err == windows.ERROR_NOT_READY || hcserr.Err == windows.ERROR_DEVICE_NOT_CONNECTED {
						log.G(ctx).WithField("path", path).WithError(hcserr.Err).Warning("retrying layer operations after failure")

						// Sleep for a little before a re-attempt. A probable cause for these issues in the first place is events not getting
						// reported in time so might be good to give some time for things to "cool down" or get back to a known state.
						time.Sleep(time.Millisecond * 100)
						continue
					}
				}
				// This was a failure case outside of the commonly known error conditions, don't retry here.
				return "", nil, lErr
			}

			// No errors in layer setup, we can leave the loop
			break
		}
		// If we got unlucky and ran into one of the two errors mentioned five times in a row and left the loop, we need to check
		// the loop error here and fail also.
		if lErr != nil {
			return "", nil, errors.Wrap(lErr, "layer retry loop failed")
		}

		// If any of the below fails, we want to detach the filter and unmount the disk.
		defer func() {
			if err != nil {
				_ = wclayer.UnprepareLayer(ctx, path)
				_ = wclayer.DeactivateLayer(ctx, path)
			}
		}()

		mountPath, err := wclayer.GetLayerMountPath(ctx, path)
		if err != nil {
			return "", nil, err
		}

		// Mount the volume to a directory on the host if requested. This is the case for job containers.
		if volumeMountPath != "" {
			if err := MountSandboxVolume(ctx, volumeMountPath, mountPath); err != nil {
				return "", nil, err
			}
		}

		// we only need to track the base path here because on unmount, that's
		// all we need
		layerMount := &LayerMount{
			HostPath: path,
		}
		layersAdded = append(layersAdded, layerMount)

		return mountPath, layersAdded, nil
	}

	if vm.OS() != "windows" {
		return "", nil, errors.New("MountWCOWLayers should only be called for WCOW")
	}

	// V2 UVM
	log.G(ctx).WithField("os", vm.OS()).Debug("hcsshim::MountWCOWLayers V2 UVM")

	defer func() {
		if err != nil {
			for _, l := range layersAdded {
				if err := vm.RemoveVSMB(ctx, l.HostPath, true); err != nil {
					log.G(ctx).WithError(err).Warn("failed to remove wcow layer on cleanup")
				}
			}
		}
	}()

	for _, layerPath := range layerFolders[:len(layerFolders)-1] {
		log.G(ctx).WithField("layerPath", layerPath).Debug("mounting layer")
		options := vm.DefaultVSMBOptions(true)
		options.TakeBackupPrivilege = true
		if vm.IsTemplate {
			vm.SetSaveableVSMBOptions(options, options.ReadOnly)
		}
		vsmb, err := vm.AddVSMB(ctx, layerPath, options)
		if err != nil {
			return "", nil, fmt.Errorf("failed to add VSMB layer: %s", err)
		}
		layerMount := &LayerMount{
			HostPath:  layerPath,
			GuestPath: vsmb.GuestPath,
		}
		layersAdded = append(layersAdded, layerMount)
		layerHostPaths = append(layerHostPaths, layerPath)
	}

	containerScratchPathInUVM := ospath.Join(vm.OS(), guestRoot)
	hostPath, err := getScratchVHDHostPath(layerFolders)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get scratch VHD path in layer folders: %s", err)
	}
	log.G(ctx).WithField("hostPath", hostPath).Debug("mounting scratch VHD")

	var options []string
	scsiMount, err := vm.AddSCSI(
		ctx,
		hostPath,
		containerScratchPathInUVM,
		false,
		vm.ScratchEncryptionEnabled(),
		options,
		uvm.VMAccessTypeIndividual,
	)
	if err != nil {
		return "", nil, fmt.Errorf("failed to add SCSI scratch VHD: %s", err)
	}
	containerScratchPathInUVM = scsiMount.UVMPath

	defer func() {
		if err != nil {
			if err := vm.RemoveSCSIMount(ctx, scsiMount.HostPath, scsiMount.UVMPath); err != nil {
				log.G(ctx).WithError(err).Warn("failed to remove scratch on cleanup")
			}
		}
	}()

	// add the scratch to the resulting layer mounts
	scratchLayerMount := &LayerMount{
		HostPath:  scsiMount.HostPath,
		GuestPath: scsiMount.UVMPath,
	}
	layersAdded = append(layersAdded, scratchLayerMount)

	// Load the filter at the C:\s<ID> location calculated above. We pass into this
	// request each of the read-only layer folders.
	var layers []hcsschema.Layer
	layers, err = GetHCSLayers(ctx, vm, layerHostPaths)
	if err != nil {
		return "", nil, err
	}
	err = vm.CombineLayersWCOW(ctx, layers, containerScratchPathInUVM)
	if err != nil {
		return "", nil, err
	}
	log.G(ctx).Debug("hcsshim::MountWCOWLayers Succeeded")
	return containerScratchPathInUVM, layersAdded, nil
}

func addLCOWLayer(ctx context.Context, vm *uvm.UtilityVM, layerPath string) (_ *LayerMount, err error) {
	// don't try to add as vpmem when we want additional devices on the uvm to be fully physically backed
	if !vm.DevicesPhysicallyBacked() {
		// We first try vPMEM and if it is full or the file is too large we
		// fall back to SCSI.
		uvmPath, err := vm.AddVPMem(ctx, layerPath)
		if err == nil {
			log.G(ctx).WithFields(logrus.Fields{
				"layerPath": layerPath,
				"layerType": "vpmem",
			}).Debug("Added LCOW layer")
			layerMount := &LayerMount{
				HostPath:  layerPath,
				GuestPath: uvmPath,
			}
			return layerMount, nil
		} else if err != uvm.ErrNoAvailableLocation && err != uvm.ErrMaxVPMemLayerSize {
			return nil, fmt.Errorf("failed to add VPMEM layer: %s", err)
		}
	}

	// TODO katiewasnothere: parse the string to see if we have a partition
	options := []string{"ro"}
	uvmPath := fmt.Sprintf(guestpath.LCOWGlobalMountPrefixFmt, vm.UVMMountCounter())
	sm, err := vm.AddSCSILayer(
		ctx,
		layerPath,
		uvmPath,
		0,
		true,
		false,
		options,
		uvm.VMAccessTypeNoop,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to add SCSI layer: %s", err)
	}

	layerMount := &LayerMount{
		HostPath:  layerPath,
		GuestPath: sm.UVMPath,
	}

	log.G(ctx).WithFields(logrus.Fields{
		"layerPath": layerPath,
		"layerType": "scsi",
	}).Debug("Added LCOW layer")
	return layerMount, nil
}

func removeLCOWLayer(ctx context.Context, vm *uvm.UtilityVM, layer *LayerMount) error {
	// Assume it was added to vPMEM and fall back to SCSI
	err := vm.RemoveVPMem(ctx, layer.HostPath)
	if err == nil {
		log.G(ctx).WithFields(logrus.Fields{
			"layerPath": layer.HostPath,
			"layerType": "vpmem",
		}).Debug("Removed LCOW layer")
		return nil
	} else if err == uvm.ErrNotAttached {
		err = vm.RemoveSCSIMount(ctx, layer.HostPath, layer.GuestPath)
		if err == nil {
			log.G(ctx).WithFields(logrus.Fields{
				"layerPath": layer.HostPath,
				"layerType": "scsi",
			}).Debug("Removed LCOW layer")
			return nil
		}
		return errors.Wrap(err, "failed to remove SCSI layer")
	}
	return errors.Wrap(err, "failed to remove VPMEM layer")
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
func UnmountContainerLayers(ctx context.Context, layerMounts []*LayerMount, containerRootPath, volumeMountPath string, vm *uvm.UtilityVM, op UnmountOperation) error {
	log.G(ctx).WithField("layerMounts", layerMounts).Debug("hcsshim::unmountContainerLayers")
	if vm == nil {
		// Must be an argon - folders are mounted on the host
		if op != UnmountOperationAll {
			return errors.New("only operation supported for host-mounted folders is unmountOperationAll")
		}
		if len(layerMounts) < 1 {
			return errors.New("need at least one layer for Unmount")
		}

		// Remove the mount point if there is one. This is the fallback case for job containers
		// if no bind mount support is available.
		if volumeMountPath != "" {
			if err := RemoveSandboxMountPoint(ctx, volumeMountPath); err != nil {
				return err
			}
		}

		baseLayer := layerMounts[len(layerMounts)-1]
		if err := wclayer.UnprepareLayer(ctx, baseLayer.HostPath); err != nil {
			return err
		}
		return wclayer.DeactivateLayer(ctx, baseLayer.HostPath)
	}

	// V2 Xenon

	// Base+Scratch as a minimum. This is different to v1 which only requires the scratch
	if len(layerMounts) < 2 {
		return errors.New("at least two layers are required for unmount")
	}

	var retError error

	// Always remove the combined layers as they are part of scsi/vsmb/vpmem
	// removals.
	if vm.OS() == "windows" {
		if err := vm.RemoveCombinedLayersWCOW(ctx, containerRootPath); err != nil {
			log.G(ctx).WithError(err).Warn("failed guest request to remove combined layers")
			retError = err
		}
	} else {
		if err := vm.RemoveCombinedLayersLCOW(ctx, containerRootPath); err != nil {
			log.G(ctx).WithError(err).Warn("failed guest request to remove combined layers")
			retError = err
		}
	}

	// Unload the SCSI scratch path
	if (op & UnmountOperationSCSI) == UnmountOperationSCSI {
		// TODO katiewasnothere: make sure that there is a scratch layer mount
		scratchFile := getScratchVHDMount(layerMounts)
		// TODO katiewasnothere: make sure scratch has a uvm path
		if err := vm.RemoveSCSIMount(ctx, scratchFile.HostPath, scratchFile.GuestPath); err != nil {
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
	if vm.OS() == "windows" && (op&UnmountOperationVSMB) == UnmountOperationVSMB {
		for _, layer := range layerMounts[:len(layerMounts)-1] {
			if e := vm.RemoveVSMB(ctx, layer.HostPath, true); e != nil {
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
	if vm.OS() == "linux" && (op&UnmountOperationVPMEM) == UnmountOperationVPMEM {
		for _, layer := range layerMounts[:len(layerMounts)-1] {
			// TODO katiewasnothere: make sure the hostpath has the vhdx ext
			if err := removeLCOWLayer(ctx, vm, layer); err != nil {
				log.G(ctx).WithError(err).Warn("remove layer failed")
				if retError == nil {
					retError = err
				} else {
					retError = errors.Wrapf(retError, err.Error())
				}
			}
		}
	}

	return retError
}

// GetHCSLayers converts host paths corresponding to container layers into HCS schema V2 layers
func GetHCSLayers(ctx context.Context, vm *uvm.UtilityVM, paths []string) (layers []hcsschema.Layer, err error) {
	for _, path := range paths {
		uvmPath, err := vm.GetVSMBUvmPath(ctx, path, true)
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

func containerRootfsPath(vm *uvm.UtilityVM, rootPath string) string {
	if vm.OS() == "windows" {
		return ospath.Join(vm.OS(), rootPath)
	}
	return ospath.Join(vm.OS(), rootPath, guestpath.RootfsPath)
}

func getScratchVHDHostPath(layerFolders []string) (string, error) {
	hostPath := filepath.Join(layerFolders[len(layerFolders)-1], "sandbox.vhdx")
	// For LCOW, we can reuse another container's scratch space (usually the sandbox container's).
	//
	// When sharing a scratch space, the `hostPath` will be a symlink to the sandbox.vhdx location to use.
	// When not sharing a scratch space, `hostPath` will be the path to the sandbox.vhdx to use.
	//
	// Evaluate the symlink here (if there is one).
	hostPath, err := filepath.EvalSymlinks(hostPath)
	if err != nil {
		return "", errors.Wrap(err, "failed to eval symlinks")
	}
	return hostPath, nil
}

// TODO katiewasnothere: this should already have the vhdx at the end
func getScratchVHDMount(layerMounts []*LayerMount) *LayerMount {
	scratchMount := layerMounts[len(layerMounts)-1]
	return scratchMount
}

// Mount the sandbox vhd to a user friendly path.
func MountSandboxVolume(ctx context.Context, hostPath, volumeName string) (err error) {
	log.G(ctx).WithFields(logrus.Fields{
		"hostpath":   hostPath,
		"volumeName": volumeName,
	}).Debug("mounting volume for container")

	if _, err := os.Stat(hostPath); os.IsNotExist(err) {
		if err := os.MkdirAll(hostPath, 0777); err != nil {
			return err
		}
	}

	defer func() {
		if err != nil {
			os.RemoveAll(hostPath)
		}
	}()

	// Make sure volumeName ends with a trailing slash as required.
	if volumeName[len(volumeName)-1] != '\\' {
		volumeName += `\` // Be nice to clients and make sure well-formed for back-compat
	}

	if err = windows.SetVolumeMountPoint(windows.StringToUTF16Ptr(hostPath), windows.StringToUTF16Ptr(volumeName)); err != nil {
		return errors.Wrapf(err, "failed to mount sandbox volume to %s on host", hostPath)
	}
	return nil
}

// Remove volume mount point. And remove folder afterwards.
func RemoveSandboxMountPoint(ctx context.Context, hostPath string) error {
	log.G(ctx).WithFields(logrus.Fields{
		"hostpath": hostPath,
	}).Debug("removing volume mount point for container")

	if err := windows.DeleteVolumeMountPoint(windows.StringToUTF16Ptr(hostPath)); err != nil {
		return errors.Wrap(err, "failed to delete sandbox volume mount point")
	}
	if err := os.Remove(hostPath); err != nil {
		return errors.Wrapf(err, "failed to remove sandbox mounted folder path %q", hostPath)
	}
	return nil
}
