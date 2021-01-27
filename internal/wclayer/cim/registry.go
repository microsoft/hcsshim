package cim

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/Microsoft/hcsshim/internal/cimfs"
	"github.com/Microsoft/hcsshim/internal/winapi"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/pkg/errors"
)

// enableCimBoot Opens the SYSTEM registry hive at path `hivePath` in
// `layerPath` and updates it to include a CIMFS Start registry key. This prepares the uvm
// to boot from a cim file if requested. The registry changes required to actually make
// the uvm boot from a cim will be added in the uvm config since we don't want every
// single uvm started with this layer to attempt to boot from a cim. (Look at
// addBootFromCimRegistryChanges for details).
// This registry key needs to be available in the early boot phase and so including it in the
// uvm config doesn't work.
func enableCimBoot(layerPath, hivePath string) (err error) {
	dataZero := make([]byte, 4)

	regChanges := []struct {
		keyPath   string
		valueName string
		valueType winapi.RegType
		data      *byte
		dataLen   uint32
	}{
		{"ControlSet001\\Services\\CimFS", "Start", winapi.REG_TYPE_DWORD, &dataZero[0], uint32(len(dataZero))},
	}

	var storeHandle winapi.OrHKey
	if err = winapi.OrOpenHive(hivePath, &storeHandle); err != nil {
		return fmt.Errorf("failed to open registry store at %s: %s", hivePath, err)
	}

	for _, change := range regChanges {
		var changeKey winapi.OrHKey
		if err = winapi.OrCreateKey(storeHandle, change.keyPath, 0, 0, 0, &changeKey, nil); err != nil {
			return fmt.Errorf("failed to open reg key %s: %s", change.keyPath, err)
		}

		if err = winapi.OrSetValue(changeKey, change.valueName, uint32(change.valueType), change.data, change.dataLen); err != nil {
			return fmt.Errorf("failed to set value for regkey %s\\%s : %s", change.keyPath, change.valueName, err)
		}
	}

	// remove the existing file first
	if err := os.Remove(hivePath); err != nil {
		return fmt.Errorf("failed to remove existing registry %s: %s", hivePath, err)
	}

	if err = winapi.OrSaveHive(winapi.OrHKey(storeHandle), hivePath, uint32(osversion.Get().MajorVersion), uint32(osversion.Get().MinorVersion)); err != nil {
		return fmt.Errorf("error saving the registry store: %s", err)
	}

	// close hive irrespective of the errors
	if err := winapi.OrCloseHive(winapi.OrHKey(storeHandle)); err != nil {
		return fmt.Errorf("error closing registry store; %s", err)
	}
	return nil

}

// mergeWithParentLayerHives merges the delta hives of current layer with the base registry
// hives of its parent layer. This function reads the parent layer cim to fetch registry
// hives of the parent layer and reads the `layerPath\\Hives` directory to read the hives
// form the current layer. The merged hives are stored in the directory provided by
// `outputDir`
func mergeWithParentLayerHives(layerPath, parentLayerPath, outputDir string) error {
	// create a temp directory to store parent layer hive files
	tmpParentLayer, err := ioutil.TempDir("", "")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %s", tmpParentLayer)
	}
	defer os.RemoveAll(tmpParentLayer)

	parentCimPath := GetCimPathFromLayer(parentLayerPath)

	for _, hv := range hives {
		err := cimfs.FetchFileFromCim(parentCimPath, filepath.Join(hivesPath, hv.base), filepath.Join(tmpParentLayer, hv.base))
		if err != nil {
			return err
		}
	}

	// merge hives
	for _, hv := range hives {
		err := mergeHive(filepath.Join(tmpParentLayer, hv.base), filepath.Join(layerPath, hivesPath, hv.delta), filepath.Join(outputDir, hv.base))
		if err != nil {
			return err
		}
	}
	return nil
}

// mergeHive merges the hive located at parentHivePath with the hive located at deltaHivePath and stores
// the result into the file at mergedHivePath. If a file already exists at path `mergedHivePath` then it
// throws an error.
func mergeHive(parentHivePath, deltaHivePath, mergedHivePath string) (err error) {
	var baseHive, deltaHive, mergedHive winapi.OrHKey
	if err := winapi.OrOpenHive(parentHivePath, &baseHive); err != nil {
		return fmt.Errorf("failed to open base hive %s: %s", parentHivePath, err)
	}
	defer func() {
		err2 := winapi.OrCloseHive(baseHive)
		if err == nil {
			err = errors.Wrap(err2, "failed to close base hive")
		}
	}()
	if err := winapi.OrOpenHive(deltaHivePath, &deltaHive); err != nil {
		return fmt.Errorf("failed to open delta hive %s: %s", deltaHivePath, err)
	}
	defer func() {
		err2 := winapi.OrCloseHive(deltaHive)
		if err == nil {
			err = errors.Wrap(err2, "failed to close delta hive")
		}
	}()
	if err := winapi.OrMergeHives([]winapi.OrHKey{baseHive, deltaHive}, &mergedHive); err != nil {
		return fmt.Errorf("failed to merge hives: %s", err)
	}
	defer func() {
		err2 := winapi.OrCloseHive(mergedHive)
		if err == nil {
			err = errors.Wrap(err2, "failed to close merged hive")
		}
	}()
	if err := winapi.OrSaveHive(mergedHive, mergedHivePath, uint32(osversion.Get().MajorVersion), uint32(osversion.Get().MinorVersion)); err != nil {
		return fmt.Errorf("failed to save hive: %s", err)
	}
	return
}

// getOsBuildNumberFromRegistry fetches the "CurrentBuild" value at path
// "Microsoft\Windows NT\CurrentVersion" from the SOFTWARE registry hive at path
// `regHivePath`. This is used to detect the build version of the uvm.
func getOsBuildNumberFromRegistry(regHivePath string) (_ string, err error) {
	var storeHandle, keyHandle winapi.OrHKey
	var dataType, dataLen uint32
	keyPath := "Microsoft\\Windows NT\\CurrentVersion"
	valueName := "CurrentBuild"
	dataLen = 16 // build version string can't be more than 5 wide chars?
	dataBuf := make([]byte, dataLen)

	if err = winapi.OrOpenHive(regHivePath, &storeHandle); err != nil {
		return "", fmt.Errorf("failed to open registry store at %s: %s", regHivePath, err)
	}
	defer winapi.OrCloseHive(storeHandle)

	if err = winapi.OrOpenKey(storeHandle, keyPath, &keyHandle); err != nil {
		return "", fmt.Errorf("failed to open key at %s: %s", keyPath, err)
	}
	defer winapi.OrCloseKey(keyHandle)

	if err = winapi.OrGetValue(keyHandle, "", valueName, &dataType, &dataBuf[0], &dataLen); err != nil {
		return "", fmt.Errorf("failed to get value of %s: %s", valueName, err)
	}

	if dataType != uint32(winapi.REG_TYPE_SZ) {
		return "", fmt.Errorf("unexpected build number data type (%d)", dataType)
	}

	return winapi.ParseUtf16LE(dataBuf[:(dataLen - 2)]), nil
}
