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
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/opencontainers/runtime-spec/specs-go"
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

// SetConfidentialOptions takes guestresource.ConfidentialOptions
// to set up our internal data structures we use to store and enforce
// security policy. The options can contain security policy enforcer type,
// encoded security policy and signed UVM reference information The security
// policy and uvm reference information can be further presented to workload
// containers for validation and attestation purposes.
func (s *SecurityOptions) SetConfidentialOptions(ctx context.Context, enforcerType string, encodedSecurityPolicy string, encodedUVMReference string) error {
	s.policyMutex.Lock()
	defer s.policyMutex.Unlock()

	if s.PolicyEnforcerSet {
		return errors.New("security policy has already been set")
	}

	hostData, err := NewSecurityPolicyDigest(encodedSecurityPolicy)
	if err != nil {
		return err
	}

	if err := validateHostData(hostData[:]); err != nil {
		return err
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

func writeFileInDir(dir string, filename string, data []byte, perm os.FileMode) error {
	st, err := os.Stat(dir)
	if err != nil {
		return err
	}

	if !st.IsDir() {
		return fmt.Errorf("not a directory %q", dir)
	}

	targetFilename := filepath.Join(dir, filename)
	return os.WriteFile(targetFilename, data, perm)
}

// Write security policy, signed UVM reference and host AMD certificate to
// container's rootfs, so that application and sidecar containers can have
// access to it. The security policy is required by containers which need to
// extract init-time claims found in the security policy. The directory path
// containing the files is exposed via UVM_SECURITY_CONTEXT_DIR env var.
// It may be an error to have a security policy but not expose it to the
// container as in that case it can never be checked as correct by a verifier.
func (s *SecurityOptions) WriteSecurityContextDir(spec *specs.Spec) error {
	encodedPolicy := s.PolicyEnforcer.EncodedSecurityPolicy()
	hostAMDCert := spec.Annotations[annotations.WCOWHostAMDCertificate]
	if len(encodedPolicy) > 0 || len(hostAMDCert) > 0 || len(s.UvmReferenceInfo) > 0 {
		// Use os.MkdirTemp to make sure that the directory is unique.
		securityContextDir, err := os.MkdirTemp(spec.Root.Path, SecurityContextDirTemplate)
		if err != nil {
			return fmt.Errorf("failed to create security context directory: %w", err)
		}
		// Make sure that files inside directory are readable
		if err := os.Chmod(securityContextDir, 0755); err != nil {
			return fmt.Errorf("failed to chmod security context directory: %w", err)
		}

		if len(encodedPolicy) > 0 {
			if err := writeFileInDir(securityContextDir, PolicyFilename, []byte(encodedPolicy), 0777); err != nil {
				return fmt.Errorf("failed to write security policy: %w", err)
			}
		}
		if len(s.UvmReferenceInfo) > 0 {
			if err := writeFileInDir(securityContextDir, ReferenceInfoFilename, []byte(s.UvmReferenceInfo), 0777); err != nil {
				return fmt.Errorf("failed to write UVM reference info: %w", err)
			}
		}

		if len(hostAMDCert) > 0 {
			if err := writeFileInDir(securityContextDir, HostAMDCertFilename, []byte(hostAMDCert), 0777); err != nil {
				return fmt.Errorf("failed to write host AMD certificate: %w", err)
			}
		}

		containerCtxDir := fmt.Sprintf("/%s", filepath.Base(securityContextDir))
		secCtxEnv := fmt.Sprintf("UVM_SECURITY_CONTEXT_DIR=%s", containerCtxDir)
		spec.Process.Env = append(spec.Process.Env, secCtxEnv)

	}
	return nil
}
