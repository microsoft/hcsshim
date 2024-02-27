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

func MountWCOWLayers(ctx context.Context, containerID string, vm *uvm.UtilityVM, wl WCOWLayers) (_ *MountedWCOWLayers, _ resources.ResourceCloser, err error) {
	switch l := wl.(type) {
	case *wcowWCIFSLayers:
		if vm == nil {
			return mountProcessIsolatedWCIFSLayers(ctx, l)
		}
		return mountHypervIsolatedWCIFSLayers(ctx, l, vm)
	case *wcowForkedCIMLayers:
		if vm == nil {
			return mountProcessIsolatedForkedCimLayers(ctx, containerID, l)
		}
		return nil, nil, fmt.Errorf("hyperv isolated containers aren't supported with forked cim layers")
	default:
		return nil, nil, fmt.Errorf("invalid layer type %T", wl)
	}
}

// Represents a single layer that is mounted and ready to use. Depending on the type of
// layers each individual layer may or may not be mounted. However, HCS still needs paths
// of individual layers and a unique ID for each layer.
type MountedWCOWLayer struct {
	// A unique layer GUID is expected by HCS for every layer
	LayerID string
	// The path at which this layer is mounted. Could be a path on the host or a path
	// inside the guest.
	MountedPath string
}

type MountedWCOWLayers struct {
	// path at which rootfs is setup - this could be a path on the host or a path
	// inside the guest
	RootFS string
	// mounted read-only layer paths are required in the container doc that we send to HCS.
	// In case of WCIFS based layers these would be layer directory paths, however, in case
	// of CimFS layers this would a single volume path at which the CIM is mounted.
	MountedLayerPaths []MountedWCOWLayer
}

// layer closers are used to correctly clean up layers once container exits. Note that
// these layer closers live in the shim process so they can't cleanup the layer in case of
// a shim crash.
//
// wcowHostWCIFSLayerCloser is used to cleanup WCIFS based layers mounted on the host for
// process isolated containers.
type wcowHostWCIFSLayerCloser struct {
	scratchLayerData
}

func (l *wcowHostWCIFSLayerCloser) Release(ctx context.Context) error {
	if err := wclayer.UnprepareLayer(ctx, l.scratchLayerPath); err != nil {
		return err
	}
	return wclayer.DeactivateLayer(ctx, l.scratchLayerPath)
}

func mountProcessIsolatedWCIFSLayers(ctx context.Context, l *wcowWCIFSLayers) (_ *MountedWCOWLayers, _ resources.ResourceCloser, err error) {
	// In some legacy layer use cases the scratch VHD might not be already created by the client
	// continue to support those scenarios.
	if err = ensureScratchVHD(ctx, l.scratchLayerPath, l.layerPaths); err != nil {
		return nil, nil, err
	}

	// Simple retry loop to handle some behavior on RS5. Loopback VHDs used to be mounted in a different manner on RS5 (ws2019) which led to some
	// very odd cases where things would succeed when they shouldn't have, or we'd simply timeout if an operation took too long. Many
	// parallel invocations of this code path and stressing the machine seem to bring out the issues, but all of the possible failure paths
	// that bring about the errors we have observed aren't known.
	//
	// On 19h1+ this *shouldn't* be needed, but the logic is to break if everything succeeded so this is harmless and shouldn't need a version check.
	var lErr error
	for i := 0; i < 5; i++ {
		lErr = func() (err error) {
			if err := wclayer.ActivateLayer(ctx, l.scratchLayerPath); err != nil {
				return err
			}

			defer func() {
				if err != nil {
					_ = wclayer.DeactivateLayer(ctx, l.scratchLayerPath)
				}
			}()

			return wclayer.PrepareLayer(ctx, l.scratchLayerPath, l.layerPaths)
		}()

		if lErr != nil {
			// Common errors seen from the RS5 behavior mentioned above is ERROR_NOT_READY and ERROR_DEVICE_NOT_CONNECTED. The former occurs when HCS
			// tries to grab the volume path of the disk but it doesn't succeed, usually because the disk isn't actually mounted. DEVICE_NOT_CONNECTED
			// has been observed after launching multiple containers in parallel on a machine under high load. This has also been observed to be a trigger
			// for ERROR_NOT_READY as well.
			var hcserr *hcserror.HcsError
			if errors.As(lErr, &hcserr) {
				if errors.Is(hcserr.Err, windows.ERROR_NOT_READY) || errors.Is(hcserr.Err, windows.ERROR_DEVICE_NOT_CONNECTED) {
					log.G(ctx).WithField("path", l.scratchLayerPath).WithError(hcserr.Err).Warning("retrying layer operations after failure")

					// Sleep for a little before a re-attempt. A probable cause for these issues in the first place is events not getting
					// reported in time so might be good to give some time for things to "cool down" or get back to a known state.
					time.Sleep(time.Millisecond * 100)
					continue
				}
			}
			// This was a failure case outside of the commonly known error conditions, don't retry here.
			return nil, nil, lErr
		}

		// No errors in layer setup, we can leave the loop
		break
	}
	// If we got unlucky and ran into one of the two errors mentioned five times in a row and left the loop, we need to check
	// the loop error here and fail also.
	if lErr != nil {
		return nil, nil, errors.Wrap(lErr, "layer retry loop failed")
	}

	// If any of the below fails, we want to detach the filter and unmount the disk.
	defer func() {
		if err != nil {
			_ = wclayer.UnprepareLayer(ctx, l.scratchLayerPath)
			_ = wclayer.DeactivateLayer(ctx, l.scratchLayerPath)
		}
	}()

	mountPath, err := wclayer.GetLayerMountPath(ctx, l.scratchLayerPath)
	if err != nil {
		return nil, nil, err
	}

	layersWithID := []MountedWCOWLayer{}
	for _, l := range l.layerPaths {
		layerID, err := wclayer.LayerID(ctx, l)
		if err != nil {
			return nil, nil, err
		}
		layersWithID = append(layersWithID, MountedWCOWLayer{
			LayerID:     layerID.String(),
			MountedPath: l,
		})
	}

	return &MountedWCOWLayers{
			RootFS:            mountPath,
			MountedLayerPaths: layersWithID,
		}, &wcowHostWCIFSLayerCloser{
			scratchLayerData: l.scratchLayerData,
		}, nil
}

