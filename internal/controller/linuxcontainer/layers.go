//go:build windows && lcow

package linuxcontainer

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/controller/device/scsi/disk"
	scsiMount "github.com/Microsoft/hcsshim/internal/controller/device/scsi/mount"
	"github.com/Microsoft/hcsshim/internal/guestpath"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/layers"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/ospath"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/wclayer"

	"github.com/Microsoft/go-winio/pkg/fs"
	"github.com/Microsoft/go-winio/pkg/guid"
	containerdtypes "github.com/containerd/containerd/api/types"
)

// scsiReservation pairs a SCSI reservation ID with its resolved guest path.
type scsiReservation struct {
	id        guid.GUID
	guestPath string
}

// scsiLayers holds the SCSI reservations for all container layers.
type scsiLayers struct {
	roLayers       []scsiReservation
	scratch        scsiReservation
	layersCombined bool
	rootfsPath     string
}

// grantVMAccess and resolvePath are converted to vars for unit testing.
var (
	grantVMAccess = wclayer.GrantVmAccess
	resolvePath   = fs.ResolvePath
)

// allocateLayers parses, reserves, and maps all LCOW layers into the guest.
func (c *Controller) allocateLayers(
	ctx context.Context,
	layerFolders []string,
	rootfs []*containerdtypes.Mount,
	isScratchEncryptionEnabled bool,
) error {
	log.G(ctx).Debug("allocating container layers")

	// Parse the rootfs mounts and layer folders into the canonical LCOW layer format.
	// todo: containerd snapshotter only assigns containerdtypes.Mount and should not support layerFolders.
	lcowLayers, err := layers.ParseLCOWLayers(rootfs, layerFolders)
	if err != nil {
		return fmt.Errorf("parse lcow layers: %w", err)
	}

	c.layers = &scsiLayers{}

	// Reserve and map each read-only layer.
	for _, roLayer := range lcowLayers.Layers {
		// Read-only layers come from the containerd snapshotter with broad read
		// permissions (typically via GrantVmGroupAccess), so no per-VM access
		// grant is needed here.

		// The layer path may be a symlink; resolve it to a real path before
		// handing it to the SCSI reservation.
		hostPath, err := resolvePath(roLayer.VHDPath)
		if err != nil {
			return fmt.Errorf("resolve symlinks for layer %s: %w", roLayer.VHDPath, err)
		}

		reservationID, err := c.scsi.Reserve(
			ctx,
			disk.Config{HostPath: hostPath, ReadOnly: true, Type: disk.TypeVirtualDisk},
			scsiMount.Config{Partition: roLayer.Partition, ReadOnly: true, Options: []string{"ro"}},
		)
		if err != nil {
			return fmt.Errorf("reserve scsi slot for layer %s: %w", roLayer.VHDPath, err)
		}

		// Store the reservation so that we can unwind in case of errors.
		c.layers.roLayers = append(c.layers.roLayers, scsiReservation{id: reservationID})

		guestPath, err := c.scsi.MapToGuest(ctx, reservationID)
		if err != nil {
			return fmt.Errorf("map layer %s to guest: %w", roLayer.VHDPath, err)
		}

		// Set the guest path on the stored reservation.
		c.layers.roLayers[len(c.layers.roLayers)-1].guestPath = guestPath
	}

	// Reserve and map the writable scratch layer.

	// The scratch path may be a symlink to a shared sandbox.vhdx from another
	// container (e.g. the sandbox container). Resolve it before granting access.
	scratchHostPath, err := resolvePath(lcowLayers.ScratchVHDPath)
	if err != nil {
		return fmt.Errorf("resolve symlinks for scratch %s: %w", lcowLayers.ScratchVHDPath, err)
	}

	// Unlike read-only layers, the scratch VHD requires explicit per-VM access.
	if err = grantVMAccess(ctx, c.vmID, scratchHostPath); err != nil {
		return fmt.Errorf("grant vm access to scratch %s: %w", scratchHostPath, err)
	}

	// Encrypted scratch disks use xfs; all others default to ext4.
	fileSystem := "ext4"
	if isScratchEncryptionEnabled {
		fileSystem = "xfs"
	}

	scratchID, err := c.scsi.Reserve(ctx,
		disk.Config{HostPath: scratchHostPath, ReadOnly: false, Type: disk.TypeVirtualDisk},
		scsiMount.Config{
			Encrypted:        isScratchEncryptionEnabled,
			EnsureFilesystem: true,
			ReadOnly:         false,
			Filesystem:       fileSystem,
		},
	)
	if err != nil {
		return fmt.Errorf("reserve scsi slot for scratch %s: %w", lcowLayers.ScratchVHDPath, err)
	}

	// Store the reservation so that we can unwind in case of errors.
	c.layers.scratch = scsiReservation{id: scratchID}

	scratchMountPath, err := c.scsi.MapToGuest(ctx, scratchID)
	if err != nil {
		return fmt.Errorf("map scratch to guest: %w", err)
	}

	// When sharing a scratch disk across multiple containers, derive a unique
	// sub-path per container to prevent upper/work directory collisions.
	c.layers.scratch.guestPath = ospath.Join("linux", scratchMountPath, "scratch", c.gcsPodID, c.gcsContainerID)
	c.layers.rootfsPath = ospath.Join("linux", guestpath.LCOWV2RootPrefixInVM, c.gcsPodID, c.gcsContainerID, guestpath.RootfsPath)

	// Combine the mapped layers as the final step.
	hcsLayers := make([]hcsschema.Layer, len(c.layers.roLayers))
	for i, roLayer := range c.layers.roLayers {
		hcsLayers[i] = hcsschema.Layer{Path: roLayer.guestPath}
	}

	if err = c.guest.AddCombinedLayers(ctx, guestresource.LCOWCombinedLayers{
		ContainerID:       c.gcsContainerID,
		ContainerRootPath: c.layers.rootfsPath,
		Layers:            hcsLayers,
		ScratchPath:       c.layers.scratch.guestPath,
	}); err != nil {
		return fmt.Errorf("add combined layers: %w", err)
	}

	// Set the layersCombined flag so that we can uncombine them during teardown.
	c.layers.layersCombined = true

	log.G(ctx).WithField("layers", log.Format(ctx, c.layers)).Trace("all LCOW layers reserved and mapped")
	return nil
}
