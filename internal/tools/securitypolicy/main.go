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
	"github.com/Microsoft/hcsshim/ext4/dmverity"
	"github.com/Microsoft/hcsshim/ext4/tar2ext4"
	sp "github.com/Microsoft/hcsshim/pkg/securitypolicy"
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

		policy, err := func() (sp.SecurityPolicy, error) {
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
	Strategy sp.EnvVarRule `toml:"strategy"`
	Rule     string        `toml:"rule"`
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

func createOpenDoorPolicy() sp.SecurityPolicy {
	return sp.SecurityPolicy{
		AllowAll: true,
	}
}

func createPolicyFromConfig(config Config) (sp.SecurityPolicy, error) {
	p := sp.SecurityPolicy{
		Containers: map[string]sp.SecurityPolicyContainer{},
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
		container := sp.SecurityPolicyContainer{
			NumCommands: len(command),
			Command:     command,
			EnvRules:    envRules,
			Layers:      map[string]string{},
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

			out, err := ioutil.TempFile("", "")
			if err != nil {
				return p, err
			}
			defer os.Remove(out.Name())

			opts := []tar2ext4.Option{
				tar2ext4.ConvertWhiteout,
				tar2ext4.MaximumDiskSize(dmverity.RecommendedVHDSizeGB),
			}

			err = tar2ext4.Convert(r, out, opts...)
			if err != nil {
				return p, err
			}

			data, err := ioutil.ReadFile(out.Name())
			if err != nil {
				return p, err
			}

			tree, err := dmverity.MerkleTree(data)
			if err != nil {
				return p, err
			}
			hash := dmverity.RootHash(tree)
			hashString := fmt.Sprintf("%x", hash)
			container.Layers = addLayer(container.Layers, hashString)
		}

		container.NumLayers = len(layers)

		// add rules for all known environment variables from the configuration
		// these are in addition to "other rules" from the policy definition file
		config, err := img.ConfigFile()
		if err != nil {
			return p, err
		}
		for _, env := range config.Config.Env {
			rule := sp.SecurityPolicyEnvironmentVariableRule{
				Strategy: sp.EnvVarRuleString,
				Rule:     env,
			}

			container.EnvRules = addEnvRule(container.EnvRules, rule)
		}

		// cri adds TERM=xterm for all workload containers. we add to all containers
		// to prevent any possble erroring
		rule := sp.SecurityPolicyEnvironmentVariableRule{
			Strategy: sp.EnvVarRuleString,
			Rule:     "TERM=xterm",
		}

		container.EnvRules = addEnvRule(container.EnvRules, rule)
		container.NumEnvRules = len(container.EnvRules)

		p.Containers = addContainer(p.Containers, container)
	}

	p.NumContainers = len(p.Containers)

	return p, nil
}

func validateEnvRules(rules []EnvironmentVariableRule) error {
	for _, rule := range rules {
		switch rule.Strategy {
		case sp.EnvVarRuleRegex:
			_, err := regexp.Compile(rule.Rule)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func convertCommand(toml []string) map[string]string {
	json := map[string]string{}

	for i, arg := range toml {
		json[strconv.Itoa(i)] = arg
	}

	return json
}

func convertEnvironmentVariableRules(toml []EnvironmentVariableRule) map[string]sp.SecurityPolicyEnvironmentVariableRule {
	json := map[string]sp.SecurityPolicyEnvironmentVariableRule{}

	for i, rule := range toml {
		jsonRule := sp.SecurityPolicyEnvironmentVariableRule{
			Strategy: rule.Strategy,
			Rule:     rule.Rule,
		}

		json[strconv.Itoa(i)] = jsonRule
	}

	return json
}

func addContainer(containers map[string]sp.SecurityPolicyContainer, container sp.SecurityPolicyContainer) map[string]sp.SecurityPolicyContainer {
	index := strconv.Itoa(len(containers))

	containers[index] = container

	return containers
}

func addLayer(layers map[string]string, layer string) map[string]string {
	index := strconv.Itoa(len(layers))

	layers[index] = layer

	return layers
}

func addEnvRule(rules map[string]sp.SecurityPolicyEnvironmentVariableRule, rule sp.SecurityPolicyEnvironmentVariableRule) map[string]sp.SecurityPolicyEnvironmentVariableRule {
	index := strconv.Itoa(len(rules))

	rules[index] = rule

	return rules
}
