package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/Microsoft/hcsshim/internal/tools/securitypolicy/helpers"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

var (
	configFile        = flag.String("c", "", "config path")
	outputType        = flag.String("t", "", "[rego|json|fragment]")
	fragmentNamespace = flag.String("n", "", "fragment namespace")
	fragmentSVN       = flag.String("v", "", "fragment svn")
	outputRaw         = flag.Bool("r", false, "whether to print the raw output")
)

func main() {
	flag.Parse()
	if flag.NArg() != 0 || len(*configFile) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	err := func() (err error) {
		configData, err := os.ReadFile(*configFile)
		if err != nil {
			return err
		}

		config := &securitypolicy.PolicyConfig{}

		err = toml.Unmarshal(configData, config)
		if err != nil {
			return err
		}

		defaultContainers := helpers.DefaultContainerConfigs()
		config.Containers = append(config.Containers, defaultContainers...)
		policyContainers, err := helpers.PolicyContainersFromConfigs(config.Containers)
		if err != nil {
			return err
		}

		var policyCode string
		if *outputType == "fragment" {
			policyCode, err = securitypolicy.MarshalFragment(
				*fragmentNamespace,
				*fragmentSVN,
				policyContainers,
				config.ExternalProcesses,
				config.Fragments)
		} else {
			policyCode, err = securitypolicy.MarshalPolicy(
				*outputType,
				config.AllowAll,
				policyContainers,
				config.ExternalProcesses,
				config.Fragments,
				config.AllowPropertiesAccess,
				config.AllowDumpStacks,
				config.AllowRuntimeLogging,
				config.AllowEnvironmentVariableDropping,
				config.AllowUnencryptedScratch,
			)
		}
		if err != nil {
			return err
		}

		if *outputRaw {
			fmt.Printf("%s\n", policyCode)
		}
		b := base64.StdEncoding.EncodeToString([]byte(policyCode))
		fmt.Printf("%s\n", b)

		return nil
	}()

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
