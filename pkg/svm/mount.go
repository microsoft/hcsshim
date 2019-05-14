package svm

// type layerEntry struct {
// 	hostPath string
// 	uvmPath  string
// 	scsi     bool
// }

// // Mount mounts layers and if necessary, creates an overlay. It's similar to
// // MountContainerLayers in hcsoci, but there's no scratch involved here as
// // we are only mounting for read-only purposes.
// func (i *instance) Mount(id string, layers []string, svmPath string) error {
// 	var layersAdded []layerEntry
// 	attachedSCSIHostPath := ""

// 	for _, layerPath := range layers {
// 		var err error
// 		uvmPath := ""
// 		hostPath := filepath.Join(layerPath, "layer.vhd")

// 		var fi os.FileInfo
// 		fi, err = os.Stat(hostPath)
// 		if err == nil && uint64(fi.Size()) > uvm.PMemMaxSizeBytes() {
// 			// Too big for PMEM. Add on SCSI instead (at /tmp/S<C>/<L>).
// 			var (
// 				controller int
// 				lun        int32
// 			)
// 			controller, lun, err = uvm.AddSCSILayer(hostPath)
// 			if err == nil {
// 				layersAdded = append(layersAdded,
// 					layerEntry{
// 						hostPath: hostPath,
// 						uvmPath:  fmt.Sprintf("/tmp/S%d/%d", controller, lun),
// 						scsi:     true,
// 					})
// 			}
// 		} else {
// 			_, uvmPath, err = uvm.AddVPMEM(hostPath, true) // UVM path is calculated. Will be /tmp/vN/
// 			if err == nil {
// 				layersAdded = append(layersAdded,
// 					layerEntry{
// 						hostPath: hostPath,
// 						uvmPath:  uvmPath,
// 					})
// 			}
// 		}
// 		if err != nil {
// 			//cleanupOnMountFailure(uvm, wcowLayersAdded, lcowlayersAdded, attachedSCSIHostPath)
// 			return nil, err
// 		}
// 	}

// }
