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
	uvm.m.Lock()
	defer uvm.m.Unlock()
	if uvm.vsmbShares == nil {
		uvm.vsmbShares = make(map[string]vsmbInfo)
	}
	if _, ok := uvm.vsmbShares[hostPath]; !ok {
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
		uvm.vsmbShares[hostPath] = vsmbInfo{
			guid:     guid.String(),
			refCount: 1,
			uvmPath:  uvmPath}
	} else {
		smbi := vsmbInfo{
			guid:     uvm.vsmbShares[hostPath].guid,
			refCount: uvm.vsmbShares[hostPath].refCount + 1,
			uvmPath:  uvm.vsmbShares[hostPath].uvmPath}
		uvm.vsmbShares[hostPath] = smbi
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
	hostPath = strings.ToLower(hostPath)
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
	hostPath = strings.ToLower(hostPath)
	vi := vsmbInfo{
		guid:     uvm.vsmbShares[hostPath].guid,
		refCount: uvm.vsmbShares[hostPath].refCount - 1,
		uvmPath:  uvm.vsmbShares[hostPath].uvmPath,
	}
	uvm.vsmbShares[hostPath] = vi
	if vi.refCount > 0 {
		logrus.Debugf("uvm::RemoveVSMB Success %s id:%s Ref-count now %d. It is still present in the utility VM", hostPath, uvm.id, vi.refCount)
		return nil
	}
	logrus.Debugf("uvm::RemoveVSMB Zero ref-count, removing. %s id:%s", hostPath, uvm.id)
	delete(uvm.vsmbShares, hostPath)
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
	if uvm.vsmbShares == nil {
		return "", fmt.Errorf("no vsmbShares in utility VM!")
	}
	if hostPath == "" {
		return "", fmt.Errorf("no hostPath passed to GetVSMBShareGUID")
	}
	uvm.m.Lock()
	defer uvm.m.Unlock()
	hostPath = strings.ToLower(hostPath)
	if _, ok := uvm.vsmbShares[hostPath]; !ok {
		return "", fmt.Errorf("%s not found as VSMB share in %s", hostPath, uvm.id)
	}
	logrus.Debugf("uvm::GetVSMBGUID Success %s id:%s guid:%s", hostPath, uvm.id, uvm.vsmbShares[hostPath].guid)
	return uvm.vsmbShares[hostPath].guid, nil
}
