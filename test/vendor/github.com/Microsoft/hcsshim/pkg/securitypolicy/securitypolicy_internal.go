package securitypolicy

import (
	"fmt"
	"strconv"
	"syscall"
)

// Internal version of SecurityPolicy
type securityPolicyInternal struct {
	Containers                       []*securityPolicyContainer
	ExternalProcesses                []*externalProcess
	Fragments                        []*fragment
	AllowPropertiesAccess            bool
	AllowDumpStacks                  bool
	AllowRuntimeLogging              bool
	AllowEnvironmentVariableDropping bool
	AllowUnencryptedScratch          bool
	AllowCapabilityDropping          bool
}

type securityPolicyFragment struct {
	Namespace         string
	SVN               string
	Containers        []*securityPolicyContainer
	ExternalProcesses []*externalProcess
	Fragments         []*fragment
}

func containersToInternal(containers []*Container) ([]*securityPolicyContainer, error) {
	result := make([]*securityPolicyContainer, len(containers))
	for i, cConf := range containers {
		cInternal, err := cConf.toInternal()
		if err != nil {
			return nil, err
		}
		result[i] = cInternal
	}

	return result, nil
}

func externalProcessToInternal(externalProcesses []ExternalProcessConfig) []*externalProcess {
	result := make([]*externalProcess, len(externalProcesses))
	for i, pConf := range externalProcesses {
		pInternal := pConf.toInternal()
		result[i] = &pInternal
	}

	return result
}

func fragmentsToInternal(fragments []FragmentConfig) []*fragment {
	result := make([]*fragment, len(fragments))
	for i, fConf := range fragments {
		fInternal := fConf.toInternal()
		result[i] = &fInternal
	}

	return result
}

func newSecurityPolicyInternal(
	containers []*Container,
	externalProcesses []ExternalProcessConfig,
	fragments []FragmentConfig,
	allowPropertiesAccess bool,
	allowDumpStacks bool,
	allowRuntimeLogging bool,
	allowDropEnvironmentVariables bool,
	allowUnencryptedScratch bool,
	allowDropCapabilities bool,
) (*securityPolicyInternal, error) {
	containersInternal, err := containersToInternal(containers)
	if err != nil {
		return nil, err
	}

	return &securityPolicyInternal{
		Containers:                       containersInternal,
		ExternalProcesses:                externalProcessToInternal(externalProcesses),
		Fragments:                        fragmentsToInternal(fragments),
		AllowPropertiesAccess:            allowPropertiesAccess,
		AllowDumpStacks:                  allowDumpStacks,
		AllowRuntimeLogging:              allowRuntimeLogging,
		AllowEnvironmentVariableDropping: allowDropEnvironmentVariables,
		AllowUnencryptedScratch:          allowUnencryptedScratch,
		AllowCapabilityDropping:          allowDropCapabilities,
	}, nil
}

func newSecurityPolicyFragment(
	namespace string,
	svn string,
	containers []*Container,
	externalProcesses []ExternalProcessConfig,
	fragments []FragmentConfig) (*securityPolicyFragment, error) {
	containersInternal, err := containersToInternal(containers)
	if err != nil {
		return nil, err
	}

	return &securityPolicyFragment{
		Namespace:         namespace,
		SVN:               svn,
		Containers:        containersInternal,
		ExternalProcesses: externalProcessToInternal(externalProcesses),
		Fragments:         fragmentsToInternal(fragments),
	}, nil
}

// Internal version of Container
type securityPolicyContainer struct {
	// The command that we will allow the container to execute
	Command []string `json:"command"`
	// The rules for determining if a given environment variable is allowed
	EnvRules []EnvRuleConfig `json:"env_rules"`
	// An ordered list of dm-verity root hashes for each layer that makes up
	// "a container". Containers are constructed as an overlay file system. The
	// order that the layers are overlayed is important and needs to be enforced
	// as part of policy.
	Layers []string `json:"layers"`
	// WorkingDir is a path to container's working directory, which all the processes
	// will default to.
	WorkingDir string `json:"working_dir"`
	// A list of constraints for determining if a given mount is allowed.
	Mounts        []mountInternal `json:"mounts"`
	AllowElevated bool            `json:"allow_elevated"`
	// A list of lists of commands that can be used to execute additional
	// processes within the container
	ExecProcesses []containerExecProcess `json:"exec_processes"`
	// A list of signals that are allowed to be sent to the container's init
	// process.
	Signals []syscall.Signal `json:"signals"`
	// Whether to allow the capture of init process standard out and standard error
	AllowStdioAccess bool `json:"allow_stdio_access"`
	// Whether to deny new privileges
	NoNewPrivileges bool `json:"no_new_privileges"`
	// The user that the container will run as
	User UserConfig `json:"user"`
	// Capability sets for the container
	Capabilities *capabilitiesInternal `json:"capabilities"`
	// Seccomp configuration for the container
	SeccompProfileSHA256 string `json:"seccomp_profile_sha256"`
}

