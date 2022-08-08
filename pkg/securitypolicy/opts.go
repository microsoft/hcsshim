package securitypolicy

type ContainerConfigOpt func(*ContainerConfig) error

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
