package uvm

import (
	"fmt"

	"github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/uvm/lcowhostedsettings"
	"github.com/sirupsen/logrus"
)

// allocateSCSI finds the next available slot on the
// SCSI controllers associated with a utility VM to use.
func (uvm *UtilityVM) allocateSCSI(hostPath string) (int, int, error) {
	uvm.m.Lock()
	defer uvm.m.Unlock()
	for controller, luns := range uvm.scsiLocations {
		for lun, si := range luns {
			if si.hostPath == "" {
				uvm.scsiLocations[controller][lun].hostPath = hostPath
				logrus.Debugf("uvm::allocateSCSI %d:%d %q", controller, lun, hostPath)
				return controller, lun, nil

			}
		}
	}
	return -1, -1, fmt.Errorf("no free SCSI locations")
}

func (uvm *UtilityVM) deallocateSCSI(controller int, lun int) error {
	uvm.m.Lock()
	defer uvm.m.Unlock()
	logrus.Debugf("uvm::deallocateSCSI %d:%d %+v", controller, lun, uvm.scsiLocations[controller][lun])
	uvm.scsiLocations[controller][lun] = scsiInfo{}

	return nil
}

// Lock must be held when calling this function
func (uvm *UtilityVM) findSCSIAttachment(findThisHostPath string) (int, int, string, error) {
	for controller, luns := range uvm.scsiLocations {
		for lun, si := range luns {
			if si.hostPath == findThisHostPath {
				logrus.Debugf("uvm::findSCSIAttachment %d:%d %+v", controller, lun, si)
				return controller, lun, si.uvmPath, nil
			}
		}
	}
	return -1, -1, "", fmt.Errorf("%s is not attached to SCSI", findThisHostPath)
}

// AddSCSI adds a SCSI disk to a utility VM at the next available location.
//
// We are in control of everything ourselves. Hence we have ref-
// counting and so-on tracking what SCSI locations are available or used.
//
// hostPath is required
// uvmPath is optional.
//
// Returns the controller ID (0..3) and LUN (0..63) where the disk is attached.
func (uvm *UtilityVM) AddSCSI(hostPath string, uvmPath string) (int, int, error) {
	controller := -1
	lun := -1
	if uvm == nil {
		return -1, -1, fmt.Errorf("no utility VM passed to AddSCSI")
	}
	logrus.Debugf("uvm::AddSCSI id:%s hostPath:%s uvmPath:%s", uvm.id, hostPath, uvmPath)

	if uvm.scsiControllerCount == 0 {
		return -1, -1, fmt.Errorf("cannot AddSCSI as the utility VM has no SCSI controller configured")
	}

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

	// TODO: This is wrong. There's no way to hot-add a SCSI attachement currently. This is a HACK
	SCSIModification := &schema2.ModifySettingsRequestV2{
		ResourceType: schema2.ResourceTypeMappedVirtualDisk,
		RequestType:  schema2.RequestTypeAdd,
		Settings: schema2.VirtualMachinesResourcesStorageAttachmentV2{
			Path: hostPath,
			Type: "VirtualDisk",
		},
		ResourceUri: fmt.Sprintf("VirtualMachine/Devices/SCSI/%d/%d", controller, lun),
	}

	// HACK HACK HACK as lun in hosted settings is needed in this workaround	if uvmPath != "" {
	var hostedSettings interface{}
	if uvm.operatingSystem == "windows" {
		hostedSettings = schema2.ContainersResourcesMappedDirectoryV2{
			ContainerPath:     uvmPath,
			Lun:               uint8(lun),
			AttachOnly:        (uvmPath == ""),
			OverwriteIfExists: true,
			// TODO: Controller: uint8(controller), // TODO NOT IN HCS API CURRENTLY
		}

	} else {
		hostedSettings = lcowhostedsettings.MappedVirtualDisk{
			MountPath:  uvmPath,
			Lun:        uint8(lun),
			Controller: uint8(controller),
			ReadOnly:   false,
		}
	}

	SCSIModification.HostedSettings = hostedSettings
	//}

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
	uvm.m.Lock()
	defer uvm.m.Unlock()

	if uvm.scsiControllerCount == 0 {
		return fmt.Errorf("cannot AddSCSI as the utility VM has no SCSI controller configured")
	}

	// Make sure is actually attached
	controller, lun, uvmPath, err := uvm.findSCSIAttachment(hostPath)
	if err != nil {
		return fmt.Errorf("cannot remove SCSI disk %s as it is not attached to container %s: %s", hostPath, uvm.id, err)
	}

	if err := uvm.removeSCSI(hostPath, uvmPath, controller, lun); err != nil {
		return fmt.Errorf("failed to remove SCSI disk %s from container %s: %s", hostPath, uvm.id, err)

	}
	return nil
}

// removeSCSI is the internally callable "unsafe" version of RemoveSCSI. The mutex
// MUST be held when calling this function.
func (uvm *UtilityVM) removeSCSI(hostPath string, uvmPath string, controller int, lun int) error {
	logrus.Debugf("uvm::RemoveSCSI id:%s hostPath:%s", uvm.id, hostPath)
	scsiModification := &schema2.ModifySettingsRequestV2{
		ResourceType: schema2.ResourceTypeMappedVirtualDisk,
		RequestType:  schema2.RequestTypeRemove,
		ResourceUri:  fmt.Sprintf("VirtualMachine/Devices/SCSI/%d/%d", controller, lun),
	}
	if uvmPath != "" {
		// Include the HostedSettings so that the GCS ejects the disk cleanly
		scsiModification.HostedSettings = schema2.ContainersResourcesMappedDirectoryV2{
			ContainerPath: uvmPath,
			Lun:           uint8(lun),
			// TODO: Controller: uint8(controller), // TODO NOT IN HCS API CURRENTLY
		}
	}
	if err := uvm.Modify(scsiModification); err != nil {
		return err
	}
	uvm.scsiLocations[controller][lun] = scsiInfo{}
	logrus.Debugf("uvm::RemoveSCSI: Success %s removed from %s %d:%d", hostPath, uvm.id, controller, lun)
	return nil
}
