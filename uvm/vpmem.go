package uvm

import (
	"fmt"
	"strconv"

	"github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/sirupsen/logrus"
)

// allocateVPMEM finds the next available VPMem slot
func (uvm *UtilityVM) allocateVPMEM(hostPath string) (int, error) {
	uvm.vpmemLocations.Lock()
	defer uvm.vpmemLocations.Unlock()
	for index, currentValue := range uvm.vpmemLocations.hostPath {
		if currentValue == "" {
			uvm.vpmemLocations.hostPath[index] = hostPath
			logrus.Debugf("uvm::allocateVPMEM %d %q", index, hostPath)
			return index, nil

		}
	}
	return -1, fmt.Errorf("no free VPMEM locations")
}

func (uvm *UtilityVM) deallocateVPMEM(location int) error {
	uvm.vpmemLocations.Lock()
	defer uvm.vpmemLocations.Unlock()
	uvm.vpmemLocations.hostPath[location] = ""
	return nil
}

// Lock must be held when calling this function
func (uvm *UtilityVM) findVPMEMAttachment(findThisHostPath string) (int, error) {
	for index, currentValue := range uvm.vpmemLocations.hostPath {
		if currentValue == findThisHostPath {
			logrus.Debugf("uvm::findVPMEMAttachment %d %s", index, findThisHostPath)
			return index, nil
		}

	}
	return -1, fmt.Errorf("%s is not attached to VPMEM", findThisHostPath)
}

// AddVPMEM adds a VPMEM disk to a utility VM at the next available location.
//
// This is only supported for v2 schema linux utility VMs
//
// Returns the location(0..255) where the device is attached, and if exposed,
// the container path which will be /tmp/vpmem<location>/ if no container path
// is supplied, or the user supplied one if it is.
func (uvm *UtilityVM) AddVPMEM(hostPath string, containerPath string, expose bool) (int, string, error) {
	location := -1
	logrus.Debugf("uvm::AddVPMEM id:%s hostPath:%s containerPath:%s expose:%t", uvm.id, hostPath, containerPath, expose)

	// BIG TODO: We need to store the hosted settings to so that on release we can tell GCS to flush.

	var err error
	location, err = uvm.allocateVPMEM(hostPath)
	if err != nil {
		return -1, "", err
	}
	controller := schema2.VirtualMachinesResourcesStorageVpmemControllerV2{}
	controller.Devices = make(map[string]schema2.VirtualMachinesResourcesStorageVpmemDeviceV2)
	controller.Devices[strconv.Itoa(location)] = schema2.VirtualMachinesResourcesStorageVpmemDeviceV2{
		HostPath:    hostPath,
		ReadOnly:    true,
		ImageFormat: "VHD1",
	}

	modification := &schema2.ModifySettingsRequestV2{
		ResourceType: schema2.ResourceTypeVPMemDevice,
		RequestType:  schema2.RequestTypeAdd,
		Settings:     controller,
	}

	if expose {
		if containerPath == "" {
			containerPath = fmt.Sprintf("/tmp/vpmem%d", location)
		}
		hostedSettings := schema2.MappedVPMemController{}
		hostedSettings.MappedDevices = make(map[int]string)
		hostedSettings.MappedDevices[location] = containerPath
		modification.HostedSettings = hostedSettings
	}

	if err := uvm.Modify(modification); err != nil {
		uvm.deallocateVPMEM(location)
		return -1, "", fmt.Errorf("uvm::AddVPMEM: failed to modify utility VM configuration: %s", err)
	}
	logrus.Debugf("uvm::AddVPMEM id:%s hostPath:%s added at %d", uvm.id, hostPath, location)
	return location, containerPath, nil
}

// RemoveVPMEM removes a VPMEM disk from a utility VM. As an external API, it
// is "safe". Internal use can call removeVPMEM.
func (uvm *UtilityVM) RemoveVPMEM(hostPath string) error {
	uvm.vpmemLocations.Lock()
	defer uvm.vpmemLocations.Unlock()

	// Make sure is actually attached
	location, err := uvm.findVPMEMAttachment(hostPath)
	if err != nil {
		return fmt.Errorf("cannot remove VPMEM %s as it is not attached to container %s: %s", hostPath, uvm.id, err)
	}

	if err := uvm.removeVPMEM(hostPath, location); err != nil {
		return fmt.Errorf("failed to remove VPMEM %s from container %s: %s", hostPath, uvm.id, err)
	}
	return nil
}

// removeVPMEM is the internally callable "unsafe" version of RemoveVPMEM. The mutex
// MUST be held when calling this function.
func (uvm *UtilityVM) removeVPMEM(hostPath string, location int) error {
	logrus.Debugf("uvm::RemoveVPMEM id:%s hostPath:%s", uvm.id, hostPath)

	vpmemModification := &schema2.ModifySettingsRequestV2{
	//			ResourceType: schema2.ResourceTypeMappedVirtualDisk,
	//			RequestType:  schema2.RequestTypeRemove,
	//			ResourceUri:  fmt.Sprintf("VirtualMachine/Devices/SCSI/%d/%d", controller, lun),

	}

	panic("JJH not yet implemented")
	if err := uvm.Modify(vpmemModification); err != nil {
		return err
	}
	uvm.vpmemLocations.hostPath[location] = ""
	logrus.Debugf("uvm::RemoveVPMEM: Success %s removed from %s %d", hostPath, uvm.id, location)
	return nil
}
