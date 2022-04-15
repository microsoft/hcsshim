package securitypolicy

type ContainerConfigOpt func(*ContainerConfig) error

// WithEnvVarRules adds environment variable constraints to container policy config.
func WithEnvVarRules(envs []EnvRuleConfig) ContainerConfigOpt {
	return func(c *ContainerConfig) error {
		c.EnvRules = append(c.EnvRules, envs...)
		return nil
	}
}

// WithExpectedMounts adds expected mounts to container policy config.
func WithExpectedMounts(em []string) ContainerConfigOpt {
	return func(c *ContainerConfig) error {
		c.ExpectedMounts = append(c.ExpectedMounts, em...)
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
