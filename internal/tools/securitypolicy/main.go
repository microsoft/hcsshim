package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"

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

		config := &Config{
			AllowAll:   false,
			Containers: []Container{},
		}

		err = toml.Unmarshal(configData, config)
		if err != nil {
			return err
		}

		policy, err := func() (securitypolicy.Policy, error) {
			if config.AllowAll {
				return createOpenDoorPolicy(), nil
			} else {
				return createPolicyFromConfig(*config)
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

type EnvironmentVariableRule struct {
	Strategy securitypolicy.EnvVarRule `toml:"strategy"`
	Rule     string                    `toml:"rule"`
}

type Container struct {
	Name     string                    `toml:"name"`
	Auth     ImageAuth                 `toml:"auth"`
	Command  []string                  `toml:"command"`
	EnvRules []EnvironmentVariableRule `toml:"env_rule"`
}

type ImageAuth struct {
	Username string `toml:"username"`
	Password string `toml:"password"`
}

type Config struct {
	AllowAll   bool        `toml:"allow_all"`
	Containers []Container `toml:"container"`
}

func createOpenDoorPolicy() securitypolicy.Policy {
	return securitypolicy.Policy{
		AllowAll: true,
	}
}

func createPolicyFromConfig(config Config) (securitypolicy.Policy, error) {
	p := securitypolicy.Policy{
		Containers: securitypolicy.Containers{
			Elements: map[string]securitypolicy.Container{},
		},
	}

	// Hardcode the pause container version and command. We still pull it
	// to get the root hash and any environment variable rules we might need.
	pause := Container{
		Name:     "k8s.gcr.io/pause:3.1",
		Command:  []string{"/pause"},
		EnvRules: []EnvironmentVariableRule{}}
	config.Containers = append(config.Containers, pause)

	for _, configContainer := range config.Containers {
		var imageOptions []remote.Option

		if configContainer.Auth.Username != "" && configContainer.Auth.Password != "" {
			auth := authn.Basic{
				Username: configContainer.Auth.Username,
				Password: configContainer.Auth.Password}
			c, _ := auth.Authorization()
			authOption := remote.WithAuth(authn.FromConfig(*c))
			imageOptions = append(imageOptions, authOption)
		}

		// validate EnvRules
		err := validateEnvRules(configContainer.EnvRules)
		if err != nil {
			return p, err
		}

		command := convertCommand(configContainer.Command)
		envRules := convertEnvironmentVariableRules(configContainer.EnvRules)
		container := securitypolicy.Container{
			Command:  command,
			EnvRules: envRules,
			Layers: securitypolicy.Layers{
				Elements: map[string]string{},
			},
		}
		ref, err := name.ParseReference(configContainer.Name)
		if err != nil {
			return p, fmt.Errorf("'%s' isn't a valid image name", configContainer.Name)
		}
		img, err := remote.Image(ref, imageOptions...)
		if err != nil {
			return p, fmt.Errorf("unable to fetch image '%s': %s", configContainer.Name, err.Error())
		}

		layers, err := img.Layers()
		if err != nil {
			return p, err
		}

		for _, layer := range layers {
			r, err := layer.Uncompressed()
			if err != nil {
				return p, err
			}

			hashString, err := tar2ext4.ConvertAndComputeRootDigest(r)
			if err != nil {
				return p, err
			}
			addLayer(&container.Layers, hashString)
		}

		// add rules for all known environment variables from the configuration
		// these are in addition to "other rules" from the policy definition file
		imgConfig, err := img.ConfigFile()
		if err != nil {
			return p, err
		}
		for _, env := range imgConfig.Config.Env {
			rule := securitypolicy.EnvRule{
				Strategy: securitypolicy.EnvVarRuleString,
				Rule:     env,
			}

			addEnvRule(&container.EnvRules, rule)
		}

		// cri adds TERM=xterm for all workload containers. we add to all containers
		// to prevent any possible error
		rule := securitypolicy.EnvRule{
			Strategy: securitypolicy.EnvVarRuleString,
			Rule:     "TERM=xterm",
		}

		addEnvRule(&container.EnvRules, rule)

		addContainer(&p.Containers, container)
	}

	return p, nil
}

func validateEnvRules(rules []EnvironmentVariableRule) error {
	for _, rule := range rules {
		switch rule.Strategy {
		case securitypolicy.EnvVarRuleRegex:
			_, err := regexp.Compile(rule.Rule)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func convertCommand(toml []string) securitypolicy.CommandArgs {
	jsn := map[string]string{}

	for i, arg := range toml {
		jsn[strconv.Itoa(i)] = arg
	}

	return securitypolicy.CommandArgs{
		Elements: jsn,
	}
}

func convertEnvironmentVariableRules(toml []EnvironmentVariableRule) securitypolicy.EnvRules {
	jsn := map[string]securitypolicy.EnvRule{}

	for i, rule := range toml {
		jsonRule := securitypolicy.EnvRule{
			Strategy: rule.Strategy,
			Rule:     rule.Rule,
		}

		jsn[strconv.Itoa(i)] = jsonRule
	}

	return securitypolicy.EnvRules{
		Elements: jsn,
	}
}

func addContainer(containers *securitypolicy.Containers, container securitypolicy.Container) {
	index := strconv.Itoa(len(containers.Elements))

	containers.Elements[index] = container
}

func addLayer(layers *securitypolicy.Layers, layer string) {
	index := strconv.Itoa(len(layers.Elements))

	layers.Elements[index] = layer
}

func addEnvRule(rules *securitypolicy.EnvRules, rule securitypolicy.EnvRule) {
	index := strconv.Itoa(len(rules.Elements))

	rules.Elements[index] = rule
}