// wcowHostForkedCIMLayerCloser is used to cleanup forked CIM layers mounted on the host for process isolated
// containers
type wcowHostForkedCIMLayerCloser struct {
	scratchLayerData
	containerID string
}

func (l *wcowHostForkedCIMLayerCloser) Release(ctx context.Context) error {
	mountPath, err := wclayer.GetLayerMountPath(ctx, l.scratchLayerPath)
	if err != nil {
		return err
	}

	if err = computestorage.DetachOverlayFilter(ctx, mountPath, hcsschema.UnionFS); err != nil {
		return err
	}

	if err = cimlayer.CleanupContainerMounts(l.containerID); err != nil {
		return err
	}
	return wclayer.DeactivateLayer(ctx, l.scratchLayerPath)
}

func mountProcessIsolatedForkedCimLayers(ctx context.Context, containerID string, l *wcowForkedCIMLayers) (_ *MountedWCOWLayers, _ resources.ResourceCloser, err error) {
	if err = wclayer.ActivateLayer(ctx, l.scratchLayerPath); err != nil {
		return nil, nil, err
	}
	defer func() {
		if err != nil {
			_ = wclayer.DeactivateLayer(ctx, l.scratchLayerPath)
		}
	}()

	mountPath, err := wclayer.GetLayerMountPath(ctx, l.scratchLayerPath)
	if err != nil {
		return nil, nil, err
	}

	volume, err := cimlayer.MountCimLayer(ctx, l.layers[0].cimPath, containerID)
	if err != nil {
		return nil, nil, fmt.Errorf("mount layer cim: %w", err)
	}
	defer func() {
		if err != nil {
			_ = cimlayer.UnmountCimLayer(ctx, l.layers[0].cimPath, containerID)
		}
	}()

	// Use the layer path for GUID rather than the mounted volume path, so that the generated layerID
	// remains same.
	layerID, err := cimlayer.LayerID(l.layers[0].cimPath, containerID)
	if err != nil {
		return nil, nil, err
	}

	layerData := computestorage.LayerData{
		FilterType: hcsschema.UnionFS,
		// Container filesystem contents are under a directory named "Files" inside the mounted cim.
		// UnionFS needs this path, so append "Files" to the layer path before passing it on.
		Layers: []hcsschema.Layer{
			{
				Id:   layerID,
				Path: filepath.Join(volume, "Files"),
			},
		},
	}

	if err = computestorage.AttachOverlayFilter(ctx, mountPath, layerData); err != nil {
		return nil, nil, err
	}
	defer func() {
		if err != nil {
			_ = computestorage.DetachOverlayFilter(ctx, mountPath, hcsschema.UnionFS)
		}
	}()

	return &MountedWCOWLayers{
			RootFS: mountPath,
			MountedLayerPaths: []MountedWCOWLayer{{
				LayerID:     layerID,
				MountedPath: volume,
			}},
		}, &wcowHostForkedCIMLayerCloser{
			containerID:      containerID,
			scratchLayerData: l.scratchLayerData,
		}, nil
}