type containerExecProcess struct {
	Command []string `json:"command"`
	// A list of signals that are allowed to be sent to this process
	Signals []syscall.Signal `json:"signals"`
}

type externalProcess struct {
	command          []string
	envRules         []EnvRuleConfig
	workingDir       string
	allowStdioAccess bool
}

// Internal version of Mount
type mountInternal struct {
	Source      string   `json:"source"`
	Destination string   `json:"destination"`
	Type        string   `json:"type"`
	Options     []string `json:"options"`
}

// Internal version of Capabilities
type capabilitiesInternal struct {
	Bounding    []string
	Effective   []string
	Inheritable []string
	Permitted   []string
	Ambient     []string
}

type fragment struct {
	issuer     string
	feed       string
	minimumSVN string
	includes   []string
}

func (c *Container) toInternal() (*securityPolicyContainer, error) {
	command, err := c.Command.toInternal()
	if err != nil {
		return nil, err
	}

	envRules, err := c.EnvRules.toInternal()
	if err != nil {
		return nil, err
	}

	layers, err := c.Layers.toInternal()
	if err != nil {
		return nil, err
	}

	mounts, err := c.Mounts.toInternal()
	if err != nil {
		return nil, err
	}

	execProcesses := make([]containerExecProcess, len(c.ExecProcesses))
	for i, ep := range c.ExecProcesses {
		execProcesses[i] = containerExecProcess(ep)
	}

	var capabilities *capabilitiesInternal
	if c.Capabilities != nil {
		c := c.Capabilities.toInternal()
		capabilities = &c
	}

	return &securityPolicyContainer{
		Command:  command,
		EnvRules: envRules,
		Layers:   layers,
		// No need to have toInternal(), because WorkingDir is a string both
		// internally and in the policy.
		WorkingDir:           c.WorkingDir,
		Mounts:               mounts,
		AllowElevated:        c.AllowElevated,
		ExecProcesses:        execProcesses,
		Signals:              c.Signals,
		AllowStdioAccess:     c.AllowStdioAccess,
		NoNewPrivileges:      c.NoNewPrivileges,
		User:                 c.User,
		Capabilities:         capabilities,
		SeccompProfileSHA256: c.SeccompProfileSHA256,
	}, nil
}

func (c CommandArgs) toInternal() ([]string, error) {
	return stringMapToStringArray(c.Elements)
}

func (e EnvRules) toInternal() ([]EnvRuleConfig, error) {
	envRulesMapLength := len(e.Elements)
	envRules := make([]EnvRuleConfig, envRulesMapLength)
	for i := 0; i < envRulesMapLength; i++ {
		eIndex := strconv.Itoa(i)
		elem, ok := e.Elements[eIndex]
		if !ok {
			return nil, fmt.Errorf("env rule with index %q doesn't exist", eIndex)
		}
		envRules[i] = elem
	}

	return envRules, nil
}

func (l Layers) toInternal() ([]string, error) {
	return stringMapToStringArray(l.Elements)
}

func (o Options) toInternal() ([]string, error) {
	return stringMapToStringArray(o.Elements)
}

func (m Mounts) toInternal() ([]mountInternal, error) {
	mountLength := len(m.Elements)
	mountConstraints := make([]mountInternal, mountLength)
	for i := 0; i < mountLength; i++ {
		mIndex := strconv.Itoa(i)
		mount, ok := m.Elements[mIndex]
		if !ok {
			return nil, fmt.Errorf("mount constraint with index %q not found", mIndex)
		}
		opts, err := mount.Options.toInternal()
		if err != nil {
			return nil, err
		}
		mountConstraints[i] = mountInternal{
			Source:      mount.Source,
			Destination: mount.Destination,
			Type:        mount.Type,
			Options:     opts,
		}
	}
	return mountConstraints, nil
}

func (p ExternalProcessConfig) toInternal() externalProcess {
	return externalProcess{
		command: p.Command,
		envRules: []EnvRuleConfig{{
			Strategy: "string",
			Rule:     "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			Required: true,
		}},
		workingDir:       p.WorkingDir,
		allowStdioAccess: p.AllowStdioAccess,
	}
}

func (f FragmentConfig) toInternal() fragment {
	return fragment{
		issuer:     f.Issuer,
		feed:       f.Feed,
		minimumSVN: f.MinimumSVN,
		includes:   f.Includes,
	}
}

func (c CapabilitiesConfig) toInternal() capabilitiesInternal {
	return capabilitiesInternal(c)
}

func stringMapToStringArray(m map[string]string) ([]string, error) {
	mapSize := len(m)
	out := make([]string, mapSize)

	for i := 0; i < mapSize; i++ {
		index := strconv.Itoa(i)
		value, ok := m[index]
		if !ok {
			return nil, fmt.Errorf("element with index %q not found", index)
		}
		out[i] = value
	}

	return out, nil
}
