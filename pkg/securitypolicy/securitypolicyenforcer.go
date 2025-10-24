package securitypolicy

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Microsoft/cosesign1go/pkg/cosesign1"
	didx509resolver "github.com/Microsoft/didx509go/pkg/did-x509-resolver"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type createEnforcerFunc func(base64EncodedPolicy string, criMounts, criPrivilegedMounts []oci.Mount, maxErrorMessageLength int) (SecurityPolicyEnforcer, error)

type EnvList []string

type ExecOptions struct {
	Groups          []IDName               // optional: empty slice or nil
	Umask           string                 // optional: "" means unspecified
	Capabilities    *oci.LinuxCapabilities // optional: nil means "none"
	NoNewPrivileges *bool                  // optional: nil means "not set"
}

type CreateContainerOptions struct {
	SandboxID            string
	Privileged           *bool
	NoNewPrivileges      *bool
	Groups               []IDName
	Umask                string
	Capabilities         *oci.LinuxCapabilities
	SeccompProfileSHA256 string
}

type SignalContainerOptions struct {
	IsInitProcess bool
	// One of these will be set depending on platform
	LinuxSignal   syscall.Signal
	WindowsSignal guestrequest.SignalValueWCOW

	LinuxStartupArgs []string
	WindowsCommand   []string
}

const (
	openDoorEnforcerName = "open_door"
)

var (
	registeredEnforcers = map[string]createEnforcerFunc{}

	// defaultConfidentialEnforcer is set to rego in
	// securitypolicyenforcer_rego.go if the relevant build tag is set.
	// Otherwise we do not support any confidential enforcers.
	defaultConfidentialEnforcer = ""
)

func init() {
	registeredEnforcers[openDoorEnforcerName] = createOpenDoorEnforcer
}

type SecurityPolicyEnforcer interface {
	EnforceDeviceMountPolicy(ctx context.Context, target string, deviceHash string) (err error)
	EnforceDeviceUnmountPolicy(ctx context.Context, unmountTarget string) (err error)
	EnforceOverlayMountPolicy(ctx context.Context, containerID string, layerPaths []string, target string) (err error)
	EnforceOverlayUnmountPolicy(ctx context.Context, target string) (err error)
	EnforceCreateContainerPolicy(
		ctx context.Context,
		sandboxID string,
		containerID string,
		argList []string,
		envList []string,
		workingDir string,
		mounts []oci.Mount,
		privileged bool,
		noNewPrivileges bool,
		user IDName,
		groups []IDName,
		umask string,
		capabilities *oci.LinuxCapabilities,
		seccompProfileSHA256 string,
	) (EnvList, *oci.LinuxCapabilities, bool, error)
	EnforceCreateContainerPolicyV2(
		ctx context.Context,
		containerID string,
		argList []string,
		envList []string,
		workingDir string,
		mounts []oci.Mount,
		user IDName,
		opts *CreateContainerOptions,
	) (EnvList, *oci.LinuxCapabilities, bool, error)
	ExtendDefaultMounts([]oci.Mount) error
	EncodedSecurityPolicy() string
	EnforceExecInContainerPolicy(
		ctx context.Context,
		containerID string,
		argList []string,
		envList []string,
		workingDir string,
		noNewPrivileges bool,
		user IDName,
		groups []IDName,
		umask string,
		capabilities *oci.LinuxCapabilities,
	) (EnvList, *oci.LinuxCapabilities, bool, error)
	EnforceExecInContainerPolicyV2(
		ctx context.Context,
		containerID string,
		argList []string,
		envList []string,
		workingDir string,
		user IDName,
		opts *ExecOptions,
	) (EnvList, *oci.LinuxCapabilities, bool, error)
	EnforceExecExternalProcessPolicy(ctx context.Context, argList []string, envList []string, workingDir string) (EnvList, bool, error)
	EnforceShutdownContainerPolicy(ctx context.Context, containerID string) error
	EnforceSignalContainerProcessPolicy(ctx context.Context, containerID string, signal syscall.Signal, isInitProcess bool, startupArgList []string) error
	EnforceSignalContainerProcessPolicyV2(ctx context.Context, containerID string, opts *SignalContainerOptions) error
	EnforcePlan9MountPolicy(ctx context.Context, target string) (err error)
	EnforcePlan9UnmountPolicy(ctx context.Context, target string) (err error)
	EnforceGetPropertiesPolicy(ctx context.Context) error
	EnforceDumpStacksPolicy(ctx context.Context) error
	EnforceRuntimeLoggingPolicy(ctx context.Context) (err error)
	LoadFragment(ctx context.Context, issuer string, feed string, rego string) error
	EnforceScratchMountPolicy(ctx context.Context, scratchPath string, encrypted bool) (err error)
	EnforceScratchUnmountPolicy(ctx context.Context, scratchPath string) (err error)
	GetUserInfo(spec *oci.Process, rootPath string) (IDName, []IDName, string, error)
	EnforceVerifiedCIMsPolicy(ctx context.Context, containerID string, layerHashes []string) (err error)
}

