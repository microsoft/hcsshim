package policy

import (
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

// For testing. Records the number of calls to each method so we can verify
// the expected interactions took place.
type MountMonitoringSecurityPolicyEnforcer struct {
	DeviceMountCalls   int
	DeviceUnmountCalls int
	OverlayMountCalls  int
}

var _ securitypolicy.SecurityPolicyEnforcer = (*MountMonitoringSecurityPolicyEnforcer)(nil)

func (p *MountMonitoringSecurityPolicyEnforcer) EnforceDeviceMountPolicy(target string, deviceHash string) (err error) {
	p.DeviceMountCalls++
	return nil
}

func (p *MountMonitoringSecurityPolicyEnforcer) EnforceDeviceUnmountPolicy(target string) (err error) {
	p.DeviceUnmountCalls++
	return nil
}

func (p *MountMonitoringSecurityPolicyEnforcer) EnforceOverlayMountPolicy(containerID string, layerPaths []string) (err error) {
	p.OverlayMountCalls++
	return nil
}

func (p *MountMonitoringSecurityPolicyEnforcer) EnforceCreateContainerPolicy(_ string, _ []string, _ []string, _ string) (err error) {
	return nil
}
