package securitypolicy

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/Microsoft/cosesign1go/pkg/cosesign1"
	didx509resolver "github.com/Microsoft/didx509go/pkg/did-x509-resolver"
	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/ot"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/pkg/amdsevsnp"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/attribute"
)

type SecurityOptions struct {
	// state required for the security policy enforcement
	PolicyEnforcer               SecurityPolicyEnforcer
	PolicyEnforcerSet            bool
	UvmReferenceInfo             string
	UvmHashEnvelopeReferenceInfo string
	policyMutex                  sync.Mutex

	// Used for both tcbReferenceInfo and platformReferenceInfo
	platformReferenceInfoMutex sync.RWMutex
	tcbReferenceInfo           []byte
	platformReferenceInfo      []byte

	logWriter io.Writer
}

// Global counter for fragment injection requests, used to create unique
// deny / success marker files even when the same fragment is injected
// multiple times.
var FragmentRequestID atomic.Uint64

//go:embed fragments_info_README
var fragmentsInfoREADME []byte

func fragmentsPath() string {
	if osType == "windows" {
		return guestpath.WCOWFragmentsPath
	}
	return guestpath.LCOWFragmentsPath
}

// EnsureFragmentDiagnosticsDir creates the fragment diagnostics directory and
// its informational README.
func (s *SecurityOptions) EnsureFragmentDiagnosticsDir() error {
	dir := fragmentsPath()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "README"), fragmentsInfoREADME, 0644)
}

func NewSecurityOptions(enforcer SecurityPolicyEnforcer, enforcerSet bool, uvmReferenceInfo string, uvmHashEnvelopeReferenceInfo string, logWriter io.Writer) *SecurityOptions {
	return &SecurityOptions{
		PolicyEnforcer:               enforcer,
		PolicyEnforcerSet:            enforcerSet,
		UvmReferenceInfo:             uvmReferenceInfo,
		UvmHashEnvelopeReferenceInfo: uvmHashEnvelopeReferenceInfo,
		logWriter:                    logWriter,
	}
}

