// +build windows

package hcsoci

// Contains functions relating to a WCOW container, as opposed to a utility VM

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

const wcowGlobalMountPrefix = "C:\\mounts\\m%d"

func allocateWindowsResources(ctx context.Context, coi *createOptionsInternal, r *Resources) error {
	if coi.Spec == nil || coi.Spec.Windows == nil || coi.Spec.Windows.LayerFolders == nil {
		return fmt.Errorf("field 'Spec.Windows.Layerfolders' is not populated")
	}

	scratchFolder := coi.Spec.Windows.LayerFolders[len(coi.Spec.Windows.LayerFolders)-1]

	// TODO: Remove this code for auto-creation. Make the caller responsible.
	// Create the directory for the RW scratch layer if it doesn't exist
	if _, err := os.Stat(scratchFolder); os.IsNotExist(err) {
		if err := os.MkdirAll(scratchFolder, 0777); err != nil {
			return fmt.Errorf("failed to auto-create container scratch folder %s: %s", scratchFolder, err)
		}
	}

	// Create sandbox.vhdx if it doesn't exist in the scratch folder. It's called sandbox.vhdx
	// rather than scratch.vhdx as in the v1 schema, it's hard-coded in HCS.
	if _, err := os.Stat(filepath.Join(scratchFolder, "sandbox.vhdx")); os.IsNotExist(err) {
		if err := wclayer.CreateScratchLayer(ctx, scratchFolder, coi.Spec.Windows.LayerFolders[:len(coi.Spec.Windows.LayerFolders)-1]); err != nil {
			return fmt.Errorf("failed to CreateSandboxLayer %s", err)
		}
	}

	if coi.Spec.Root == nil {
		coi.Spec.Root = &specs.Root{}
	}

	if coi.Spec.Root.Path == "" && (coi.HostingSystem != nil || coi.Spec.Windows.HyperV == nil) {
		log.G(ctx).Debug("hcsshim::allocateWindowsResources mounting storage")
		containerRootPath, err := MountContainerLayers(ctx, coi.Spec.Windows.LayerFolders, r.containerRootInUVM, coi.HostingSystem)
		if err != nil {
			return fmt.Errorf("failed to mount container storage: %s", err)
		}
		coi.Spec.Root.Path = containerRootPath
		layers := &ImageLayers{
			vm:                 coi.HostingSystem,
			containerRootInUVM: r.containerRootInUVM,
			layers:             coi.Spec.Windows.LayerFolders,
		}
		r.layers = layers
	}

	// Validate each of the mounts. If this is a V2 Xenon, we have to add them as
	// VSMB shares to the utility VM. For V1 Xenon and Argons, there's nothing for
	// us to do as it's done by HCS.
	for i, mount := range coi.Spec.Mounts {
		if mount.Destination == "" || mount.Source == "" {
			return fmt.Errorf("invalid OCI spec - a mount must have both source and a destination: %+v", mount)
		}
		switch mount.Type {
		case "":
		case "physical-disk":
		case "virtual-disk":
		case "automanage-virtual-disk":
		default:
			return fmt.Errorf("invalid OCI spec - Type '%s' not supported", mount.Type)
		}

		if coi.HostingSystem != nil && schemaversion.IsV21(coi.actualSchemaVersion) {
			uvmPath := fmt.Sprintf(wcowGlobalMountPrefix, coi.HostingSystem.UVMMountCounter())
			readOnly := false
			for _, o := range mount.Options {
				if strings.ToLower(o) == "ro" {
					readOnly = true
					break
				}
			}
			l := log.G(ctx).WithField("mount", fmt.Sprintf("%+v", mount))
			if mount.Type == "physical-disk" {
				l.Debug("hcsshim::allocateWindowsResources Hot-adding SCSI physical disk for OCI mount")
				scsiMount, err := coi.HostingSystem.AddSCSIPhysicalDisk(ctx, mount.Source, uvmPath, readOnly)
				if err != nil {
					return fmt.Errorf("adding SCSI physical disk mount %+v: %s", mount, err)
				}
				coi.Spec.Mounts[i].Type = ""
				r.resources = append(r.resources, scsiMount)
			} else if mount.Type == "virtual-disk" || mount.Type == "automanage-virtual-disk" {
				l.Debug("hcsshim::allocateWindowsResources Hot-adding SCSI virtual disk for OCI mount")
				scsiMount, err := coi.HostingSystem.AddSCSI(ctx, mount.Source, uvmPath, readOnly, uvm.VMAccessTypeIndividual)
				if err != nil {
					return fmt.Errorf("adding SCSI virtual disk mount %+v: %s", mount, err)
				}
				coi.Spec.Mounts[i].Type = ""
				if mount.Type == "automanage-virtual-disk" {
					r.resources = append(r.resources, &AutoManagedVHD{hostPath: scsiMount.HostPath})
				}
				r.resources = append(r.resources, scsiMount)
			} else {
				if uvm.IsPipe(mount.Source) {
					pipe, err := coi.HostingSystem.AddPipe(ctx, mount.Source)
					if err != nil {
						return fmt.Errorf("failed to add named pipe to UVM: %s", err)
					}
					r.resources = append(r.resources, pipe)
				} else {
					l.Debug("hcsshim::allocateWindowsResources Hot-adding VSMB share for OCI mount")
					options := coi.HostingSystem.DefaultVSMBOptions(readOnly)
					share, err := coi.HostingSystem.AddVSMB(ctx, mount.Source, options)
					if err != nil {
						return fmt.Errorf("failed to add VSMB share to utility VM for mount %+v: %s", mount, err)
					}
					r.resources = append(r.resources, share)
				}
			}
		}
	}

	if cs, ok := coi.Spec.Windows.CredentialSpec.(string); ok {
		// Only need to create a CCG instance for v2 containers
		if schemaversion.IsV21(coi.actualSchemaVersion) {
			hypervisorIsolated := coi.HostingSystem != nil
			ccgState, ccgInstance, err := CreateCredentialGuard(ctx, coi.actualID, cs, hypervisorIsolated)
			if err != nil {
				return err
			}
			coi.ccgState = ccgState
			r.resources = append(r.resources, ccgInstance)
			//TODO dcantah: If/when dynamic service table entries is supported register the RpcEndpoint with hvsocket here
		}
	}
	return nil
}
