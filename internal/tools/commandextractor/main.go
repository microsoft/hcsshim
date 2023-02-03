package main

/*
This executable creates JSON command files that are compatible with the
`policyenginesimulator` tool. It does that by reading a TOML configuration
file in the same format as the TOML configuration files used by
`securitypolicytool`. The program process that configuration and then extracts
container information by inspecting the referenced images directly (downloading
them if necessary, which can create a delay) to extract layer information,
environment variables, and so forth. It shares this code with the
`securitypolicytool`.

It then uses that information to construct the input
objects and enforcement points which would be the result of HCS/GCS exercising
that configuration. This means:

1. Loading all fragments (using paths to local copies of the Rego)
2. Executing all external processes
3. Grabbing debug information from the UVM
4. For each container:
   a. Mount all layers
   b. Mount an overlay
   c. Mount a scratch volume
   d. Mount all plan9s
   e. Create a container
   f. Run all container processes
   g. Shutdown a container
   h. Unmount plan9s
   i. Unmount scratch
   j. Unmount an overlay
   k. Unmount all layers

And writes all of these to the output as JSON commands.
*/

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/Microsoft/hcsshim/internal/tools/securitypolicy/helpers"
	sp "github.com/Microsoft/hcsshim/pkg/securitypolicy"
	"github.com/pelletier/go-toml"
)

