package securitypolicy

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Microsoft/cosesign1go/pkg/cosesign1"
	didx509resolver "github.com/Microsoft/didx509go/pkg/did-x509-resolver"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
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

// Fragment extends current security policy with additional constraints
// from the incoming fragment. Note that it is base64 encoded over the bridge/
//
// There are three checking steps:
// 1 - Unpack the cose document and check it was actually signed with the cert
// chain inside its header
// 2 - Check that the issuer field did:x509 identifier is for that cert chain
// (ie fingerprint of a non leaf cert and the subject matches the leaf cert)
// 3 - Check that this issuer/feed match the requirement of the user provided
// security policy (done in the regoby LoadFragment)
func (s *SecurityOptions) InjectFragment(ctx context.Context, fragment *guestresource.SecurityPolicyFragment) (err error) {
	log.G(ctx).WithField("fragment", fmt.Sprintf("%+v", fragment)).Debug("VerifyAndExtractFragment")

	raw, err := base64.StdEncoding.DecodeString(fragment.Fragment)
	if err != nil {
		return fmt.Errorf("failed to decode fragment: %w", err)
	}
	blob := []byte(fragment.Fragment)
	// keep a copy of the fragment, so we can manually figure out what went wrong
	// will be removed eventually. Give it a unique name to avoid any potential
	// race conditions.
	sha := sha256.New()
	sha.Write(blob)
	timestamp := time.Now()
	fragmentPath := fmt.Sprintf("fragment-%x-%d.blob", sha.Sum(nil), timestamp.UnixMilli())
	_ = os.WriteFile(filepath.Join(os.TempDir(), fragmentPath), blob, 0644)

	unpacked, err := cosesign1.UnpackAndValidateCOSE1CertChain(raw)
	if err != nil {
		return fmt.Errorf("InjectFragment failed COSE validation: %w", err)
	}

	payloadString := string(unpacked.Payload[:])
	issuer := unpacked.Issuer
	feed := unpacked.Feed
	chainPem := unpacked.ChainPem

	log.G(ctx).WithFields(logrus.Fields{
		"issuer":   issuer, // eg the DID:x509:blah....
		"feed":     feed,
		"cty":      unpacked.ContentType,
		"chainPem": chainPem,
	}).Debugf("unpacked COSE1 cert chain")

	log.G(ctx).WithFields(logrus.Fields{
		"payload": payloadString,
	}).Tracef("unpacked COSE1 payload")

	if len(issuer) == 0 || len(feed) == 0 { // must both be present
		return fmt.Errorf("either issuer and feed must both be provided in the COSE_Sign1 protected header")
	}

	// Resolve returns a did doc that we don't need
	// we only care if there was an error or not
	_, err = didx509resolver.Resolve(unpacked.ChainPem, issuer, true)
	if err != nil {
		log.G(ctx).Printf("Badly formed fragment - did resolver failed to match fragment did:x509 from chain with purported issuer %s, feed %s - err %s", issuer, feed, err.Error())
		return fmt.Errorf("failed to resolve DID: %w", err)
	}

	// now offer the payload fragment to the policy
	err = s.PolicyEnforcer.LoadFragment(ctx, issuer, feed, payloadString)
	if err != nil {
		return fmt.Errorf("error loading security policy fragment: %w", err)
	}
	return nil
}
