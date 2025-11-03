package securitypolicy

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type SecurityOptions struct {
	// state required for the security policy enforcement
	PolicyEnforcer    SecurityPolicyEnforcer
	PolicyEnforcerSet bool
	UvmReferenceInfo  string
	policyMutex       sync.Mutex
	logWriter         io.Writer
}

func NewSecurityOptions(enforcer SecurityPolicyEnforcer, enforcerSet bool, uvmReferenceInfo string, logWriter io.Writer) *SecurityOptions {
	return &SecurityOptions{
		PolicyEnforcer:    enforcer,
		PolicyEnforcerSet: enforcerSet,
		UvmReferenceInfo:  uvmReferenceInfo,
		logWriter:         logWriter,
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

	// This is one of two points at which we might change our logging.
	// At this time, we now have a policy and can determine what the policy
	// author put as policy around runtime logging.
	// The other point is on startup where we take a flag to set the default
	// policy enforcer to use before a policy arrives. After that flag is set,
	// we use the enforcer in question to set up logging as well.
	if err = s.PolicyEnforcer.EnforceRuntimeLoggingPolicy(ctx); err == nil {
		logrus.SetOutput(s.logWriter)
	} else {
		logrus.SetOutput(io.Discard)
	}

	s.PolicyEnforcer = p
	s.PolicyEnforcerSet = true
	s.UvmReferenceInfo = encodedUVMReference

	return nil
}
