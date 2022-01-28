package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/Microsoft/hcsshim/ext4/tar2ext4"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

var (
	configFile = flag.String("c", "", "config")
	outputJSON = flag.Bool("j", false, "json")
)

func main() {
	flag.Parse()
	if flag.NArg() != 0 || len(*configFile) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	err := func() (err error) {
		configData, err := ioutil.ReadFile(*configFile)
		if err != nil {
			return err
		}

		config := &securitypolicy.PolicyConfig{}

		err = toml.Unmarshal(configData, config)
		if err != nil {
			return err
		}

		policy, err := func() (*securitypolicy.SecurityPolicy, error) {
			if config.AllowAll {
				return securitypolicy.NewOpenDoorPolicy(), nil
			} else {
				return createPolicyFromConfig(config)
			}
		}()

		if err != nil {
			return err
		}

		j, err := json.Marshal(policy)
		if err != nil {
			return err
		}
		if *outputJSON {
			fmt.Printf("%s\n", j)
		}
		b := base64.StdEncoding.EncodeToString(j)
		fmt.Printf("%s\n", b)

		return nil
	}()

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func createPolicyFromConfig(config *securitypolicy.PolicyConfig) (*securitypolicy.SecurityPolicy, error) {
	// Hardcode the pause container version and command. We still pull it
	// to get the root hash and any environment variable rules we might need.
	pause := securitypolicy.NewContainerConfig(
		"k8s.gcr.io/pause:3.1",
		[]string{"/pause"},
		[]securitypolicy.EnvRule{},
		securitypolicy.AuthConfig{},
	)
	config.Containers = append(config.Containers, pause)

	var policyContainers []*securitypolicy.Container
	for _, containerConfig := range config.Containers {
		var imageOptions []remote.Option

		if containerConfig.Auth.Username != "" && containerConfig.Auth.Password != "" {
			auth := authn.Basic{
				Username: containerConfig.Auth.Username,
				Password: containerConfig.Auth.Password}
			c, _ := auth.Authorization()
			authOption := remote.WithAuth(authn.FromConfig(*c))
			imageOptions = append(imageOptions, authOption)
		}

		ref, err := name.ParseReference(containerConfig.ImageName)
		if err != nil {
			return nil, fmt.Errorf("'%s' isn't a valid image name", containerConfig.ImageName)
		}
		img, err := remote.Image(ref, imageOptions...)
		if err != nil {
			return nil, fmt.Errorf("unable to fetch image '%s': %s", containerConfig.ImageName, err.Error())
		}

		layers, err := img.Layers()
		if err != nil {
			return nil, err
		}

		var layerHashes []string
		for _, layer := range layers {
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

		// add rules for all known environment variables from the configuration
		// these are in addition to "other rules" from the policy definition file
		imgConfig, err := img.ConfigFile()
		if err != nil {
			return nil, err
		}

		envRules := containerConfig.EnvRules
		for _, env := range imgConfig.Config.Env {
			rule := securitypolicy.EnvRule{
				Strategy: securitypolicy.EnvVarRuleString,
				Rule:     env,
			}
			envRules = append(envRules, rule)
		}
		// cri adds TERM=xterm for all workload containers. we add to all containers
		// to prevent any possible error
		rule := securitypolicy.EnvRule{
			Strategy: securitypolicy.EnvVarRuleString,
			Rule:     "TERM=xterm",
		}
		envRules = append(envRules, rule)

		container, err := securitypolicy.NewContainer(containerConfig.Command, layerHashes, envRules)
		if err != nil {
			return nil, err
		}
		policyContainers = append(policyContainers, container)
	}

	return securitypolicy.NewSecurityPolicy(false, policyContainers), nil
}
