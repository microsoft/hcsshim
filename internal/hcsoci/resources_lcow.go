//go:build windows
// +build windows

package hcsoci

// Contains functions relating to a LCOW container, as opposed to a utility VM

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"

	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/Microsoft/hcsshim/internal/layers"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/resources"
	"github.com/Microsoft/hcsshim/internal/uvm/scsi"
)

func allocateLinuxResources(ctx context.Context, coi *createOptionsInternal, r *resources.Resources, isSandbox bool) error {
	if coi.Spec.Root == nil {
		coi.Spec.Root = &specs.Root{}
	}
	containerRootInUVM := r.ContainerRootInUVM()
	if coi.LCOWLayers != nil {
		log.G(ctx).Debug("hcsshim::allocateLinuxResources mounting storage")
		rootPath, scratchPath, closer, err := layers.MountLCOWLayers(ctx, coi.actualID, coi.LCOWLayers, containerRootInUVM, coi.HostingSystem)
		if err != nil {
			return errors.Wrap(err, "failed to mount container storage")
		}
		coi.Spec.Root.Path = rootPath
		// If this is the pause container in a hypervisor-isolated pod, we can skip cleanup of
		// layers, as that happens automatically when the UVM is terminated.
		if !isSandbox || coi.HostingSystem == nil {
			r.SetLayers(closer)
		}
		r.SetLcowScratchPath(scratchPath)
	} else if coi.Spec.Root.Path != "" {
		// This is the "Plan 9" root filesystem.
		// TODO: We need a test for this. Ask @jstarks how you can even lay this out on Windows.
		hostPath := coi.Spec.Root.Path
		uvmPathForContainersFileSystem := path.Join(r.ContainerRootInUVM(), guestpath.RootfsPath)
		share, err := coi.HostingSystem.AddPlan9(ctx, hostPath, uvmPathForContainersFileSystem, coi.Spec.Root.Readonly, false, nil)
		if err != nil {
			return errors.Wrap(err, "adding plan9 root")
		}
		coi.Spec.Root.Path = uvmPathForContainersFileSystem
		r.Add(share)
	} else {
		return errors.New("must provide either Windows.LayerFolders or Root.Path")
	}

	for i, mount := range coi.Spec.Mounts {
		switch mount.Type {
		case "bind":
		case "physical-disk":
		case "virtual-disk":
		default:
			// Unknown mount type
			continue
		}
		if mount.Destination == "" || mount.Source == "" {
			return fmt.Errorf("invalid OCI spec - a mount must have both source and a destination: %+v", mount)
		}

		if coi.HostingSystem != nil {
			hostPath := mount.Source
			uvmPathForShare := path.Join(containerRootInUVM, fmt.Sprintf(guestpath.LCOWMountPathPrefixFmt, i))
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
				scsiMount, err := coi.HostingSystem.SCSIManager.AddPhysicalDisk(
					ctx,
					hostPath,
					readOnly,
					coi.HostingSystem.ID(),
					&scsi.MountConfig{Options: mount.Options},
				)
				if err != nil {
					return errors.Wrapf(err, "adding SCSI physical disk mount %+v", mount)
				}

				uvmPathForFile = scsiMount.GuestPath()
				r.Add(scsiMount)
				coi.Spec.Mounts[i].Type = "none"
			} else if mount.Type == "virtual-disk" {
				l.Debug("hcsshim::allocateLinuxResources Hot-adding SCSI virtual disk for OCI mount")

				// if the scsi device is already attached then we take the uvm path that the function below returns
				// that is where it was previously mounted in UVM
				scsiMount, err := coi.HostingSystem.SCSIManager.AddVirtualDisk(
					ctx,
					hostPath,
					readOnly,
					coi.HostingSystem.ID(),
					&scsi.MountConfig{Options: mount.Options},
				)
				if err != nil {
					return errors.Wrapf(err, "adding SCSI virtual disk mount %+v", mount)
				}

				uvmPathForFile = scsiMount.GuestPath()
				r.Add(scsiMount)
				coi.Spec.Mounts[i].Type = "none"
			} else if strings.HasPrefix(mount.Source, guestpath.SandboxMountPrefix) {
				// Mounts that map to a path in UVM are specified with 'sandbox://' prefix.
				// example: sandbox:///a/dirInUvm destination:/b/dirInContainer
				uvmPathForFile = mount.Source
			} else if strings.HasPrefix(mount.Source, guestpath.HugePagesMountPrefix) {
				// currently we only support 2M hugepage size
				hugePageSubDirs := strings.Split(strings.TrimPrefix(mount.Source, guestpath.HugePagesMountPrefix), "/")
				if len(hugePageSubDirs) < 2 {
					return errors.Errorf(
						`%s mount path is invalid, expected format: %s<hugepage-size>/<hugepage-src-location>`,
						mount.Source,
						guestpath.HugePagesMountPrefix,
					)
				}

				// hugepages:// should be followed by pagesize
				if hugePageSubDirs[0] != "2M" {
					return errors.Errorf(`only 2M (megabytes) pagesize is supported, got %s`, hugePageSubDirs[0])
				}
				// Hugepages inside a container are backed by a mount created inside a UVM.
				uvmPathForFile = mount.Source
			} else {
				st, err := os.Stat(hostPath)
				if err != nil {
					return errors.Wrap(err, "could not open bind mount target")
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
					return errors.Wrapf(err, "adding plan9 mount %+v", mount)
				}
				r.Add(share)
			}
			coi.Spec.Mounts[i].Source = uvmPathForFile
		}
	}

	if coi.HostingSystem == nil {
		return nil
	}

	if coi.hasWindowsAssignedDevices() {
		windowsDevices, closers, err := handleAssignedDevicesLCOW(ctx, coi.HostingSystem, coi.Spec.Annotations, coi.Spec.Windows.Devices)
		if err != nil {
			return err
		}
		r.Add(closers...)
		coi.Spec.Windows.Devices = windowsDevices
	}
	return nil
}
