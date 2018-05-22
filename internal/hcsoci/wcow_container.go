// +build windows

package hcsoci

// Contains functions relating to a WCOW container, as opposed to a utility VM

import (
	"fmt"
	"os"
	"path/filepath"

	hcsschemav2 "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

func allocateWindowsResources(coi *createOptionsInternal, resources *Resources) error {
	sandboxFolder := coi.Spec.Windows.LayerFolders[len(coi.Spec.Windows.LayerFolders)-1]
	logrus.Debugf("hcsshim::allocateWindowsResources Sandbox folder: %s", sandboxFolder)

	// Create the directory for the RW sandbox layer if it doesn't exist
	if _, err := os.Stat(sandboxFolder); os.IsNotExist(err) {
		logrus.Debugf("hcsshim::allocateWindowsResources container sandbox folder does not exist so creating: %s ", sandboxFolder)
		if err := os.MkdirAll(sandboxFolder, 0777); err != nil {
			return fmt.Errorf("failed to auto-create container sandbox folder %s: %s", sandboxFolder, err)
		}
	}

	// Create sandbox.vhdx if it doesn't exist in the sandbox folder
	if _, err := os.Stat(filepath.Join(sandboxFolder, "sandbox.vhdx")); os.IsNotExist(err) {
		logrus.Debugf("hcsshim::allocateWindowsResources container sandbox.vhdx does not exist so creating in %s ", sandboxFolder)
		if err := wclayer.CreateSandboxLayer(sandboxFolder, coi.Spec.Windows.LayerFolders[:len(coi.Spec.Windows.LayerFolders)-1]); err != nil {
			return fmt.Errorf("failed to CreateSandboxLayer %s", err)
		}
	}

	// Do we need to auto-mount on behalf of the end user?
	if coi.Spec.Root == nil {
		coi.Spec.Root = &specs.Root{}
	}
	if coi.Spec.Root.Path == "" && (coi.HostingSystem != nil || coi.Spec.Windows.HyperV == nil) {
		logrus.Debugln("hcsshim::allocateWindowsResources Auto-mounting storage")
		mcl, err := mountContainerLayers(coi.Spec.Windows.LayerFolders, coi.HostingSystem)
		if err != nil {
			return fmt.Errorf("failed to auto-mount container storage: %s", err)
		}
		if coi.HostingSystem == nil {
			coi.Spec.Root.Path = mcl.(string) // Argon v1 or v2
		} else {
			coi.Spec.Root.Path = mcl.(hcsschemav2.CombinedLayersV2).ContainerRootPath // v2 Xenon WCOW
		}
		resources.Layers = coi.Spec.Windows.LayerFolders
	}

	// Auto-mount the mounts. There's only something to do for v2 xenons. In argons and v1 xenon,
	// it's done by the HCS directly.
	for _, mount := range coi.Spec.Mounts {
		if mount.Destination == "" || mount.Source == "" {
			return fmt.Errorf("invalid OCI spec - a mount must have both source and a destination: %+v", mount)
		}

		if coi.HostingSystem != nil {
			logrus.Debugf("hcsshim::allocateWindowsResources Hot-adding VSMB share for OCI mount %+v", mount)
			// Hot-add the VSMB shares to the utility VM
			// TODO: What are the right flags. Asked swernli
			//
			// Answer: If readonly set, the following. If read-write, no flags.
			//			::Schema::VirtualMachines::Resources::Storage::VSmbShareFlags::ReadOnly |
			//::Schema::VirtualMachines::Resources::Storage::VSmbShareFlags::CacheIO |
			//::Schema::VirtualMachines::Resources::Storage::VSmbShareFlags::ShareRead |
			//::Schema::VirtualMachines::Resources::Storage::VSmbShareFlags::ForceLevelIIOplocks;
			//

			// TODO: Read-only
			err := coi.HostingSystem.AddVSMB(mount.Source, "", hcsschemav2.VsmbFlagReadOnly|hcsschemav2.VsmbFlagPseudoOplocks|hcsschemav2.VsmbFlagTakeBackupPrivilege|hcsschemav2.VsmbFlagCacheIO|hcsschemav2.VsmbFlagShareRead)
			if err != nil {
				return fmt.Errorf("failed to add VSMB share to utility VM for mount %+v: %s", mount, err)
			}
			resources.VSMBMounts = append(resources.VSMBMounts, mount.Source)
		}
	}

	return nil
}
