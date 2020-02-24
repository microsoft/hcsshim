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

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oci"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

const lcowMountPathPrefix = "/mounts/m%d"
const lcowGlobalMountPrefix = "/run/mounts/m%d"

// keep lcowNvidiaMountPath value in sync with opengcs
const lcowNvidiaMountPath = "/run/nvidia"

// getGPUVHDPath gets the gpu vhd path from the shim options or uses the default if no
// shim option is set. Right now we only support Nvidia gpus, so this will default to
// a gpu vhd with nvidia files
func getGPUVHDPath(coi *createOptionsInternal) (string, error) {
	gpuVHDPath, ok := coi.Spec.Annotations[oci.AnnotationGPUVHDPath]
	if !ok || gpuVHDPath == "" {
		return "", fmt.Errorf("no gpu vhd specified %s", gpuVHDPath)
	}
	if _, err := os.Stat(gpuVHDPath); err != nil {
		return "", errors.Wrapf(err, "failed to find gpu support vhd %s", gpuVHDPath)
	}
	return gpuVHDPath, nil
}

func allocateLinuxResources(ctx context.Context, coi *createOptionsInternal, resources *Resources) error {
	if coi.Spec.Root == nil {
		coi.Spec.Root = &specs.Root{}
	}
	if coi.Spec.Windows != nil && len(coi.Spec.Windows.LayerFolders) > 0 {
		log.G(ctx).Debug("hcsshim::allocateLinuxResources mounting storage")
		rootPath, err := MountContainerLayers(ctx, coi.Spec.Windows.LayerFolders, resources.containerRootInUVM, coi.HostingSystem)
		if err != nil {
			return fmt.Errorf("failed to mount container storage: %s", err)
		}
		if coi.HostingSystem == nil {
			coi.Spec.Root.Path = rootPath // Argon v1 or v2
		} else {
			coi.Spec.Root.Path = rootPath // v2 Xenon LCOW
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
			uvmPathForShare := path.Join(resources.containerRootInUVM, fmt.Sprintf(lcowMountPathPrefix, i))
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
				uvmPathForShare = fmt.Sprintf(lcowGlobalMountPrefix, i)
				_, _, uvmPath, err := coi.HostingSystem.AddSCSIPhysicalDisk(ctx, hostPath, uvmPathForShare, readOnly)
				if err != nil {
					return fmt.Errorf("adding SCSI physical disk mount %+v: %s", mount, err)
				}

				uvmPathForFile = uvmPath
				uvmPathForShare = uvmPath
				resources.scsiMounts = append(resources.scsiMounts, scsiMount{path: hostPath})
				coi.Spec.Mounts[i].Type = "none"
			} else if mount.Type == "virtual-disk" || mount.Type == "automanage-virtual-disk" {
				l.Debug("hcsshim::allocateLinuxResources Hot-adding SCSI virtual disk for OCI mount")
				uvmPathForShare = fmt.Sprintf(lcowGlobalMountPrefix, i)

				// if the scsi device is already attached then we take the uvm path that the function below returns
				// that is where it was previously mounted in UVM
				_, _, uvmPath, err := coi.HostingSystem.AddSCSI(ctx, hostPath, uvmPathForShare, readOnly)
				if err != nil {
					return fmt.Errorf("adding SCSI virtual disk mount %+v: %s", mount, err)
				}

				uvmPathForFile = uvmPath
				uvmPathForShare = uvmPath
				resources.scsiMounts = append(resources.scsiMounts, scsiMount{path: hostPath, autoManage: mount.Type == "automanage-virtual-disk"})
				coi.Spec.Mounts[i].Type = "none"
			} else if strings.HasPrefix(mount.Source, "sandbox://") {
				// Mounts that map to a path in UVM are specified with 'sandbox://' prefix.
				// example: sandbox:///a/dirInUvm destination:/b/dirInContainer
				uvmPathForFile = mount.Source
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

	addGPUVHD := false
	for i, d := range coi.Spec.Windows.Devices {
		switch d.IDType {
		case "gpu":
			addGPUVHD = true
			v := hcsschema.VirtualPciDevice{
				Functions: []hcsschema.VirtualPciFunction{
					{
						DeviceInstancePath: d.ID,
					},
				},
			}
			vmBusGUID, err := coi.HostingSystem.AssignDevice(ctx, v)
			if err != nil {
				return errors.Wrapf(err, "failed to assign gpu device %s to pod %s", d.ID, coi.HostingSystem.ID())
			}
			resources.vpciDevices = append(resources.vpciDevices, vmBusGUID)

			// update device ID so the gcs knows which devices to map into the container
			coi.Spec.Windows.Devices[i].ID = vmBusGUID
		}
	}

	if addGPUVHD {
		gpuSupportVhdPath, err := getGPUVHDPath(coi)
		if err != nil {
			return errors.Wrapf(err, "failed to add gpu vhd to %v", coi.HostingSystem.ID())
		}
		// use lcowNvidiaMountPath since we only support nvidia gpus right now
		// must use scsi here since DDA'ing a hyper-v pci device is not supported on VMs that have ANY virtual memory
		_, _, _, err = coi.HostingSystem.AddSCSI(ctx, gpuSupportVhdPath, lcowNvidiaMountPath, true)
		if err != nil {
			return errors.Wrapf(err, "failed to add scsi device %s in the UVM %s at %s", gpuSupportVhdPath, coi.HostingSystem.ID(), lcowNvidiaMountPath)
		}
		resources.scsiMounts = append(resources.scsiMounts, scsiMount{path: gpuSupportVhdPath})
	}

	return nil
}
