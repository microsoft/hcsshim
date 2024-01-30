//go:build windows
// +build windows

package layers

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/ospath"
	"github.com/Microsoft/hcsshim/internal/resources"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/internal/uvm/scsi"
)

type LCOWLayer struct {
	VHDPath   string
	Partition uint64
}

// LCOWLayers defines a set of LCOW layers.
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
		if retErr == nil { //nolint:govet // nilness: consistency with below
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
			return "", "", nil, fmt.Errorf("failed to add LCOW layer: %w", err)
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

	mConfig := &scsi.MountConfig{
		Encrypted: vm.ScratchEncryptionEnabled(),
		// For scratch disks, we support formatting the disk if it is not already
		// formatted.
		EnsureFilesystem: true,
		Filesystem:       "ext4",
	}
	if vm.ScratchEncryptionEnabled() {
		// Encrypted scratch devices are formatted with xfs
		mConfig.Filesystem = "xfs"
	}
	scsiMount, err := vm.SCSIManager.AddVirtualDisk(
		ctx,
		hostPath,
		false,
		vm.ID(),
		mConfig,
	)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to add SCSI scratch VHD: %w", err)
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
		} else if !errors.Is(err, uvm.ErrNoAvailableLocation) && !errors.Is(err, uvm.ErrMaxVPMemLayerSize) {
			return "", nil, fmt.Errorf("failed to add VPMEM layer: %w", err)
		}
	}

	sm, err := vm.SCSIManager.AddVirtualDisk(
		ctx,
		layer.VHDPath,
		true,
		"",
		&scsi.MountConfig{
			Partition: layer.Partition,
			Options:   []string{"ro"},
		},
	)
	if err != nil {
		return "", nil, fmt.Errorf("failed to add SCSI layer: %w", err)
	}
	log.G(ctx).WithFields(logrus.Fields{
		"layerPath":      layer.VHDPath,
		"layerPartition": layer.Partition,
		"layerType":      "scsi",
	}).Debug("Added LCOW layer")
	return sm.GuestPath(), sm, nil
}
