//go:build windows
// +build windows

package bridge

import (
	"errors"
	"fmt"
	"sync"

	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

type Host struct {
	// state required for the security policy enforcement
	policyMutex               sync.Mutex
	securityPolicyEnforcer    securitypolicy.SecurityPolicyEnforcer
	securityPolicyEnforcerSet bool
}

type SecurityPoliyEnforcer struct {
	// State required for the security policy enforcement
	securityPolicyEnforcer    securitypolicy.SecurityPolicyEnforcer
	securityPolicyEnforcerSet bool
}

func NewHost(initialEnforcer securitypolicy.SecurityPolicyEnforcer) *Host {
	return &Host{
		securityPolicyEnforcer:    initialEnforcer,
		securityPolicyEnforcerSet: false,
	}
}

func (h *Host) isSecurityPolicyEnforcerInitialized() bool { //nolint:unused
	return h.securityPolicyEnforcer != nil
}

func (h *Host) SetWCOWConfidentialUVMOptions(securityPolicyRequest *guestresource.WCOWConfidentialOptions) error {
	h.policyMutex.Lock()
	defer h.policyMutex.Unlock()

	if h.securityPolicyEnforcerSet {
		return errors.New("security policy has already been set")
	}

	// This limit ensures messages are below the character truncation limit that
	// can be imposed by an orchestrator
	maxErrorMessageLength := 3 * 1024

	// Initialize security policy enforcer for a given enforcer type and
	// encoded security policy.
	p, err := securitypolicy.CreateSecurityPolicyEnforcer(
		"rego",
		securityPolicyRequest.EncodedSecurityPolicy,
		DefaultCRIMounts(),
		DefaultCRIPrivilegedMounts(),
		maxErrorMessageLength,
	)
	if err != nil {
		return fmt.Errorf("error creating security policy enforcer: %w", err)
	}

	h.securityPolicyEnforcer = p
	h.securityPolicyEnforcerSet = true

	return nil
}