type command struct {
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

var (
	policyPath    = flag.String("policy", "", "path policy configuration TOML")
	fragmentsPath = flag.String("fragments", "", "path to one or more fragment configuration TOML files")
)

var numContainers int = 0

func generateContainerID() string {
	id := fmt.Sprintf("container%d", numContainers)
	numContainers += 1
	return id
}

var numLayers int = 0

func generateLayerID() string {
	id := fmt.Sprintf("layer%d", numLayers)
	numLayers += 1
	return id
}

var numPlan9 int = 0

func generatePlan9Source(containerID string) string {
	source := fmt.Sprintf(
		"%s/%s%s",
		guestpath.LCOWRootPrefixInUVM, containerID,
		fmt.Sprintf(guestpath.LCOWMountPathPrefixFmt, numPlan9),
	)
	numPlan9 += 1
	return source
}

var numSandboxes int = 0

func generateSandboxID() string {
	id := fmt.Sprintf("sandbox%d", numSandboxes)
	numSandboxes += 1
	return id
}

func fragmentToCommand(fragment sp.FragmentConfig) command {
	return command{
		Name: "load_fragment",
		Input: map[string]interface{}{
			"issuer":     fragment.Issuer,
			"feed":       fragment.Feed,
			"local_path": "relative/path/to/local/fragment.rego",
		},
	}
}

func externalProcessToCommand(process sp.ExternalProcessConfig) command {
	return command{
		Name: "exec_external",
		Input: map[string]interface{}{
			"argList":    process.Command,
			"workingDir": process.WorkingDir,
			"envList":    []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
		},
	}
}

// BEGIN COPY
// These functions reproduce the functionality of exported functions with
// the same name from github.com/Microsoft/hcsshim/internal/guest/spec
// which are not built on windows and thus cannot be used by this tool.
// If they change, then this tool will no longer work correctly.

func sandboxRootDir(sandboxID string) string {
	return filepath.Join(guestpath.LCOWRootPrefixInUVM, sandboxID)
}

func sandboxMountsDir(sandboxID string) string {
	return filepath.Join(sandboxRootDir(sandboxID), "sandboxMounts")
}

func hugePagesMountsDir(sandboxID string) string {
	return filepath.Join(sandboxRootDir(sandboxID), "hugepages")
}

func sandboxMountSource(sandboxID, path string) string {
	mountsDir := sandboxMountsDir(sandboxID)
	subPath := strings.TrimPrefix(path, guestpath.SandboxMountPrefix)
	return filepath.Join(mountsDir, subPath)
}

func hugePagesMountSource(sandboxID, path string) string {
	mountsDir := hugePagesMountsDir(sandboxID)
	subPath := strings.TrimPrefix(path, guestpath.HugePagesMountPrefix)
	return filepath.Join(mountsDir, subPath)
}

// END COPY

func toArray(array sp.StringArrayMap) []string {
	numElements := len(array.Elements)
	elements := make([]string, 0, numElements)
	for i := 0; i < numElements; i++ {
		elements = append(elements, array.Elements[strconv.Itoa(i)])
	}
	return elements
}

func containerToCommands(container *sp.Container) []command {
	commands := make([]command, 0)
	numLayers := len(container.Layers.Elements)
	layerPaths := make([]string, numLayers)
	for layer := 0; layer < numLayers; layer++ {
		layerTarget := fmt.Sprintf("/mnt/%s", generateLayerID())
		layerPaths[layer] = layerTarget
		deviceHash := container.Layers.Elements[strconv.Itoa(numLayers-layer-1)]
		commands = append(commands, command{
			Name: "mount_device",
			Input: map[string]interface{}{
				"deviceHash": deviceHash,
				"target":     layerTarget,
			},
		})
	}

	containerID := generateContainerID()
	sandboxID := generateSandboxID()
	overlayTarget := fmt.Sprintf("/mnt/%s", generateLayerID())
	commands = append(commands, command{
		Name: "mount_overlay",
		Input: map[string]interface{}{
			"containerID": containerID,
			"layerPaths":  layerPaths,
			"target":      overlayTarget,
		},
	})

	// mount scratch
	scratchTarget := fmt.Sprintf("/mnt/%s", generateLayerID())
	commands = append(commands, command{
		Name: "scratch_mount",
		Input: map[string]interface{}{
			"target":    scratchTarget,
			"encrypted": true,
		},
	})

	envList := make([]string, 0, container.EnvRules.Length)
	for _, env := range container.EnvRules.Elements {
		if env.Strategy == "string" {
			envList = append(envList, env.Rule)
		}
	}

	mountPathPrefix := strings.Replace(guestpath.LCOWMountPathPrefixFmt, "%d", "[0-9]+", 1)
	plan9Sources := []string{}
	mounts := make([]interface{}, 0, container.Mounts.Length)
	for _, mount := range container.Mounts.Elements {
		source := mount.Source
		if strings.HasPrefix(mount.Source, "plan9://") {
			source = generatePlan9Source(containerID)
			plan9Sources = append(plan9Sources, source)
			commands = append(commands, command{
				Name: "plan9_mount",
				Input: map[string]interface{}{
					"rootPrefix":      guestpath.LCOWRootPrefixInUVM,
					"mountPathPrefix": mountPathPrefix,
					"target":          source,
				},
			})
		} else if strings.HasPrefix(source, guestpath.SandboxMountPrefix) {
			source = sandboxMountSource(sandboxID, source)
		} else if strings.HasPrefix(source, guestpath.HugePagesMountPrefix) {
			source = hugePagesMountSource(sandboxID, source)
		}

		mounts = append(mounts, map[string]interface{}{
			"destination": mount.Destination,
			"source":      source,
			"options":     toArray(sp.StringArrayMap(mount.Options)),
			"type":        mount.Type,
		})
	}

	argList := toArray(sp.StringArrayMap(container.Command))

	commands = append(commands, command{
		Name: "create_container",
		Input: map[string]interface{}{
			"containerID":  containerID,
			"argList":      argList,
			"envList":      envList,
			"workingDir":   container.WorkingDir,
			"sandboxDir":   sandboxMountsDir(sandboxID),
			"hugePagesDir": hugePagesMountsDir(sandboxID),
			"mounts":       mounts,
		},
	})

	for _, signal := range container.Signals {
		commands = append(commands, command{
			Name: "signal_container_process",
			Input: map[string]interface{}{
				"containerID":   containerID,
				"signal":        signal,
				"isInitProcess": true,
				"argList":       argList,
			},
		})
	}

	for _, process := range container.ExecProcesses {
		commands = append(commands, command{
			Name: "exec_in_container",
			Input: map[string]interface{}{
				"containerID": containerID,
				"argList":     process.Command,
				"envList":     envList,
				"workingDir":  container.WorkingDir,
			},
		})

		for _, signal := range process.Signals {
			commands = append(commands, command{
				Name: "signal_container_process",
				Input: map[string]interface{}{
					"containerID":   containerID,
					"signal":        signal,
					"isInitProcess": false,
					"argList":       process.Command,
				},
			})
		}
	}

	commands = append(commands, command{
		Name: "shutdown_container",
		Input: map[string]interface{}{
			"containerID": containerID,
		},
	})

	for _, plan9Target := range plan9Sources {
		commands = append(commands, command{
			Name: "plan9_unmount",
			Input: map[string]interface{}{
				"unmountTarget": plan9Target,
			},
		})
	}

	commands = append(commands, command{
		Name: "scratch_unmount",
		Input: map[string]interface{}{
			"unmountTarget": scratchTarget,
		},
	})

	commands = append(commands, command{
		Name: "unmount_overlay",
		Input: map[string]interface{}{
			"unmountTarget": overlayTarget,
		},
	})

	for _, unmountTarget := range layerPaths {
		commands = append(commands, command{
			Name: "unmount_device",
			Input: map[string]interface{}{
				"unmountTarget": unmountTarget,
			},
		})
	}

	return commands
}

func policyConfigToCommands(path string) []command {
	configData, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("error reading policy TOML: %v", err)
	}

	config := &sp.PolicyConfig{}

	err = toml.Unmarshal(configData, config)
	if err != nil {
		log.Fatalf("error reading policy TOML: %v", err)
	}

	defaultContainers := helpers.DefaultContainerConfigs()
	config.Containers = append(config.Containers, defaultContainers...)
	policyContainers, err := helpers.PolicyContainersFromConfigs(config.Containers)
	if err != nil {
		log.Fatalf("error reading containers from images: %v", err)
	}

	commands := []command{}

	// load all fragments first
	for _, fragment := range config.Fragments {
		commands = append(commands, fragmentToCommand(fragment))
	}

	// run all external processes
	for _, process := range config.ExternalProcesses {
		commands = append(commands, externalProcessToCommand(process))
	}

	// debugging methods
	if config.AllowPropertiesAccess {
		commands = append(commands, command{
			Name:  "get_properties",
			Input: map[string]interface{}{},
		})
	}

	if config.AllowDumpStacks {
		commands = append(commands, command{
			Name:  "dump_stacks",
			Input: map[string]interface{}{},
		})
	}

	if config.AllowRuntimeLogging {
		commands = append(commands, command{
			Name:  "runtime_logging",
			Input: map[string]interface{}{},
		})
	}

	// create all containers and run their processes
	for _, container := range policyContainers {
		commands = append(commands, containerToCommands(container)...)
	}

	return commands
}

