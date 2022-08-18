//go:build linux && rego
// +build linux,rego

package securitypolicy

import (
	"errors"

	oci "github.com/opencontainers/runtime-spec/specs-go"
)

const regoEnforcer = "rego"

func init() {
	registeredEnforcers[regoEnforcer] = createRegoEnforcer
	// Overriding the value inside init guarantees that this assignment happens
	// after the variable has been initialized in securitypolicy.go and there
	// are no race conditions. When multiple init functions are defined in a
	// single package, the order of their execution is determined by the
	// filename.
	defaultEnforcer = regoEnforcer
}

// RegoEnforcer is a stub implementation of a security policy, which will be
// based on [Rego] policy language. The detailed implementation will be
// introduced in the subsequent PRs and documentation updated accordingly.
//
// [Rego]: https://www.openpolicyagent.org/docs/latest/policy-language/
type RegoEnforcer struct{}

var (
	_                 SecurityPolicyEnforcer = (*RegoEnforcer)(nil)
	ErrNotImplemented                        = errors.New("not implemented")
)

func createRegoEnforcer(_ SecurityPolicyState, _, _ []oci.Mount) (SecurityPolicyEnforcer, error) {
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
