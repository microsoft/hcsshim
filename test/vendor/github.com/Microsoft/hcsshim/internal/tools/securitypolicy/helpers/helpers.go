package helpers

import (
	"fmt"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/Microsoft/hcsshim/ext4/tar2ext4"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
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

	// cri adds TERM=xterm for all workload containers. we add to all containers
	// to prevent any possible error
	envVars := append(imgConfig.Config.Env, "TERM=xterm")

	return envVars, nil
}

// DefaultContainerConfigs returns a hardcoded slice of container configs, which should
// be included by default in the security policy.
// The slice includes only a sandbox pause container.
func DefaultContainerConfigs() []securitypolicy.ContainerConfig {
	pause := securitypolicy.NewContainerConfig(
		"k8s.gcr.io/pause:3.1",
		[]string{"/pause"},
		[]securitypolicy.EnvRuleConfig{},
		securitypolicy.AuthConfig{},
		"",
		[]string{},
		[]securitypolicy.MountConfig{},
	)
	return []securitypolicy.ContainerConfig{pause}
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

// PolicyContainersFromConfigs returns a slice of securitypolicy.Container generated
// from a slice of securitypolicy.ContainerConfig's
func PolicyContainersFromConfigs(containerConfigs []securitypolicy.ContainerConfig) ([]*securitypolicy.Container, error) {
	var policyContainers []*securitypolicy.Container
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

		// add rules for all known environment variables from the configuration
		// these are in addition to "other rules" from the policy definition file
		envVars, err := ParseEnvFromImage(img)
		if err != nil {
			return nil, err
		}
		envRules := securitypolicy.NewEnvVarRules(envVars)
		envRules = append(envRules, containerConfig.EnvRules...)

		workingDir, err := ParseWorkingDirFromImage(img)
		if err != nil {
			return nil, err
		}

		if containerConfig.WorkingDir != "" {
			workingDir = containerConfig.WorkingDir
		}

		container, err := securitypolicy.CreateContainerPolicy(
			containerConfig.Command,
			layerHashes,
			envRules,
			workingDir,
			containerConfig.ExpectedMounts,
			containerConfig.Mounts,
		)
		if err != nil {
			return nil, err
		}
		policyContainers = append(policyContainers, container)
	}

	return policyContainers, nil
}
