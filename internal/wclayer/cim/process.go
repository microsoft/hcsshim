package cim

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/hcsshim/computestorage"
	"github.com/Microsoft/hcsshim/internal/cimfs"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	"github.com/docker/docker/pkg/ioutils"
	"golang.org/x/sys/windows"
)

// createPlaceHolderHives Creates the empty place holder system registry hives inside the layer
// directory pointed by `layerPath`.
// HCS APIs called by setupBaseOSLayer expects the registry hive files in the layer
// directory at path `layerPath + regFilesPath` but in case of the cim the hives are
// stored inside the cim and the setupBaseOSLayer call fails if it doesn't find those
// files so we create empty placeholder hives inside the layer directory.
func createPlaceHolderHives(layerPath string) error {
	regDir := filepath.Join(layerPath, regFilesPath)
	if err := os.MkdirAll(regDir, 0777); err != nil {
		return fmt.Errorf("error while creating placeholder registry hives directory: %s", err)
	}
	for _, hv := range hives {
		if _, err := os.Create(filepath.Join(regDir, hv.name)); err != nil {
			return fmt.Errorf("error while creating registry value at: %s, %s", filepath.Join(regDir, hv.name), err)
		}
	}
	return nil
}

// processBaseLayer takes care of the special handling (such as creating the VHDs,
// generating the reparse points, updating BCD store etc) that is required for the base
// layer of an image. This function takes care of that processing once all layer files are
// written to the cim and hence this function expects that the cim is mountable. This
// function creates VHD files inside the directory pointed by `layerPath` and expects the
// the layer cim is present at the usual location retrieved by `GetCimPathFromLayer`.
func processBaseLayer(ctx context.Context, layerPath string, hasUtilityVM bool) (err error) {
	// process container base layer
	if err = createPlaceHolderHives(layerPath); err != nil {
		return err
	}
	baseVhdPath := filepath.Join(layerPath, containerBaseVhd)
	diffVhdPath := filepath.Join(layerPath, containerScratchVhd)
	defaultVhdSize := uint64(20)
	if err = computestorage.SetupContainerBaseLayer(ctx, layerPath, baseVhdPath, diffVhdPath, defaultVhdSize); err != nil {
		return fmt.Errorf("failed to setup container base layer: %s", err)
	}

	if hasUtilityVM {
		// process utilityVM base layer
		// setupUtilityVMBaseLayer needs to access some of the layer files so we mount the cim
		// and pass the path of the mounted cim as layerpath to setupUtilityVMBaseLayer.
		mountpath, err := cimfs.Mount(GetCimPathFromLayer(layerPath))
		if err != nil {
			return fmt.Errorf("failed to mount cim : %s", err)
		}
		defer func() {
			// Try to unmount irrespective of errors
			cimfs.Unmount(mountpath)
		}()

		baseVhdPath = filepath.Join(layerPath, utilityVMPath, utilityVMBaseVhd)
		diffVhdPath = filepath.Join(layerPath, utilityVMPath, utilityVMScratchVhd)
		defaultVhdSize = uint64(10)
		if err := computestorage.SetupUtilityVMBaseLayer(ctx, filepath.Join(mountpath, utilityVMPath), baseVhdPath, diffVhdPath, defaultVhdSize); err != nil {
			return fmt.Errorf("failed to setup utility vm base layer: %s", err)
		}
	}
	return nil
}

// createBaseLayerHives creates the base registry hives inside the given cim.
func createBaseLayerHives(cimWriter *cimfs.CimFsWriter) error {
	// make hives directory
	hivesDirInfo := &winio.FileBasicInfo{
		CreationTime:   windows.NsecToFiletime(time.Now().UnixNano()),
		LastAccessTime: windows.NsecToFiletime(time.Now().UnixNano()),
		LastWriteTime:  windows.NsecToFiletime(time.Now().UnixNano()),
		ChangeTime:     windows.NsecToFiletime(time.Now().UnixNano()),
		FileAttributes: 16,
	}
	err := cimWriter.AddFile(hivesPath, hivesDirInfo, 0, []byte{}, []byte{}, []byte{})
	if err != nil {
		return fmt.Errorf("failed while creating hives directory in the cim")
	}
	// add hard links from base hive files.
	for _, hv := range hives {
		err := cimWriter.AddLink(filepath.Join(regFilesPath, hv.name),
			filepath.Join(hivesPath, hv.base))
		if err != nil {
			return fmt.Errorf("failed while creating base registry hives in the cim: %s", err)
		}
	}
	return nil
}

// detectImageOsVersion tries to detect the windows build number (like 17763, 19042 etc.)
// of the image by looking at the registry keys of the layer registry files.  `tmpDir` is
// a temporary directory used to store the temporary work files.  This function also
// creates a file named `uvmbuildversion` in the layer directory which contains the build
// number for future reference.
func detectImageOsVersion(layerPath, tmpDir string) (uint16, error) {
	// detect the build number of the uvm before doing anything else
	layerRelativeSoftwareHivePath := filepath.Join(utilityVMPath, regFilesPath, "SOFTWARE")
	tmpSoftwareHivePath := filepath.Join(tmpDir, "SOFTWARE")
	if err := cimfs.FetchFileFromCim(GetCimPathFromLayer(layerPath), layerRelativeSoftwareHivePath, tmpSoftwareHivePath); err != nil {
		return 0, err
	}
	defer os.Remove(tmpSoftwareHivePath)

	osvStr, err := getOsBuildNumberFromRegistry(tmpSoftwareHivePath)
	if err != nil {
		return 0, err
	}

	osv, err := strconv.ParseUint(osvStr, 10, 16)
	if err != nil {
		return 0, err
	}

	if err := ioutil.WriteFile(filepath.Join(layerPath, uvmBuildVersionFileName), []byte(osvStr), 0644); err != nil {
		return uint16(osv), fmt.Errorf("failed to write uvm build version file: %s", err)
	}

	return uint16(osv), nil
}

