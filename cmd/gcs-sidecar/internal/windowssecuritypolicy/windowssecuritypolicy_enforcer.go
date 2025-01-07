//go:build windows
// +build windows

package windowssecuritypolicy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"syscall"

	oci "github.com/opencontainers/runtime-spec/specs-go"

	//specInternal "github.com/Microsoft/hcsshim/internal/guest/spec"
	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/pkg/errors"
)

type createEnforcerFunc func(base64EncodedPolicy string, criMounts, criPrivilegedMounts []oci.Mount, maxErrorMessageLength int) (SecurityPolicyEnforcer, error)

type EnvList []string

const (
	regoEnforcerName = "rego"
	openDoorEnforcer = "open_door"
)

var (
	registeredEnforcers = map[string]createEnforcerFunc{}
	defaultEnforcer     = regoEnforcerName
)

func init() {
	registeredEnforcers[regoEnforcerName] = createRegoEnforcer
	registeredEnforcers[openDoorEnforcer] = createOpenDoorEnforcer
	// Overriding the value inside init guarantees that this assignment happens
	// after the variable has been initialized in securitypolicy.go and there
	// are no race conditions. When multiple init functions are defined in a
	// single package, the order of their execution is determined by the
	// filename.
	defaultEnforcer = regoEnforcerName
	defaultMarshaller = regoMarshaller
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
	EnforceExecExternalProcessPolicy(ctx context.Context, argList []string, envList []string, workingDir string) (EnvList, bool, error)
	EnforceShutdownContainerPolicy(ctx context.Context, containerID string) error
	EnforceSignalContainerProcessPolicy(ctx context.Context, containerID string, signal syscall.Signal, isInitProcess bool, startupArgList []string) error
	EnforcePlan9MountPolicy(ctx context.Context, target string) (err error)
	EnforcePlan9UnmountPolicy(ctx context.Context, target string) (err error)
	EnforceGetPropertiesPolicy(ctx context.Context) error
	EnforceDumpStacksPolicy(ctx context.Context) error
	EnforceRuntimeLoggingPolicy(ctx context.Context) (err error)
	LoadFragment(ctx context.Context, issuer string, feed string, code string) error
	EnforceScratchMountPolicy(ctx context.Context, scratchPath string, encrypted bool) (err error)
	EnforceScratchUnmountPolicy(ctx context.Context, scratchPath string) (err error)
	GetUserInfo(containerID string, spec *oci.Process) (IDName, []IDName, string, error)
}

type stringSet map[string]struct{}

func (s stringSet) add(item string) {
	s[item] = struct{}{}
}

func (s stringSet) contains(item string) bool {
	_, contains := s[item]
	return contains
}

func newSecurityPolicyFromBase64JSON(base64EncodedPolicy string) (*SecurityPolicy, error) {
	// base64 decode the incoming policy string
	// its base64 encoded because it is coming from an annotation
	// annotations are a map of string to string
	// we want to store a complex json object so.... base64 it is
	jsonPolicy, err := base64.StdEncoding.DecodeString(base64EncodedPolicy)
	if err != nil {
		return nil, errors.Wrap(err, "unable to decode policy from Base64 format")
	}

	// json unmarshall the decoded to a SecurityPolicy
	securityPolicy := new(SecurityPolicy)
	err = json.Unmarshal(jsonPolicy, securityPolicy)
	if err != nil {
		return nil, errors.Wrap(err, "unable to unmarshal JSON policy")
	}

	return securityPolicy, nil
}

// createAllowAllEnforcer creates and returns OpenDoorSecurityPolicyEnforcer instance.
// Both AllowAll and Containers cannot be set at the same time.
func createOpenDoorEnforcer(base64EncodedPolicy string, _, _ []oci.Mount, _ int) (SecurityPolicyEnforcer, error) {
	// This covers the case when an "open_door" enforcer was requested, but no
	// actual security policy was passed. This can happen e.g. when a container
	// scratch is created for the first time.
	if base64EncodedPolicy == "" {
		return &OpenDoorSecurityPolicyEnforcer{}, nil
	}

	securityPolicy, err := newSecurityPolicyFromBase64JSON(base64EncodedPolicy)
	if err != nil {
		return nil, err
	}

	policyContainers := securityPolicy.Containers
	if !securityPolicy.AllowAll || policyContainers.Length > 0 || len(policyContainers.Elements) > 0 {
		return nil, ErrInvalidOpenDoorPolicy
	}
	return &OpenDoorSecurityPolicyEnforcer{
		encodedSecurityPolicy: base64EncodedPolicy,
	}, nil
}

// CreateSecurityPolicyEnforcer returns an appropriate enforcer for input parameters.
// When `enforcer` isn't return either an AllowAll or default enforcer.
// Returns an error if the requested `enforcer` implementation isn't registered.
func CreateSecurityPolicyEnforcer(
	enforcer string,
	base64EncodedPolicy string,
	criMounts,
	criPrivilegedMounts []oci.Mount,
	maxErrorMessageLength int,
) (SecurityPolicyEnforcer, error) {
	if enforcer == "" {
		enforcer = defaultEnforcer
		if base64EncodedPolicy == "" {
			enforcer = openDoorEnforcer
		}
	}
	if createEnforcer, ok := registeredEnforcers[enforcer]; !ok {
		return nil, fmt.Errorf("unknown enforcer: %q", enforcer)
	} else {
		return createEnforcer(base64EncodedPolicy, criMounts, criPrivilegedMounts, maxErrorMessageLength)
	}
}

