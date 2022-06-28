package securitypolicy

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	oci "github.com/opencontainers/runtime-spec/specs-go"
)

var PolicyCode string = `
package main

import future.keywords.every
import future.keywords.in

default mount_device := false
mount_device := true {
    some container in data.policy.containers
    some layer in container.layers
    input.deviceHash == layer
}

mount_device := true {
	data.policy.allow_all
}

default mount_overlay := false
mount_overlay := true {
    some container in data.policy.containers
    count(container.layers) == count(input.layerPaths)
    every i, path in input.layerPaths {
        container.layers[i] == data.devices[path]
    }
}

mount_overlay := true {
	data.policy.allow_all
}

command_ok(container) {
    count(input.argList) == count(container.command)
    every i, arg in input.argList {
        container.command[i] == arg
    }
}

env_ok(pattern, "string", value) {
    pattern == value
}

env_ok(pattern, "re2", value) {
    regex.match(pattern, value)
}

envList_ok(container) {
    every env in input.envList {
        some rule in container.env_rules
        env_ok(rule.pattern, rule.strategy, env)
    }
}

mountList_ok(container) {
    every mount in input.mounts {
        some constraint in container.mounts
        mount.type == constraint.type
        regex.match(constraint.source, mount.source)
        mount.destination != ""
        mount.destination == constraint.destination
        every option in mount.options {
            some constraintOption in constraint.options
            option == constraintOption
        }
    }
}

default create_container := false
create_container := true {
    not input.containerID in data.started
    some container in data.policy.containers
    command_ok(container)
    envList_ok(container)
    mountList_ok(container)
    input.workingDir == container.working_dir
    input.allowElevated == container.allow_elevated
}

create_container := true {
	data.policy.allow_all
}
`

var Indent string = "    "

type RegoPolicy struct {
	containersCode string
	policyCode     string
	data           map[string]interface{}
}

func toOptions(values []string) Options {
	elements := make(map[string]string)
	for i, value := range values {
		elements[fmt.Sprint(i)] = value
	}
	return Options{
		Length:   len(values),
		Elements: elements,
	}
}

func (mounts *Mounts) Append(other []oci.Mount) {
	start := mounts.Length + 1
	for i, mount := range other {
		mounts.Elements[fmt.Sprint(i+start)] = Mount{
			Source:      mount.Source,
			Destination: mount.Destination,
			Type:        mount.Type,
			Options:     toOptions(mount.Options),
		}
	}

	mounts.Length += len(other)
}

func injectMounts(policy *SecurityPolicy, defaultMounts []oci.Mount, privilegedMounts []oci.Mount) error {
	for _, container := range policy.Containers.Elements {
		if container.AllowElevated {
			container.Mounts.Append(privilegedMounts)
		}

		container.Mounts.Append(defaultMounts)
	}

	return nil
}

func (array StringArrayMap) MarshalRego() (string, error) {
	values := make([]string, array.Length)
	for key, value := range array.Elements {
		index, err := strconv.Atoi(key)
		if err != nil {
			return "", fmt.Errorf("string array map index %v not an int", key)
		}
		values[index] = value
	}

	return fmt.Sprintf("[%s]", strings.Join(values, ",")), nil
}

func writeCommand(builder *strings.Builder, command CommandArgs, indent string) error {
	array, err := (StringArrayMap(command)).MarshalRego()
	if err != nil {
		return err
	}

	if _, err := builder.WriteString(fmt.Sprintf("%scommand: %s,", indent, array)); err != nil {
		return err
	}

	return nil
}

func (e EnvRuleConfig) MarshalRego() string {
	return fmt.Sprintf("{\"pattern\": \"%s\", \"strategy\": \"%s\"}", e.Rule, e.Strategy)
}

func (e EnvRules) MarshalRego() (string, error) {
	values := make([]string, e.Length)
	for key, value := range e.Elements {
		index, err := strconv.Atoi(key)
		if err != nil {
			return "", fmt.Errorf("string array map index %v not an int", key)
		}
		values[index] = value.MarshalRego()
	}

	return fmt.Sprintf("[%s]", strings.Join(values, ",")), nil
}

