package uvm

import (
	"fmt"
	//"path/filepath"
	"strings"

	"github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	"github.com/Microsoft/hcsshim/uvm/lcowhostedsettings"
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

	hostPath = strings.ToLower(hostPath)
	logrus.Debugf("uvm::AddPlan9 %s %s %d id:%s", hostPath, uvmPath, flags, uvm.id)
	uvm.m.Lock()
	defer uvm.m.Unlock()
	if uvm.plan9Shares == nil {
		uvm.plan9Shares = make(map[string]plan9Info)
	}
	if _, ok := uvm.plan9Shares[hostPath]; !ok {
		guid, err := wclayer.NameToGuid(hostPath) // TODO: We should hash the full hostpath on VSMB too
		if err != nil {
			logrus.Debugf("Failed NamedToGuid", err)
			return err
		}

		uvm.plan9PortCounter++ // TODO: This is temporary. Each share currently requires a unique port in HCS and GCS. This will change so we use a single port (9999)

		modification := &schema2.ModifySettingsRequestV2{
			ResourceType: schema2.ResourceTypePlan9Share,
			RequestType:  schema2.RequestTypeAdd,
			Settings: schema2.VirtualMachinesResourcesStoragePlan9ShareV2{
				Name: guid.String(),
				Path: hostPath,
				Port: uvm.plan9PortCounter,
			},
			ResourceUri: fmt.Sprintf("virtualmachine/devices/plan9shares/%s", guid.String()),
			HostedSettings: lcowhostedsettings.MappedDirectory{
				MountPath: uvmPath,
				Port:      uvm.plan9PortCounter,
				ReadOnly:  (flags | schema2.VPlan9FlagReadOnly) == schema2.VPlan9FlagReadOnly,
			},
		}

		if err := uvm.Modify(modification); err != nil {
			return err
		}
		uvm.plan9Shares[hostPath] = plan9Info{
			refCount: 1,
			uvmPath:  uvmPath,
			guid:     guid.String(),
			port:     uvm.plan9PortCounter,
		}
	} else {
		p9i := plan9Info{
			refCount: uvm.plan9Shares[hostPath].refCount + 1,
			uvmPath:  uvm.plan9Shares[hostPath].uvmPath,
			guid:     uvm.plan9Shares[hostPath].guid,
			port:     uvm.plan9Shares[hostPath].port,
		}
		uvm.plan9Shares[hostPath] = p9i
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
	hostPath = strings.ToLower(hostPath)
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
	hostPath = strings.ToLower(hostPath)
	p9i := plan9Info{
		refCount: uvm.plan9Shares[hostPath].refCount - 1,
		uvmPath:  uvm.plan9Shares[hostPath].uvmPath,
		guid:     uvm.plan9Shares[hostPath].guid,
		port:     uvm.plan9Shares[hostPath].port,
	}
	uvm.plan9Shares[hostPath] = p9i
	if p9i.refCount > 0 {
		logrus.Debugf("uvm::RemovePlan9 Success %s id:%s Ref-count now %d. It is still present in the utility VM", hostPath, uvm.id, p9i.refCount)
		return nil
	}
	logrus.Debugf("uvm::RemovePlan9 Zero ref-count, removing. %s id:%s", hostPath, uvm.id)
	delete(uvm.plan9Shares, hostPath)
	modification := &schema2.ModifySettingsRequestV2{
		ResourceType: schema2.ResourceTypePlan9Share,
		RequestType:  schema2.RequestTypeRemove,
		Settings: schema2.VirtualMachinesResourcesStoragePlan9ShareV2{
			Name: p9i.guid,
			Port: p9i.port,
		},
		ResourceUri: fmt.Sprintf("virtualmachine/devices/plan9shares/%s", p9i.guid),
		HostedSettings: lcowhostedsettings.MappedDirectory{
			MountPath: p9i.uvmPath,
			Port:      p9i.port,
		},
	}
	if err := uvm.Modify(modification); err != nil {
		return fmt.Errorf("failed to remove plan9 share %s from %s: %+v: %s", hostPath, uvm.id, modification, err)
	}
	logrus.Debugf("uvm::RemovePlan9 Success %s id:%s successfully removed from utility VM", hostPath, uvm.id)
	return nil
}
