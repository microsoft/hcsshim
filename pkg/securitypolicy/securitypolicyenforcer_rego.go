//go:build linux && rego
// +build linux,rego

package securitypolicy

import (
	"context"
	"errors"

	"github.com/Microsoft/hcsshim/internal/log"
	oci "github.com/opencontainers/runtime-spec/specs-go"
)

const regoEnforcer = "rego"

func init() {
	registeredEnforcers[regoEnforcer] = CreateRegoEnforcer
	defaultEnforcer = regoEnforcer
	log.G(context.Background()).Debugf("registered enforcers: %+v", registeredEnforcers)
}

type RegoEnforcer struct{}

var (
	_                 SecurityPolicyEnforcer = (*RegoEnforcer)(nil)
	ErrNotImplemented                        = errors.New("not implemented")
)

func CreateRegoEnforcer(_ SecurityPolicyState, _, _ []oci.Mount) (SecurityPolicyEnforcer, error) {
	return &RegoEnforcer{}, nil
}

func (RegoEnforcer) EnforceDeviceMountPolicy(_, _ string) error {
	return ErrNotImplemented
}

func (RegoEnforcer) EnforceDeviceUnmountPolicy(_ string) error {
	return ErrNotImplemented
}

func (RegoEnforcer) EnforceOverlayMountPolicy(_ string, _ []string) error {
	return ErrNotImplemented
}

func (RegoEnforcer) EnforceCreateContainerPolicy(_ string, _, _ []string, _ string) error {
	return ErrNotImplemented
}

func (RegoEnforcer) EnforceWaitMountPointsPolicy(_ string, _ *oci.Spec) error {
	return ErrNotImplemented
}

func (RegoEnforcer) EnforceMountPolicy(_, _ string, _ *oci.Spec) error {
	return ErrNotImplemented
}

func (RegoEnforcer) ExtendDefaultMounts(_ []oci.Mount) error {
	return ErrNotImplemented
}

func (RegoEnforcer) EncodedSecurityPolicy() string {
	return ""
}
