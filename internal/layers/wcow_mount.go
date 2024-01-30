//go:build windows
// +build windows

package layers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"

	"github.com/Microsoft/go-winio/pkg/fs"
	"github.com/Microsoft/hcsshim/computestorage"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/hcserror"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/resources"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/internal/uvm/scsi"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	cimlayer "github.com/Microsoft/hcsshim/internal/wclayer/cim"
)

// MountWCOWLayers is a helper for clients to hide all the complexity of layer mounting for WCOW.
// Layer folder are in order: [rolayerN..rolayer1, base] scratch
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
		return mountWCOWHostLayers(ctx, layerFolders, containerID, volumeMountPath)
	}

	if vm.OS() != "windows" {
		return "", nil, errors.New("MountWCOWLayers should only be called for WCOW")
	}

	return mountWCOWIsolatedLayers(ctx, containerID, layerFolders, volumeMountPath, vm)
}

type wcowHostLayersCloser struct {
	containerID     string
	volumeMountPath string
	layers          []string
}

func ReleaseCimFSHostLayers(ctx context.Context, scratchLayerFolderPath, containerID string) error {
	mountPath, err := wclayer.GetLayerMountPath(ctx, scratchLayerFolderPath)
	if err != nil {
		return err
	}

	if err = computestorage.DetachOverlayFilter(ctx, mountPath, hcsschema.UnionFS); err != nil {
		return err
	}

	return cimlayer.CleanupContainerMounts(containerID)
}

func (lc *wcowHostLayersCloser) Release(ctx context.Context) error {
	if lc.volumeMountPath != "" {
		if err := RemoveSandboxMountPoint(ctx, lc.volumeMountPath); err != nil {
			return err
		}
	}
	scratchLayerFolderPath := lc.layers[len(lc.layers)-1]
	var err error
	if cimlayer.IsCimLayer(lc.layers[0]) {
		err = ReleaseCimFSHostLayers(ctx, scratchLayerFolderPath, lc.containerID)
	} else {
		err = wclayer.UnprepareLayer(ctx, scratchLayerFolderPath)
	}
	if err != nil {
		return err
	}
	return wclayer.DeactivateLayer(ctx, scratchLayerFolderPath)
}

func mountWCOWHostLegacyLayers(ctx context.Context, layerFolders []string, volumeMountPath string) (_ string, err error) {
	if len(layerFolders) < 2 {
		return "", errors.New("need at least two layers - base and scratch")
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
			var hcserr *hcserror.HcsError
			if errors.As(lErr, &hcserr) {
				if errors.Is(hcserr.Err, windows.ERROR_NOT_READY) || errors.Is(hcserr.Err, windows.ERROR_DEVICE_NOT_CONNECTED) {
					log.G(ctx).WithField("path", path).WithError(hcserr.Err).Warning("retrying layer operations after failure")

					// Sleep for a little before a re-attempt. A probable cause for these issues in the first place is events not getting
					// reported in time so might be good to give some time for things to "cool down" or get back to a known state.
					time.Sleep(time.Millisecond * 100)
					continue
				}
			}
			// This was a failure case outside of the commonly known error conditions, don't retry here.
			return "", lErr
		}

		// No errors in layer setup, we can leave the loop
		break
	}
	// If we got unlucky and ran into one of the two errors mentioned five times in a row and left the loop, we need to check
	// the loop error here and fail also.
	if lErr != nil {
		return "", errors.Wrap(lErr, "layer retry loop failed")
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
	return mountPath, nil

}

