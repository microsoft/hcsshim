// Windows Syscall layer for computestorage.dll introduced in Windows RS5 1809 for
// managing containers storage via the HCS (Host Compute Service)

package computestorage

import (
	"encoding/json"
	"syscall"

	"github.com/Microsoft/hcsshim/internal/interop"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
)

//go:generate go run ../../mksyscall_windows.go -output zsyscall_windows.go computestorage.go

//sys hcsImportLayer(layerPath string, sourceFolderPath string, layerData string) (hr error) = computestorage.HcsImportLayer?
//sys hcsExportLayer(layerPath string, exportFolderPath string, layerData string, options string) (hr error) = computestorage.HcsExportLayer?
//sys hcsExportLegacyWritableLayer(writableLayerMountPath string, writableLayerFolderPath string, exportFolderPath string, layerData string) (hr error) = computestorage.hcsExportLegacyWritableLayer?
//sys hcsDestroyLayer(layerPath string) (hr error) = computestorage.hcsDestroyLayer?
//sys hcsSetupBaseOSLayer(layerPath string, vhdHandle syscall.Handle, options string) (hr error) = computestorage.hcsSetupBaseOSLayer?
//sys hcsInitializeWritableLayer(writableLayerPath string, layerData string, options string) (hr error) = computestorage.hcsInitializeWritableLayer?
//sys hcsInitializeLegacyWritableLayer(writableLayerMountPath string, writableLayerFolderPath string, layerData string, options string) (hr error) = computestorage.hcsInitializeLegacyWritableLayer?
//sys hcsAttachLayerStorageFilter(layerPath string, layerData string) (hr error) = computestorage.hcsAttachLayerStorageFilter?
//sys hcsDetachLayerStorageFilter(layerPath string) (hr error) = computestorage.hcsDetachLayerStorageFilter?
//sys hcsFormatWritableLayerVhd(vhdHandle syscall.Handle) (hr error) = computestorage.hcsFormatWritableLayerVhd?
//sys hcsGetLayerVhdMountPath(vhdHandle syscall.Handle, mountPath **uint16) (hr error) = computestorage.hcsGetLayerVhdMountPath?

// LayerData is the data used to describe parent layer information.
type LayerData struct {
	SchemaVersion hcsschema.Version `json:"SchemaVersion,omitempty"`

	Layers []hcsschema.Layer `json:"Layers,omitempty"`
}

// ExportLayerOptions are the set of options that are used with the `computestorage.HcsExportLayer` syscall.
type ExportLayerOptions struct {
	IsWritableLayer bool `json:"IsWritableLayer,omitempty"`
}

// OsLayerType is the type of layer being operated on.
type OsLayerType string

const (
	// OsLayerTypeContainer is a container layer.
	OsLayerTypeContainer OsLayerType = "Container"
	// OsLayerTypeVM is a virtual machine layer.
	OsLayerTypeVM OsLayerType = "Vm"
)

// OsLayerOptions are the set of options that are used with the `computestorage.HcsSetupBaseOSLayer` syscall.
type OsLayerOptions struct {
	Type                       OsLayerType `json:"Type,omitempty"`
	DisableCiCacheOptimization bool        `json:"DisableCiCacheOptimization,omitempty"`
}

// HcsImportLayer imports a container layer.
//
// `layerPath` is a path to a directory to import the layer to. If the directory
// does not exist it will be automatically created.
//
// `sourceFolderpath` is a pre-existing folder that contains the layer to
// import.
//
// `layerData` is the parent layer data.
func HcsImportLayer(layerPath, sourceFolderPath string, layerData LayerData) error {
	bytes, err := json.Marshal(layerData)
	if err != nil {
		return err
	}

	return hcsImportLayer(layerPath, sourceFolderPath, string(bytes))
}

// HcsExportLayer exports a container layer.
//
// `layerPath` is a path to a directory containing the layer to export.
//
// `exportFolderPath` is a pre-existing folder to export the layer to.
//
// `layerData` is the parent layer data.
//
// `options` are the export options applied to the exported layer.
func HcsExportLayer(layerPath, exportFolderPath string, layerData LayerData, options ExportLayerOptions) error {
	ldbytes, err := json.Marshal(layerData)
	if err != nil {
		return err
	}

	obytes, err := json.Marshal(options)
	if err != nil {
		return err
	}

	return hcsExportLayer(layerPath, exportFolderPath, string(ldbytes), string(obytes))
}

// HcsDestroyLayer deletes a container layer.
//
// `layerPath` is a path to a directory containing the layer to delete.
func HcsDestroyLayer(layerPath string) error {
	return hcsDestroyLayer(layerPath)
}

// HcsSetupBaseOSLayer sets up a layer that contains a base OS for a container.
//
// `layerPath` is a path to a directory containing the layer.
//
// `vhdHandle` is an empty file handle of `options.Type == OsLayerTypeContainer`
// or else it is a file handle to the 'SystemTemplateBase.vhdx' if `options.Type
// == OsLayerTypeVm`.
//
// `options` are the options applied while processing the layer.
func HcsSetupBaseOSLayer(layerPath string, vhdHandle syscall.Handle, options OsLayerOptions) error {
	bytes, err := json.Marshal(options)
	if err != nil {
		return err
	}

	return hcsSetupBaseOSLayer(layerPath, vhdHandle, string(bytes))
}

// HcsInitializeWritableLayer initializes a writable layer for a container.
//
// `writableLayerPath` is a path to a directory the layer is mounted. If the
// path does not end in a `\` the platform will append it automatically.
//
// `layerData` is the parent read-only layer data.
func HcsInitializeWritableLayer(writableLayerPath string, layerData LayerData) error {
	bytes, err := json.Marshal(layerData)
	if err != nil {
		return err
	}

	// options are not used in the platform as of RS5
	return hcsInitializeWritableLayer(writableLayerPath, string(bytes), "")
}

// HcsAttachLayerStorageFilter sets up the layer storage filter on a writable
// container layer.
//
// `layerPath` is a path to a directory the writable layer is mounted. If the
// path does not end in a `\` the platform will append it automatically.
//
// `layerData` is the parent read-only layer data.
func HcsAttachLayerStorageFilter(layerPath string, layerData LayerData) error {
	bytes, err := json.Marshal(layerData)
	if err != nil {
		return err
	}

	return hcsAttachLayerStorageFilter(layerPath, string(bytes))
}

// HcsDetachLayerStorageFilter detaches the layer storage filter from a writable
// container layer.
//
// `layerPath` is the path to a directory the writable layer is mounted. If the
// path does not end in a `\` the platform will append it automatically.
func HcsDetachLayerStorageFilter(layerPath string) error {
	return hcsDetachLayerStorageFilter(layerPath)
}

// HcsFormatWritableLayerVhd formats a virtual disk for the use as a writable container layer.
//
// `vhdHandle` is the handle to a writable layer vhd.
func HcsFormatWritableLayerVhd(vhdHandle syscall.Handle) error {
	return hcsFormatWritableLayerVhd(vhdHandle)
}

// HcsGetLayerVhdMountPath returns the volume path for a virtual disk of a writable container layer.
//
// `vhdHandle` is the handle to a writable layer vhd.
func HcsGetLayerVhdMountPath(vhdHandle syscall.Handle) (string, error) {
	var mountPath *uint16
	err := hcsGetLayerVhdMountPath(vhdHandle, &mountPath)
	if err != nil {
		return "", err
	}
	return interop.ConvertAndFreeCoTaskMemString(mountPath), nil
}
