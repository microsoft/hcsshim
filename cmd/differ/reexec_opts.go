//go:build windows

package main

import (
	"os"

	"github.com/Microsoft/hcsshim/internal/winapi"
)

// reExecOpts are options to change how a subcommand is re-exec'ed

type reExecOpt func(*reExecConfig) error

func defaultReExecOpts() []reExecOpt {
	return []reExecOpt{
		withPrivileges([]string{winapi.SeChangeNotifyPrivilege}),
		usingEnv([]string{
			mediaTypeEnvVar,
			payloadPineEnvVar,
			logLevelEnvVar,
			logETWProviderEnvVar,
		}),
	}
}

func withPrivileges(keep []string) reExecOpt {
	return func(c *reExecConfig) error {
		c.privs = append(c.privs, keep...)
		return nil
	}
}

// withEnvValues appends the environment variables in the form `k=v` to the re-exec's environment
func withEnvValues(env []string) reExecOpt {
	return func(c *reExecConfig) error {
		c.env = append(c.env, env...)
		return nil
	}
}

// usingEnvs looks up the env names and, if they exist, appends them to the re-exec's environment
func usingEnv(env []string) reExecOpt {
	return func(c *reExecConfig) error {
		for _, k := range env {
			if v, ok := os.LookupEnv(k); ok {
				c.env = append(c.env, k+"="+v)
			}
		}
		return nil
	}
}
