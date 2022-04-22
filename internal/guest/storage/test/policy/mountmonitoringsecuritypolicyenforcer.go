//go:build linux
// +build linux

package policy

import (
	oci "github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

// MountMonitoringSecurityPolicyEnforcer is used for testing and records the
// number of calls to each method, so we can verify the expected interactions
// took place.
type MountMonitoringSecurityPolicyEnforcer struct {
	DeviceMountCalls   int
	DeviceUnmountCalls int
	OverlayMountCalls  int
}

var _ securitypolicy.SecurityPolicyEnforcer = (*MountMonitoringSecurityPolicyEnforcer)(nil)

func (p *MountMonitoringSecurityPolicyEnforcer) EnforceDeviceMountPolicy(_ string, _ string) error {
	p.DeviceMountCalls++
	return nil
}

func (p *MountMonitoringSecurityPolicyEnforcer) EnforceDeviceUnmountPolicy(_ string) error {
	p.DeviceUnmountCalls++
	return nil
}

func (p *MountMonitoringSecurityPolicyEnforcer) EnforceOverlayMountPolicy(_ string, _ []string) error {
	p.OverlayMountCalls++
	return nil
}

func (MountMonitoringSecurityPolicyEnforcer) EnforceCreateContainerPolicy(_ string, _ []string, _ []string, _ string) error {
	return nil
}

func (MountMonitoringSecurityPolicyEnforcer) EnforceMountPolicy(_, _ string, _ *oci.Spec) error {
	return nil
}

func (MountMonitoringSecurityPolicyEnforcer) EnforceExpectedMountsPolicy(_ string, _ *oci.Spec) error {
	return nil
}

func (MountMonitoringSecurityPolicyEnforcer) ExtendDefaultMounts(_ []oci.Mount) error {
	return nil
}
