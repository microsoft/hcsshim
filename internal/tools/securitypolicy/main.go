package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"os"

	"github.com/pelletier/go-toml"

	"github.com/Microsoft/hcsshim/internal/tools/securitypolicy/helpers"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

var (
	configFile        = flag.String("c", "", "config path")
	outputType        = flag.String("t", "", "[rego|fragment]")
	guestOS           = flag.String("os", "linux", "guest OS [linux|windows]")
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

		var policyCode string
		if *outputType == "fragment" {
			switch *guestOS {
			case "linux":
				// windows_container entries are ignored when targeting linux.
				config.Containers = append(config.Containers, helpers.DefaultContainerConfigs()...)
				policyContainers, cerr := helpers.PolicyContainersFromConfigs(config.Containers)
				if cerr != nil {
					return cerr
				}
				policyCode, err = securitypolicy.MarshalFragment(
					*fragmentNamespace,
					*fragmentSVN,
					policyContainers,
					config.ExternalProcesses,
					config.Fragments)
			case "windows":
				// container (linux) entries are ignored when targeting windows.
				policyContainers, cerr := helpers.PolicyWindowsContainersFromConfigs(context.Background(), config.WindowsContainers)
				if cerr != nil {
					return cerr
				}
				policyCode, err = securitypolicy.MarshalWindowsFragment(
					*fragmentNamespace,
					*fragmentSVN,
					policyContainers,
					config.ExternalProcesses,
					config.Fragments)
			default:
				return fmt.Errorf("unsupported guest OS %q", *guestOS)
			}
		} else {
			switch *guestOS {
			case "linux":
				// windows_container entries are ignored when targeting linux.
				config.Containers = append(config.Containers, helpers.DefaultContainerConfigs()...)
				policyContainers, cerr := helpers.PolicyContainersFromConfigs(config.Containers)
				if cerr != nil {
					return cerr
				}
				policyCode, err = securitypolicy.MarshalPolicy(
					*outputType,
					config.AllowAll,
					policyContainers,
					config.ExternalProcesses,
					config.Fragments,
					config.AllowPropertiesAccess,
					config.AllowDumpStacks,
					config.AllowRuntimeLogging,
					config.AllowHostNetwork,
					config.AllowEnvironmentVariableDropping,
					config.AllowUnencryptedScratch,
					config.AllowCapabilityDropping,
					config.AllowRegistryChangesDropping,
					config.AllowLogProviderDropping,
				)
			case "windows":
				// container (linux) entries are ignored when targeting windows.
				policyContainers, cerr := helpers.PolicyWindowsContainersFromConfigs(context.Background(), config.WindowsContainers)
				if cerr != nil {
					return cerr
				}
				policyCode, err = securitypolicy.MarshalWindowsPolicy(
					*outputType,
					config.AllowAll,
					policyContainers,
					config.ExternalProcesses,
					config.Fragments,
					config.AllowPropertiesAccess,
					config.AllowDumpStacks,
					config.AllowRuntimeLogging,
					config.AllowHostNetwork,
					config.AllowEnvironmentVariableDropping,
					config.AllowUnencryptedScratch,
					config.AllowCapabilityDropping,
					config.AllowRegistryChangesDropping,
					config.AllowLogProviderDropping,
				)
			default:
				return fmt.Errorf("unsupported guest OS %q", *guestOS)
			}
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
