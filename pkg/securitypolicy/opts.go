package securitypolicy

type ContainerConfigOpt func(config *ContainerConfig) error

type PolicyConfigOpt func(config *PolicyConfig) error

// WithEnvVarRules adds environment variable constraints to container policy config.
func WithEnvVarRules(envs []EnvRuleConfig) ContainerConfigOpt {
	return func(c *ContainerConfig) error {
		c.EnvRules = append(c.EnvRules, envs...)
		return nil
	}
}

// WithWorkingDir sets working directory in container policy config.
func WithWorkingDir(wd string) ContainerConfigOpt {
	return func(c *ContainerConfig) error {
		c.WorkingDir = wd
		return nil
	}
}

// WithMountConstraints extends ContainerConfig.Mounts with provided mount
// constraints.
func WithMountConstraints(mc []MountConfig) ContainerConfigOpt {
	return func(c *ContainerConfig) error {
		c.Mounts = append(c.Mounts, mc...)
		return nil
	}
}

// WithAllowElevated allows container to run in an elevated/privileged mode.
func WithAllowElevated(elevated bool) ContainerConfigOpt {
	return func(c *ContainerConfig) error {
		c.AllowElevated = elevated
		return nil
	}
}

// WithCommand sets ContainerConfig.Command in container policy config.
func WithCommand(cmd []string) ContainerConfigOpt {
	return func(c *ContainerConfig) error {
		c.Command = cmd
		return nil
	}
}

// WithAllowStdioAccess enables or disables container init process stdio.
func WithAllowStdioAccess(stdio bool) ContainerConfigOpt {
	return func(c *ContainerConfig) error {
		c.AllowStdioAccess = stdio
		return nil
	}
}

// WithExecProcesses allows specified exec processes.
func WithExecProcesses(execs []ExecProcessConfig) ContainerConfigOpt {
	return func(c *ContainerConfig) error {
		c.ExecProcesses = append(c.ExecProcesses, execs...)
		return nil
	}
}

// WithAllowPrivilegeEscalation allows escalating of privileges by clearing the NoNewPrivileges flag
func WithAllowPrivilegeEscalation(allow bool) ContainerConfigOpt {
	return func(c *ContainerConfig) error {
		c.AllowPrivilegeEscalation = allow
		return nil
	}
}

// WithUser sets user in container policy config.
func WithUser(user UserConfig) ContainerConfigOpt {
	return func(c *ContainerConfig) error {
		c.User = &user
		return nil
	}
}

// WithCapabilities sets capabilities in container policy config.
func WithCapabilities(capabilities *CapabilitiesConfig) ContainerConfigOpt {
	return func(c *ContainerConfig) error {
		c.Capabilities = capabilities
		return nil
	}
}

// WithSeccompProfilePath sets seccomp profile path in container policy config.
func WithSeccompProfilePath(path string) ContainerConfigOpt {
	return func(c *ContainerConfig) error {
		c.SeccompProfilePath = path
		return nil
	}
}

// WithContainers adds containers to security policy.
func WithContainers(containers []ContainerConfig) PolicyConfigOpt {
	return func(config *PolicyConfig) error {
		config.Containers = append(config.Containers, containers...)
		return nil
	}
}

func WithAllowUnencryptedScratch(allow bool) PolicyConfigOpt {
	return func(config *PolicyConfig) error {
		config.AllowUnencryptedScratch = allow
		return nil
	}
}

func WithAllowEnvVarDropping(allow bool) PolicyConfigOpt {
	return func(config *PolicyConfig) error {
		config.AllowEnvironmentVariableDropping = allow
		return nil
	}
}

func WithAllowCapabilityDropping(allow bool) PolicyConfigOpt {
	return func(config *PolicyConfig) error {
		config.AllowCapabilityDropping = allow
		return nil
	}
}

func WithAllowRuntimeLogging(allow bool) PolicyConfigOpt {
	return func(config *PolicyConfig) error {
		config.AllowRuntimeLogging = allow
		return nil
	}
}

func WithExternalProcesses(processes []ExternalProcessConfig) PolicyConfigOpt {
	return func(config *PolicyConfig) error {
		config.ExternalProcesses = append(config.ExternalProcesses, processes...)
		return nil
	}
}

func WithAllowPropertiesAccess(allow bool) PolicyConfigOpt {
	return func(config *PolicyConfig) error {
		config.AllowPropertiesAccess = allow
		return nil
	}
}

func WithAllowDumpStacks(allow bool) PolicyConfigOpt {
	return func(config *PolicyConfig) error {
		config.AllowDumpStacks = allow
		return nil
	}
}