func fragmentConfigToCommands(path string) []command {
	configData, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("error reading fragment TOML: %v", err)
	}

	config := &sp.PolicyConfig{}

	err = toml.Unmarshal(configData, config)
	if err != nil {
		log.Fatalf("error reading fragment TOML: %v", err)
	}

	policyContainers, err := helpers.PolicyContainersFromConfigs(config.Containers)
	if err != nil {
		log.Fatalf("error reading containers from images: %v", err)
	}

	commands := []command{}

	// load all fragments first
	for _, fragment := range config.Fragments {
		commands = append(commands, fragmentToCommand(fragment))
	}

	// run all external processes
	for _, process := range config.ExternalProcesses {
		commands = append(commands, externalProcessToCommand(process))
	}

	// create all containers and run their processes
	for _, container := range policyContainers {
		commands = append(commands, containerToCommands(container)...)
	}

	return commands
}

func main() {
	flag.Parse()
	if flag.NArg() != 0 || len(*policyPath) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	commands := policyConfigToCommands(*policyPath)
	fragments := strings.Split(*fragmentsPath, " ")
	for _, fragment := range fragments {
		commands = append(commands, fragmentConfigToCommands(fragment)...)
	}

	contents, err := json.MarshalIndent(commands, "", "    ")
	if err != nil {
		log.Fatalf("error when serializing commands: %v", err)
	}

	fmt.Printf("%s\n", string(contents[:]))
}
