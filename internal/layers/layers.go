//go:build windows
// +build windows

package layers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Microsoft/go-winio/pkg/fs"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"

	"github.com/Microsoft/hcsshim/internal/guestpath"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/hcserror"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/ospath"
	"github.com/Microsoft/hcsshim/internal/resources"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/internal/uvm/scsi"
	"github.com/Microsoft/hcsshim/internal/wclayer"
)

type LCOWLayer struct {
	VHDPath   string
	Partition uint64
}

// Defines a set of LCOW layers.
// For future extensibility, the LCOWLayer type could be swapped for an interface,
// and we could either call some method on the interface to "apply" it directly to the UVM,
// or type cast it to the various types that we support, and use the one it matches.
// This would allow us to support different "types" of mounts, such as raw VHD, VHD+partition, etc.
type LCOWLayers struct {
	// Should be in order from top-most layer to bottom-most layer.
	Layers         []*LCOWLayer
	ScratchVHDPath string
}

type lcowLayersCloser struct {
	uvm                     *uvm.UtilityVM
	guestCombinedLayersPath string
	scratchMount            resources.ResourceCloser
	layerClosers            []resources.ResourceCloser
}

func (lc *lcowLayersCloser) Release(ctx context.Context) (retErr error) {
	if err := lc.uvm.RemoveCombinedLayersLCOW(ctx, lc.guestCombinedLayersPath); err != nil {
		log.G(ctx).WithError(err).Error("failed RemoveCombinedLayersLCOW")
		if retErr == nil {
			retErr = fmt.Errorf("first error: %w", err)
		}
	}
	if err := lc.scratchMount.Release(ctx); err != nil {
		log.G(ctx).WithError(err).Error("failed LCOW scratch mount release")
		if retErr == nil {
			retErr = fmt.Errorf("first error: %w", err)
		}
	}
	for i, closer := range lc.layerClosers {
		if err := closer.Release(ctx); err != nil {
			log.G(ctx).WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"layerIndex":    i,
			}).Error("failed releasing LCOW layer")
			if retErr == nil {
				retErr = fmt.Errorf("first error: %w", err)
			}
		}
	}
	return
}

// MountLCOWLayers is a helper for clients to hide all the complexity of layer mounting for LCOW
// Layer folder are in order: base, [rolayer1..rolayern,] scratch
// Returns the path at which the `rootfs` of the container can be accessed. Also, returns the path inside the
// UVM at which container scratch directory is located. Usually, this path is the path at which the container
// scratch VHD is mounted. However, in case of scratch sharing this is a directory under the UVM scratch.
func MountLCOWLayers(ctx context.Context, containerID string, layers *LCOWLayers, guestRoot string, vm *uvm.UtilityVM) (_, _ string, _ resources.ResourceCloser, err error) {
	if vm == nil {
		return "", "", nil, errors.New("MountLCOWLayers cannot be called for process-isolated containers")
	}

	if vm.OS() != "linux" {
		return "", "", nil, errors.New("MountLCOWLayers should only be called for LCOW")
	}

	// V2 UVM
	log.G(ctx).WithField("os", vm.OS()).Debug("hcsshim::MountLCOWLayers V2 UVM")

	var (
		layerClosers      []resources.ResourceCloser
		lcowUvmLayerPaths []string
	)
	defer func() {
		if err != nil {
			for _, closer := range layerClosers {
				if err := closer.Release(ctx); err != nil {
					log.G(ctx).WithError(err).Warn("failed to remove lcow layer on cleanup")
				}
			}
		}
	}()

	for _, layer := range layers.Layers {
		log.G(ctx).WithField("layerPath", layer.VHDPath).Debug("mounting layer")
		uvmPath, closer, err := addLCOWLayer(ctx, vm, layer)
		if err != nil {
			return "", "", nil, fmt.Errorf("failed to add LCOW layer: %s", err)
		}
		layerClosers = append(layerClosers, closer)
		lcowUvmLayerPaths = append(lcowUvmLayerPaths, uvmPath)
	}

	hostPath := layers.ScratchVHDPath
	hostPath, err = filepath.EvalSymlinks(hostPath)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to eval symlinks on scratch path: %w", err)
	}
	log.G(ctx).WithField("hostPath", hostPath).Debug("mounting scratch VHD")

	scsiMount, err := vm.SCSIManager.AddVirtualDisk(
		ctx,
		hostPath,
		false,
		vm.ID(),
		&scsi.MountConfig{Encrypted: vm.ScratchEncryptionEnabled()},
	)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to add SCSI scratch VHD: %s", err)
	}

	// handles the case where we want to share a scratch disk for multiple containers instead
	// of mounting a new one. Pass a unique value for `ScratchPath` to avoid container upper and
	// work directories colliding in the UVM.
	containerScratchPathInUVM := ospath.Join("linux", scsiMount.GuestPath(), "scratch", containerID)

	defer func() {
		if err != nil {
			if err := scsiMount.Release(ctx); err != nil {
				log.G(ctx).WithError(err).Warn("failed to remove scratch on cleanup")
			}
		}
	}()

	rootfs := ospath.Join(vm.OS(), guestRoot, guestpath.RootfsPath)
	err = vm.CombineLayersLCOW(ctx, containerID, lcowUvmLayerPaths, containerScratchPathInUVM, rootfs)
	if err != nil {
		return "", "", nil, err
	}
	log.G(ctx).Debug("hcsshim::MountLCOWLayers Succeeded")
	closer := &lcowLayersCloser{
		uvm:                     vm,
		guestCombinedLayersPath: rootfs,
		scratchMount:            scsiMount,
		layerClosers:            layerClosers,
	}
	return rootfs, containerScratchPathInUVM, closer, nil
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
func MountWCOWLayers(ctx context.Context, containerID string, layerFolders []string, volumeMountPath string, vm *uvm.UtilityVM) (_ string, _ resources.ResourceCloser, err error) {
	if vm == nil {
		return mountWCOWHostLayers(ctx, layerFolders, volumeMountPath)
	}

	if vm.OS() != "windows" {
		return "", nil, errors.New("MountWCOWLayers should only be called for WCOW")
	}

	return mountWCOWIsolatedLayers(ctx, containerID, layerFolders, volumeMountPath, vm)
}