func writeEnvRules(builder *strings.Builder, envRules EnvRules, indent string) error {
	array, err := envRules.MarshalRego()

	if err != nil {
		return err
	}

	_, err = builder.WriteString(fmt.Sprintf("%s\"env_rules\": %s\n", indent, array))
	return err
}

func writeLayers(builder *strings.Builder, layers Layers, indent string) error {
	array, err := (StringArrayMap(layers)).MarshalRego()
	if err != nil {
		return err
	}

	if _, err := builder.WriteString(fmt.Sprintf("%slayers: %s,", indent, array)); err != nil {
		return err
	}

	return nil
}

func (m Mount) MarshalRego() (string, error) {
	options, err := StringArrayMap(m.Options).MarshalRego()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("{\"destination\": \"%s\", \"options\": %s, \"source\": \"%s\", \"type\": \"%s\"}", m.Destination, options, m.Source, m.Type), nil
}

func (m Mounts) MarshalRego() (string, error) {
	values := make([]string, m.Length)
	for key, value := range m.Elements {
		index, err := strconv.Atoi(key)
		if err != nil {
			return "", fmt.Errorf("string array map index %v not an int", key)
		}
		mount, err := value.MarshalRego()
		if err != nil {
			return "", err
		}

		values[index] = mount
	}

	return fmt.Sprintf("[%s]", strings.Join(values, ",")), nil
}

func writeMounts(builder *strings.Builder, mounts Mounts, indent string) error {
	array, err := mounts.MarshalRego()

	if err != nil {
		return err
	}

	_, err = builder.WriteString(fmt.Sprintf("%s\"env_rules\": %s\n", indent, array))
	return err
}

func writeContainer(builder *strings.Builder, container Container, indent string) error {
	if _, err := builder.WriteString(fmt.Sprintf("%s{\n", indent)); err != nil {
		return err
	}

	if err := writeCommand(builder, container.Command, indent+Indent); err != nil {
		return err
	}

	if err := writeEnvRules(builder, container.EnvRules, indent+Indent); err != nil {
		return err
	}

	if err := writeLayers(builder, container.Layers, indent+Indent); err != nil {
		return err
	}

	if err := writeMounts(builder, container.Mounts, indent+Indent); err != nil {
		return err
	}

	if _, err := builder.WriteString(fmt.Sprintf("%s\"allow_elevated\": %v,", indent, container.AllowElevated)); err != nil {
		return err
	}

	if _, err := builder.WriteString(fmt.Sprintf("%s\"working_dir\": %s", indent, container.WorkingDir)); err != nil {
		return err
	}

	if _, err := builder.WriteString(fmt.Sprintf("%s}\n", indent)); err != nil {
		return err
	}

	return nil
}

func addContainers(builder *strings.Builder, containers Containers) error {
	if _, err := builder.WriteString("containers := [\n"); err != nil {
		return err
	}

	for _, container := range containers.Elements {
		if err := writeContainer(builder, container, Indent); err != nil {
			return err
		}
	}

	if _, err := builder.WriteString("]\n"); err != nil {
		return err
	}

	return nil
}

func (p SecurityPolicy) MarshalRego() (string, error) {
	builder := new(strings.Builder)
	if _, err := builder.WriteString(fmt.Sprintf("package policy\nallow_all := %v", p.AllowAll)); err != nil {
		return "", err
	}

	if err := addContainers(builder, p.Containers); err != nil {
		return "", err
	}

	return builder.String(), nil
}

func NewRegoPolicyFromBase64Json(base64policy string, defaultMounts []oci.Mount, privilegedMounts []oci.Mount) (*RegoPolicy, error) {
	jsonPolicy, err := base64.StdEncoding.DecodeString(base64policy)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 security policy: %w", err)
	}

	securityPolicy := new(SecurityPolicy)
	if err := json.Unmarshal(jsonPolicy, securityPolicy); err != nil {
		return nil, errors.Wrap(err, "unable to unmarshal JSON policy")
	}

	if err := injectMounts(securityPolicy, defaultMounts, privilegedMounts); err != nil {
		return nil, err
	}

	policy := new(RegoPolicy)
	policy.policyCode = PolicyCode
	if policy.containersCode, err = securityPolicy.MarshalRego(); err != nil {
		return nil, fmt.Errorf("failed to convert json to rego: %w", err)
	}

	policy.data = make(map[string]interface{})
	return policy, nil
}
