package helpers

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/hcsshim/ext4/tar2ext4"
	sp "github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

// RemoteImageFromImageName parses a given imageName reference and creates a v1.Image with
// provided remote.Option opts.
func RemoteImageFromImageName(imageName string, opts ...remote.Option) (v1.Image, error) {
	ref, err := name.ParseReference(imageName)
	if err != nil {
		return nil, err
	}

	return remote.Image(ref, opts...)
}

// ComputeLayerHashes computes cryptographic digests of image layers and returns
// them as slice of string hashes.
func ComputeLayerHashes(img v1.Image) ([]string, error) {
	imgLayers, err := img.Layers()
	if err != nil {
		return nil, err
	}

	var layerHashes []string

	for _, layer := range imgLayers {
		r, err := layer.Uncompressed()
		if err != nil {
			return nil, err
		}

		hashString, err := tar2ext4.ConvertAndComputeRootDigest(r)
		if err != nil {
			return nil, err
		}
		layerHashes = append(layerHashes, hashString)
	}
	return layerHashes, nil
}

// ParseEnvFromImage inspects the image spec and adds security policy rules for
// environment variables from the spec. Additionally, includes "TERM=xterm"
// rule, which is added for linux containers by CRI.
func ParseEnvFromImage(img v1.Image) ([]string, error) {
	imgConfig, err := img.ConfigFile()
	if err != nil {
		return nil, err
	}

	return imgConfig.Config.Env, nil
}

// DefaultContainerConfigs returns a hardcoded slice of container configs, which should
// be included by default in the security policy.
// The slice includes only a sandbox pause container.
func DefaultContainerConfigs() []sp.ContainerConfig {
	pause := sp.ContainerConfig{
		ImageName: "k8s.gcr.io/pause:3.1",
		Command:   []string{"/pause"},
	}
	return []sp.ContainerConfig{pause}
}

// ParseWorkingDirFromImage inspects the image spec and returns working directory if
// one was set via CWD Docker directive, otherwise returns "/".
func ParseWorkingDirFromImage(img v1.Image) (string, error) {
	imgConfig, err := img.ConfigFile()
	if err != nil {
		return "", err
	}

	if imgConfig.Config.WorkingDir != "" {
		return imgConfig.Config.WorkingDir, nil
	}
	return "/", nil
}

// ParseCommandFromImage inspects the image and returns the command args, which
// is a combination of ENTRYPOINT and CMD Docker directives.
func ParseCommandFromImage(img v1.Image) ([]string, error) {
	imgConfig, err := img.ConfigFile()
	if err != nil {
		return nil, err
	}

	cmdArgs := imgConfig.Config.Entrypoint
	cmdArgs = append(cmdArgs, imgConfig.Config.Cmd...)
	return cmdArgs, nil
}

// ParseUserFromImage inspects the image and returns the user and group
func ParseUserFromImage(img v1.Image) (sp.IDNameConfig, sp.IDNameConfig, error) {
	imgConfig, err := img.ConfigFile()
	var user sp.IDNameConfig
	var group sp.IDNameConfig
	if err != nil {
		return user, group, err
	}

	userString := imgConfig.Config.User
	// valid values are "user", "user:group", "uid", "uid:gid", "user:gid", "uid:group"
	// "" is also valid, and means any user
	// we assume that any string that is not a number is a username, and thus
	// that any string that is a number is a uid. It is possible to have a username
	// that is a number, but that is not supported here.
	if userString == "" {
		// not specified, use any
		user.Strategy = sp.IDNameStrategyAny
		group.Strategy = sp.IDNameStrategyAny
	} else {
		parts := strings.Split(userString, ":")
		if len(parts) == 1 {
			// only user specified, use any
			group.Strategy = sp.IDNameStrategyAny
			user.Rule = parts[0]
			_, err := strconv.ParseUint(parts[0], 10, 32)
			if err == nil {
				user.Strategy = sp.IDNameStrategyID
			} else {
				user.Strategy = sp.IDNameStrategyName
			}
		} else if len(parts) == 2 {
			_, err := strconv.ParseUint(parts[0], 10, 32)
			user.Rule = parts[0]
			if err == nil {
				user.Strategy = sp.IDNameStrategyID
			} else {
				user.Strategy = sp.IDNameStrategyName
			}

			_, err = strconv.ParseUint(parts[1], 10, 32)
			group.Rule = parts[1]
			if err == nil {
				group.Strategy = sp.IDNameStrategyID
			} else {
				group.Strategy = sp.IDNameStrategyName
			}
		}
	}

	return user, group, nil
}

