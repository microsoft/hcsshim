package uvm

import (
	"fmt"

	"github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/sirupsen/logrus"
)

// AddVSMB adds a VSMB share to a utility VM. Each VSMB share is ref-counted and
// only added if it isn't already.
func (uvm *UtilityVM) AddVSMB(hostPath string, uvmPath string, flags int32) error {
	if uvm.operatingSystem != "windows" {
		return errNotSupported
	}

	logrus.Debugf("uvm::AddVSMB %s %s %d id:%s", hostPath, uvmPath, flags, uvm.id)
	uvm.m.Lock()
	defer uvm.m.Unlock()
	if uvm.vsmbShares == nil {
		uvm.vsmbShares = make(map[string]*vsmbInfo)
	}
	if _, ok := uvm.vsmbShares[hostPath]; !ok {
		uvm.vsmbCounter++

		modification := &schema2.ModifySettingsRequestV2{
			ResourceType: schema2.ResourceTypeVSmbShare,
			RequestType:  schema2.RequestTypeAdd,
			Settings: schema2.VirtualMachinesResourcesStorageVSmbShareV2{
				Name:  fmt.Sprintf("%d", uvm.vsmbCounter),
				Flags: flags,
				Path:  hostPath,
			},
			ResourceUri: fmt.Sprintf("virtualmachine/devices/virtualsmbshares/%d", uvm.vsmbCounter),
		}

		// TODO: Hosted settings to support mapped directories on Windows
		if uvmPath != "" {
			panic("not yet implemented TODO TODO TODO - hostedSettings for VSMB")
		}

		if err := uvm.Modify(modification); err != nil {
			return err
		}
		uvm.vsmbShares[hostPath] = &vsmbInfo{
			idCounter: uvm.vsmbCounter,
			refCount:  1,
			uvmPath:   uvmPath}
	} else {
		uvm.vsmbShares[hostPath].refCount++
	}
	logrus.Debugf("hcsshim::AddVSMB Success %s: refcount=%d %+v", hostPath, uvm.vsmbShares[hostPath].refCount, uvm.vsmbShares[hostPath])
	return nil
}

// RemoveVSMB removes a VSMB share from a utility VM. Each VSMB share is ref-counted
// and only actually removed when the ref-count drops to zero.
func (uvm *UtilityVM) RemoveVSMB(hostPath string) error {
	if uvm.operatingSystem != "windows" {
		return errNotSupported
	}
	logrus.Debugf("uvm::RemoveVSMB %s id:%s", hostPath, uvm.id)
	uvm.m.Lock()
	defer uvm.m.Unlock()
	if _, ok := uvm.vsmbShares[hostPath]; !ok {
		return fmt.Errorf("%s is not present as a VSMB share in %s, cannot remove", hostPath, uvm.id)
	}
	return uvm.removeVSMB(hostPath, uvm.vsmbShares[hostPath].uvmPath)
}

// removeVSMB is the internally callable "unsafe" version of RemoveVSMB. The mutex
// MUST be held when calling this function.
func (uvm *UtilityVM) removeVSMB(hostPath, uvmPath string) error {
	uvm.vsmbShares[hostPath].refCount--
	if uvm.vsmbShares[hostPath].refCount > 0 {
		logrus.Debugf("uvm::RemoveVSMB Success %s id:%s Ref-count now %d. It is still present in the utility VM", hostPath, uvm.id, uvm.vsmbShares[hostPath].refCount)
		return nil
	}
	logrus.Debugf("uvm::RemoveVSMB Zero ref-count, removing. %s id:%s", hostPath, uvm.id)
	modification := &schema2.ModifySettingsRequestV2{
		ResourceType: schema2.ResourceTypeVSmbShare,
		RequestType:  schema2.RequestTypeRemove,
		Settings:     schema2.VirtualMachinesResourcesStorageVSmbShareV2{Name: fmt.Sprintf("%d", uvm.vsmbShares[hostPath].idCounter)},
		ResourceUri:  fmt.Sprintf("virtualmachine/devices/virtualsmbshares/%d", uvm.vsmbShares[hostPath].idCounter),
	}
	if err := uvm.Modify(modification); err != nil {
		return fmt.Errorf("failed to remove vsmb share %s from %s: %s: %s", hostPath, uvm.id, modification, err)
	}
	delete(uvm.vsmbShares, hostPath)
	logrus.Debugf("uvm::RemoveVSMB Success %s id:%s successfully removed from utility VM", hostPath, uvm.id)
	return nil
}

// GetVSMBCounter returns the counter used to mount a VSMB share in a utility VM
func (uvm *UtilityVM) GetVSMBCounter(hostPath string) (uint64, error) {
	if uvm.vsmbShares == nil {
		return 0, fmt.Errorf("no vsmbShares in utility VM!")
	}
	if hostPath == "" {
		return 0, fmt.Errorf("no hostPath passed to GetVSMBShareCounter")
	}
	uvm.m.Lock()
	defer uvm.m.Unlock()
	if _, ok := uvm.vsmbShares[hostPath]; !ok {
		return 0, fmt.Errorf("%s not found as VSMB share in %s", hostPath, uvm.id)
	}
	logrus.Debugf("uvm::GetVSMBCounter Success %s id:%s counter:%d", hostPath, uvm.id, uvm.vsmbShares[hostPath].idCounter)
	return uvm.vsmbShares[hostPath].idCounter, nil
}
