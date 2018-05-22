package uvm

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	"github.com/sirupsen/logrus"
)

// AddVSMB adds a VSMB share to a utility VM. Each VSMB share is ref-counted and
// only added if it isn't already.
func (uvm *UtilityVM) AddVSMB(hostPath string, uvmPath string, flags int32) error {
	if uvm.operatingSystem != "windows" {
		return errNotSupported
	}

	hostPath = strings.ToLower(hostPath)
	logrus.Debugf("uvm::AddVSMB %s %s %d id:%s", hostPath, uvmPath, flags, uvm.id)
	uvm.vsmbShares.Lock()
	defer uvm.vsmbShares.Unlock()
	if uvm.vsmbShares.vsmbInfo == nil {
		uvm.vsmbShares.vsmbInfo = make(map[string]vsmbInfo)
	}
	if _, ok := uvm.vsmbShares.vsmbInfo[hostPath]; !ok {
		_, filename := filepath.Split(hostPath)
		guid, err := wclayer.NameToGuid(filename)
		if err != nil {
			return err
		}

		modification := &schema2.ModifySettingsRequestV2{
			ResourceType: schema2.ResourceTypeVSmbShare,
			RequestType:  schema2.RequestTypeAdd,
			Settings: schema2.VirtualMachinesResourcesStorageVSmbShareV2{
				Name:  guid.String(),
				Flags: flags,
				Path:  hostPath,
			},
			ResourceUri: fmt.Sprintf("virtualmachine/devices/virtualsmbshares/%s", guid.String()),
		}

		// TODO: Hosted settings to support mapped directories on Windows
		if uvmPath != "" {
			panic("not yet implemented TODO TODO TODO - hostedSettings for VSMB")
		}

		if err := uvm.Modify(modification); err != nil {
			return err
		}
		uvm.vsmbShares.vsmbInfo[hostPath] = vsmbInfo{
			guid:     guid.String(),
			refCount: 1,
			uvmPath:  uvmPath}
	} else {
		smbi := vsmbInfo{
			guid:     uvm.vsmbShares.vsmbInfo[hostPath].guid,
			refCount: uvm.vsmbShares.vsmbInfo[hostPath].refCount + 1,
			uvmPath:  uvm.vsmbShares.vsmbInfo[hostPath].uvmPath}
		uvm.vsmbShares.vsmbInfo[hostPath] = smbi
	}
	logrus.Debugf("hcsshim::AddVSMB Success %s: refcount=%d %+v", hostPath, uvm.vsmbShares.vsmbInfo[hostPath].refCount, uvm.vsmbShares.vsmbInfo[hostPath])
	return nil
}

// RemoveVSMB removes a VSMB share from a utility VM. Each VSMB share is ref-counted
// and only actually removed when the ref-count drops to zero.
func (uvm *UtilityVM) RemoveVSMB(hostPath string) error {
	if uvm.operatingSystem != "windows" {
		return errNotSupported
	}
	hostPath = strings.ToLower(hostPath)
	logrus.Debugf("uvm::RemoveVSMB %s id:%s", hostPath, uvm.id)
	uvm.vsmbShares.Lock()
	defer uvm.vsmbShares.Unlock()
	if _, ok := uvm.vsmbShares.vsmbInfo[hostPath]; !ok {
		return fmt.Errorf("%s is not present as a VSMB share in %s, cannot remove", hostPath, uvm.id)
	}
	return uvm.removeVSMB(hostPath, uvm.vsmbShares.vsmbInfo[hostPath].uvmPath)
}

// removeVSMB is the internally callable "unsafe" version of RemoveVSMB. The mutex
// MUST be held when calling this function.
func (uvm *UtilityVM) removeVSMB(hostPath, uvmPath string) error {
	hostPath = strings.ToLower(hostPath)
	vi := vsmbInfo{
		guid:     uvm.vsmbShares.vsmbInfo[hostPath].guid,
		refCount: uvm.vsmbShares.vsmbInfo[hostPath].refCount - 1,
		uvmPath:  uvm.vsmbShares.vsmbInfo[hostPath].uvmPath,
	}
	uvm.vsmbShares.vsmbInfo[hostPath] = vi
	if vi.refCount > 0 {
		logrus.Debugf("uvm::RemoveVSMB Success %s id:%s Ref-count now %d. It is still present in the utility VM", hostPath, uvm.id, vi.refCount)
		return nil
	}
	logrus.Debugf("uvm::RemoveVSMB Zero ref-count, removing. %s id:%s", hostPath, uvm.id)
	delete(uvm.vsmbShares.vsmbInfo, hostPath)
	modification := &schema2.ModifySettingsRequestV2{
		ResourceType: schema2.ResourceTypeVSmbShare,
		RequestType:  schema2.RequestTypeRemove,
		Settings:     schema2.VirtualMachinesResourcesStorageVSmbShareV2{Name: vi.guid},
		ResourceUri:  fmt.Sprintf("virtualmachine/devices/virtualsmbshares/%s", vi.guid),
	}
	if err := uvm.Modify(modification); err != nil {
		return fmt.Errorf("failed to remove vsmb share %s from %s: %s: %s", hostPath, uvm.id, modification, err)
	}
	logrus.Debugf("uvm::RemoveVSMB Success %s id:%s successfully removed from utility VM", hostPath, uvm.id)
	return nil
}

// GetVSMBGUID returns the GUID used to mount a VSMB share in a utility VM
func (uvm *UtilityVM) GetVSMBGUID(hostPath string) (string, error) {
	if uvm.vsmbShares.vsmbInfo == nil {
		return "", fmt.Errorf("no vsmbShares in utility VM!")
	}
	if hostPath == "" {
		return "", fmt.Errorf("no hostPath passed to GetVSMBShareGUID")
	}
	uvm.vsmbShares.Lock()
	defer uvm.vsmbShares.Unlock()
	hostPath = strings.ToLower(hostPath)
	if _, ok := uvm.vsmbShares.vsmbInfo[hostPath]; !ok {
		return "", fmt.Errorf("%s not found as VSMB share in %s", hostPath, uvm.id)
	}
	logrus.Debugf("uvm::GetVSMBGUID Success %s id:%s guid:%s", hostPath, uvm.id, uvm.vsmbShares.vsmbInfo[hostPath].guid)
	return uvm.vsmbShares.vsmbInfo[hostPath].guid, nil
}
