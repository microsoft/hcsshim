package securitypolicy

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/pkg/errors"
)

type EnvVarRule string

const (
	EnvVarRuleString EnvVarRule = "string"
	EnvVarRuleRegex  EnvVarRule = "re2"
)

// PolicyConfig contains toml or JSON config for security policy.
type PolicyConfig struct {
	AllowAll   bool              `json:"allow_all" toml:"allow_all"`
	Containers []ContainerConfig `json:"containers" toml:"container"`
}

// AuthConfig contains toml or JSON config for registry authentication.
type AuthConfig struct {
	Username string `json:"username" toml:"username"`
	Password string `json:"password" toml:"password"`
}

// EnvRuleConfig contains toml or JSON config for environment variable
// security policy enforcement.
type EnvRuleConfig struct {
	Strategy EnvVarRule `json:"strategy" toml:"strategy"`
	Rule     string     `json:"rule" toml:"rule"`
}

// ContainerConfig contains toml or JSON config for container described
// in security policy.
type ContainerConfig struct {
	ImageName      string          `json:"image_name" toml:"image_name"`
	Command        []string        `json:"command" toml:"command"`
	Auth           AuthConfig      `json:"auth" toml:"auth"`
	EnvRules       []EnvRuleConfig `json:"env_rules" toml:"env_rule"`
	WorkingDir     string          `json:"working_dir" toml:"working_dir"`
	ExpectedMounts []string        `json:"expected_mounts" toml:"expected_mounts"`
	Mounts         []MountConfig   `json:"mounts" toml:"mount"`
}

// MountConfig contains toml or JSON config for mount security policy
// constraint description.
type MountConfig struct {
	HostPath      string `json:"host_path" toml:"host_path"`
	ContainerPath string `json:"container_path" toml:"container_path"`
	Readonly      bool   `json:"readonly" toml:"readonly"`
}

// NewContainerConfig creates a new ContainerConfig from the given values.
func NewContainerConfig(
	imageName string,
	command []string,
	envRules []EnvRuleConfig,
	auth AuthConfig,
	workingDir string,
	expectedMounts []string,
	mounts []MountConfig,
) ContainerConfig {
	return ContainerConfig{
		ImageName:      imageName,
		Command:        command,
		EnvRules:       envRules,
		Auth:           auth,
		WorkingDir:     workingDir,
		ExpectedMounts: expectedMounts,
		Mounts:         mounts,
	}
}

// NewEnvVarRules creates slice of EnvRuleConfig's from environment variables
// strings slice.
func NewEnvVarRules(envVars []string) []EnvRuleConfig {
	var rules []EnvRuleConfig
	for _, env := range envVars {
		r := EnvRuleConfig{
			Strategy: EnvVarRuleString,
			Rule:     env,
		}
		rules = append(rules, r)
	}
	return rules
}

// NewOpenDoorPolicy creates a new SecurityPolicy with AllowAll set to `true`
func NewOpenDoorPolicy() *SecurityPolicy {
	return &SecurityPolicy{
		AllowAll: true,
	}
}

// NewSecurityPolicyDigest decodes base64 encoded policy string, computes
// and returns sha256 digest
func NewSecurityPolicyDigest(base64policy string) ([]byte, error) {
	jsonPolicy, err := base64.StdEncoding.DecodeString(base64policy)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 security policy: %w", err)
	}
	digest := sha256.New()
	digest.Write(jsonPolicy)
	digestBytes := digest.Sum(nil)
	return digestBytes, nil
}

// SecurityPolicyState is a structure that holds user supplied policy to enforce
// we keep both the encoded representation and the unmarshalled representation
// because different components need to have access to either of these
type SecurityPolicyState struct {
	EncodedSecurityPolicy EncodedSecurityPolicy `json:"EncodedSecurityPolicy,omitempty"`
	SecurityPolicy        `json:"SecurityPolicy,omitempty"`
}

// EncodedSecurityPolicy is a JSON representation of SecurityPolicy that has
// been base64 encoded for storage in an annotation embedded within another
// JSON configuration
type EncodedSecurityPolicy struct {
	SecurityPolicy string `json:"SecurityPolicy,omitempty"`
}

