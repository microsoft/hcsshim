package hcsshim

import (
	"github.com/Microsoft/hcsshim/internal/wclayer"
)

func ActivateLayer(info DriverInfo, id string) error {
	return wclayer.ActivateLayer(info, id)
}
func CreateLayer(info DriverInfo, id, parent string) error {
	return wclayer.CreateLayer(info, id, parent)
}
func CreateSandboxLayer(info DriverInfo, layerId, parentId string, parentLayerPaths []string) error {
	return wclayer.CreateSandboxLayer(info, layerId, parentId, parentLayerPaths)
}
func DeactivateLayer(info DriverInfo, id string) error {
	return wclayer.DeactivateLayer(info, id)
}
func DestroyLayer(info DriverInfo, id string) error {
	return wclayer.DestroyLayer(info, id)
}
func ExpandSandboxSize(info DriverInfo, layerId string, size uint64) error {
	return wclayer.ExpandSandboxSize(info, layerId, size)
}
func ExportLayer(info DriverInfo, layerId string, exportFolderPath string, parentLayerPaths []string) error {
	return wclayer.ExportLayer(info, layerId, exportFolderPath, parentLayerPaths)
}
func GetLayerMountPath(info DriverInfo, id string) (string, error) {
	return wclayer.GetLayerMountPath(info, id)
}
func GetSharedBaseImages() (imageData string, err error) {
	return wclayer.GetSharedBaseImages()
}
func ImportLayer(info DriverInfo, layerID string, importFolderPath string, parentLayerPaths []string) error {
	return wclayer.ImportLayer(info, layerID, importFolderPath, parentLayerPaths)
}
func LayerExists(info DriverInfo, id string) (bool, error) {
	return wclayer.LayerExists(info, id)
}
func PrepareLayer(info DriverInfo, layerId string, parentLayerPaths []string) error {
	return wclayer.PrepareLayer(info, layerId, parentLayerPaths)
}
func ProcessBaseLayer(path string) error {
	return wclayer.ProcessBaseLayer(path)
}
func ProcessUtilityVMImage(path string) error {
	return wclayer.ProcessUtilityVMImage(path)
}
func UnprepareLayer(info DriverInfo, layerId string) error {
	return wclayer.UnprepareLayer(info, layerId)
}

type DriverInfo = wclayer.DriverInfo
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
	return wclayer.NewLayerReader(info, layerID, parentLayerPaths)
}

type LayerWriter = wclayer.LayerWriter

func NewLayerWriter(info DriverInfo, layerID string, parentLayerPaths []string) (LayerWriter, error) {
	return wclayer.NewLayerWriter(info, layerID, parentLayerPaths)
}

type WC_LAYER_DESCRIPTOR = wclayer.WC_LAYER_DESCRIPTOR
