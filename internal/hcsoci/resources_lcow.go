// +build windows

package hcsoci

// Contains functions relating to a LCOW container, as opposed to a utility VM

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Microsoft/hcsshim/internal/guestrequest"
	"github.com/Microsoft/hcsshim/internal/log"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

const rootfsPath = "rootfs"
const mountPathPrefix = "m"

func allocateLinuxResources(ctx context.Context, coi *createOptionsInternal, resources *Resources) error {
	if coi.Spec.Root == nil {
		coi.Spec.Root = &specs.Root{}
	}
	if coi.Spec.Windows != nil && len(coi.Spec.Windows.LayerFolders) > 0 {
		log.G(ctx).Debug("hcsshim::allocateLinuxResources mounting storage")
		mcl, err := MountContainerLayers(ctx, coi.Spec.Windows.LayerFolders, resources.containerRootInUVM, coi.HostingSystem)
		if err != nil {
			return fmt.Errorf("failed to mount container storage: %s", err)
		}
		if coi.HostingSystem == nil {
			coi.Spec.Root.Path = mcl.(string) // Argon v1 or v2
		} else {
			coi.Spec.Root.Path = mcl.(guestrequest.CombinedLayers).ContainerRootPath // v2 Xenon LCOW
		}
		resources.layers = coi.Spec.Windows.LayerFolders
	} else if coi.Spec.Root.Path != "" {
		// This is the "Plan 9" root filesystem.
		// TODO: We need a test for this. Ask @jstarks how you can even lay this out on Windows.
		hostPath := coi.Spec.Root.Path
		uvmPathForContainersFileSystem := path.Join(resources.containerRootInUVM, rootfsPath)
		share, err := coi.HostingSystem.AddPlan9(ctx, hostPath, uvmPathForContainersFileSystem, coi.Spec.Root.Readonly, false, nil)
		if err != nil {
			return fmt.Errorf("adding plan9 root: %s", err)
		}
		coi.Spec.Root.Path = uvmPathForContainersFileSystem
		resources.plan9Mounts = append(resources.plan9Mounts, share)
	} else {
		return errors.New("must provide either Windows.LayerFolders or Root.Path")
	}

	for i, mount := range coi.Spec.Mounts {
		switch mount.Type {
		case "bind":
		case "physical-disk":
		case "virtual-disk":
		case "automanage-virtual-disk":
		default:
			// Unknown mount type
			continue
		}
		if mount.Destination == "" || mount.Source == "" {
			return fmt.Errorf("invalid OCI spec - a mount must have both source and a destination: %+v", mount)
		}

		if coi.HostingSystem != nil {
			hostPath := mount.Source
			uvmPathForShare := path.Join(resources.containerRootInUVM, mountPathPrefix+strconv.Itoa(i))
			uvmPathForFile := uvmPathForShare

			readOnly := false
			for _, o := range mount.Options {
				if strings.ToLower(o) == "ro" {
					readOnly = true
					break
				}
			}
			l := log.G(ctx).WithField("mount", fmt.Sprintf("%+v", mount))
			if mount.Type == "physical-disk" {
				l.Debug("hcsshim::allocateLinuxResources Hot-adding SCSI physical disk for OCI mount")
				_, _, err := coi.HostingSystem.AddSCSIPhysicalDisk(ctx, hostPath, uvmPathForShare, readOnly)
				if err != nil {
					return fmt.Errorf("adding SCSI physical disk mount %+v: %s", mount, err)
				}
				resources.scsiMounts = append(resources.scsiMounts, scsiMount{path: hostPath})
				coi.Spec.Mounts[i].Type = "none"
			} else if mount.Type == "virtual-disk" || mount.Type == "automanage-virtual-disk" {
				l.Debug("hcsshim::allocateLinuxResources Hot-adding SCSI virtual disk for OCI mount")
				_, _, err := coi.HostingSystem.AddSCSI(ctx, hostPath, uvmPathForShare, readOnly)
				if err != nil {
					return fmt.Errorf("adding SCSI virtual disk mount %+v: %s", mount, err)
				}
				resources.scsiMounts = append(resources.scsiMounts, scsiMount{path: hostPath, autoManage: mount.Type == "automanage-virtual-disk"})
				coi.Spec.Mounts[i].Type = "none"
			} else if strings.HasPrefix(mount.Source, "sandbox://") {
				// Mounts that map to a path in UVM are specified with 'sandbox://' prefix.
				// we remove the prefix before passing them to GCS. Mount format looks like below
				// source: sandbox:///a/dirInUvm destination:/b/dirInContainer
				uvmPathForFile = strings.TrimPrefix(mount.Source, "sandbox://")
			} else {
				st, err := os.Stat(hostPath)
				if err != nil {
					return fmt.Errorf("could not open bind mount target: %s", err)
				}
				restrictAccess := false
				var allowedNames []string
				if !st.IsDir() {
					// Map the containing directory in, but restrict the share to a single
					// file.
					var fileName string
					hostPath, fileName = filepath.Split(hostPath)
					allowedNames = append(allowedNames, fileName)
					restrictAccess = true
					uvmPathForFile = path.Join(uvmPathForShare, fileName)
				}
				l.Debug("hcsshim::allocateLinuxResources Hot-adding Plan9 for OCI mount")
				share, err := coi.HostingSystem.AddPlan9(ctx, hostPath, uvmPathForShare, readOnly, restrictAccess, allowedNames)
				if err != nil {
					return fmt.Errorf("adding plan9 mount %+v: %s", mount, err)
				}
				resources.plan9Mounts = append(resources.plan9Mounts, share)
			}
			coi.Spec.Mounts[i].Source = uvmPathForFile
		}
	}

	return nil
}
