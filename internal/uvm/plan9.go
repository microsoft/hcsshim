package uvm

import (
	"fmt"

	"github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/uvm/lcowhostedsettings"
	"github.com/sirupsen/logrus"
)

// AddPlan9 adds a Plan9 share to a utility VM. Each Plan9 share is ref-counted and
// only added if it isn't already.
func (uvm *UtilityVM) AddPlan9(hostPath string, uvmPath string, flags int32) error {
	if uvm.operatingSystem != "linux" {
		return errNotSupported
	}
	if uvmPath == "" {
		return fmt.Errorf("uvmPath must be passed to AddPlan9")
	}

	logrus.Debugf("uvm::AddPlan9 %s %s %d id:%s", hostPath, uvmPath, flags, uvm.id)
	uvm.m.Lock()
	defer uvm.m.Unlock()
	if uvm.plan9Shares == nil {
		uvm.plan9Shares = make(map[string]*plan9Info)
	}
	if _, ok := uvm.plan9Shares[hostPath]; !ok {
		uvm.plan9Counter++

		modification := &schema2.ModifySettingsRequestV2{
			ResourceType: schema2.ResourceTypePlan9Share,
			RequestType:  schema2.RequestTypeAdd,
			Settings: schema2.VirtualMachinesResourcesStoragePlan9ShareV2{
				Name: fmt.Sprintf("%d", uvm.plan9Counter),
				Path: hostPath,
				Port: int32(uvm.plan9Counter), // TODO: Temporary. Will all use a single port (9999)
			},
			ResourceUri: fmt.Sprintf("virtualmachine/devices/plan9shares/%d", uvm.plan9Counter),
			HostedSettings: lcowhostedsettings.MappedDirectory{
				MountPath: uvmPath,
				Port:      int32(uvm.plan9Counter), // TODO: Temporary. Will all use a single port (9999)
				ReadOnly:  (flags & schema2.VPlan9FlagReadOnly) == schema2.VPlan9FlagReadOnly,
			},
		}

		if err := uvm.Modify(modification); err != nil {
			return err
		}
		uvm.plan9Shares[hostPath] = &plan9Info{
			refCount:  1,
			uvmPath:   uvmPath,
			idCounter: uvm.plan9Counter,
			port:      int32(uvm.plan9Counter), // TODO: Temporary. Will all use a single port (9999)
		}
	} else {
		uvm.plan9Shares[hostPath].refCount++
	}
	logrus.Debugf("hcsshim::AddPlan9 Success %s: refcount=%d %+v", hostPath, uvm.plan9Shares[hostPath].refCount, uvm.plan9Shares[hostPath])
	return nil
}

// RemovePlan9 removes a Plan9 share from a utility VM. Each Plan9 share is ref-counted
// and only actually removed when the ref-count drops to zero.
func (uvm *UtilityVM) RemovePlan9(hostPath string) error {
	if uvm.operatingSystem != "linux" {
		return errNotSupported
	}
	logrus.Debugf("uvm::RemovePlan9 %s id:%s", hostPath, uvm.id)
	uvm.m.Lock()
	defer uvm.m.Unlock()
	if _, ok := uvm.plan9Shares[hostPath]; !ok {
		return fmt.Errorf("%s is not present as a Plan9 share in %s, cannot remove", hostPath, uvm.id)
	}
	return uvm.removePlan9(hostPath, uvm.plan9Shares[hostPath].uvmPath)
}

// removePlan9 is the internally callable "unsafe" version of RemovePlan9. The mutex
// MUST be held when calling this function.
func (uvm *UtilityVM) removePlan9(hostPath, uvmPath string) error {
	uvm.plan9Shares[hostPath].refCount--
	if uvm.plan9Shares[hostPath].refCount > 0 {
		logrus.Debugf("uvm::RemovePlan9 Success %s id:%s Ref-count now %d. It is still present in the utility VM", hostPath, uvm.id, uvm.plan9Shares[hostPath].refCount)
		return nil
	}
	logrus.Debugf("uvm::RemovePlan9 Zero ref-count, removing. %s id:%s", hostPath, uvm.id)
	modification := &schema2.ModifySettingsRequestV2{
		ResourceType: schema2.ResourceTypePlan9Share,
		RequestType:  schema2.RequestTypeRemove,
		Settings: schema2.VirtualMachinesResourcesStoragePlan9ShareV2{
			Name: fmt.Sprintf("%d", uvm.plan9Shares[hostPath].idCounter),
			Port: uvm.plan9Shares[hostPath].port,
		},
		ResourceUri: fmt.Sprintf("virtualmachine/devices/plan9shares/%d", uvm.plan9Shares[hostPath].idCounter),
		HostedSettings: lcowhostedsettings.MappedDirectory{
			MountPath: uvm.plan9Shares[hostPath].uvmPath,
			Port:      uvm.plan9Shares[hostPath].port,
		},
	}
	if err := uvm.Modify(modification); err != nil {
		return fmt.Errorf("failed to remove plan9 share %s from %s: %+v: %s", hostPath, uvm.id, modification, err)
	}
	delete(uvm.plan9Shares, hostPath)
	logrus.Debugf("uvm::RemovePlan9 Success %s id:%s successfully removed from utility VM", hostPath, uvm.id)
	return nil
}
