// +build windows

package hcsoci

// Contains functions relating to a WCOW container, as opposed to a utility VM

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Microsoft/hcsshim/internal/hcs"
	hcsschemav2 "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func createWCOWContainer(coi *createOptionsExInternal) (*hcs.System, error) {

	sandboxFolder := coi.Spec.Windows.LayerFolders[len(coi.Spec.Windows.LayerFolders)-1]
	logrus.Debugf("hcsshim::createWCOWContainer Sandbox folder: %s", sandboxFolder)

	// Create the directory for the RW sandbox layer if it doesn't exist
	if _, err := os.Stat(sandboxFolder); os.IsNotExist(err) {
		logrus.Debugf("hcsshim::createWCOWContainer container sandbox folder does not exist so creating: %s ", sandboxFolder)
		if err := os.MkdirAll(sandboxFolder, 0777); err != nil {
			return nil, fmt.Errorf("failed to auto-create container sandbox folder %s: %s", sandboxFolder, err)
		}
	}

	// Create sandbox.vhdx if it doesn't exist in the sandbox folder
	if _, err := os.Stat(filepath.Join(sandboxFolder, "sandbox.vhdx")); os.IsNotExist(err) {
		logrus.Debugf("hcsshim::createWCOWContainer container sandbox.vhdx does not exist so creating in %s ", sandboxFolder)
		if err := wclayer.CreateSandboxLayer(sandboxFolder, coi.Spec.Windows.LayerFolders[:len(coi.Spec.Windows.LayerFolders)-1]); err != nil {
			return nil, fmt.Errorf("failed to CreateSandboxLayer %s", err)
		}
	}

	// Do we need to auto-mount on behalf of the end user?
	weMountedStorage := false
	origSpecRoot := coi.Spec.Root
	if coi.Spec.Root == nil {
		coi.Spec.Root = &specs.Root{}
	}
	if coi.Spec.Root.Path == "" && (coi.HostingSystem != nil || coi.Spec.Windows.HyperV == nil) {
		logrus.Debugln("hcsshim::createWCOWContainer Auto-mounting storage")
		mcl, err := MountContainerLayers(coi.Spec.Windows.LayerFolders, coi.HostingSystem)
		if err != nil {
			return nil, fmt.Errorf("failed to auto-mount container storage: %s", err)
		}
		weMountedStorage = true
		if coi.HostingSystem == nil {
			coi.Spec.Root.Path = mcl.(string) // Argon v1 or v2
		} else {
			coi.Spec.Root.Path = mcl.(hcsschemav2.CombinedLayersV2).ContainerRootPath // v2 Xenon WCOW
		}
	}

	// Auto-mount the mounts. There's only something to do for v2 xenons. In argons and v1 xenon,
	// it's done by the HCS directly.
	var vsmbMountsAddedByUs []string
	for _, mount := range coi.Spec.Mounts {
		if mount.Destination == "" || mount.Source == "" {
			thisError := fmt.Errorf("invalid OCI spec - a mount must have both source and a destination: %+v", mount)
			thisError = undoMountOnFailure(coi, origSpecRoot, weMountedStorage, vsmbMountsAddedByUs, thisError)
			return nil, thisError
		}

		if coi.HostingSystem != nil {
			logrus.Debugf("hcsshim::createWCOWContainer Hot-adding VSMB share for OCI mount %+v", mount)
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
			/* DISABLED
			err := coi.HostingSystem.AddVSMB(mount.Source, hcsschemav2.VsmbFlagReadOnly|hcsschemav2.VsmbFlagPseudoOplocks|hcsschemav2.VsmbFlagTakeBackupPrivilege|hcsschemav2.VsmbFlagCacheIO|hcsschemav2.VsmbFlagShareRead)
			if err != nil {
				thisError := fmt.Errorf("failed to add VSMB share to utility VM for mount %+v: %s", mount, err)
				thisError = undoMountOnFailure(coi, origSpecRoot, weMountedStorage, vsmbMountsAddedByUs, thisError)
				return nil, thisError
			} else {
				vsmbMountsAddedByUs = append(vsmbMountsAddedByUs, mount.Source)
			}
			*/
		}
	}

	hcsDocument, err := createHCSContainerDocument(coi, "windows")
	if err != nil {
		err = undoMountOnFailure(coi, origSpecRoot, weMountedStorage, vsmbMountsAddedByUs, err)
		return nil, err
	}

	return hcs.CreateComputeSystem(coi.actualId, hcsDocument, "")
}

func undoMountOnFailure(coi *createOptionsExInternal, origSpecRoot *specs.Root, weMountedStorage bool, vsmbMountsAddedByUs []string, currentError error) error {
	logrus.Debugf("hcsshim::undoMountOnFailure Unwinding container layers")
	retError := currentError
	if weMountedStorage {
		logrus.Debugf("hcsshim::undoMountOnFailure Unwinding container layers")
		if unmountError := UnmountContainerLayers(coi.Spec.Windows.LayerFolders, coi.HostingSystem, UnmountOperationAll); unmountError != nil {
			logrus.Warnf("may have leaked container layers storage on unwind: %s", unmountError)
			retError = errors.Wrapf(currentError, fmt.Sprintf("may have leaked some storage - hcsshim auto-mounted container storage, but was unable to complete the unmount: %s", unmountError))
		}
		coi.Spec.Root = origSpecRoot
	}

	/* DISABLED
	// Unwind vsmb bind-mounts
	for _, vsmbMountAddedByUs := range vsmbMountsAddedByUs {
		logrus.Debugf("hcsshim::undoMountOnFailure Unwinding VSMB Mount %s", vsmbMountAddedByUs)
		if unmountError := coi.HostingSystem.RemoveVSMB(vsmbMountAddedByUs); unmountError != nil {
			logrus.Warnf("may have leaked vsmb for bind-mount on unwind: %s: %s", vsmbMountAddedByUs, unmountError)
			retError = errors.Wrapf(currentError, fmt.Sprintf("may have leaked some storage - hcsshim auto-mounted vsmb bind-mount, but was unable to complete the unmount: %s", unmountError))
		}
	}
	*/

	return retError
}