type wcowHostLayersCloser struct {
	volumeMountPath        string
	scratchLayerFolderPath string
}

func (lc *wcowHostLayersCloser) Release(ctx context.Context) error {
	if lc.volumeMountPath != "" {
		if err := RemoveSandboxMountPoint(ctx, lc.volumeMountPath); err != nil {
			return err
		}
	}
	if err := wclayer.UnprepareLayer(ctx, lc.scratchLayerFolderPath); err != nil {
		return err
	}
	return wclayer.DeactivateLayer(ctx, lc.scratchLayerFolderPath)
}

func mountWCOWHostLayers(ctx context.Context, layerFolders []string, volumeMountPath string) (_ string, _ resources.ResourceCloser, err error) {
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

	closer := &wcowHostLayersCloser{
		volumeMountPath:        volumeMountPath,
		scratchLayerFolderPath: path,
	}
	return mountPath, closer, nil
}

type wcowIsolatedLayersCloser struct {
	uvm                     *uvm.UtilityVM
	guestCombinedLayersPath string
	scratchMount            resources.ResourceCloser
	layerClosers            []resources.ResourceCloser
}

func (lc *wcowIsolatedLayersCloser) Release(ctx context.Context) (retErr error) {
	if err := lc.uvm.RemoveCombinedLayersWCOW(ctx, lc.guestCombinedLayersPath); err != nil {
		log.G(ctx).WithError(err).Error("failed RemoveCombinedLayersWCOW")
		if retErr == nil {
			retErr = fmt.Errorf("first error: %w", err)
		}
	}
	if err := lc.scratchMount.Release(ctx); err != nil {
		log.G(ctx).WithError(err).Error("failed WCOW scratch mount release")
		if retErr == nil {
			retErr = fmt.Errorf("first error: %w", err)
		}
	}
	for i, closer := range lc.layerClosers {
		if err := closer.Release(ctx); err != nil {
			log.G(ctx).WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"layerIndex":    i,
			}).Error("failed releasing WCOW layer")
			if retErr == nil {
				retErr = fmt.Errorf("first error: %w", err)
			}
		}
	}
	return
}