// PolicyContainersFromConfigs returns a slice of sp.Container generated
// from a slice of sp.ContainerConfig's
func PolicyContainersFromConfigs(containerConfigs []sp.ContainerConfig) ([]*sp.Container, error) {
	var policyContainers []*sp.Container
	for _, containerConfig := range containerConfigs {
		var imageOptions []remote.Option

		if containerConfig.Auth.Username != "" && containerConfig.Auth.Password != "" {
			auth := authn.Basic{
				Username: containerConfig.Auth.Username,
				Password: containerConfig.Auth.Password}
			c, _ := auth.Authorization()
			authOption := remote.WithAuth(authn.FromConfig(*c))
			imageOptions = append(imageOptions, authOption)
		}

		img, err := RemoteImageFromImageName(containerConfig.ImageName, imageOptions...)
		if err != nil {
			return nil, fmt.Errorf("unable to fetch image: %w", err)
		}

		layerHashes, err := ComputeLayerHashes(img)
		if err != nil {
			return nil, err
		}

		commandArgs := containerConfig.Command
		if len(commandArgs) == 0 {
			commandArgs, err = ParseCommandFromImage(img)
			if err != nil {
				return nil, err
			}
		}
		// add rules for all known environment variables from the configuration
		// these are in addition to "other rules" from the policy definition file
		envVars, err := ParseEnvFromImage(img)
		if err != nil {
			return nil, err
		}

		// we want all environment variables which we've extracted from the
		// image to be required
		envRules := sp.NewEnvVarRules(envVars, true)

		// cri adds TERM=xterm for all workload containers. we add to all containers
		// to prevent any possible error
		envRules = append(envRules, sp.EnvRuleConfig{
			Rule:     "TERM=xterm",
			Strategy: sp.EnvVarRuleString,
			Required: false,
		})

		envRules = append(envRules, containerConfig.EnvRules...)

		workingDir, err := ParseWorkingDirFromImage(img)
		if err != nil {
			return nil, err
		}

		if containerConfig.WorkingDir != "" {
			workingDir = containerConfig.WorkingDir
		}

		user, group, err := ParseUserFromImage(img)
		if err != nil {
			return nil, err
		}

		container, err := sp.CreateContainerPolicy(
			commandArgs,
			layerHashes,
			envRules,
			workingDir,
			containerConfig.Mounts,
			containerConfig.AllowElevated,
			containerConfig.ExecProcesses,
			containerConfig.Signals,
			containerConfig.AllowStdioAccess,
			!containerConfig.AllowPrivilegeEscalation,
			setDefaultUser(containerConfig.User, user, group),
			setDefaultCapabilities(containerConfig.Capabilities),
			setDefaultSeccomp(containerConfig.SeccompProfilePath),
		)
		if err != nil {
			return nil, err
		}
		policyContainers = append(policyContainers, container)
	}

	return policyContainers, nil
}

func setDefaultUser(config *sp.UserConfig, user, group sp.IDNameConfig) sp.UserConfig {
	if config != nil {
		return *config
	}

	// 0022 is the default umask for containers in docker
	return sp.UserConfig{
		UserIDName:   user,
		GroupIDNames: []sp.IDNameConfig{group},
		Umask:        "0022",
	}
}

func setDefaultCapabilities(config *sp.CapabilitiesConfig) *sp.CapabilitiesConfig {
	if config != nil {
		// For any that is missing, we put an empty set to give the user the
		// quickest path when they get a runtime error to figuring out the issue.
		// Our only other reasonable option would be to bail here with an error
		// message.
		if config.Bounding == nil {
			config.Bounding = make([]string, 0)
		}
		if config.Effective == nil {
			config.Effective = make([]string, 0)
		}
		if config.Inheritable == nil {
			config.Inheritable = make([]string, 0)
		}
		if config.Permitted == nil {
			config.Permitted = make([]string, 0)
		}
		if config.Ambient == nil {
			config.Ambient = make([]string, 0)
		}
	}

	return config
}

func setDefaultSeccomp(seccompProfilePath string) string {
	if len(seccompProfilePath) == 0 {
		return ""
	}

	var seccomp specs.LinuxSeccomp

	buff, err := os.ReadFile(seccompProfilePath)
	if err != nil {
		log.Fatalf("unable to read seccomp profile: %v", err)
	}

	err = json.Unmarshal(buff, &seccomp)
	if err != nil {
		log.Fatalf("unable to parse seccomp profile: %v", err)
	}

	profileSHA256, err := sp.MeasureSeccompProfile(&seccomp)
	if err != nil {
		log.Fatalf("unable to measure seccomp profile: %v", err)
	}

	return profileSHA256
}
