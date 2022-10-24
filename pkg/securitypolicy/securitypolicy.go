package securitypolicy

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/pkg/errors"
)

var ErrInvalidOpenDoorPolicy = errors.New("allow_all cannot be set to 'true' when Containers are non-empty")

type EnvVarRule string

const (
	EnvVarRuleString EnvVarRule = "string"
	EnvVarRuleRegex  EnvVarRule = "re2"
)

// PolicyConfig contains toml or JSON config for security policy.
type PolicyConfig struct {
	AllowAll              bool                    `json:"allow_all" toml:"allow_all"`
	Containers            []ContainerConfig       `json:"containers" toml:"container"`
	ExternalProcesses     []ExternalProcessConfig `json:"external_processes" toml:"external_process"`
	Fragments             []FragmentConfig        `json:"fragments" toml:"fragment"`
	AllowPropertiesAccess bool                    `json:"allow_properties_access" toml:"allow_properties_access"`
	AllowDumpStacks       bool                    `json:"allow_dump_stacks" toml:"allow_dump_stacks"`
	AllowRuntimeLogging   bool                    `json:"allow_runtime_logging" toml:"allow_runtime_logging"`
}

// ExternalProcessConfig contains toml or JSON config for running external processes in the UVM.
type ExternalProcessConfig struct {
	Command    []string `json:"command" toml:"command"`
	WorkingDir string   `json:"working_dir" toml:"working_dir"`
}

// FragmentConfig contains toml or JSON config for including elements from fragments.
type FragmentConfig struct {
	Issuer     string   `json:"issuer" toml:"issuer"`
	Feed       string   `json:"feed" toml:"feed"`
	MinimumSVN string   `json:"minimum_svn" toml:"minimum_svn"`
	Includes   []string `json:"includes" toml:"include"`
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
	Required bool       `json:"required" toml:"required"`
}

// ContainerConfig contains toml or JSON config for container described
// in security policy.
type ContainerConfig struct {
	ImageName     string              `json:"image_name" toml:"image_name"`
	Command       []string            `json:"command" toml:"command"`
	Auth          AuthConfig          `json:"auth" toml:"auth"`
	EnvRules      []EnvRuleConfig     `json:"env_rules" toml:"env_rule"`
	WorkingDir    string              `json:"working_dir" toml:"working_dir"`
	Mounts        []MountConfig       `json:"mounts" toml:"mount"`
	AllowElevated bool                `json:"allow_elevated" toml:"allow_elevated"`
	ExecProcesses []ExecProcessConfig `json:"exec_processes" toml:"exec_process"`
	Signals       []syscall.Signal    `json:"signals" toml:"signals"`
}

// MountConfig contains toml or JSON config for mount security policy
// constraint description.
type MountConfig struct {
	HostPath      string `json:"host_path" toml:"host_path"`
	ContainerPath string `json:"container_path" toml:"container_path"`
	Readonly      bool   `json:"readonly" toml:"readonly"`
}

// ExecProcessConfig contains toml or JSON config for exec process security
// policy constraint description
type ExecProcessConfig struct {
	Command []string         `json:"command" toml:"command"`
	Signals []syscall.Signal `json:"signals" toml:"signals"`
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

// EncodedSecurityPolicy is a JSON representation of SecurityPolicy that has
// been base64 encoded for storage in an annotation embedded within another
// JSON configuration
type EncodedSecurityPolicy struct {
	SecurityPolicy string `json:"SecurityPolicy,omitempty"`
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
	Command       CommandArgs         `json:"command"`
	EnvRules      EnvRules            `json:"env_rules"`
	Layers        Layers              `json:"layers"`
	WorkingDir    string              `json:"working_dir"`
	Mounts        Mounts              `json:"mounts"`
	AllowElevated bool                `json:"allow_elevated"`
	ExecProcesses []ExecProcessConfig `json:"-"`
	Signals       []syscall.Signal    `json:"-"`
}

// StringArrayMap wraps an array of strings as a string map.
type StringArrayMap struct {
	Length   int               `json:"length"`
	Elements map[string]string `json:"elements"`
}

type Layers StringArrayMap

type CommandArgs StringArrayMap

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
	mounts []MountConfig,
	allowElevated bool,
	execProcesses []ExecProcessConfig,
	signals []syscall.Signal,
) (*Container, error) {
	if err := validateEnvRules(envRules); err != nil {
		return nil, err
	}
	if err := validateMountConstraint(mounts); err != nil {
		return nil, err
	}
	return &Container{
		Command:       newCommandArgs(command),
		Layers:        newLayers(layers),
		EnvRules:      newEnvRules(envRules),
		WorkingDir:    workingDir,
		Mounts:        newMountConstraints(mounts),
		AllowElevated: allowElevated,
		ExecProcesses: execProcesses,
		Signals:       signals,
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
// e.g., vhd:// or evd://
// TODO: (anmaxvl) Do we need to set/validate Linux rootfs propagation?
// In case we do, update securityPolicyContainer and Container structs
// as well as mount enforcement logic.
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
