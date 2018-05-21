// +build windows

package hcsoci

// Contains functions relating to a LCOW container, as opposed to a utility VM

import (
	"fmt"

	"github.com/Microsoft/hcsshim/internal/hcs"
	hcsschemav2 "github.com/Microsoft/hcsshim/internal/schema2"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

func createLCOWContainer(coi *createOptionsInternal) (*hcs.System, error) {

	sandboxFolder := coi.Spec.Windows.LayerFolders[len(coi.Spec.Windows.LayerFolders)-1]
	logrus.Debugf("hcsshim::createLCOWContainer Sandbox folder: %s", sandboxFolder)

	//	// Create the directory for the RW sandbox layer if it doesn't exist
	//	if _, err := os.Stat(sandboxFolder); os.IsNotExist(err) {
	//		logrus.Debugf("hcsshim::createLCOWContainer container sandbox folder does not exist so creating: %s ", sandboxFolder)
	//		if err := os.MkdirAll(sandboxFolder, 0777); err != nil {
	//			return nil, fmt.Errorf("failed to auto-create container sandbox folder %s: %s", sandboxFolder, err)
	//		}
	//	}

	//	// Create sandbox.vhdx if it doesn't exist in the sandbox folder
	//	if _, err := os.Stat(filepath.Join(sandboxFolder, "sandbox.vhdx")); os.IsNotExist(err) {
	//		logrus.Debugf("hcsshim::createLCOWContainer container sandbox.vhdx does not exist so creating in %s ", sandboxFolder)
	//		di := DriverInfo{HomeDir: filepath.Dir(sandboxFolder)}
	//		if err := CreateSandboxLayer(di, filepath.Base(sandboxFolder), coi.Spec.Windows.LayerFolders[0], coi.Spec.Windows.LayerFolders[:len(coi.Spec.Windows.LayerFolders)-1]); err != nil {
	//			return nil, fmt.Errorf("failed to CreateSandboxLayer %s", err)
	//		}
	//	}

	// Do we need to auto-mount on behalf of the end user?
	weMountedStorage := false
	origSpecRoot := coi.Spec.Root
	if coi.Spec.Root == nil {
		coi.Spec.Root = &specs.Root{}
	}
	if coi.Spec.Root.Path == "" {
		logrus.Debugln("hcsshim::createLCOWContainer Auto-mounting storage")
		mcl, err := MountContainerLayers(coi.Spec.Windows.LayerFolders, coi.HostingSystem)
		if err != nil {
			return nil, fmt.Errorf("failed to auto-mount container storage: %s", err)
		}
		weMountedStorage = true
		if coi.HostingSystem == nil {
			coi.Spec.Root.Path = mcl.(string) // Argon v1 or v2
		} else {
			coi.Spec.Root.Path = mcl.(hcsschemav2.CombinedLayersV2).ContainerRootPath // v2 Xenon LCOW
		}
	}

	//	// Auto-mount the mounts. There's only something to do for v2 xenons. In argons and v1 xenon,
	//	// it's done by the HCS directly.
	var vpmemMountsAddedByUs []string
	//	for _, mount := range coi.Spec.Mounts {
	//		if mount.Destination == "" || mount.Source == "" {
	//			thisError := fmt.Errorf("invalid OCI spec - a mount must have both source and a destination: %+v", mount)
	//			thisError = undoMountOnFailure(coi, origSpecRoot, weMountedStorage, vpmemMountsAddedByUs, thisError)
	//			return nil, thisError
	//		}

	//		if coi.HostingSystem != nil {
	//			logrus.Debugf("hcsshim::createLCOWContainer Hot-adding VPMEM share for OCI mount %+v", mount)

	//			// TODO: Read-only
	//			err := AddVPMEM(coi.HostingSystem, mount.Source, hcsschemav2.VsmbFlagReadOnly|hcsschemav2.VsmbFlagPseudoOplocks|hcsschemav2.VsmbFlagTakeBackupPrivilege|hcsschemav2.VsmbFlagCacheIO|hcsschemav2.VsmbFlagShareRead)
	//			if err != nil {
	//				thisError := fmt.Errorf("failed to add VPMEM share to utility VM for mount %+v: %s", mount, err)
	//				thisError = undoMountOnFailure(coi, origSpecRoot, weMountedStorage, vpmemMountsAddedByUs, thisError)
	//				return nil, thisError
	//			} else {
	//				vpmemMountsAddedByUs = append(vpmemMountsAddedByUs, mount.Source)
	//			}
	//		}
	//	}

	hcsDocument, err := createHCSContainerDocument(coi, "linux")
	if err != nil {
		err = undoMountOnFailure(coi, origSpecRoot, weMountedStorage, vpmemMountsAddedByUs, err)
		return nil, err
	}

	return hcs.CreateComputeSystem(coi.actualID, hcsDocument)
}
