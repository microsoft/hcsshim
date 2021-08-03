package policy

import (
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

// For testing. Records the number of calls to each method so we can verify
// the expected interactions took place.
type MountMonitoringSecurityPolicyEnforcer struct {
	PmemMountCalls    int
	OverlayMountCalls int
}

var _ securitypolicy.SecurityPolicyEnforcer = (*MountMonitoringSecurityPolicyEnforcer)(nil)

func (p *MountMonitoringSecurityPolicyEnforcer) EnforcePmemMountPolicy(target string, deviceHash string) (err error) {
	p.PmemMountCalls++
	return nil
}

func (p *MountMonitoringSecurityPolicyEnforcer) EnforceOverlayMountPolicy(containerID string, layerPaths []string) (err error) {
	p.OverlayMountCalls++
	return nil
}