func mountWCOWHostCimFSLayers(ctx context.Context, layerFolders []string, containerID, volumeMountPath string) (_ string, err error) {
	scratchLayer := layerFolders[len(layerFolders)-1]
	topMostLayer := layerFolders[0]
	if err = wclayer.ActivateLayer(ctx, scratchLayer); err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			_ = wclayer.DeactivateLayer(ctx, scratchLayer)
		}
	}()

	mountPath, err := wclayer.GetLayerMountPath(ctx, scratchLayer)
	if err != nil {
		return "", err
	}

	volume, err := cimlayer.MountCimLayer(ctx, cimlayer.GetCimPathFromLayer(topMostLayer), containerID)
	if err != nil {
		return "", fmt.Errorf("mount layer cim for %s: %w", topMostLayer, err)
	}
	defer func() {
		if err != nil {
			_ = cimlayer.UnmountCimLayer(ctx, cimlayer.GetCimPathFromLayer(topMostLayer), containerID)
		}
	}()

	// Use the layer path for GUID rather than the mounted volume path, so that the generated layerID
	// remains same.
	layerID, err := wclayer.LayerID(ctx, topMostLayer)
	if err != nil {
		return "", err
	}

	layerData := computestorage.LayerData{
		FilterType: hcsschema.UnionFS,
		// Container filesystem contents are under a directory named "Files" inside the mounted cim.
		// UnionFS needs this path, so append "Files" to the layer path before passing it on.
		Layers: []hcsschema.Layer{
			{
				Id:   layerID.String(),
				Path: filepath.Join(volume, "Files"),
			},
		},
	}

	if err = computestorage.AttachOverlayFilter(ctx, mountPath, layerData); err != nil {
		return "", err
	}
	return mountPath, nil
}

func mountWCOWHostLayers(ctx context.Context, layerFolders []string, containerID, volumeMountPath string) (_ string, _ resources.ResourceCloser, err error) {
	var mountPath string
	if cimlayer.IsCimLayer(layerFolders[0]) {
		mountPath, err = mountWCOWHostCimFSLayers(ctx, layerFolders, containerID, volumeMountPath)
	} else {
		mountPath, err = mountWCOWHostLegacyLayers(ctx, layerFolders, volumeMountPath)
	}
	if err != nil {
		return "", nil, err
	}
	closer := &wcowHostLayersCloser{
		volumeMountPath: volumeMountPath,
		layers:          layerFolders,
		containerID:     containerID,
	}
	defer func() {
		if err != nil {
			_ = closer.Release(ctx)
		}
	}()

	// Mount the volume to a directory on the host if requested. This is the case for job containers.
	if volumeMountPath != "" {
		if err := MountSandboxVolume(ctx, volumeMountPath, mountPath); err != nil {
			return "", nil, err
		}
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
		if retErr == nil { //nolint:govet // nilness: consistency with below
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
			return "", nil, fmt.Errorf("failed to add VSMB layer: %w", err)
		}
		layersAdded = append(layersAdded, layerPath)
		layerClosers = append(layerClosers, mount)
	}

	hostPath, err := getScratchVHDPath(layerFolders)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get scratch VHD path in layer folders: %w", err)
	}
	log.G(ctx).WithField("hostPath", hostPath).Debug("mounting scratch VHD")

	scsiMount, err := vm.SCSIManager.AddVirtualDisk(ctx, hostPath, false, vm.ID(), &scsi.MountConfig{})
	if err != nil {
		return "", nil, fmt.Errorf("failed to add SCSI scratch VHD: %w", err)
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

// ToHostHcsSchemaLayers converts the layer paths for Argon into HCS schema V2 layers
func ToHostHcsSchemaLayers(ctx context.Context, containerID string, roLayers []string) ([]hcsschema.Layer, error) {
	if cimlayer.IsCimLayer(roLayers[0]) {
		return cimLayersToHostHcsSchemaLayers(ctx, containerID, roLayers)
	}
	layers := []hcsschema.Layer{}
	for _, layerPath := range roLayers {
		layerID, err := wclayer.LayerID(ctx, layerPath)
		if err != nil {
			return nil, err
		}
		layers = append(layers, hcsschema.Layer{Id: layerID.String(), Path: layerPath})
	}
	return layers, nil
}

// cimLayersToHostHcsSchemaLayers converts given cimfs Argon layers to HCS schema V2 layers.
func cimLayersToHostHcsSchemaLayers(ctx context.Context, containerID string, paths []string) ([]hcsschema.Layer, error) {
	topMostLayer := paths[0]
	cimPath := cimlayer.GetCimPathFromLayer(topMostLayer)
	volume, err := cimlayer.GetCimMountPath(cimPath, containerID)
	if err != nil {
		return nil, err
	}
	// Use the layer path for GUID rather than the mounted volume path, so that the generated layerID
	// remains same everywhere
	layerID, err := wclayer.LayerID(ctx, topMostLayer)
	if err != nil {
		return nil, err
	}
	// Note that when passing the hcsschema formatted layer, "Files" SHOULDN'T be appended to the volume
	// path. The internal code automatically does that.
	return []hcsschema.Layer{{Id: layerID.String(), Path: volume}}, nil

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