//nolint:unused
type stringSet map[string]struct{}

//nolint:unused
func (s stringSet) add(item string) {
	s[item] = struct{}{}
}

//nolint:unused
func (s stringSet) contains(item string) bool {
	_, contains := s[item]
	return contains
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
func ExtractAndVerifyFragment(ctx context.Context, fragment *guestresource.LCOWSecurityPolicyFragment) (issuer string, feed string, payloadString string, err error) {
	log.G(ctx).WithField("fragment", fmt.Sprintf("%+v", fragment)).Debug("VerifyAndExtractFragment")

	raw, err := base64.StdEncoding.DecodeString(fragment.Fragment)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to decode fragment: %w", err)
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
		return "", "", "", fmt.Errorf("InjectFragment failed COSE validation: %w", err)
	}

	payloadString = string(unpacked.Payload[:])
	issuer = unpacked.Issuer
	feed = unpacked.Feed
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
		return "", "", "", fmt.Errorf("either issuer and feed must both be provided in the COSE_Sign1 protected header")
	}

	// Resolve returns a did doc that we don't need
	// we only care if there was an error or not
	_, err = didx509resolver.Resolve(unpacked.ChainPem, issuer, true)
	if err != nil {
		log.G(ctx).Printf("Badly formed fragment - did resolver failed to match fragment did:x509 from chain with purported issuer %s, feed %s - err %s", issuer, feed, err.Error())
		return "", "", "", err
	}

	return issuer, feed, payloadString, nil
}

// CreateSecurityPolicyEnforcer returns an appropriate enforcer for input
// parameters.  Returns an error if the requested `enforcer` implementation
// isn't registered.
//
// This function can be called both on confidential and non-confidential
// containers, but in the non-confidential case the policy would be empty.
// Normally enforcer is not specified, in which case we use either the default
// for confidential (Rego), or the open door enforcer, depending on whether
// policy is not empty.  However, the host may override this.  This override is
// not measured in the SNP hostData, and so the enforcer must make sure the
// policy provided is a valid policy for that enforcer. (For example, for
// open_door, it must either be empty or contain only the "allow_all" field set
// to true.)
func CreateSecurityPolicyEnforcer(
	enforcer string,
	base64EncodedPolicy string,
	criMounts,
	criPrivilegedMounts []oci.Mount,
	maxErrorMessageLength int,
) (SecurityPolicyEnforcer, error) {
	if enforcer == "" {
		if base64EncodedPolicy == "" {
			enforcer = openDoorEnforcerName
		} else {
			if defaultConfidentialEnforcer == "" {
				return nil, fmt.Errorf("GCS not built with Rego support")
			}
			enforcer = defaultConfidentialEnforcer
		}
	}

	if createEnforcer, ok := registeredEnforcers[enforcer]; !ok {
		return nil, fmt.Errorf("unknown enforcer: %q", enforcer)
	} else {
		return createEnforcer(base64EncodedPolicy, criMounts, criPrivilegedMounts, maxErrorMessageLength)
	}
}

type OpenDoorSecurityPolicyEnforcer struct {
	encodedSecurityPolicy string
}

// Creates and returns OpenDoorSecurityPolicyEnforcer instance (used for
// non-confidential containers).  The provided base64EncodedPolicy must be
// empty.
func createOpenDoorEnforcer(base64EncodedPolicy string, _, _ []oci.Mount, _ int) (SecurityPolicyEnforcer, error) {
	if base64EncodedPolicy == "" {
		return &OpenDoorSecurityPolicyEnforcer{}, nil
	}

	return nil, ErrInvalidOpenDoorPolicy
}

var _ SecurityPolicyEnforcer = (*OpenDoorSecurityPolicyEnforcer)(nil)

func (OpenDoorSecurityPolicyEnforcer) EnforceDeviceMountPolicy(context.Context, string, string) error {
	return nil
}

func (OpenDoorSecurityPolicyEnforcer) EnforceDeviceUnmountPolicy(context.Context, string) error {
	return nil
}

func (OpenDoorSecurityPolicyEnforcer) EnforceOverlayMountPolicy(context.Context, string, []string, string) error {
	return nil
}

func (OpenDoorSecurityPolicyEnforcer) EnforceOverlayUnmountPolicy(context.Context, string) error {
	return nil
}