// NewSecurityPolicyState constructs SecurityPolicyState from base64Policy
// string. It first decodes base64 policy and returns the security policy
// struct and encoded security policy for given policy. The security policy
// is transmitted as json in an annotation, so we first have to remove the
// base64 encoding that allows the JSON based policy to be passed as a string.
// From there, we decode the JSON and set up our security policy struct
func NewSecurityPolicyState(base64Policy string) (*SecurityPolicyState, error) {
	// construct an encoded security policy that holds the base64 representation
	encodedSecurityPolicy := EncodedSecurityPolicy{
		SecurityPolicy: base64Policy,
	}

	// base64 decode the incoming policy string
	// its base64 encoded because it is coming from an annotation
	// annotations are a map of string to string
	// we want to store a complex json object so.... base64 it is
	jsonPolicy, err := base64.StdEncoding.DecodeString(base64Policy)
	if err != nil {
		return nil, errors.Wrap(err, "unable to decode policy from Base64 format")
	}

	// json unmarshall the decoded to a SecurityPolicy
	securityPolicy := SecurityPolicy{}
	err = json.Unmarshal(jsonPolicy, &securityPolicy)
	if err != nil {
		return nil, errors.Wrap(err, "unable to unmarshal JSON policy")
	}

	return &SecurityPolicyState{
		SecurityPolicy:        securityPolicy,
		EncodedSecurityPolicy: encodedSecurityPolicy,
	}, nil
}

type SecurityPolicy struct {
	// Flag that when set to true allows for all checks to pass. Currently, used
	// to run with security policy enforcement "running dark"; checks can be in
	// place but the default policy that is created on startup has AllowAll set
	// to true, thus making policy enforcement effectively "off" from a logical
	// standpoint. Policy enforcement isn't actually off as the policy is "allow
	// everything".
	AllowAll bool `json:"allow_all"`
	// One or more containers that are allowed to run
	Containers Containers `json:"containers"`
}

// EncodeToString returns base64 encoded string representation of SecurityPolicy.
func (sp *SecurityPolicy) EncodeToString() (string, error) {
	jsn, err := json.Marshal(sp)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(jsn), nil
}

type Containers struct {
	Length   int                  `json:"length"`
	Elements map[string]Container `json:"elements"`
}

type Container struct {
	Command        CommandArgs    `json:"command"`
	EnvRules       EnvRules       `json:"env_rules"`
	Layers         Layers         `json:"layers"`
	WorkingDir     string         `json:"working_dir"`
	ExpectedMounts ExpectedMounts `json:"expected_mounts"`
	Mounts         Mounts         `json:"mounts"`
}

// StringArrayMap wraps an array of strings as a string map.
type StringArrayMap struct {
	Length   int               `json:"length"`
	Elements map[string]string `json:"elements"`
}

type Layers StringArrayMap

type CommandArgs StringArrayMap

type ExpectedMounts StringArrayMap

type Options StringArrayMap

type EnvRules struct {
	Length   int                      `json:"length"`
	Elements map[string]EnvRuleConfig `json:"elements"`
}

type Mount struct {
	Source      string  `json:"source"`
	Destination string  `json:"destination"`
	Type        string  `json:"type"`
	Options     Options `json:"options"`
}

type Mounts struct {
	Length   int              `json:"length"`
	Elements map[string]Mount `json:"elements"`
}

// CreateContainerPolicy creates a new Container policy instance from the
// provided constraints or an error if parameter validation fails.
func CreateContainerPolicy(
	command, layers []string,
	envRules []EnvRuleConfig,
	workingDir string,
	eMounts []string,
	mounts []MountConfig,
) (*Container, error) {
	if err := validateEnvRules(envRules); err != nil {
		return nil, err
	}
	if err := validateMountConstraint(mounts); err != nil {
		return nil, err
	}
	return &Container{
		Command:        newCommandArgs(command),
		Layers:         newLayers(layers),
		EnvRules:       newEnvRules(envRules),
		WorkingDir:     workingDir,
		ExpectedMounts: newExpectedMounts(eMounts),
		Mounts:         newMountConstraints(mounts),
	}, nil
}

// NewSecurityPolicy creates a new SecurityPolicy from the provided values.
func NewSecurityPolicy(allowAll bool, containers []*Container) *SecurityPolicy {
	containersMap := map[string]Container{}
	for i, c := range containers {
		containersMap[strconv.Itoa(i)] = *c
	}
	return &SecurityPolicy{
		AllowAll: allowAll,
		Containers: Containers{
			Elements: containersMap,
		},
	}
}

