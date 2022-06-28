//go:build linux
// +build linux

package securitypolicy

import (
	"fmt"
	"testing"

	oci "github.com/opencontainers/runtime-spec/specs-go"
)

func newCommandFromInternal(args []string) CommandArgs {
	command := CommandArgs{}
	command.Length = len(args)
	command.Elements = make(map[string]string)
	for i, arg := range args {
		command.Elements[fmt.Sprint(i)] = arg
	}
	return command
}

func newEnvRulesFromInternal(rules []EnvRuleConfig) EnvRules {
	envRules := EnvRules{}
	envRules.Length = len(rules)
	envRules.Elements = make(map[string]EnvRuleConfig)
	for i, rule := range rules {
		envRules.Elements[fmt.Sprint(i)] = rule
	}
	return envRules
}

func newLayersFromInternal(hashes []string) Layers {
	layers := Layers{}
	layers.Length = len(hashes)
	layers.Elements = make(map[string]string)
	for i, hash := range hashes {
		layers.Elements[fmt.Sprint(i)] = hash
	}
	return layers
}

func newOptionsFromInternal(optionsInternal []string) Options {
	options := Options{}
	options.Length = len(optionsInternal)
	options.Elements = make(map[string]string)
	for i, arg := range optionsInternal {
		options.Elements[fmt.Sprint(i)] = arg
	}
	return options
}

func newMountsFromInternal(mountsInternal []mountInternal) Mounts {
	mounts := Mounts{}
	mounts.Length = len(mountsInternal)
	mounts.Elements = make(map[string]Mount)
	for i, mount := range mountsInternal {
		mounts.Elements[fmt.Sprint(i)] = Mount{
			Source:      mount.Source,
			Destination: mount.Destination,
			Options:     newOptionsFromInternal(mount.Options),
			Type:        mount.Type,
		}
	}

	return mounts
}

func TestConvert(t *testing.T) {
	gc := generateContainers(testRand, 4)
	securityPolicy := SecurityPolicy{}
	securityPolicy.AllowAll = false
	securityPolicy.Containers.Length = len(gc.containers)
	securityPolicy.Containers.Elements = make(map[string]Container)
	for i, c := range gc.containers {
		container := Container{
			AllowElevated: c.allowElevated,
			WorkingDir:    c.WorkingDir,
			Command:       newCommandFromInternal(c.Command),
			EnvRules:      newEnvRules(c.EnvRules),
			Layers:        newLayersFromInternal(c.Layers),
			Mounts:        newMountsFromInternal(c.Mounts),
		}
		securityPolicy.Containers.Elements[fmt.Sprint(i)] = container
	}

	base64policy, err := securityPolicy.EncodeToString()

	if err != nil {
		t.Fatalf("unable to encode policy to base64: %v", err)
	}

	_, err = NewRegoPolicyFromBase64Json(base64policy, []oci.Mount{}, []oci.Mount{})
	if err != nil {
		t.Fatalf("unable to convert policy to rego: %v", err)
	}
}