// Some of the layer files that are generated during the processBaseLayer call must be added back
// inside the cim, some registry file links must be updated. This function takes care of all those
// steps. This function opens the cim file for writing and updates it.
func postProcessBaseLayer(ctx context.Context, layerPath string) (err error) {
	var layerRelativeSystemHivePath, tmpSystemHivePath string

	// fetch some files from the cim before opening it for writing.
	tmpDir, err := ioutils.TempDir("", "post-process-layer")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory at %s: %s", tmpDir, err)
	}
	defer os.RemoveAll(tmpDir)

	uvmBuildNumber, err := detectImageOsVersion(layerPath, tmpDir)
	if err != nil {
		return fmt.Errorf("failed to get os version of uvm: %s", err)
	}

	if uvmBuildNumber >= cimfs.MinimumCimFSBuild {
		layerRelativeSystemHivePath = filepath.Join(wclayer.UtilityVMPath, wclayer.RegFilesPath, "SYSTEM")
		tmpSystemHivePath = filepath.Join(tmpDir, "SYSTEM")
		if err := cimfs.FetchFileFromCim(GetCimPathFromLayer(layerPath), layerRelativeSystemHivePath, tmpSystemHivePath); err != nil {
			return err
		}

		if err := enableCimBoot(layerPath, tmpSystemHivePath); err != nil {
			return fmt.Errorf("failed to setup cim image for uvm boot: %s", err)
		}
	}

	// open the cim for writing
	cimWriter, err := cimfs.Create(GetCimDirFromLayer(layerPath), GetCimNameFromLayer(layerPath), "")
	if err != nil {
		return fmt.Errorf("failed to open cim at path %s: %s", layerPath, err)
	}
	defer func() {
		if err2 := cimWriter.Close(); err2 != nil {
			if err == nil {
				err = err2
			}
		}
	}()

	if err := createBaseLayerHives(cimWriter); err != nil {
		return err
	}

	// add the layout file generated during processBaseLayer inside the cim.
	if err := cimWriter.AddFileFromPath(layoutFileName, filepath.Join(layerPath, layoutFileName), []byte{}, []byte{}, []byte{}); err != nil {
		return fmt.Errorf("failed while adding layout file to cim: %s", err)
	}

	// add the BCD file updated during processBaseLayer inside the cim.
	if err := cimWriter.AddFileFromPath(bcdFilePath, filepath.Join(layerPath, bcdFilePath), []byte{}, []byte{}, []byte{}); err != nil {
		return fmt.Errorf("failed while adding BCD file to cim: %s", err)
	}

	if uvmBuildNumber >= cimfs.MinimumCimFSBuild {
		// This MUST come after createBaselayerHives otherwise createBaseLayerHives will overwrite the
		// changed system hive file.
		if err := cimWriter.AddFileFromPath(layerRelativeSystemHivePath, tmpSystemHivePath, []byte{}, []byte{}, []byte{}); err != nil {
			return fmt.Errorf("failed while updating SYSTEM registry inside cim: %s", err)
		}
	}
	return nil
}

// processNonBaseLayer takes care of the processing required for a non base layer. As of now
// the only processing required for non base layer is to merge the delta registry hives of the
// non-base layer with it's parent layer. This function opens the cim of the current layer for
// writing and updates it.
func processNonBaseLayer(ctx context.Context, layerPath string, parentLayerPaths []string) (err error) {
	// create a temp directory to store merged hive files of the current layer
	tmpCurrentLayer, err := ioutil.TempDir("", "")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %s", tmpCurrentLayer)
	}
	defer os.RemoveAll(tmpCurrentLayer)

	if err := mergeWithParentLayerHives(layerPath, parentLayerPaths[0], tmpCurrentLayer); err != nil {
		return err
	}

	// open the cim for writing
	cimWriter, err := cimfs.Create(GetCimDirFromLayer(layerPath), GetCimNameFromLayer(layerPath), "")
	if err != nil {
		return fmt.Errorf("failed to open cim at path %s: %s", layerPath, err)
	}
	defer func() {
		if err2 := cimWriter.Close(); err2 != nil {
			if err != nil {
				err = err2
			}
		}
	}()

	// add merged hives into the cim layer
	mergedHives, err := ioutil.ReadDir(tmpCurrentLayer)
	if err != nil {
		return fmt.Errorf("failed to enumerate hive files: %s", err)
	}
	for _, hv := range mergedHives {
		cimHivePath := filepath.Join(hivesPath, hv.Name())
		if err := cimWriter.AddFileFromPath(cimHivePath, filepath.Join(tmpCurrentLayer, hv.Name()), []byte{}, []byte{}, []byte{}); err != nil {
			return err
		}
	}
	return nil
}