// newMountConstraint creates an internal mount constraint object from given
// source, destination, type and options.
func newMountConstraint(src, dst string, mType string, mOpts []string) mountInternal {
	return mountInternal{
		Source:      src,
		Destination: dst,
		Type:        mType,
		Options:     mOpts,
	}
}

/*type standardEnforcerOpt func(e *StandardSecurityPolicyEnforcer) error

// WithPrivilegedMounts converts the input mounts to internal mount constraints
// and extends existing internal mount constraints if the container is allowed
// to be executed in elevated mode.
func WithPrivilegedMounts(mounts []oci.Mount) standardEnforcerOpt {
	return func(e *StandardSecurityPolicyEnforcer) error {
		for _, c := range e.Containers {
			if c.AllowElevated {
				for _, m := range mounts {
					mi := mountInternal{
						Source:      m.Source,
						Destination: m.Destination,
						Type:        m.Type,
						Options:     m.Options,
					}
					c.Mounts = append(c.Mounts, mi)
				}
			}
		}
		return nil
	}
}*/

// validate checks given OCI mount against mount policy. Destination is checked
// by direct string comparisons and Source is checked via a regular expression.
// This is done this way, because container path (Destination) is always fixed,
// however, the host/UVM path (Source) can include IDs generated at runtime and
// impossible to know in advance.
//
// NOTE: Different matching strategies can be added by introducing a separate
// path matching config, which isn't needed at the moment.
func (m *mountInternal) validate(mSpec oci.Mount) error {
	if m.Type != mSpec.Type {
		return fmt.Errorf("mount type not allowed by policy: expected=%q, actual=%q", m.Type, mSpec.Type)
	}
	if ok, _ := regexp.MatchString(m.Source, mSpec.Source); !ok {
		return fmt.Errorf("mount source not allowed by policy: expected=%q, actual=%q", m.Source, mSpec.Source)
	}
	if m.Destination != mSpec.Destination && m.Destination != "" {
		return fmt.Errorf("mount destination not allowed by policy: expected=%q, actual=%q", m.Destination, mSpec.Destination)
	}
	if !stringSlicesEqual(m.Options, mSpec.Options) {
		return fmt.Errorf("mount options not allowed by policy: expected=%q, actual=%q", m.Options, mSpec.Options)
	}
	return nil
}

// matchMount matches given OCI mount against mount constraints. If no match
// found, the mount is not allowed.
func (c *securityPolicyContainer) matchMount(sandboxID string, m oci.Mount) (err error) {
	for _, constraint := range c.Mounts {
		// now that we know the sandboxID we can get the actual path for
		// various destination path types by adding a UVM mount prefix
		constraint = substituteUVMPath(sandboxID, constraint)
		if err = constraint.validate(m); err == nil {
			return nil
		}
	}
	return fmt.Errorf("mount is not allowed by policy: %+v", m)
}

// substituteUVMPath substitutes mount prefix to an appropriate path inside
// UVM. At policy generation time, it's impossible to tell what the sandboxID
// will be, so the prefix substitution needs to happen during runtime.
func substituteUVMPath(sandboxID string, m mountInternal) mountInternal {
	if strings.HasPrefix(m.Source, guestpath.SandboxMountPrefix) {
		//m.Source = specInternal.SandboxMountSource(sandboxID, m.Source)
	} else if strings.HasPrefix(m.Source, guestpath.HugePagesMountPrefix) {
		//m.Source = specInternal.HugePagesMountSource(sandboxID, m.Source)
	}
	return m
}

func stringSlicesEqual(slice1, slice2 []string) bool {
	if len(slice1) != len(slice2) {
		return false
	}

	for i := 0; i < len(slice1); i++ {
		if slice1[i] != slice2[i] {
			return false
		}
	}
	return true
}


type OpenDoorSecurityPolicyEnforcer struct {
	encodedSecurityPolicy string
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

func (OpenDoorSecurityPolicyEnforcer) EnforceExecInContainerPolicy(_ context.Context, _ string, _ []string, envList []string, _ string, _ bool, _ IDName, _ []IDName, _ string, caps *oci.LinuxCapabilities) (EnvList, *oci.LinuxCapabilities, bool, error) {
	return envList, caps, true, nil
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

func (OpenDoorSecurityPolicyEnforcer) GetUserInfo(containerID string, spec *oci.Process) (IDName, []IDName, string, error) {
	return IDName{}, nil, "", nil
}

type ClosedDoorSecurityPolicyEnforcer struct {
	encodedSecurityPolicy string //nolint:unused
}

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

func (ClosedDoorSecurityPolicyEnforcer) EnforceExecInContainerPolicy(context.Context, string, []string, []string, string, bool, IDName, []IDName, string, *oci.LinuxCapabilities) (EnvList, *oci.LinuxCapabilities, bool, error) {
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

func (ClosedDoorSecurityPolicyEnforcer) GetUserInfo(containerID string, spec *oci.Process) (IDName, []IDName, string, error) {
	return IDName{}, nil, "", nil
}
