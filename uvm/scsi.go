package uvm

import (
	"fmt"

	"github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/sirupsen/logrus"
)

// allocateSCSI finds the next available slot on the
// SCSI controllers associated with a utility VM to use.
func (uvm *UtilityVM) allocateSCSI(hostPath string) (int, int, error) {
	uvm.scsiLocations.Lock()
	defer uvm.scsiLocations.Unlock()
	for controller, luns := range uvm.scsiLocations.hostPath {
		for lun, hp := range luns {
			if hp == "" {
				uvm.scsiLocations.hostPath[controller][lun] = hostPath
				logrus.Debugf("uvm::allocateSCSI %d:%d %q", controller, lun, hostPath)
				return controller, lun, nil

			}
		}
	}
	return -1, -1, fmt.Errorf("no free SCSI locations")
}

func (uvm *UtilityVM) deallocateSCSI(controller int, lun int) error {
	uvm.scsiLocations.Lock()
	defer uvm.scsiLocations.Unlock()
	logrus.Debugf("uvm::deallocateSCSI %d:%d %q", controller, lun, uvm.scsiLocations.hostPath[controller][lun])
	uvm.scsiLocations.hostPath[controller][lun] = ""

	return nil
}

// Lock must be held when calling this function
func (uvm *UtilityVM) findSCSIAttachment(findThisHostPath string) (int, int, error) {
	for controller, slots := range uvm.scsiLocations.hostPath {
		for slot, hostPath := range slots {
			if hostPath == findThisHostPath {
				logrus.Debugf("uvm::findSCSIAttachment %d:%d %s", controller, slot, hostPath)
				return controller, slot, nil
			}
		}
	}
	return -1, -1, fmt.Errorf("%s is not attached to SCSI", findThisHostPath)
}

// AddSCSI adds a SCSI disk to a utility VM at the next available location.
//
// In the v1 world, we do the modify call, HCS allocates a place on the SCSI bus,
// and we have to query back to HCS to determine where it landed.
//
// In the v2 world, we are in control of everything ourselves. Hence we have ref-
// counting and so-on tracking what SCSI locations are available or used.
//
// hostPath is required
// containerPath is optional.
//
// Returns the controller ID (0..3) and LUN (0..63) where the disk is attached.
func (uvm *UtilityVM) AddSCSI(hostPath string, containerPath string) (int, int, error) {
	controller := -1
	lun := -1
	if uvm == nil {
		return -1, -1, fmt.Errorf("no utility VM passed to AddSCSI")
	}
	logrus.Debugf("uvm::AddSCSI id:%s hostPath:%s containerPath:%s", uvm.id, hostPath, containerPath)

	// Keep in case we do add v1 here.
	//	if uvm.OperatingSystem == "linux" && uvm.SchemaVersion.IsV10() {
	//		modification := &ResourceModificationRequestResponse{
	//			Resource: "MappedVirtualDisk",
	//			Data: MappedVirtualDisk{
	//				HostPath:          hostPath,
	//				ContainerPath:     containerPath,
	//				CreateInUtilityVM: true,
	//				AttachOnly:        (containerPath == ""),
	//			},
	//			Request: "Add",
	//		}
	//		if err := uvm.Modify(modification); err != nil {
	//			return -1, -1, fmt.Errorf("uvm::AddSCSI: failed to modify utility VM configuration: %s", err)
	//		}

	//		// Get the list of mapped virtual disks to find the controller and LUN IDs
	//		logrus.Debugf("uvm::AddSCSI: %s querying mapped virtual disks", hostPath)
	//		mvdControllers, err := uvm.mappedVirtualDisks()
	//		if err != nil {
	//			return -1, -1, fmt.Errorf("failed to get mapped virtual disks: %s", err)
	//		}

	//		// Find our mapped disk from the list of all currently added.
	//		for controllerNumber, controllerElement := range mvdControllers {
	//			for diskNumber, diskElement := range controllerElement.MappedVirtualDisks {
	//				if diskElement.HostPath == hostPath {
	//					controller = controllerNumber
	//					lun = diskNumber
	//					break
	//				}
	//			}
	//		}
	//		if controller == -1 || lun == -1 {
	//			// We're somewhat stuffed here. Can't remove it as we don't know the controller/lun
	//			return -1, -1, fmt.Errorf("failed to find %s in mapped virtual disks after hot-adding", hostPath)
	//		}

	//		uvm.scsiLocations.Lock()
	//		defer uvm.scsiLocations.Unlock()
	//		if uvm.scsiLocations.hostPath[controller][lun] != "" {
	//			uvm.removeSCSI(hostPath, controller, lun)
	//			return -1, -1, fmt.Errorf("internal consistency error - %d:%d is in use by %s", controller, lun, hostPath)
	//		}
	//		uvm.scsiLocations.hostPath[controller][lun] = hostPath

	//		logrus.Debugf("uvm::AddSCSI success id:%s hostPath:%s added at %d:%d sv:%s", uvm.id, hostPath, controller, lun, uvm.SchemaVersion.String())
	//		return controller, lun, nil
	//	}

	var err error
	controller, lun, err = uvm.allocateSCSI(hostPath)
	if err != nil {
		return -1, -1, err
	}

	// TODO: Currently GCS doesn't support more than one SCSI controller. @jhowardmsft/@swernli. This will hopefully be fixed in GCS for RS5.
	// It will also require the HostedSettings to be extended in the call below to include the controller as well as the LUN.
	if controller > 0 {
		return -1, -1, fmt.Errorf("too many SCSI attachments")
	}

	SCSIModification := &schema2.ModifySettingsRequestV2{
		ResourceType: schema2.ResourceTypeMappedVirtualDisk,
		RequestType:  schema2.RequestTypeAdd,
		Settings: schema2.VirtualMachinesResourcesStorageAttachmentV2{
			Path: hostPath,
			Type: "VirtualDisk",
		},
		ResourceUri: fmt.Sprintf("VirtualMachine/Devices/SCSI/%d/%d", controller, lun),
		HostedSettings: schema2.ContainersResourcesMappedDirectoryV2{
			ContainerPath:     containerPath,
			Lun:               uint8(lun),
			AttachOnly:        (containerPath == ""),
			OverwriteIfExists: true,
			// TODO: Controller: uint8(controller), // TODO NOT IN HCS API CURRENTLY
		},
	}
	if err := uvm.Modify(SCSIModification); err != nil {
		uvm.deallocateSCSI(controller, lun)
		return -1, -1, fmt.Errorf("uvm::AddSCSI: failed to modify utility VM configuration: %s", err)
	}
	logrus.Debugf("uvm::AddSCSI id:%s hostPath:%s added at %d:%d", uvm.id, hostPath, controller, lun)
	return controller, lun, nil

}

