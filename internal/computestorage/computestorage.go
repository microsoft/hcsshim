// Windows Syscall layer for computestorage.dll introduced in Windows RS5 1809 for
// managing containers storage via the HCS (Host Compute Service)

package computestorage

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