func (OpenDoorSecurityPolicyEnforcer) EnforceCreateContainerPolicy(_ context.Context, _, _ string, _ []string, envList []string, _ string, _ []oci.Mount, _ bool, _ bool, _ IDName, _ []IDName, _ string, caps *oci.LinuxCapabilities, _ string) (EnvList, *oci.LinuxCapabilities, bool, error) {
	return envList, caps, true, nil
}

func (OpenDoorSecurityPolicyEnforcer) EnforceCreateContainerPolicyV2(
	ctx context.Context,
	containerID string,
	argList []string,
	envList []string,
	workingDir string,
	mounts []oci.Mount,
	user IDName,
	opts *CreateContainerOptions,
) (EnvList, *oci.LinuxCapabilities, bool, error) {
	return envList, opts.Capabilities, true, nil
}

func (OpenDoorSecurityPolicyEnforcer) EnforceExecInContainerPolicy(_ context.Context, _ string, _ []string, envList []string, _ string, _ bool, _ IDName, _ []IDName, _ string, caps *oci.LinuxCapabilities) (EnvList, *oci.LinuxCapabilities, bool, error) {
	return envList, caps, true, nil
}

func (OpenDoorSecurityPolicyEnforcer) EnforceExecInContainerPolicyV2(
	ctx context.Context,
	containerID string,
	argList []string,
	envList []string,
	workingDir string,
	user IDName,
	opts *ExecOptions,
) (EnvList, *oci.LinuxCapabilities, bool, error) {
	return envList, opts.Capabilities, true, nil
}

func (OpenDoorSecurityPolicyEnforcer) EnforceExecExternalProcessPolicy(_ context.Context, _ []string, envList []string, _ string) (EnvList, bool, error) {
	return envList, true, nil
}

func (*OpenDoorSecurityPolicyEnforcer) EnforceShutdownContainerPolicy(context.Context, string) error {
	return nil
}

func (*OpenDoorSecurityPolicyEnforcer) EnforceSignalContainerProcessPolicy(context.Context, string, syscall.Signal, bool, []string) error {
	return nil
}

func (*OpenDoorSecurityPolicyEnforcer) EnforceSignalContainerProcessPolicyV2(ctx context.Context, containerID string, opts *SignalContainerOptions) error {
	return nil
}

func (*OpenDoorSecurityPolicyEnforcer) EnforcePlan9MountPolicy(context.Context, string) error {
	return nil
}

func (*OpenDoorSecurityPolicyEnforcer) EnforcePlan9UnmountPolicy(context.Context, string) error {
	return nil
}

func (OpenDoorSecurityPolicyEnforcer) EnforceGetPropertiesPolicy(context.Context) error {
	return nil
}

func (OpenDoorSecurityPolicyEnforcer) EnforceDumpStacksPolicy(context.Context) error {
	return nil
}

func (OpenDoorSecurityPolicyEnforcer) LoadFragment(context.Context, string, string, string) error {
	return nil
}

func (OpenDoorSecurityPolicyEnforcer) ExtendDefaultMounts([]oci.Mount) error {
	return nil
}

func (OpenDoorSecurityPolicyEnforcer) EnforceRuntimeLoggingPolicy(context.Context) error {
	return nil
}

func (oe *OpenDoorSecurityPolicyEnforcer) EncodedSecurityPolicy() string {
	return oe.encodedSecurityPolicy
}

func (OpenDoorSecurityPolicyEnforcer) EnforceScratchMountPolicy(context.Context, string, bool) error {
	return nil
}

func (OpenDoorSecurityPolicyEnforcer) EnforceScratchUnmountPolicy(context.Context, string) error {
	return nil
}

func (OpenDoorSecurityPolicyEnforcer) GetUserInfo(spec *oci.Process, rootPath string) (IDName, []IDName, string, error) {
	return IDName{}, nil, "", nil
}

func (OpenDoorSecurityPolicyEnforcer) EnforceVerifiedCIMsPolicy(ctx context.Context, containerID string, layerHashes []string) error {
	return nil
}

type ClosedDoorSecurityPolicyEnforcer struct{}

var _ SecurityPolicyEnforcer = (*ClosedDoorSecurityPolicyEnforcer)(nil)

func (ClosedDoorSecurityPolicyEnforcer) EnforceDeviceMountPolicy(context.Context, string, string) error {
	return errors.New("mounting is denied by policy")
}

func (ClosedDoorSecurityPolicyEnforcer) EnforceDeviceUnmountPolicy(context.Context, string) error {
	return errors.New("unmounting is denied by policy")
}

func (ClosedDoorSecurityPolicyEnforcer) EnforceOverlayMountPolicy(context.Context, string, []string, string) error {
	return errors.New("creating an overlay fs is denied by policy")
}