// RemoveSCSI removes a SCSI disk from a utility VM. As an external API, it
// is "safe". Internal use can call removeSCSI.
func (uvm *UtilityVM) RemoveSCSI(hostPath string) error {
	uvm.scsiLocations.Lock()
	defer uvm.scsiLocations.Unlock()

	// Make sure is actually attached
	controller, lun, err := uvm.findSCSIAttachment(hostPath)
	if err != nil {
		return fmt.Errorf("cannot remove SCSI disk %s as it is not attached to container %s: %s", hostPath, uvm.id, err)
	}

	if err := uvm.removeSCSI(hostPath, controller, lun); err != nil {
		return fmt.Errorf("failed to remove SCSI disk %s from container %s: %s", hostPath, uvm.id, err)

	}
	return nil
}

// removeSCSI is the internally callable "unsafe" version of RemoveSCSI. The mutex
// MUST be held when calling this function.
func (uvm *UtilityVM) removeSCSI(hostPath string, controller int, lun int) error {
	var scsiModification interface{}
	logrus.Debugf("uvm::RemoveSCSI id:%s hostPath:%s", uvm.id, hostPath)

	// Keep in case we do add v1 here.
	//	if uvm.OperatingSystem == "linux" && uvm.SchemaVersion.IsV10() {
	//		scsiModification = &ResourceModificationRequestResponse{
	//			Resource: "MappedVirtualDisk",
	//			Data: MappedVirtualDisk{
	//				HostPath:          hostPath,
	//				CreateInUtilityVM: true,
	//			},
	//			Request: "Remove",
	//		}
	//	} else {
	scsiModification = &schema2.ModifySettingsRequestV2{
		ResourceType: schema2.ResourceTypeMappedVirtualDisk,
		RequestType:  schema2.RequestTypeRemove,
		ResourceUri:  fmt.Sprintf("VirtualMachine/Devices/SCSI/%d/%d", controller, lun),

		// BIG BIG BIG TODO TODO TODO HERE. After talking to swernli, the Hosted Settings MUST be included
		// or else the GCS won't be notified to unmount. Assuming it was exposed, of course.

	}
	if err := uvm.Modify(scsiModification); err != nil {
		return err
	}
	uvm.scsiLocations.hostPath[controller][lun] = ""
	logrus.Debugf("uvm::RemoveSCSI: Success %s removed from %s %d:%d", hostPath, uvm.id, controller, lun)
	return nil
}