type wcowIsolatedWCIFSLayerCloser struct {
	uvm                     *uvm.UtilityVM
	guestCombinedLayersPath string
	scratchMount            resources.ResourceCloser
	layerClosers            []resources.ResourceCloser
}

func (lc *wcowIsolatedWCIFSLayerCloser) Release(ctx context.Context) (retErr error) {
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

func mountHypervIsolatedWCIFSLayers(ctx context.Context, l *wcowWCIFSLayers, vm *uvm.UtilityVM) (_ *MountedWCOWLayers, _ resources.ResourceCloser, err error) {
	log.G(ctx).WithField("os", vm.OS()).Debug("hcsshim::MountWCOWLayers V2 UVM")

	// In some legacy layer use cases the scratch VHD might not be already created by the client
	// continue to support those scenarios.
	if err = ensureScratchVHD(ctx, l.scratchLayerPath, l.layerPaths); err != nil {
		return nil, nil, err
	}

	var (
		layersAdded  []*uvm.VSMBShare
		layerClosers []resources.ResourceCloser
	)
	defer func() {
		if err != nil {
			for _, l := range layersAdded {
				if err := l.Release(ctx); err != nil {
					log.G(ctx).WithError(err).Warn("failed to remove wcow layer on cleanup")
				}
			}
		}
	}()

	for _, layerPath := range l.layerPaths {
		log.G(ctx).WithField("layerPath", layerPath).Debug("mounting layer")
		options := vm.DefaultVSMBOptions(true)
		options.TakeBackupPrivilege = true
		mount, err := vm.AddVSMB(ctx, layerPath, options)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to add VSMB layer: %w", err)
		}
		layersAdded = append(layersAdded, mount)
		layerClosers = append(layerClosers, mount)
	}

	hostPath := filepath.Join(l.scratchLayerPath, "sandbox.vhdx")
	log.G(ctx).WithField("hostPath", hostPath).Debug("mounting scratch VHD")

	scsiMount, err := vm.SCSIManager.AddVirtualDisk(ctx, hostPath, false, vm.ID(), &scsi.MountConfig{})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to add SCSI scratch VHD: %w", err)
	}
	containerScratchPathInUVM := scsiMount.GuestPath()

	defer func() {
		if err != nil {
			if err := scsiMount.Release(ctx); err != nil {
				log.G(ctx).WithError(err).Warn("failed to remove scratch on cleanup")
			}
		}
	}()

	ml := &MountedWCOWLayers{
		RootFS: containerScratchPathInUVM,
	}
	// Windows GCS needs the layers in the HCS format. Convert to that format before
	// sending to GCS
	hcsLayers := []hcsschema.Layer{}
	for _, a := range layersAdded {
		uvmPath, err := vm.GetVSMBUvmPath(ctx, a.HostPath, true)
		if err != nil {
			return nil, nil, err
		}
		layerID, err := wclayer.LayerID(ctx, a.HostPath)
		if err != nil {
			return nil, nil, err
		}
		ml.MountedLayerPaths = append(ml.MountedLayerPaths, MountedWCOWLayer{
			LayerID:     layerID.String(),
			MountedPath: uvmPath,
		})
		hcsLayers = append(hcsLayers, hcsschema.Layer{
			Id:   layerID.String(),
			Path: uvmPath,
		})
	}

	err = vm.CombineLayersWCOW(ctx, hcsLayers, ml.RootFS)
	if err != nil {
		return nil, nil, err
	}
	log.G(ctx).Debug("hcsshim::MountWCOWLayers Succeeded")

	return ml, &wcowIsolatedWCIFSLayerCloser{
		uvm:                     vm,
		guestCombinedLayersPath: ml.RootFS,
		scratchMount:            scsiMount,
		layerClosers:            layerClosers,
	}, nil
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