func (ClosedDoorSecurityPolicyEnforcer) EnforceOverlayUnmountPolicy(context.Context, string) error {
	return errors.New("removing an overlay fs is denied by policy")
}

func (ClosedDoorSecurityPolicyEnforcer) EnforceCreateContainerPolicy(context.Context, string, string, []string, []string, string, []oci.Mount, bool, bool, IDName, []IDName, string, *oci.LinuxCapabilities, string) (EnvList, *oci.LinuxCapabilities, bool, error) {
	return nil, nil, false, errors.New("running commands is denied by policy")
}

func (ClosedDoorSecurityPolicyEnforcer) EnforceCreateContainerPolicyV2(
	ctx context.Context,
	containerID string,
	argList []string,
	envList []string,
	workingDir string,
	mounts []oci.Mount,
	user IDName,
	opts *CreateContainerOptions,
) (EnvList, *oci.LinuxCapabilities, bool, error) {
	return nil, nil, false, errors.New("running commands is denied by policy")
}

func (ClosedDoorSecurityPolicyEnforcer) EnforceExecInContainerPolicy(context.Context, string, []string, []string, string, bool, IDName, []IDName, string, *oci.LinuxCapabilities) (EnvList, *oci.LinuxCapabilities, bool, error) {
	return nil, nil, false, errors.New("starting additional processes in a container is denied by policy")
}

func (ClosedDoorSecurityPolicyEnforcer) EnforceExecInContainerPolicyV2(
	ctx context.Context,
	containerID string,
	argList []string,
	envList []string,
	workingDir string,
	user IDName,
	opts *ExecOptions,
) (EnvList, *oci.LinuxCapabilities, bool, error) {
	return nil, nil, false, errors.New("starting additional processes in a container is denied by policy")
}

func (ClosedDoorSecurityPolicyEnforcer) EnforceExecExternalProcessPolicy(context.Context, []string, []string, string) (EnvList, bool, error) {
	return nil, false, errors.New("starting additional processes in uvm is denied by policy")
}

func (*ClosedDoorSecurityPolicyEnforcer) EnforceShutdownContainerPolicy(context.Context, string) error {
	return errors.New("shutting down containers is denied by policy")
}

func (*ClosedDoorSecurityPolicyEnforcer) EnforceSignalContainerProcessPolicy(context.Context, string, syscall.Signal, bool, []string) error {
	return errors.New("signalling container processes is denied by policy")
}

func (*ClosedDoorSecurityPolicyEnforcer) EnforceSignalContainerProcessPolicyV2(ctx context.Context, containerID string, opts *SignalContainerOptions) error {
	return errors.New("signalling container processes is denied by policy")
}

func (*ClosedDoorSecurityPolicyEnforcer) EnforcePlan9MountPolicy(context.Context, string) error {
	return errors.New("mounting is denied by policy")
}

func (*ClosedDoorSecurityPolicyEnforcer) EnforcePlan9UnmountPolicy(context.Context, string) error {
	return errors.New("unmounting is denied by policy")
}

func (ClosedDoorSecurityPolicyEnforcer) EnforceGetPropertiesPolicy(context.Context) error {
	return errors.New("getting container properties is denied by policy")
}

func (ClosedDoorSecurityPolicyEnforcer) EnforceDumpStacksPolicy(context.Context) error {
	return errors.New("getting stack dumps is denied by policy")
}

func (ClosedDoorSecurityPolicyEnforcer) LoadFragment(context.Context, string, string, string) error {
	return errors.New("loading fragments is denied by policy")
}

func (ClosedDoorSecurityPolicyEnforcer) ExtendDefaultMounts(_ []oci.Mount) error {
	return nil
}

func (ClosedDoorSecurityPolicyEnforcer) EnforceRuntimeLoggingPolicy(context.Context) error {
	return errors.New("runtime logging is denied by policy")
}

func (ClosedDoorSecurityPolicyEnforcer) EncodedSecurityPolicy() string {
	return ""
}

func (ClosedDoorSecurityPolicyEnforcer) EnforceScratchMountPolicy(context.Context, string, bool) error {
	return errors.New("mounting scratch is denied by the policy")
}

func (ClosedDoorSecurityPolicyEnforcer) EnforceScratchUnmountPolicy(context.Context, string) error {
	return errors.New("unmounting scratch is denied by the policy")
}

func (ClosedDoorSecurityPolicyEnforcer) GetUserInfo(spec *oci.Process, rootPath string) (IDName, []IDName, string, error) {
	return IDName{}, nil, "", nil
}

func (ClosedDoorSecurityPolicyEnforcer) EnforceVerifiedCIMsPolicy(ctx context.Context, containerID string, layerHashes []string) error {
	return nil
}
