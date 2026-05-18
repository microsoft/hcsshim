//go:build windows && lcow

package linuxcontainer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	plan9Mount "github.com/Microsoft/hcsshim/internal/controller/device/plan9/mount"
	"github.com/Microsoft/hcsshim/internal/controller/device/plan9/share"
	"github.com/Microsoft/hcsshim/internal/controller/device/scsi/disk"
	scsiMount "github.com/Microsoft/hcsshim/internal/controller/device/scsi/mount"
	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/opencontainers/runtime-spec/specs-go"
)

// Mount type constants.
const (
	// mountTypeBind is a regular host-directory bind mount served via a Plan9 share.
	mountTypeBind = "bind"

	// mountTypePhysicalDisk hot-adds a physical pass-through disk via the SCSI controller.
	mountTypePhysicalDisk = "physical-disk"

	// mountTypeVirtualDisk hot-adds a VHD or VHDX via the SCSI controller.
	mountTypeVirtualDisk = "virtual-disk"

	// mountTypeExtensibleVirtualDisk hot-adds an extensible virtual disk via the SCSI controller.
	mountTypeExtensibleVirtualDisk = "extensible-virtual-disk"

	// mountTypeNone signals that the mount is a disk-backed device mount whose
	// filesystem will be resolved when the guest actually mounts the device.
	mountTypeNone = "none"
)

// allocateMounts reserves and maps host-side resources for each OCI mount,
// rewriting mount sources in the spec to their guest-visible paths.
func (c *Controller) allocateMounts(ctx context.Context, spec *specs.Spec) error {
	for idx := range spec.Mounts {
		mount := &spec.Mounts[idx]

		if mount.Destination == "" || mount.Source == "" {
			return fmt.Errorf("invalid mount: both source and destination are required: %+v", mount)
		}

		// Check if the mount is read-only.
		isReadOnly := isReadOnlyMount(mount)

		// Dispatch to a mount-type-specific handler.
		switch mount.Type {
		case mountTypeVirtualDisk, mountTypePhysicalDisk, mountTypeExtensibleVirtualDisk:
			if err := c.allocateSCSIMount(ctx, mount, isReadOnly); err != nil {
				return err
			}
		case mountTypeBind:
			// Hugepages mounts are backed by a pre-existing mount inside the UVM.
			if strings.HasPrefix(mount.Source, guestpath.HugePagesMountPrefix) {
				if err := validateHugePageMount(mount.Source); err != nil {
					return err
				}
				continue
			}

			// Guest-internal paths resolve entirely inside the UVM.
			if isGuestInternalPath(mount.Source) {
				continue
			}

			// All remaining bind mounts are host directories served via Plan9.
			// Allocate them.
			if err := c.allocatePlan9Mount(ctx, mount, isReadOnly); err != nil {
				return err
			}
		default:
			// Unknown mount types (e.g. tmpfs, devpts, proc) are passed through
			// to the guest without host-side resource reservation/allocation.
		}
	}

	log.G(ctx).Debug("all OCI mounts allocated successfully")
	return nil
}

// allocateSCSIMount resolves the host path, grants VM access, and reserves+maps
// a SCSI slot for any disk-backed mount type.
func (c *Controller) allocateSCSIMount(ctx context.Context, mount *specs.Mount, isReadOnly bool) error {
	// Build disk config based on mount type.
	var diskConfig disk.Config
	switch mount.Type {
	case mountTypeVirtualDisk, mountTypePhysicalDisk:
		// Resolve any symlinks to get the real host path for the disk.
		hostPath, err := resolvePath(mount.Source)
		if err != nil {
			return fmt.Errorf("resolve symlinks for mount source %s: %w", mount.Source, err)
		}

		// The VM needs explicit access to the disk before it can be attached.
		if err = grantVMAccess(ctx, c.vmID, hostPath); err != nil {
			return fmt.Errorf("grant vm access to %s: %w", hostPath, err)
		}

		// Physical disks use pass-through; everything else is a virtual disk.
		diskType := disk.TypeVirtualDisk
		if mount.Type == mountTypePhysicalDisk {
			diskType = disk.TypePassThru
		}

		// Create the final disk config.
		diskConfig = disk.Config{HostPath: hostPath, ReadOnly: isReadOnly, Type: diskType}

	case mountTypeExtensibleVirtualDisk:
		// EVD paths encode the provider type in the source URI.
		evdType, sourcePath, err := parseExtensibleVirtualDiskPath(mount.Source)
		if err != nil {
			return fmt.Errorf("parse extensible virtual disk path: %w", err)
		}

		// Resolve any symlinks to get the real host path for the disk.
		hostPath, err := resolvePath(sourcePath)
		if err != nil {
			return fmt.Errorf("resolve symlinks for mount source %s: %w", sourcePath, err)
		}

		// Create the final disk config.
		diskConfig = disk.Config{HostPath: hostPath, ReadOnly: isReadOnly, Type: disk.TypeExtensibleVirtualDisk, EVDType: evdType}

	default:
		return fmt.Errorf("unsupported scsi mount type %q", mount.Type)
	}

	// Check if this is a block dev mount.
	isBlockDev := strings.HasPrefix(mount.Destination, guestpath.BlockDevMountPrefix)

	// Reserve the mount.
	reservationID, err := c.scsi.Reserve(ctx, diskConfig, scsiMount.Config{
		ReadOnly: isReadOnly,
		Options:  mount.Options,
		BlockDev: isBlockDev,
	})
	if err != nil {
		return fmt.Errorf("reserve scsi mount for %s: %w", mount.Source, err)
	}

	// Store the reservation so that we can unwind in case of errors.
	c.scsiResources = append(c.scsiResources, reservationID)

	// Map the device into the guest.
	guestPath, err := c.scsi.MapToGuest(ctx, reservationID)
	if err != nil {
		return fmt.Errorf("map scsi mount %s to guest: %w", mount.Source, err)
	}

	// Rewrite source to guest path; block-device mounts retain bind type.
	mount.Source = guestPath
	mount.Type = mountTypeNone
	if isBlockDev {
		mount.Type = mountTypeBind
	}

	return nil
}