func validateEnvRules(rules []EnvRuleConfig) error {
	for _, rule := range rules {
		switch rule.Strategy {
		case EnvVarRuleRegex:
			if _, err := regexp.Compile(rule.Rule); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateMountConstraint(mounts []MountConfig) error {
	for _, m := range mounts {
		if _, err := regexp.Compile(m.HostPath); err != nil {
			return err
		}
	}
	return nil
}

func newCommandArgs(args []string) CommandArgs {
	command := map[string]string{}
	for i, arg := range args {
		command[strconv.Itoa(i)] = arg
	}
	return CommandArgs{
		Elements: command,
	}
}

func newEnvRules(rs []EnvRuleConfig) EnvRules {
	envRules := map[string]EnvRuleConfig{}
	for i, r := range rs {
		envRules[strconv.Itoa(i)] = r
	}
	return EnvRules{
		Elements: envRules,
	}
}

func newLayers(ls []string) Layers {
	layers := map[string]string{}
	for i, l := range ls {
		layers[strconv.Itoa(i)] = l
	}
	return Layers{
		Elements: layers,
	}
}

func newExpectedMounts(em []string) ExpectedMounts {
	mounts := map[string]string{}
	for i, m := range em {
		mounts[strconv.Itoa(i)] = m
	}
	return ExpectedMounts{
		Elements: mounts,
	}
}

func newMountOptions(opts []string) Options {
	mountOpts := map[string]string{}
	for i, o := range opts {
		mountOpts[strconv.Itoa(i)] = o
	}
	return Options{
		Elements: mountOpts,
	}
}

// newOptionsFromConfig applies the same logic as CRI plugin to generate
// mount options given readonly and propagation config.
// TODO: (anmaxvl) update when support for other mount types is added,
//   e.g., vhd:// or evd://
// TODO: (anmaxvl) Do we need to set/validate Linux rootfs propagation?
//   In case we do, update securityPolicyContainer and Container structs
//   as well as mount enforcement logic.
func newOptionsFromConfig(mCfg *MountConfig) []string {
	mountOpts := []string{"rbind"}

	if strings.HasPrefix(mCfg.HostPath, guestpath.SandboxMountPrefix) ||
		strings.HasPrefix(mCfg.HostPath, guestpath.HugePagesMountPrefix) {
		mountOpts = append(mountOpts, "rshared")
	} else {
		mountOpts = append(mountOpts, "rprivate")
	}

	if mCfg.Readonly {
		mountOpts = append(mountOpts, "ro")
	} else {
		mountOpts = append(mountOpts, "rw")
	}
	return mountOpts
}

// newMountTypeFromConfig mimics the behavior in CRI when figuring out OCI
// mount type.
func newMountTypeFromConfig(mCfg *MountConfig) string {
	if strings.HasPrefix(mCfg.HostPath, guestpath.SandboxMountPrefix) ||
		strings.HasPrefix(mCfg.HostPath, guestpath.HugePagesMountPrefix) {
		return "bind"
	}
	return "none"
}

// newMountFromConfig converts user provided MountConfig into internal representation
// of mount constraint.
func newMountFromConfig(mCfg *MountConfig) Mount {
	opts := newOptionsFromConfig(mCfg)
	return Mount{
		Source:      mCfg.HostPath,
		Destination: mCfg.ContainerPath,
		Type:        newMountTypeFromConfig(mCfg),
		Options:     newMountOptions(opts),
	}
}

// newMountConstraints creates Mounts from a given array of MountConfig's.
func newMountConstraints(mountConfigs []MountConfig) Mounts {
	mounts := map[string]Mount{}
	for i, mc := range mountConfigs {
		mounts[strconv.Itoa(i)] = newMountFromConfig(&mc)
	}
	return Mounts{
		Elements: mounts,
	}
}

// Custom JSON marshalling to add `length` field that matches the number of
// elements present in the `elements` field.

func (c Containers) MarshalJSON() ([]byte, error) {
	type Alias Containers
	return json.Marshal(&struct {
		Length int `json:"length"`
		*Alias
	}{
		Length: len(c.Elements),
		Alias:  (*Alias)(&c),
	})
}

func (e EnvRules) MarshalJSON() ([]byte, error) {
	type Alias EnvRules
	return json.Marshal(&struct {
		Length int `json:"length"`
		*Alias
	}{
		Length: len(e.Elements),
		Alias:  (*Alias)(&e),
	})
}

func (s StringArrayMap) MarshalJSON() ([]byte, error) {
	type Alias StringArrayMap
	return json.Marshal(&struct {
		Length int `json:"length"`
		*Alias
	}{
		Length: len(s.Elements),
		Alias:  (*Alias)(&s),
	})
}

func (c CommandArgs) MarshalJSON() ([]byte, error) {
	return json.Marshal(StringArrayMap(c))
}

func (l Layers) MarshalJSON() ([]byte, error) {
	return json.Marshal(StringArrayMap(l))
}

func (o Options) MarshalJSON() ([]byte, error) {
	return json.Marshal(StringArrayMap(o))
}

func (em ExpectedMounts) MarshalJSON() ([]byte, error) {
	return json.Marshal(StringArrayMap(em))
}

func (m Mounts) MarshalJSON() ([]byte, error) {
	type Alias Mounts
	return json.Marshal(&struct {
		Length int `json:"length"`
		*Alias
	}{
		Length: len(m.Elements),
		Alias:  (*Alias)(&m),
	})
}
