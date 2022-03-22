package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/Microsoft/hcsshim/internal/tools/securitypolicy/helpers"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
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
	// Add default containers to the policy config to get the root hash
	// and any environment variable rules we might need
	defaultContainers := helpers.DefaultContainerConfigs()
	config.Containers = append(config.Containers, defaultContainers...)
	policyContainers, err := helpers.PolicyContainersFromConfigs(config.Containers)
	if err != nil {
		return nil, err
	}
	return securitypolicy.NewSecurityPolicy(false, policyContainers), nil
}
