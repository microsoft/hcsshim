package hcsshim

import (
	"path/filepath"

	"github.com/Microsoft/hcsshim/internal/wclayer"
)

func layerPath(info *DriverInfo, id string) string {
	return filepath.Join(info.HomeDir, id)
}

func ActivateLayer(info DriverInfo, id string) error {
	return wclayer.ActivateLayer(layerPath(&info, id))
}
func CreateLayer(info DriverInfo, id, parent string) error {
	return wclayer.CreateLayer(layerPath(&info, id), parent)
}
func CreateSandboxLayer(info DriverInfo, layerId, parentId string, parentLayerPaths []string) error {
	return wclayer.CreateSandboxLayer(layerPath(&info, layerId), parentLayerPaths)
}
func DeactivateLayer(info DriverInfo, id string) error {
	return wclayer.DeactivateLayer(layerPath(&info, id))
}
func DestroyLayer(info DriverInfo, id string) error {
	return wclayer.DestroyLayer(layerPath(&info, id))
}
func ExpandSandboxSize(info DriverInfo, layerId string, size uint64) error {
	return wclayer.ExpandSandboxSize(layerPath(&info, layerId), size)
}
func ExportLayer(info DriverInfo, layerId string, exportFolderPath string, parentLayerPaths []string) error {
	return wclayer.ExportLayer(layerPath(&info, layerId), exportFolderPath, parentLayerPaths)
}
func GetLayerMountPath(info DriverInfo, id string) (string, error) {
	return wclayer.GetLayerMountPath(layerPath(&info, id))
}
func GetSharedBaseImages() (imageData string, err error) {
	return wclayer.GetSharedBaseImages()
}
func ImportLayer(info DriverInfo, layerID string, importFolderPath string, parentLayerPaths []string) error {
	return wclayer.ImportLayer(layerPath(&info, layerID), importFolderPath, parentLayerPaths)
}
func LayerExists(info DriverInfo, id string) (bool, error) {
	return wclayer.LayerExists(layerPath(&info, id))
}
func PrepareLayer(info DriverInfo, layerId string, parentLayerPaths []string) error {
	return wclayer.PrepareLayer(layerPath(&info, layerId), parentLayerPaths)
}
func ProcessBaseLayer(path string) error {
	return wclayer.ProcessBaseLayer(path)
}
func ProcessUtilityVMImage(path string) error {
	return wclayer.ProcessUtilityVMImage(path)
}
func UnprepareLayer(info DriverInfo, layerId string) error {
	return wclayer.UnprepareLayer(layerPath(&info, layerId))
}

type DriverInfo struct {
	Flavour int
	HomeDir string
}

type FilterLayerReader = wclayer.FilterLayerReader
type FilterLayerWriter = wclayer.FilterLayerWriter
type GUID = wclayer.GUID

func NameToGuid(name string) (id GUID, err error) {
	return wclayer.NameToGuid(name)
}
func NewGUID(source string) *GUID {
	return wclayer.NewGUID(source)
}

type LayerReader = wclayer.LayerReader

func NewLayerReader(info DriverInfo, layerID string, parentLayerPaths []string) (LayerReader, error) {
	return wclayer.NewLayerReader(layerPath(&info, layerID), parentLayerPaths)
}

type LayerWriter = wclayer.LayerWriter

func NewLayerWriter(info DriverInfo, layerID string, parentLayerPaths []string) (LayerWriter, error) {
	return wclayer.NewLayerWriter(layerPath(&info, layerID), parentLayerPaths)
}

type WC_LAYER_DESCRIPTOR = wclayer.WC_LAYER_DESCRIPTOR
