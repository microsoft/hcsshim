//go:build windows
// +build windows

package bridge

import (
	"context"
	"errors"
	"fmt"

	"github.com/Microsoft/hcsshim/cmd/gcs-sidecar/internal/windowssecuritypolicy"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

func (s *SecurityPoliyEnforcer) SetWCOWConfidentialUVMOptions(securityPolicyRequest *guestresource.WCOWConfidentialOptions) error {
	s.policyMutex.Lock()
	defer s.policyMutex.Unlock()

	if s.securityPolicyEnforcerSet {
		return errors.New("security policy has already been set")
	}

	// this limit ensures messages are below the character truncation limit that
	// can be imposed by an orchestrator
	maxErrorMessageLength := 3 * 1024

	// Initialize security policy enforcer for a given enforcer type and
	// encoded security policy.
	p, err := windowssecuritypolicy.CreateSecurityPolicyEnforcer(
		"rego",
		securityPolicyRequest.EncodedSecurityPolicy,
		DefaultCRIMounts(),
		DefaultCRIPrivilegedMounts(),
		maxErrorMessageLength,
	)
	if err != nil {
		return fmt.Errorf("error creating security policy enforcer: %v", err)
	}

	/*
			// TODO(kiashok): logging for c-wcow?

			// This is one of two points at which we might change our logging.
			// At this time, we now have a policy and can determine what the policy
			// author put as policy around runtime logging.
			// The other point is on startup where we take a flag to set the default
			// policy enforcer to use before a policy arrives. After that flag is set,
			// we use the enforcer in question to set up logging as well.
			if err = p.EnforceRuntimeLoggingPolicy(ctx); err == nil {
				logrus.SetOutput(h.logWriter)
			} else {
				logrus.SetOutput(io.Discard)
			}

		hostData, err := securitypolicy.NewSecurityPolicyDigest(r.EncodedSecurityPolicy)
		if err != nil {
			return err
		}

		if err := validateHostData(hostData[:]); err != nil {
			return err
		}
	*/

	s.securityPolicyEnforcer = p
	s.securityPolicyEnforcerSet = true
	// TODO(kiashok): Update the following
	// s.uvmReferenceInfo = s.EncodedUVMReference

	return nil
}

func ExecProcess(ctx context.Context, containerID string, params hcsschema.ProcessParameters) error {
	/*

		err = h.securityPolicyEnforcer.EnforceExecExternalProcessPolicy(
			ctx,
			params.CommandArgs,
			processParamEnvTOOCIEnv(params.Environment),
			params.WorkingDirectory,
		)
		if err != nil {
			return errors.Wrapf(err, "exec is denied due to policy")
		}
	*/
	return nil
}

func signalProcess(containerID string, processID uint32, signal guestrequest.SignalValueWCOW) error {
	/*
		err = h.securityPolicyEnforcer.EnforceSignalContainerProcessPolicy(ctx, containerID, signal, signalingInitProcess, startupArgList)
		if err != nil {
			return err
		}
	*/

	return nil
}
func resizeConsole(containerID string, height uint16, width uint16) error {
	// not validated in clcow
	return nil
}