func mountWCOWIsolatedLayers(ctx context.Context, containerID string, layerFolders []string, volumeMountPath string, vm *uvm.UtilityVM) (_ string, _ resources.ResourceCloser, err error) {
	log.G(ctx).WithField("os", vm.OS()).Debug("hcsshim::MountWCOWLayers V2 UVM")

	var (
		layersAdded  []string
		layerClosers []resources.ResourceCloser
	)
	defer func() {
		if err != nil {
			for _, l := range layerClosers {
				if err := l.Release(ctx); err != nil {
					log.G(ctx).WithError(err).Warn("failed to remove wcow layer on cleanup")
				}
			}
		}
	}()

	for _, layerPath := range layerFolders[:len(layerFolders)-1] {
		log.G(ctx).WithField("layerPath", layerPath).Debug("mounting layer")
		options := vm.DefaultVSMBOptions(true)
		options.TakeBackupPrivilege = true
		mount, err := vm.AddVSMB(ctx, layerPath, options)
		if err != nil {
			return "", nil, fmt.Errorf("failed to add VSMB layer: %s", err)
		}
		layersAdded = append(layersAdded, layerPath)
		layerClosers = append(layerClosers, mount)
	}

	hostPath, err := getScratchVHDPath(layerFolders)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get scratch VHD path in layer folders: %s", err)
	}
	log.G(ctx).WithField("hostPath", hostPath).Debug("mounting scratch VHD")

	scsiMount, err := vm.SCSIManager.AddVirtualDisk(ctx, hostPath, false, vm.ID(), &scsi.MountConfig{})
	if err != nil {
		return "", nil, fmt.Errorf("failed to add SCSI scratch VHD: %s", err)
	}
	containerScratchPathInUVM := scsiMount.GuestPath()

	defer func() {
		if err != nil {
			if err := scsiMount.Release(ctx); err != nil {
				log.G(ctx).WithError(err).Warn("failed to remove scratch on cleanup")
			}
		}
	}()

	// Load the filter at the C:\s<ID> location calculated above. We pass into this
	// request each of the read-only layer folders.
	var layers []hcsschema.Layer
	layers, err = GetHCSLayers(ctx, vm, layersAdded)
	if err != nil {
		return "", nil, err
	}
	err = vm.CombineLayersWCOW(ctx, layers, containerScratchPathInUVM)
	if err != nil {
		return "", nil, err
	}
	log.G(ctx).Debug("hcsshim::MountWCOWLayers Succeeded")
	closer := &wcowIsolatedLayersCloser{
		uvm:                     vm,
		guestCombinedLayersPath: containerScratchPathInUVM,
		scratchMount:            scsiMount,
		layerClosers:            layerClosers,
	}
	return containerScratchPathInUVM, closer, nil
}

func addLCOWLayer(ctx context.Context, vm *uvm.UtilityVM, layer *LCOWLayer) (uvmPath string, _ resources.ResourceCloser, err error) {
	// Don't add as VPMEM when we want additional devices on the UVM to be fully physically backed.
	// Also don't use VPMEM when we need to mount a specific partition of the disk, as this is only
	// supported for SCSI.
	if !vm.DevicesPhysicallyBacked() && layer.Partition == 0 {
		// We first try vPMEM and if it is full or the file is too large we
		// fall back to SCSI.
		mount, err := vm.AddVPMem(ctx, layer.VHDPath)
		if err == nil {
			log.G(ctx).WithFields(logrus.Fields{
				"layerPath": layer.VHDPath,
				"layerType": "vpmem",
			}).Debug("Added LCOW layer")
			return mount.GuestPath, mount, nil
		} else if err != uvm.ErrNoAvailableLocation && err != uvm.ErrMaxVPMemLayerSize {
			return "", nil, fmt.Errorf("failed to add VPMEM layer: %s", err)
		}
	}

	sm, err := vm.SCSIManager.AddVirtualDisk(ctx, layer.VHDPath, true, "", &scsi.MountConfig{Partition: layer.Partition, Options: []string{"ro"}})
	if err != nil {
		return "", nil, fmt.Errorf("failed to add SCSI layer: %s", err)
	}
	log.G(ctx).WithFields(logrus.Fields{
		"layerPath":      layer.VHDPath,
		"layerPartition": layer.Partition,
		"layerType":      "scsi",
	}).Debug("Added LCOW layer")
	return sm.GuestPath(), sm, nil
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

func getScratchVHDPath(layerFolders []string) (string, error) {
	hostPath := filepath.Join(layerFolders[len(layerFolders)-1], "sandbox.vhdx")
	// For LCOW, we can reuse another container's scratch space (usually the sandbox container's).
	//
	// When sharing a scratch space, the `hostPath` will be a symlink to the sandbox.vhdx location to use.
	// When not sharing a scratch space, `hostPath` will be the path to the sandbox.vhdx to use.
	//
	// Evaluate the symlink here (if there is one).
	hostPath, err := fs.ResolvePath(hostPath)
	if err != nil {
		return "", errors.Wrap(err, "failed to resolve path")
	}
	return hostPath, nil
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
