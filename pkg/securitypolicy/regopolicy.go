package securitypolicy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/pkg/errors"

	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/storage/inmem"
	"github.com/open-policy-agent/opa/topdown"
	oci "github.com/opencontainers/runtime-spec/specs-go"
)

var MainCode string = `
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
	length := count(container.layers)
    count(input.layerPaths) == length
    every i, path in input.layerPaths {
        container.layers[length - i - 1] == data.devices[path]
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

default command_matches := false
command_matches := true {
	some container in data.policy.containers
	command_ok(container)
}

reason["invalid command"] {
	not command_matches
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

default envList_matches := false
envList_matches := true {
	some container in data.policy.containers
	envList_ok(container)
}

reason["invalid env list"] {
	not envList_matches
}

workingDirectory_ok(container) {
	input.workingDir == container.working_dir
}

default workingDirectory_matches := false
workingDirectory_matches := true {
	some container in data.policy.containers
	workingDirectory_ok(container)
}

reason["invalid working directory"] {
	not workingDirectory_matches
}

default container_started := false
container_started := true {
	input.containerID in data.started
}

reason["container already started"] {
	container_started
}

default create_container := false
create_container := true {
    not container_started
    some container in data.policy.containers
    command_ok(container)
    envList_ok(container)
	workingDirectory_ok(container)
}

create_container := true {
	data.policy.allow_all
}
`

var Indent string = "    "

type RegoPolicy struct {
	// Rego which describes policy behavior (see above)
	mainCode string
	// Rego which describes policy objects (containers, etc.)
	policyCode string
	// Mutex to prevent concurrent access to fields
	mutex *sync.Mutex
	// Rego data object, used to store policy state
	data map[string]interface{}
	// Base64 encoded (JSON) policy
	base64policy string
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
	for i := 0; i < array.Length; i++ {
		if value, found := array.Elements[fmt.Sprint(i)]; found {
			values[i] = fmt.Sprintf("\"%s\"", value)
		} else {
			return "", fmt.Errorf("\"%d\" missing from elements", i)
		}
	}

	return fmt.Sprintf("[%s]", strings.Join(values, ",")), nil
}

func writeCommand(builder *strings.Builder, command CommandArgs, indent string) error {
	array, err := (StringArrayMap(command)).MarshalRego()
	if err != nil {
		return err
	}

	if _, err := builder.WriteString(fmt.Sprintf("%s\"command\": %s,\n", indent, array)); err != nil {
		return err
	}

	return nil
}

func (e EnvRuleConfig) MarshalRego() string {
	return fmt.Sprintf("{\"pattern\": \"%s\", \"strategy\": \"%s\"}", e.Rule, e.Strategy)
}

func (e EnvRules) MarshalRego() (string, error) {
	values := make([]string, e.Length)
	for i := 0; i < e.Length; i++ {
		if value, found := e.Elements[fmt.Sprint(i)]; found {
			values[i] = value.MarshalRego()
		} else {
			return "", fmt.Errorf("\"%d\" missing from env rules elements", i)
		}
	}

	return fmt.Sprintf("[%s]", strings.Join(values, ",")), nil
}

func writeEnvRules(builder *strings.Builder, envRules EnvRules, indent string) error {
	array, err := envRules.MarshalRego()

	if err != nil {
		return err
	}

	_, err = builder.WriteString(fmt.Sprintf("%s\"env_rules\": %s,\n", indent, array))
	return err
}

