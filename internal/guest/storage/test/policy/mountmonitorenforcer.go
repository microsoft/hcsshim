package policy

import (
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

// For testing. Records the number of calls to each method so we can verify
// the expected interactions took place.
type MountMonitoringEnforcer struct {
	DeviceMountCalls   int
	DeviceUnmountCalls int
	OverlayMountCalls  int
}

var _ securitypolicy.PolicyEnforcer = (*MountMonitoringEnforcer)(nil)

func (p *MountMonitoringEnforcer) EnforceDeviceMountPolicy(_ string, _ string) (err error) {
	p.DeviceMountCalls++
	return nil
}

func (p *MountMonitoringEnforcer) EnforceDeviceUnmountPolicy(_ string) (err error) {
	p.DeviceUnmountCalls++
	return nil
}

func (p *MountMonitoringEnforcer) EnforceOverlayMountPolicy(_ string, _ []string) (err error) {
	p.OverlayMountCalls++
	return nil
}

func (p *MountMonitoringEnforcer) EnforceCreateContainerPolicy(_ string, _ []string, _ []string) (err error) {
	return nil
}