// allocatePlan9Mount reserves and maps a Plan9 share for a host-backed bind mount.
func (c *Controller) allocatePlan9Mount(ctx context.Context, mount *specs.Mount, isReadOnly bool) error {
	// Ensure that mount source exists.
	fileInfo, err := os.Stat(mount.Source)
	if err != nil {
		return fmt.Errorf("stat bind mount source %s: %w", mount.Source, err)
	}

	shareConfig := share.Config{
		HostPath: mount.Source,
		ReadOnly: isReadOnly,
	}

	// For single-file mounts, share the containing directory but restrict
	// access to the specific file.
	if !fileInfo.IsDir() {
		hostDir, fileName := filepath.Split(mount.Source)
		shareConfig.HostPath = hostDir
		shareConfig.Restrict = true
		shareConfig.AllowedNames = []string{fileName}
	}

	// Reserve the plan9 share.
	reservationID, err := c.plan9.Reserve(ctx, shareConfig, plan9Mount.Config{ReadOnly: isReadOnly})
	if err != nil {
		return fmt.Errorf("reserve plan9 share for %s: %w", mount.Source, err)
	}

	// Store the reservation so that we can unwind in case of errors.
	c.plan9Resources = append(c.plan9Resources, reservationID)

	// Map the share into the guest.
	guestPath, err := c.plan9.MapToGuest(ctx, reservationID)
	if err != nil {
		return fmt.Errorf("map plan9 share %s to guest: %w", mount.Source, err)
	}

	mount.Source = guestPath
	return nil
}

// --- Helpers ---

// parseExtensibleVirtualDiskPath extracts the EVD type and source path from
// a path with the format "evd://<type>/<path>".
func parseExtensibleVirtualDiskPath(hostPath string) (evdType, sourcePath string, err error) {
	const evdPrefix = "evd://"

	if !strings.HasPrefix(hostPath, evdPrefix) {
		return "", "", fmt.Errorf("invalid extensible virtual disk path %q: missing %q prefix", hostPath, evdPrefix)
	}

	trimmed := strings.TrimPrefix(hostPath, evdPrefix)
	idx := strings.Index(trimmed, "/")
	if idx <= 0 {
		return "", "", fmt.Errorf("invalid extensible virtual disk path %q: expected format %s<type>/<path>", hostPath, evdPrefix)
	}

	return trimmed[:idx], trimmed[idx+1:], nil
}

// validateHugePageMount checks that a hugepages mount source has the expected format.
func validateHugePageMount(source string) error {
	// Expected format: "hugepages://<size>/<location>"
	parts := strings.Split(strings.TrimPrefix(source, guestpath.HugePagesMountPrefix), "/")
	if len(parts) < 2 {
		return fmt.Errorf("invalid hugepages mount path %s: expected format %s<size>/<location>", source, guestpath.HugePagesMountPrefix)
	}
	// Only 2M (megabyte) hugepages are currently supported.
	if parts[0] != "2M" {
		return fmt.Errorf("unsupported hugepage size %s: only 2M is supported", parts[0])
	}
	return nil
}

// isReadOnlyMount returns true if the mount options contain the "ro" flag.
func isReadOnlyMount(mount *specs.Mount) bool {
	for _, option := range mount.Options {
		if strings.EqualFold(option, "ro") {
			return true
		}
	}
	return false
}

// isGuestInternalPath reports whether the path uses a UVM-internal prefix
// that resolves inside the guest.
func isGuestInternalPath(path string) bool {
	// Mounts that map to a path in UVM are specified with a 'sandbox://', 'sandbox-tmp://', or 'uvm://' prefix.
	// examples:
	//  - sandbox:///a/dirInUvm destination:/b/dirInContainer
	//  - sandbox-tmp:///a/dirInUvm destination:/b/dirInContainer
	//  - uvm:///a/dirInUvm destination:/b/dirInContainer
	return strings.HasPrefix(path, guestpath.SandboxMountPrefix) ||
		strings.HasPrefix(path, guestpath.SandboxTmpfsMountPrefix) ||
		strings.HasPrefix(path, guestpath.UVMMountPrefix)
}
