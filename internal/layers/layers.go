// +build windows

// Package layers deals with container layer mounting/unmounting for LCOW and WCOW
package layers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/hcserror"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/ospath"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	cimlayer "github.com/Microsoft/hcsshim/internal/wclayer/cim"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

// ImageLayers contains all the layers for an image.
type ImageLayers struct {
	vm                 *uvm.UtilityVM
	containerRootInUVM string
	volumeMountPath    string
	layers             []string
	// In some instances we may want to avoid cleaning up the image layers, such as when tearing
	// down a sandbox container since the UVM will be torn down shortly after and the resources
	// can be cleaned up on the host.
	skipCleanup bool
}

func NewImageLayers(vm *uvm.UtilityVM, containerRootInUVM string, layers []string, volumeMountPath string, skipCleanup bool) *ImageLayers {
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

// mountArgonLayersWithRetries tries to mount argon layers with `retryCount` retries on failures.  This is required to
// handle some behavior on RS5. Loopback VHDs used to be mounted in a different manor on RS5 (ws2019) which led to some
// very odd cases where things would succeed when they shouldn't have, or we'd simply timeout if an operation took too
// long. Many parallel invocations of this code path and stressing the machine seem to bring out the issues, but all of
// the possible failure paths that bring about the errors we have observed aren't known.
//
// On 19h1+ this *shouldn't* be needed, but the logic is to break if everything succeeded so this is harmless and
// shouldn't need a version check.
func mountArgonLayersWithRetries(ctx context.Context, scratchLayer string, parentLayers []string, retryCount int) error {
	var lErr error
	for i := 0; i < retryCount; i++ {
		lErr = func() (err error) {
			if err := wclayer.ActivateLayer(ctx, scratchLayer); err != nil {
				return err
			}

			defer func() {
				if err != nil {
					_ = wclayer.DeactivateLayer(ctx, scratchLayer)
				}
			}()

			return wclayer.PrepareLayer(ctx, scratchLayer, parentLayers)
		}()

		if lErr != nil {
			// Common errors seen from the RS5 behavior mentioned above is ERROR_NOT_READY and
			// ERROR_DEVICE_NOT_CONNECTED. The former occurs when HCS tries to grab the volume path of the
			// disk but it doesn't succeed, usually because the disk isn't actually
			// mounted. DEVICE_NOT_CONNECTED has been observed after launching multiple containers in
			// parallel on a machine under high load. This has also been observed to be a trigger for
			// ERROR_NOT_READY as well.
			if hcserr, ok := lErr.(*hcserror.HcsError); ok {
				if hcserr.Err == windows.ERROR_NOT_READY || hcserr.Err == windows.ERROR_DEVICE_NOT_CONNECTED {
					// Sleep for a little before a re-attempt. A probable cause for these issues in
					// the first place is events not getting reported in time so might be good to
					// give some time for things to "cool down" or get back to a known state.
					time.Sleep(time.Millisecond * 100)
					continue
				}
			}
			// This was a failure case outside of the commonly known error conditions, don't retry here.
			return lErr
		}

		// No errors in layer setup, we can leave the loop
		break
	}
	// If we got unlucky and ran into one of the two errors mentioned several times in a row and left the loop, we
	// need to check the loop error here and fail also.
	if lErr != nil {
		return errors.Wrap(lErr, "layer retry loop failed")
	}
	return nil
}

// mountArgonLayers mounts the layers on the host for running argon containers. If the layers are in the cim
// format cim is mounted.
func mountArgonLayers(ctx context.Context, layerFolders []string, volumeMountPath string) (_ string, err error) {
	log.G(ctx).Debug("hcsshim::mountArgonLayers")

	if len(layerFolders) < 2 {
		return "", errors.New("need at least two layers - base and scratch")
	}
	path := layerFolders[len(layerFolders)-1]
	rest := layerFolders[:len(layerFolders)-1]
	// If layers are in the cim format mount the cim of the topmost layer.
	if strings.Contains(rest[0], "Volume") {
		rest = []string{rest[0]}
	}

	if err := mountArgonLayersWithRetries(ctx, path, rest, 5); err != nil {
		return "", err
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
		return "", err
	}

	// Mount the volume to a directory on the host if requested. This is the case for job containers.
	if volumeMountPath != "" {
		if err := mountSandboxVolume(ctx, volumeMountPath, mountPath); err != nil {
			return "", err
		}
	}

	return mountPath, nil
}

// mountXenonCimLayers mounts the given cim layers on the given uvm.  For cim layers there
// are two cases:
// 1. If the UVM image supports mounting the cim directly inside the uvm then share the
// directory on the host which has the cim over VSMB and then mount the cim inside the
// uvm. (This mounting will happen inside the shim)
// 2. If the UVM image is running an older windows version and doesn't support mounting
// the cim then the cim must be mounted on the host (which containerd must have already
// done). We expose that mount to the uvm over VSMB.
func mountXenonCimLayers(ctx context.Context, layerFolders []string, vm *uvm.UtilityVM) (_ string, err error) {
	if !strings.Contains(layerFolders[0], "Volume") {
		return "", fmt.Errorf("expected a path to mounted cim volume, found: %s", layerFolders[0])
	}
	if !cimlayer.IsCimLayer(layerFolders[1]) {
		return "", fmt.Errorf("mount cim layer requested for non-cim layer: %s", layerFolders[1])
	}
	// We only need to mount the topmost cim
	cimPath := cimlayer.GetCimPathFromLayer(layerFolders[1])
	options := vm.DefaultVSMBOptions(true)
	if vm.MountCimSupported() {
		// Mounting cim inside uvm needs direct map.
		options.NoDirectmap = false
		// Always add the parent directory of the cim as a vsmb mount because
		// there are region files in that directory that also should be shared in
		// the uvm.
		hostCimDir := filepath.Dir(cimPath)
		// Add the VSMB share
		if _, err := vm.AddVSMB(ctx, hostCimDir, options); err != nil {
			return "", fmt.Errorf("failed while sharing cim file inside uvm: %s", err)
		}
		defer func() {
			if err != nil {
				vm.RemoveVSMB(ctx, hostCimDir, true)
			}
		}()
		// get path for that share
		uvmCimDir, err := vm.GetVSMBUvmPath(ctx, hostCimDir, true)
		if err != nil {
			return "", fmt.Errorf("failed to get vsmb uvm path: %s", err)
		}
		mountCimPath, err := vm.MountInUVM(ctx, filepath.Join(uvmCimDir, filepath.Base(cimPath)))
		if err != nil {
			return "", err
		}
		return mountCimPath, nil
	} else {
		cimHostMountPath := layerFolders[0]
		if _, err := vm.AddVSMB(ctx, cimHostMountPath, options); err != nil {
			return "", fmt.Errorf("failed while sharing mounted cim inside uvm: %s", err)
		}
		// get path for that share
		cimVsmbPath, err := vm.GetVSMBUvmPath(ctx, cimHostMountPath, true)
		if err != nil {
			return "", fmt.Errorf("failed to get vsmb uvm path: %s", err)
		}
		return cimVsmbPath, nil
	}
}

// unmountXenonCimLayers unmounts the given cim layers from the given uvm.  For cim layers
// there are two cases:
// 1. If the UVM image supports mounting the cim directly inside the uvm then we must have
// exposed the cim folder over VSMB and mouted the cim inside the uvm. So unmouunt the cim
// from uvm and remove that VSMB share
// 2. If the UVM image is running an older windows version and doesn't support mounting
// the cim, then we must have exposed the mounted cim on the host to the uvm over VSMB. So
// remove the VSMB mount. (containerd will take care of unmounting the cim)
func unmountXenonCimLayers(ctx context.Context, layerFolders []string, vm *uvm.UtilityVM) (err error) {
	if !strings.Contains(layerFolders[0], "Volume") {
		return fmt.Errorf("expected a path to mounted cim volume, found: %s", layerFolders[0])
	}
	if !cimlayer.IsCimLayer(layerFolders[1]) {
		return fmt.Errorf("unmount cim layer requested for non-cim layer: %s", layerFolders[1])
	}
	cimPath := cimlayer.GetCimPathFromLayer(layerFolders[1])
	if vm.MountCimSupported() {
		hostCimDir := filepath.Dir(cimPath)
		uvmCimDir, err := vm.GetVSMBUvmPath(ctx, hostCimDir, true)
		if err != nil {
			return fmt.Errorf("failed to get vsmb uvm path while mounting cim: %s", err)
		}
		if err = vm.UnmountFromUVM(ctx, filepath.Join(uvmCimDir, filepath.Base(cimPath))); err != nil {
			return errors.Wrap(err, "failed to remove cim layer from the uvm")
		}
		return vm.RemoveVSMB(ctx, hostCimDir, true)

	} else {
		if err = vm.RemoveVSMB(ctx, layerFolders[0], true); err != nil {
			log.G(ctx).Warnf("failed to remove VSMB share: %s", err)
		}
	}
	return nil
}

// mountXenonLayers mounts the container layers inside the uvm. For legacy layers the
// layer folders are simply added as VSMB shares on the host.
func mountXenonLayers(ctx context.Context, containerID string, layerFolders []string, guestRoot string, vm *uvm.UtilityVM) (_ string, err error) {
	log.G(ctx).Debug("hcsshim::mountXenonLayers")
	var (
		layersAdded       []string
		lcowUvmLayerPaths []string
	)
	defer func() {
		if err != nil {
			if vm.OS() == "windows" {
				if err := unmountXenonWcowLayers(ctx, layerFolders, vm); err != nil {
					log.G(ctx).WithError(err).Warn("failed cleanup xenon layers")
				}
			} else {
				for _, l := range layersAdded {
					if err := removeLCOWLayer(ctx, vm, l); err != nil {
						log.G(ctx).WithError(err).Warn("failed to remove lcow layer on cleanup")
					}
				}
			}
		}
	}()

	if vm.OS() == "windows" {
		if cimlayer.IsCimLayer(layerFolders[1]) {
			_, err := mountXenonCimLayers(ctx, layerFolders, vm)
			if err != nil {
				return "", fmt.Errorf("failed to mount cim layers : %s", err)
			}
			layersAdded = append(layersAdded, layerFolders[0])
		} else {
			for _, layerPath := range layerFolders[:len(layerFolders)-1] {
				log.G(ctx).WithField("layerPath", layerPath).Debug("mounting layer")
				options := vm.DefaultVSMBOptions(true)
				options.TakeBackupPrivilege = true
				if vm.IsTemplate {
					vm.SetSaveableVSMBOptions(options, options.ReadOnly)
				}
				if _, err := vm.AddVSMB(ctx, layerPath, options); err != nil {
					return "", fmt.Errorf("failed to add VSMB layer: %s", err)
				}
				layersAdded = append(layersAdded, layerPath)

			}
		}
	} else {
		for _, layerPath := range layerFolders[:len(layerFolders)-1] {
			var (
				layerPath = filepath.Join(layerPath, "layer.vhd")
				uvmPath   string
			)
			uvmPath, err = addLCOWLayer(ctx, vm, layerPath)
			if err != nil {
				return "", fmt.Errorf("failed to add LCOW layer: %s", err)
			}
			layersAdded = append(layersAdded, layerPath)
			lcowUvmLayerPaths = append(lcowUvmLayerPaths, uvmPath)
		}
	}

	containerScratchPathInUVM := ospath.Join(vm.OS(), guestRoot)
	hostPath, err := getScratchVHDPath(layerFolders)
	if err != nil {
		return "", fmt.Errorf("failed to get scratch VHD path in layer folders: %s", err)
	}
	log.G(ctx).WithField("hostPath", hostPath).Debug("mounting scratch VHD")

	var options []string
	scsiMount, err := vm.AddSCSI(ctx, hostPath, containerScratchPathInUVM, false, options, uvm.VMAccessTypeIndividual)
	if err != nil {
		return "", fmt.Errorf("failed to add SCSI scratch VHD: %s", err)
	}

	// This handles the case where we want to share a scratch disk for multiple containers instead
	// of mounting a new one. Pass a unique value for `ScratchPath` to avoid container upper and
	// work directories colliding in the UVM.
	if scsiMount.RefCount() > 1 && vm.OS() == "linux" {
		scratchFmt := fmt.Sprintf("container_%s", filepath.Base(containerScratchPathInUVM))
		containerScratchPathInUVM = ospath.Join("linux", scsiMount.UVMPath, scratchFmt)
	} else {
		containerScratchPathInUVM = scsiMount.UVMPath
	}

	defer func() {
		if err != nil {
			if err := vm.RemoveSCSI(ctx, hostPath); err != nil {
				log.G(ctx).WithError(err).Warn("failed to remove scratch on cleanup")
			}
		}
	}()

	var rootfs string
	if vm.OS() == "windows" {
		// Load the filter at the C:\s<ID> location calculated above. We pass into this
		// request each of the read-only layer folders.
		var layers []hcsschema.Layer
		if cimlayer.IsCimLayer(layerFolders[1]) {
			layers, err = GetCimHCSLayer(ctx, vm, cimlayer.GetCimPathFromLayer(layerFolders[1]), layerFolders[0])
			if err != nil {
				return "", fmt.Errorf("failed to get hcs layer: %s", err)
			}
		} else {
			layers, err = GetHCSLayers(ctx, vm, layersAdded)
			if err != nil {
				return "", err
			}
		}
		err = vm.CombineLayersWCOW(ctx, layers, containerScratchPathInUVM)
		rootfs = containerScratchPathInUVM
	} else {
		rootfs = ospath.Join(vm.OS(), guestRoot, uvm.RootfsPath)
		err = vm.CombineLayersLCOW(ctx, containerID, lcowUvmLayerPaths, containerScratchPathInUVM, rootfs)
	}
	if err != nil {
		return "", fmt.Errorf("failed to combine layers: %s", err)
	}
	log.G(ctx).Debug("hcsshim::mountContainerLayers Succeeded")
	return rootfs, nil

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
// Job container:     Returns the mount path on the host as a volume guid, with the volume mounted on
// 	              the host at `volumeMountPath`.
// TODO dcantah: Keep better track of the layers that are added, don't simply discard the SCSI, VSMB, etc. resource types gotten inside.
func MountContainerLayers(ctx context.Context, containerID string, layerFolders []string, guestRoot, volumeMountPath string, vm *uvm.UtilityVM) (_ string, err error) {
	log.G(ctx).WithField("layerFolders", layerFolders).Debug("hcsshim::mountContainerLayers")
	if vm == nil {
		return mountArgonLayers(ctx, layerFolders, volumeMountPath)
	} else {
		return mountXenonLayers(ctx, containerID, layerFolders, guestRoot, vm)
	}
}

func addLCOWLayer(ctx context.Context, vm *uvm.UtilityVM, layerPath string) (uvmPath string, err error) {
	// don't try to add as vpmem when we want additional devices on the uvm to be fully physically backed
	if !vm.DevicesPhysicallyBacked() {
		// We first try vPMEM and if it is full or the file is too large we
		// fall back to SCSI.
		uvmPath, err = vm.AddVPMem(ctx, layerPath)
		if err == nil {
			log.G(ctx).WithFields(logrus.Fields{
				"layerPath": layerPath,
				"layerType": "vpmem",
			}).Debug("Added LCOW layer")
			return uvmPath, nil
		} else if err != uvm.ErrNoAvailableLocation && err != uvm.ErrMaxVPMemLayerSize {
			return "", fmt.Errorf("failed to add VPMEM layer: %s", err)
		}
	}

	options := []string{"ro"}
	uvmPath = fmt.Sprintf(uvm.LCOWGlobalMountPrefix, vm.UVMMountCounter())
	sm, err := vm.AddSCSI(ctx, layerPath, uvmPath, true, options, uvm.VMAccessTypeNoop)
	if err != nil {
		return "", fmt.Errorf("failed to add SCSI layer: %s", err)
	}
	log.G(ctx).WithFields(logrus.Fields{
		"layerPath": layerPath,
		"layerType": "scsi",
	}).Debug("Added LCOW layer")
	return sm.UVMPath, nil
}

func removeLCOWLayer(ctx context.Context, vm *uvm.UtilityVM, layerPath string) error {
	// Assume it was added to vPMEM and fall back to SCSI
	err := vm.RemoveVPMem(ctx, layerPath)
	if err == nil {
		log.G(ctx).WithFields(logrus.Fields{
			"layerPath": layerPath,
			"layerType": "vpmem",
		}).Debug("Removed LCOW layer")
		return nil
	} else if err == uvm.ErrNotAttached {
		err = vm.RemoveSCSI(ctx, layerPath)
		if err == nil {
			log.G(ctx).WithFields(logrus.Fields{
				"layerPath": layerPath,
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

// unmountXenonWcowLayers unmounts the container layers inside the uvm. For legacy layers
// the layer folders are just vsmb shares and so we just need to remove that vsmb
// share.
func unmountXenonWcowLayers(ctx context.Context, layerFolders []string, vm *uvm.UtilityVM) error {
	if cimlayer.IsCimLayer(layerFolders[1]) {
		if e := unmountXenonCimLayers(ctx, layerFolders, vm); e != nil {
			return errors.Wrap(e, "failed to remove cim layers")
		}
	} else {
		for _, layerPath := range layerFolders[:len(layerFolders)-1] {
			if e := vm.RemoveVSMB(ctx, layerPath, true); e != nil {
				log.G(ctx).WithError(e).Warn("remove VSMB failed")
				return errors.Wrap(e, "failed to remove layer from the uvm")
			}
		}
	}
	return nil
}

// UnmountContainerLayers is a helper for clients to hide all the complexity of layer unmounting
func UnmountContainerLayers(ctx context.Context, layerFolders []string, containerRootPath, volumeMountPath string, vm *uvm.UtilityVM, op UnmountOperation) error {
	log.G(ctx).WithField("layerFolders", layerFolders).Debug("hcsshim::unmountContainerLayers")
	if vm == nil {
		// Must be an argon - folders are mounted on the host
		if op != UnmountOperationAll {
			return errors.New("only operation supported for host-mounted folders is unmountOperationAll")
		}
		if len(layerFolders) < 1 {
			return errors.New("need at least one layer for Unmount")
		}

		// Remove the mount point if there is one. This is the case for job containers.
		if volumeMountPath != "" {
			if err := removeSandboxMountPoint(ctx, volumeMountPath); err != nil {
				return err
			}
		}

		path := layerFolders[len(layerFolders)-1]
		if err := wclayer.UnprepareLayer(ctx, path); err != nil {
			return err
		}
		if err := wclayer.DeactivateLayer(ctx, path); err != nil {
			return err
		}
		return nil
	}

	// V2 Xenon

	// Base+Scratch as a minimum. This is different to v1 which only requires the scratch
	if len(layerFolders) < 2 {
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
		hostScratchFile, err := getScratchVHDPath(layerFolders)
		if err != nil {
			return errors.Wrap(err, "failed to get scratch VHD path in layer folders")
		}
		if err := vm.RemoveSCSI(ctx, hostScratchFile); err != nil {
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
		if e := unmountXenonWcowLayers(ctx, layerFolders, vm); e != nil {
			if retError == nil {
				retError = e
			} else {
				retError = errors.Wrapf(retError, e.Error())
			}
		}
	}

	// Remove each of the read-only layers from VPMEM (or SCSI). These's are ref-counted
	// and only removed once the count drops to zero. This allows multiple containers to
	// share layers. Note that SCSI is used on large layers.
	if vm.OS() == "linux" && (op&UnmountOperationVPMEM) == UnmountOperationVPMEM {
		for _, layerPath := range layerFolders[:len(layerFolders)-1] {
			hostPath := filepath.Join(layerPath, "layer.vhd")
			if err := removeLCOWLayer(ctx, vm, hostPath); err != nil {
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

// GetCimHCSLayer finds the uvm mount path of the given cim and returns a hcs schema v2
// layer of it.  The cim must have already been mounted inside the uvm.
func GetCimHCSLayer(ctx context.Context, vm *uvm.UtilityVM, cimPath, cimMountLocation string) (layers []hcsschema.Layer, err error) {
	var uvmPath string
	if vm.MountCimSupported() {
		hostCimDir := filepath.Dir(cimPath)
		uvmCimDir, err := vm.GetVSMBUvmPath(ctx, hostCimDir, true)
		if err != nil {
			return nil, fmt.Errorf("failed to get vsmb uvm path: %s", err)
		}
		uvmPath, err = vm.GetCimUvmMountPathNt(filepath.Join(uvmCimDir, filepath.Base(cimPath)))
		if err != nil {
			return nil, err
		}
	} else {
		uvmPath, err = vm.GetVSMBUvmPath(ctx, cimMountLocation, true)
		if err != nil {
			return nil, err
		}
	}
	// Note: the LayerID must still be calculated with the cim path. The layer id
	// calculations fail if we pass it the volume path and that results in very
	// cryptic errors when starting containers.
	layerID, err := wclayer.LayerID(ctx, cimPath)
	if err != nil {
		return nil, err
	}
	layers = append(layers, hcsschema.Layer{Id: layerID.String(), Path: uvmPath})
	return layers, nil
}

func containerRootfsPath(vm *uvm.UtilityVM, rootPath string) string {
	if vm.OS() == "windows" {
		return ospath.Join(vm.OS(), rootPath)
	}
	return ospath.Join(vm.OS(), rootPath, uvm.RootfsPath)
}

func getScratchVHDPath(layerFolders []string) (string, error) {
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

// Mount the sandbox vhd to a user friendly path.
func mountSandboxVolume(ctx context.Context, hostPath, volumeName string) (err error) {
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
func removeSandboxMountPoint(ctx context.Context, hostPath string) error {
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