func writeLayers(builder *strings.Builder, layers Layers, indent string) error {
	array, err := (StringArrayMap(layers)).MarshalRego()
	if err != nil {
		return err
	}

	if _, err := builder.WriteString(fmt.Sprintf("%s\"layers\": %s,\n", indent, array)); err != nil {
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
	for i := 0; i < m.Length; i++ {
		if value, found := m.Elements[fmt.Sprint(i)]; found {
			if mount, err := value.MarshalRego(); err == nil {
				values[i] = mount
			} else {
				return "", err
			}
		} else {
			return "", fmt.Errorf("\"%d\" missing from mounts elements", i)
		}
	}

	return fmt.Sprintf("[%s]", strings.Join(values, ",")), nil
}

func writeMounts(builder *strings.Builder, mounts Mounts, indent string) error {
	array, err := mounts.MarshalRego()

	if err != nil {
		return err
	}

	_, err = builder.WriteString(fmt.Sprintf("%s\"mounts\": %s,\n", indent, array))
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

	if _, err := builder.WriteString(fmt.Sprintf("%s\"allow_elevated\": %v,\n", indent+Indent, container.AllowElevated)); err != nil {
		return err
	}

	if _, err := builder.WriteString(fmt.Sprintf("%s\"working_dir\": \"%s\"\n", indent+Indent, container.WorkingDir)); err != nil {
		return err
	}

	if _, err := builder.WriteString(fmt.Sprintf("%s}", indent)); err != nil {
		return err
	}

	return nil
}

func addContainers(builder *strings.Builder, containers Containers) error {
	if _, err := builder.WriteString("containers := [\n"); err != nil {
		return err
	}

	for i := 0; i < containers.Length; i++ {
		if container, found := containers.Elements[fmt.Sprint(i)]; found {
			if err := writeContainer(builder, container, Indent); err != nil {
				return err
			}

			var end string
			if i < containers.Length-1 {
				end = ",\n"
			} else {
				end = "\n"
			}

			if _, err := builder.WriteString(end); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("\"%d\" missing from containers elements", i)
		}
	}

	if _, err := builder.WriteString("]\n"); err != nil {
		return err
	}

	return nil
}

func (p SecurityPolicy) MarshalRego() (string, error) {
	builder := new(strings.Builder)
	if _, err := builder.WriteString(fmt.Sprintf("package policy\nallow_all := %v\n", p.AllowAll)); err != nil {
		return "", err
	}

	if err := addContainers(builder, p.Containers); err != nil {
		return "", err
	}

	return builder.String(), nil
}

func NewRegoPolicyFromBase64Json(base64policy string, defaultMounts []oci.Mount, privilegedMounts []oci.Mount) (*RegoPolicy, error) {
	securityPolicy := new(SecurityPolicy)
	if jsonPolicy, err := base64.StdEncoding.DecodeString(base64policy); err == nil {
		if err2 := json.Unmarshal(jsonPolicy, securityPolicy); err2 != nil {
			return nil, errors.Wrap(err2, "unable to unmarshal JSON policy")
		}
	} else {
		return nil, fmt.Errorf("failed to decode base64 security policy: %w", err)
	}

	if policy, err := NewRegoPolicyFromSecurityPolicy(securityPolicy, defaultMounts, privilegedMounts); err == nil {
		policy.base64policy = base64policy
		return policy, nil
	} else {
		return nil, err
	}
}

func NewRegoPolicyFromSecurityPolicy(securityPolicy *SecurityPolicy, defaultMounts []oci.Mount, privilegedMounts []oci.Mount) (*RegoPolicy, error) {
	if err := injectMounts(securityPolicy, defaultMounts, privilegedMounts); err != nil {
		return nil, err
	}

	policy := new(RegoPolicy)
	policy.mainCode = MainCode
	if code, err := securityPolicy.MarshalRego(); err == nil {
		policy.policyCode = code
	} else {
		return nil, fmt.Errorf("failed to convert json to rego: %w", err)
	}

	policy.data = make(map[string]interface{})
	policy.mutex = new(sync.Mutex)
	policy.base64policy = ""
	return policy, nil
}

func (policy RegoPolicy) Query(input map[string]interface{}) (rego.ResultSet, error) {
	store := inmem.NewFromObject(policy.data)

	var buf bytes.Buffer
	rule := input["name"].(string)
	query := rego.New(
		rego.Query(fmt.Sprintf("data.main.%s", rule)),
		rego.Module("main", policy.mainCode),
		rego.Module("policy", policy.policyCode),
		rego.Input(input),
		rego.Store(store),
		rego.EnablePrintStatements(true),
		rego.PrintHook(topdown.NewPrintHook(&buf)))

	ctx := context.Background()
	results, err := query.Eval(ctx)
	if err != nil {
		fmt.Println("Policy", policy.policyCode)
		fmt.Println(err)
		return results, err
	}

	output := buf.String()
	if len(output) > 0 {
		fmt.Println(output)
	}

	return results, nil
}

func (policy *RegoPolicy) EnforceDeviceMountPolicy(target string, deviceHash string) error {
	policy.mutex.Lock()
	defer policy.mutex.Unlock()

	input := map[string]interface{}{
		"name":       "mount_device",
		"target":     target,
		"deviceHash": deviceHash,
	}
	result, err := policy.Query(input)
	if err != nil {
		return err
	}

	if result.Allowed() {
		if devices, found := policy.data["devices"]; found {
			deviceMap := devices.(map[string]string)
			if _, e := deviceMap[target]; e {
				log.Fatalf("device %s already mounted", target)
			}
			deviceMap[target] = deviceHash
		} else {
			policy.data["devices"] = map[string]string{target: deviceHash}
		}
		return nil
	} else {
		return errors.New("device mount not allowed by policy")
	}
}

func (policy *RegoPolicy) EnforceOverlayMountPolicy(containerID string, layerPaths []string) error {
	policy.mutex.Lock()
	defer policy.mutex.Unlock()

	input := map[string]interface{}{
		"name":        "mount_overlay",
		"containerID": containerID,
		"layerPaths":  layerPaths,
	}
	result, err := policy.Query(input)
	if err != nil {
		return err
	}

	if result.Allowed() {
		if containers, found := policy.data["containers"]; found {
			containerMap := containers.(map[string]interface{})
			if _, found := containerMap[containerID]; found {
				return fmt.Errorf("container %s already mounted", containerID)
			} else {
				containerMap[containerID] = map[string]interface{}{
					"layerPaths": layerPaths,
				}
			}
		} else {
			policy.data["containers"] = map[string]interface{}{
				containerID: map[string]interface{}{
					"layerPaths": layerPaths,
				},
			}
		}
		return nil
	} else {
		return errors.New("overlay mount not allowed by policy")
	}
}

func (policy *RegoPolicy) EnforceCreateContainerPolicy(containerID string,
	argList []string,
	envList []string,
	workingDir string,
) error {
	policy.mutex.Lock()
	defer policy.mutex.Unlock()

	input := map[string]interface{}{
		"name":        "create_container",
		"containerID": containerID,
		"argList":     argList,
		"envList":     envList,
		"workingDir":  workingDir,
	}
	result, err := policy.Query(input)
	if err != nil {
		return err
	}

	if result.Allowed() {
		if started, found := policy.data["started"]; found {
			startedArray := started.([]string)
			policy.data["started"] = append(startedArray, containerID)
		} else {
			policy.data["started"] = []string{containerID}
		}
		return nil
	} else {
		input["name"] = "reason"
		result, err := policy.Query(input)
		if err != nil {
			return err
		}

		reasons := []string{}
		for _, reason := range result[0].Expressions[0].Value.([]interface{}) {
			reasons = append(reasons, reason.(string))
		}
		return fmt.Errorf("container creation not allowed by policy. Reasons: [%s]", strings.Join(reasons, ","))
	}
}

func (policy *RegoPolicy) EnforceDeviceUnmountPolicy(unmountTarget string) error {
	policy.mutex.Lock()
	defer policy.mutex.Unlock()
	devices := policy.data["devices"].(map[string]string)
	delete(devices, unmountTarget)
	return nil
}

func (policy *RegoPolicy) EnforceWaitMountPointsPolicy(containerID string, spec *oci.Spec) error {
	return errors.New("not implemented)")
}

func (policy *RegoPolicy) EnforceMountPolicy(sandboxID, containerID string, spec *oci.Spec) error {
	return errors.New("not implemented)")
}

func (policy *RegoPolicy) ExtendDefaultMounts([]oci.Mount) error {
	return errors.New("not implemented)")
}

func (policy *RegoPolicy) EncodedSecurityPolicy() string {
	return policy.base64policy
}