// SetConfidentialOptions takes guestresource.ConfidentialOptions
// to set up our internal data structures we use to store and enforce
// security policy. The options can contain security policy enforcer type,
// encoded security policy and signed UVM reference information The security
// policy and uvm reference information can be further presented to workload
// containers for validation and attestation purposes.
func (s *SecurityOptions) SetConfidentialOptions(ctx context.Context, enforcerType string, encodedSecurityPolicy string, encodedUVMReference string, encodedUVMHashEnvelopeReference string) error {
	s.policyMutex.Lock()
	defer s.policyMutex.Unlock()

	if s.PolicyEnforcerSet {
		return errors.New("security policy has already been set")
	}

	// Pre-create this directory so that we can mount this dir into
	// containers even if no fragments have been injected yet when the
	// container starts.
	if err := s.EnsureFragmentDiagnosticsDir(); err != nil {
		// This is not fatal, don't fail here.
		log.G(ctx).WithError(err).Error("failed to prepare injected fragments directory")
	}

	hostData, err := NewSecurityPolicyDigest(encodedSecurityPolicy)
	if err != nil {
		return err
	}

	if err := amdsevsnp.ValidateHostData(hostData[:]); err != nil {
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
	if err = p.EnforceRuntimeLoggingPolicy(ctx); err == nil {
		logrus.SetOutput(s.logWriter)
	} else {
		logrus.SetOutput(io.Discard)
	}

	s.PolicyEnforcer = p
	s.PolicyEnforcerSet = true
	s.UvmReferenceInfo = encodedUVMReference
	s.UvmHashEnvelopeReferenceInfo = encodedUVMHashEnvelopeReference

	return nil
}

// Media types carried by the fragment-injection delivery mechanism. The host
// may deliver blobs of different types through the same path; the guest decides
// how to treat each one based on its media type.
const (
	// mediaTypeFragment is a Rego security policy fragment. This is the default
	// when the host does not specify a media type (older hosts).
	mediaTypeFragment = "application/cose-x509+rego"
	// mediaTypeTransparencyTrustList is a signed Transparency Trust List (TTL).
	mediaTypeTransparencyTrustList = "application/vnd.transparency-trust-list.v1+cose"
	// mediaTypeTCBReferenceInfo is a TCB reference information COSE blob made
	// available to subsequently created containers.
	mediaTypeTCBReferenceInfo = "application/vnd.tcb-reference-info.v1+cose"
	// mediaTypePlatformReferenceInfo is a platform reference information COSE
	// blob made available to subsequently created containers.
	mediaTypePlatformReferenceInfo = "application/vnd.platform-reference-info.v1+cose"
)

// asInt64 coerces a CBOR-decoded integer value (which may be returned as
// int64, uint64 or int by different decoders) to an int64.
func asInt64(v interface{}) (int64, error) {
	switch n := v.(type) {
	case int64:
		return n, nil
	case int:
		return int64(n), nil
	case uint64:
		if n > math.MaxInt64 {
			return 0, errors.New("unable to convert uint64 to int64 due to overflow")
		}
		return int64(n), nil
	case uint:
		// uint is 64bit on 64bit platforms, so can overflow int64
		if n > math.MaxInt64 {
			return 0, errors.New("unable to convert uint to int64 due to overflow")
		}
		return int64(n), nil
	default:
		return 0, errors.Errorf("expected integer type, got %T", v)
	}
}

func writeFileIfNotExists(filename string, data []byte, perm os.FileMode) error {
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if os.IsExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func writeFragmentMetadata(ctx context.Context, filename, issuer, feed string, headerSVN *int64) {
	metadata := struct {
		Issuer    string `json:"issuer"`
		Feed      string `json:"feed"`
		HeaderSVN *int64 `json:"headerSvn"`
	}{
		Issuer:    issuer,
		Feed:      feed,
		HeaderSVN: headerSVN,
	}
	contents, err := json.Marshal(metadata)
	if err != nil {
		log.G(ctx).WithError(err).Warn("failed to marshal injected fragment metadata")
		return
	}
	if err := writeFileIfNotExists(filename, contents, 0644); err != nil {
		log.G(ctx).WithError(err).Warn("failed to write injected fragment metadata")
	}
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
	ctx, span := ot.StartSpan(ctx, "securitypolicy::InjectFragment")
	defer span.End()
	defer func() { ot.SetSpanStatus(span, err) }()
	span.SetAttributes(attribute.String("fragment", fmt.Sprintf("%+v", fragment)))
	currReqID := FragmentRequestID.Add(1)

	// An empty media type defaults to a Rego policy fragment, for backward
	// compatibility with older hosts that do not set the field.
	mediaType := fragment.MediaType
	if mediaType == "" {
		mediaType = mediaTypeFragment
	}

	raw, err := base64.StdEncoding.DecodeString(fragment.Fragment)
	if err != nil {
		return fmt.Errorf("failed to decode fragment: %w", err)
	}
	sha := sha256.Sum256(raw)
	shaHex := hex.EncodeToString(sha[:])
	thisFragmentDir := filepath.Join(fragmentsPath(), shaHex)
	defer func() {
		markerName := fmt.Sprintf("%d.succeed", currReqID)
		var markerContents []byte
		if err != nil {
			markerName = fmt.Sprintf("%d.deny", currReqID)
			markerContents = []byte(err.Error())
		}
		if markerErr := os.WriteFile(filepath.Join(thisFragmentDir, markerName), markerContents, 0644); markerErr != nil {
			log.G(ctx).WithError(markerErr).Warnf("failed to write injected fragment outcome marker %s", markerName)
		}
	}()

	if err := os.MkdirAll(thisFragmentDir, 0755); err != nil {
		return fmt.Errorf("failed to create injected fragment directory: %w", err)
	}
	if err := writeFileIfNotExists(filepath.Join(thisFragmentDir, "fragment.cose"), raw, 0644); err != nil {
		return fmt.Errorf("failed to write fragment.cose: %w", err)
	}
	switch mediaType {
	case mediaTypeFragment, mediaTypeTransparencyTrustList, mediaTypeTCBReferenceInfo, mediaTypePlatformReferenceInfo:
	default:
		// The host (azcri) only ever injects blobs whose media type it knows
		// we handle, so receiving an unrecognized one means either a host bug
		// or a newer host paired with an older guest. Fail loudly rather than
		// silently ignoring it; a failed injection is non-fatal to the host.
		return fmt.Errorf("cannot inject fragment blob with unsupported media type %q", mediaType)
	}

	unpacked, err := cosesign1.UnpackAndValidateCOSE1CertChain(raw)
	if err != nil {
		return fmt.Errorf("InjectFragment failed COSE validation: %w", err)
	}
	if err := writeFileIfNotExists(filepath.Join(thisFragmentDir, "fragment"), unpacked.Payload, 0644); err != nil {
		return fmt.Errorf("failed to write injected fragment payload: %w", err)
	}

	// We do not need to validate this.
	if mediaType == mediaTypeTCBReferenceInfo || mediaType == mediaTypePlatformReferenceInfo {
		s.platformReferenceInfoMutex.Lock()
		if mediaType == mediaTypeTCBReferenceInfo {
			s.tcbReferenceInfo = append(s.tcbReferenceInfo[:0], raw...)
		} else {
			s.platformReferenceInfo = append(s.platformReferenceInfo[:0], raw...)
		}
		s.platformReferenceInfoMutex.Unlock()
		return nil
	}

	cwtClaimsRaw, hasCwtClaims := unpacked.Protected[cosesign1.COSE_Header_CWTClaims]
	var cwtClaims map[any]any
	if hasCwtClaims {
		var ok bool
		cwtClaims, ok = cwtClaimsRaw.(map[any]any)
		if !ok {
			return fmt.Errorf("CWT claims header present, expected it to be a map[any]any, but got %T", cwtClaimsRaw)
		}
	}

	payloadString := string(unpacked.Payload[:])
	issuer := unpacked.Issuer
	feed := unpacked.Feed
	chainPem := unpacked.ChainPem

	log.G(ctx).WithFields(logrus.Fields{
		"issuer":    issuer, // eg the DID:x509:blah....
		"feed":      feed,
		"cty":       unpacked.ContentType,
		"chainPem":  chainPem,
		"cwtClaims": cwtClaims,
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

	var svnFromCwt *int64 = nil
	if hasCwtClaims {
		svnFromCwtRaw, hasSvn := cwtClaims["svn"]
		if hasSvn {
			svn, err := asInt64(svnFromCwtRaw)
			if err != nil {
				return errors.Wrap(err, "SVN present in CWT claims, but failed to convert it to int64")
			}
			svnFromCwt = &svn
		}
	}
	writeFragmentMetadata(ctx, filepath.Join(thisFragmentDir, "metadata.json"), issuer, feed, svnFromCwt)

	switch mediaType {
	case mediaTypeTransparencyTrustList:
		// A TTL must carry its SVN in the COSE header; there is nowhere else for
		// it to go.
		if svnFromCwt == nil {
			return fmt.Errorf("transparency trust list is missing an SVN in its CWT claims")
		}

		parsedTTL, err := cosesign1.ParseTTLPayload(unpacked.Payload)
		if err != nil {
			return errors.Wrap(err, "failed to parse transparency trust list payload")
		}

		// feed is the "subject" in the new-style envelope terminology.
		log.G(ctx).WithFields(logrus.Fields{
			"issuer":     issuer,
			"subject":    feed,
			"svn":        svnFromCwt,
			"numLedgers": len(parsedTTL),
		}).Debugf("Offering transparency trust list containing %d ledgers with issuer %s subject %s to policy enforcer", len(parsedTTL), issuer, feed)
		if log.G(ctx).Logger.IsLevelEnabled(logrus.TraceLevel) {
			for ledgerName, ledger := range parsedTTL {
				for keyID, entry := range ledger {
					log.G(ctx).WithFields(logrus.Fields{
						"issuer":  issuer,
						"subject": feed,
						"svn":     svnFromCwt,
						"ledger":  ledgerName,
						"keyID":   keyID,
					}).Tracef("Transparency trust list contains entry for ledger %s kid %s entry %T", ledgerName, keyID, entry)
				}
			}
		}
		if err := s.PolicyEnforcer.LoadTransparencyTrustList(ctx, LoadTransparencyTrustListOptions{
			Issuer:    issuer,
			Subject:   feed,
			SVN:       *svnFromCwt,
			ParsedTTL: parsedTTL,
		}); err != nil {
			return errors.Wrap(err, "error loading transparency trust list")
		}

	case mediaTypeFragment:
		// now offer the payload fragment to the policy
		fragmentOptions := LoadFragmentOptions{
			Issuer:    issuer,
			Feed:      feed,
			HeaderSVN: svnFromCwt,
			Rego:      payloadString,
			Receipts:  unpacked.Receipts,
		}
		log.G(ctx).WithFields(logrus.Fields{
			"issuer":      issuer,
			"feed":        feed,
			"headerSvn":   svnFromCwt,
			"numReceipts": len(unpacked.Receipts),
		}).Debugf("Offering fragment with issuer %s feed %s to policy enforcer with %d receipts", issuer, feed, len(unpacked.Receipts))
		log.G(ctx).WithFields(logrus.Fields{
			"feed": feed,
			"rego": payloadString,
		}).Tracef("Fragment Rego content")
		if err = s.PolicyEnforcer.LoadFragment(ctx, fragmentOptions); err != nil {
			return errors.Wrap(err, "error loading security policy fragment")
		}
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
//
// On success it returns the path (in the UVM/guest namespace) of the created
// security context directory, or an empty string if no directory was created
// because there was nothing to write.
func (s *SecurityOptions) WriteSecurityContextDir(spec *specs.Spec) (string, error) {
	encodedPolicy := s.PolicyEnforcer.EncodedSecurityPolicy()
	s.platformReferenceInfoMutex.RLock()
	var tcbReferenceInfoBase64, platformReferenceInfoBase64 string
	if len(s.tcbReferenceInfo) > 0 {
		tcbReferenceInfoBase64 = base64.StdEncoding.EncodeToString(s.tcbReferenceInfo)
	}
	if len(s.platformReferenceInfo) > 0 {
		platformReferenceInfoBase64 = base64.StdEncoding.EncodeToString(s.platformReferenceInfo)
	}
	s.platformReferenceInfoMutex.RUnlock()
	hostAMDCert := spec.Annotations[annotations.WCOWHostAMDCertificate]
	if len(encodedPolicy) > 0 || len(hostAMDCert) > 0 || len(s.UvmReferenceInfo) > 0 || len(s.UvmHashEnvelopeReferenceInfo) > 0 || len(tcbReferenceInfoBase64) > 0 || len(platformReferenceInfoBase64) > 0 {
		// Use os.MkdirTemp to make sure that the directory is unique.
		securityContextDir, err := os.MkdirTemp(spec.Root.Path, SecurityContextDirTemplate)
		if err != nil {
			return "", fmt.Errorf("failed to create security context directory: %w", err)
		}
		// Make sure that files inside directory are readable
		if err := os.Chmod(securityContextDir, 0755); err != nil {
			return "", fmt.Errorf("failed to chmod security context directory: %w", err)
		}

		if len(encodedPolicy) > 0 {
			if err := writeFileInDir(securityContextDir, PolicyFilename, []byte(encodedPolicy), 0777); err != nil {
				return "", fmt.Errorf("failed to write security policy: %w", err)
			}
		}
		if len(s.UvmReferenceInfo) > 0 {
			if err := writeFileInDir(securityContextDir, ReferenceInfoFilename, []byte(s.UvmReferenceInfo), 0777); err != nil {
				return "", fmt.Errorf("failed to write UVM reference info: %w", err)
			}
		}
		if len(s.UvmHashEnvelopeReferenceInfo) > 0 {
			if err := writeFileInDir(securityContextDir, HashEnvelopeReferenceInfoFilename, []byte(s.UvmHashEnvelopeReferenceInfo), 0777); err != nil {
				return "", fmt.Errorf("failed to write UVM hash envelope reference info: %w", err)
			}
		}
		if len(tcbReferenceInfoBase64) > 0 {
			if err := writeFileInDir(securityContextDir, TCBReferenceInfoFilename, []byte(tcbReferenceInfoBase64), 0777); err != nil {
				return "", fmt.Errorf("failed to write TCB reference info: %w", err)
			}
		}
		if len(platformReferenceInfoBase64) > 0 {
			if err := writeFileInDir(securityContextDir, PlatformReferenceInfoFilename, []byte(platformReferenceInfoBase64), 0777); err != nil {
				return "", fmt.Errorf("failed to write platform reference info: %w", err)
			}
		}

		if len(hostAMDCert) > 0 {
			if err := writeFileInDir(securityContextDir, HostAMDCertFilename, []byte(hostAMDCert), 0777); err != nil {
				return "", fmt.Errorf("failed to write host AMD certificate: %w", err)
			}
		}

		containerCtxDir := fmt.Sprintf("/%s", filepath.Base(securityContextDir))
		secCtxEnv := fmt.Sprintf("UVM_SECURITY_CONTEXT_DIR=%s", containerCtxDir)
		spec.Process.Env = append(spec.Process.Env, secCtxEnv)

		return securityContextDir, nil
	}
	return "", nil
}
