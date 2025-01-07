package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"path"
	"strings"

	"github.com/Microsoft/hcsshim/internal/guestpath"
	rpi "github.com/Microsoft/hcsshim/internal/regopolicyinterpreter"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

type command struct {
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

var (
	commandsPath = flag.String("commands", "", "path commands JSON file")
	policyPath   = flag.String("policy", "", "path to policy Rego file")
	dataPath     = flag.String("data", "", "path initial data state JSON file (optional)")
	logPath      = flag.String("log", "", "path to output log file")
	logLevelName = flag.String("logLevel", "Info", "None|Info|Results|Metadata")
)

func readCommands() []command {
	content, err := os.ReadFile(*commandsPath)
	if err != nil {
		log.Fatal(err)
	}

	var commands []command
	err = json.Unmarshal(content, &commands)
	if err != nil {
		log.Fatal(err)
	}

	return commands
}

func createInterpreter() *rpi.RegoPolicyInterpreter {
	content, err := os.ReadFile(*policyPath)
	if err != nil {
		log.Fatal(err)
	}

	policyCode := string(content)

	var logLevel rpi.LogLevel
	switch strings.ToLower(*logLevelName) {
	case "none":
		logLevel = rpi.LogNone

	case "info":
		logLevel = rpi.LogInfo

	case "results":
		logLevel = rpi.LogResults

	case "metadata":
		logLevel = rpi.LogMetadata

	default:
		log.Fatalf("unrecognized log level: %s", *logLevelName)
	}

	var data map[string]interface{}
	if len(*dataPath) > 0 {
		contents, err := os.ReadFile(*dataPath)
		if err != nil {
			log.Fatal(err)
		}

		err = json.Unmarshal(contents, &data)
		if err != nil {
			log.Fatalf("error loading initial data state: %v", err)
		}
	} else {
		data = map[string]interface{}{
			"defaultMounts":                   []interface{}{},
			"privilegedMounts":                []interface{}{},
			"sandboxPrefix":                   guestpath.SandboxMountPrefix,
			"hugePagesPrefix":                 guestpath.HugePagesMountPrefix,
			"defaultPrivilegedCapabilities":   securitypolicy.DefaultPrivilegedCapabilities(),
			"defaultUnprivilegedCapabilities": securitypolicy.DefaultUnprivilegedCapabilities(),
		}
	}

	r, err := rpi.NewRegoPolicyInterpreter(policyCode, data)
	r.UpdateOSType("linux")
	if err != nil {
		log.Fatal(err)
	}

	if len(*logPath) > 0 {
		if _, err := os.Stat(*logPath); err == nil {
			os.Remove(*logPath)
		}

		if logLevel == rpi.LogNone {
			log.Println("logPath provided but logLevel set to None: turning off logging")
		} else {
			log.Printf("turning on logging to %s with level %s\n", *logPath, *logLevelName)
		}
		err = r.EnableLogging(*logPath, logLevel)
		if err != nil {
			log.Fatal(err)
		}
	}

	r.AddModule("framework.rego", &rpi.RegoModule{Namespace: "framework", Code: securitypolicy.FrameworkCode})
	r.AddModule("api.rego", &rpi.RegoModule{Namespace: "api", Code: securitypolicy.APICode})

	return r
}

func parseNamespace(rego string) string {
	lines := strings.Split(rego, "\n")
	parts := strings.Split(lines[0], " ")
	if parts[0] != "package" || len(parts) < 2 {
		log.Fatal("package definition required on first line of Rego module")
	}

	namespace := parts[1]
	return namespace
}

func loadLocalFragment(commandsDir string, input map[string]interface{}) *rpi.RegoModule {
	var localPath string
	var ok bool
	if localPath, ok = input["local_path"].(string); !ok {
		log.Println(input)
		log.Fatal("'load_fragment' requires a 'local_path' member in 'input' which points to a local Rego file with the fragment logic")
	}

	content, err := os.ReadFile(localPath)
	if err != nil {
		localPath = path.Join(commandsDir, localPath)
		content, err = os.ReadFile(localPath)
		if err != nil {
			log.Fatalf("unable to load fragment: %v", err)
		}
	}

	code := string(content)
	return &rpi.RegoModule{
		Namespace: parseNamespace(code),
		Feed:      input["feed"].(string),
		Issuer:    input["issuer"].(string),
		Code:      code,
	}
}

func main() {
	flag.Parse()
	if flag.NArg() != 0 || len(*policyPath) == 0 || len(*commandsPath) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	commands := readCommands()
	rego := createInterpreter()

	for i, command := range commands {
		var fragment *rpi.RegoModule
		if command.Name == "load_fragment" {
			fragment = loadLocalFragment(path.Dir(*commandsPath), command.Input)
			rego.AddModule(fragment.ID(), fragment)
		}

		result, err := rego.Query("data.policy."+command.Name, command.Input)
		if err != nil {
			inputJSON, _ := json.Marshal(command.Input)
			log.Fatalf("query of %s with input %s failed with error %v",
				command.Name,
				inputJSON,
				err)
		}

		addModule, _ := result.Bool("add_module")

		removeModule := true
		if fragment != nil && addModule {
			removeModule = false
		}

		allowed, err := result.Bool("allowed")
		if err != nil {
			log.Fatalf("policy result missing required `allowed` key: %v", err)
		}

		if allowed {
			log.Printf("%02d> %s ok\n", i, command.Name)
		} else {
			log.Printf("%02d> %s not allowed", i, command.Name)
			command.Input["rule"] = command.Name
			result, err = rego.Query("data.policy.reason", command.Input)
			if err != nil {
				log.Fatalf("unable to get reason for failure: %v", err)
			}

			if !result.IsEmpty() {
				errors, _ := result.Value("errors")
				log.Printf("Reason: %v", errors)
			}
		}

		if removeModule && fragment != nil {
			rego.RemoveModule(fragment.ID())
		}
	}
}
