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
func (uvm *UtilityVM) AddVSMB(path string, flags int32) error {
	path = strings.ToLower(path)
	logrus.Debugf("uvm::AddVSMB %s id:%s", path, uvm.id)
	uvm.vsmbShares.Lock()
	defer uvm.vsmbShares.Unlock()
	if uvm.vsmbShares.shares == nil {
		uvm.vsmbShares.shares = make(map[string]vsmbShare)
	}
	if _, ok := uvm.vsmbShares.shares[path]; !ok {
		_, filename := filepath.Split(path)
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
				Path:  path,
			},
			ResourceUri: fmt.Sprintf("virtualmachine/devices/virtualsmbshares/%s", guid.String()),
		}
		if err := uvm.Modify(modification); err != nil {
			return err
		}
		uvm.vsmbShares.shares[path] = vsmbShare{guid: guid.String(), refCount: 1}
	} else {
		s := vsmbShare{guid: uvm.vsmbShares.shares[path].guid, refCount: uvm.vsmbShares.shares[path].refCount + 1}
		uvm.vsmbShares.shares[path] = s
	}
	logrus.Debugf("hcsshim::AddVSMB Success %s: refcount=%d GUID %s", path, uvm.vsmbShares.shares[path].refCount, uvm.vsmbShares.shares[path].guid)
	return nil
}

// RemoveVSMB removes a VSMB share from a utility VM. Each VSMB share is ref-counted
// and only actually removed when the ref-count drops to zero.
func (uvm *UtilityVM) RemoveVSMB(path string) error {
	path = strings.ToLower(path)
	logrus.Debugf("uvm::RemoveVSMB %s id:%s", path, uvm.id)
	uvm.vsmbShares.Lock()
	defer uvm.vsmbShares.Unlock()
	if _, ok := uvm.vsmbShares.shares[path]; !ok {
		return fmt.Errorf("%s is not present as a VSMB share in %s, cannot remove", path, uvm.id)
	}
	return uvm.removeVSMB(path)
}

// removeVSMB is the internally callable "unsafe" version of RemoveVSMB. The mutex
// MUST be held when calling this function.
func (uvm *UtilityVM) removeVSMB(path string) error {
	path = strings.ToLower(path)
	s := vsmbShare{guid: uvm.vsmbShares.shares[path].guid, refCount: uvm.vsmbShares.shares[path].refCount - 1}
	uvm.vsmbShares.shares[path] = s
	if s.refCount > 0 {
		logrus.Debugf("uvm::RemoveVSMB Success %s id:%s Ref-count now %d. It is still present in the utility VM", path, uvm.id, s.refCount)
		return nil
	}
	logrus.Debugf("uvm::RemoveVSMB Zero ref-count, removing. %s id:%s", path, uvm.id)
	delete(uvm.vsmbShares.shares, path)
	modification := &schema2.ModifySettingsRequestV2{
		ResourceType: schema2.ResourceTypeVSmbShare,
		RequestType:  schema2.RequestTypeRemove,
		Settings:     schema2.VirtualMachinesResourcesStorageVSmbShareV2{Name: s.guid},
		ResourceUri:  fmt.Sprintf("virtualmachine/devices/virtualsmbshares/%s", s.guid),
	}
	if err := uvm.Modify(modification); err != nil {
		return fmt.Errorf("failed to remove vsmb share %s from %s: %s: %s", path, uvm.id, modification, err)
	}
	logrus.Debugf("uvm::RemoveVSMB Success %s id:%s successfully removed from utility VM", path, uvm.id)
	return nil
}

// GetVSMBGUID returns the GUID used to mount a VSMB share in a utility VM
// TODO: Rename path to hostPath
func (uvm *UtilityVM) GetVSMBGUID(path string) (string, error) {
	if uvm.vsmbShares.shares == nil {
		return "", fmt.Errorf("no vsmbShares in utility VM!")
	}
	if path == "" {
		return "", fmt.Errorf("no path passed to GetVSMBShareGUID")
	}
	uvm.vsmbShares.Lock()
	defer uvm.vsmbShares.Unlock()
	path = strings.ToLower(path)
	if _, ok := uvm.vsmbShares.shares[path]; !ok {
		return "", fmt.Errorf("%s not found as VSMB share in %s", path, uvm.id)
	}
	logrus.Debugf("uvm::GetVSMBGUID Success %s id:%s guid:%s", path, uvm.id, uvm.vsmbShares.shares[path].guid)
	return uvm.vsmbShares.shares[path].guid, nil
}
