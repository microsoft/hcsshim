package securitypolicy

import (
	"context"
	"fmt"
	"sync"

	"github.com/pkg/errors"
)

type SecurityOptions struct {
	// state required for the security policy enforcement
	PolicyEnforcer        SecurityPolicyEnforcer
	PolicyEnforcerSet     bool
	UvmReferenceInfo      string
	policyMutex           sync.Mutex
	EnforcerType          string
	EncodedSecurityPolicy string
}

func NewSecurityOptions(enforcer SecurityPolicyEnforcer, enforcerSet bool, uvmReferenceInfo string) *SecurityOptions {
	return &SecurityOptions{
		PolicyEnforcer:        enforcer,
		PolicyEnforcerSet:     enforcerSet,
		UvmReferenceInfo:      uvmReferenceInfo,
		EnforcerType:          "",
		EncodedSecurityPolicy: "",
	}
}

func (s *SecurityOptions) SetConfidentialOptions(ctx context.Context, enforcerType string, encodedSecurityPolicy string, encodedUVMReference string) error {
	s.policyMutex.Lock()
	defer s.policyMutex.Unlock()

	if s.PolicyEnforcerSet {
		return errors.New("security policy has already been set")
	}
	// This limit ensures messages are below the character truncation limit that
	// can be imposed by an orchestrator
	maxErrorMessageLength := 3 * 1024

	// Initialize security policy enforcer for a given enforcer type and
	// encoded security policy.
	p, err := CreateSecurityPolicyEnforcer(
		enforcerType,
		encodedSecurityPolicy,
		DefaultCRIMounts(),
		DefaultCRIPrivilegedMounts(),
		maxErrorMessageLength,
	)
	if err != nil {
		return fmt.Errorf("error creating security policy enforcer: %w", err)
	}

	s.PolicyEnforcer = p
	s.PolicyEnforcerSet = true
	s.UvmReferenceInfo = encodedUVMReference

	return nil
}
