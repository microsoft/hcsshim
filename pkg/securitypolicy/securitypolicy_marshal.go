package securitypolicy

/** TODO
 *  Once JSON output/input functionality is removed, this code should be
 *  moved to the securitypolicy tool.
 */

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"syscall"
)

type marshalFunc func(allowAll bool, containers []*Container, externalProcesses []ExternalProcessConfig, allowPropertiesAccess bool, allowDumpStacks bool) (string, error)

const (
	jsonMarshaller = "json"
	regoMarshaller = "rego"
)

var (
	registeredMarshallers = map[string]marshalFunc{}
	defaultMarshaller     = jsonMarshaller
)

func init() {
	registeredMarshallers[jsonMarshaller] = marshalJSON
	registeredMarshallers[regoMarshaller] = marshalRego
}

//go:embed policy.rego
var policyRegoTemplate string

//go:embed open_door.rego
var openDoorRegoTemplate string

func marshalJSON(allowAll bool, containers []*Container, _ []ExternalProcessConfig, _ bool, _ bool) (string, error) {
	var policy *SecurityPolicy
	if allowAll {
		if len(containers) > 0 {
			return "", ErrInvalidOpenDoorPolicy
		}

		policy = NewOpenDoorPolicy()
	} else {
		policy = NewSecurityPolicy(allowAll, containers)
	}

	policyCode, err := json.Marshal(policy)
	if err != nil {
		return "", err
	}

	return string(policyCode), nil
}

func marshalRego(allowAll bool, containers []*Container, externalProcesses []ExternalProcessConfig, allowPropertiesAccess bool, allowDumpStacks bool) (string, error) {
	if allowAll {
		if len(containers) > 0 {
			return "", ErrInvalidOpenDoorPolicy
		}

		return openDoorRegoTemplate, nil
	}

	var policy securityPolicyInternal
	policy.Containers = make([]*securityPolicyContainer, len(containers))
	for i, cConf := range containers {
		cInternal, err := cConf.toInternal()
		if err != nil {
			return "", err
		}
		policy.Containers[i] = &cInternal
	}

	policy.ExternalProcesses = make([]*externalProcess, len(externalProcesses))
	for i, pConf := range externalProcesses {
		pInternal := pConf.toInternal()
		policy.ExternalProcesses[i] = &pInternal
	}

	policy.AllowPropertiesAccess = allowPropertiesAccess
	policy.AllowDumpStacks = allowDumpStacks

	return policy.marshalRego(), nil
}

func MarshalPolicy(marshaller string, allowAll bool, containers []*Container, externalProcesses []ExternalProcessConfig, allowPropertiesAccess bool, allowDumpStacks bool) (string, error) {
	if marshaller == "" {
		marshaller = defaultMarshaller
	}

	if marshal, ok := registeredMarshallers[marshaller]; !ok {
		return "", fmt.Errorf("unknown marshaller: %q", marshaller)
	} else {
		return marshal(allowAll, containers, externalProcesses, allowPropertiesAccess, allowDumpStacks)
	}
}

// Custom JSON marshalling to add `length` field that matches the number of
// elements present in the `elements` field.

func (c Containers) MarshalJSON() ([]byte, error) {
	type Alias Containers
	return json.Marshal(&struct {
		Length int `json:"length"`
		*Alias
	}{
		Length: len(c.Elements),
		Alias:  (*Alias)(&c),
	})
}

func (e EnvRules) MarshalJSON() ([]byte, error) {
	type Alias EnvRules
	return json.Marshal(&struct {
		Length int `json:"length"`
		*Alias
	}{
		Length: len(e.Elements),
		Alias:  (*Alias)(&e),
	})
}

func (s StringArrayMap) MarshalJSON() ([]byte, error) {
	type Alias StringArrayMap
	return json.Marshal(&struct {
		Length int `json:"length"`
		*Alias
	}{
		Length: len(s.Elements),
		Alias:  (*Alias)(&s),
	})
}

func (c CommandArgs) MarshalJSON() ([]byte, error) {
	return json.Marshal(StringArrayMap(c))
}

func (l Layers) MarshalJSON() ([]byte, error) {
	return json.Marshal(StringArrayMap(l))
}

func (o Options) MarshalJSON() ([]byte, error) {
	return json.Marshal(StringArrayMap(o))
}

func (m Mounts) MarshalJSON() ([]byte, error) {
	type Alias Mounts
	return json.Marshal(&struct {
		Length int `json:"length"`
		*Alias
	}{
		Length: len(m.Elements),
		Alias:  (*Alias)(&m),
	})
}

// Marshaling for creating Rego policy code

var indentUsing string = "    "

type stringArray []string
type signalArray []syscall.Signal

func (array stringArray) marshalRego() string {
	values := make([]string, len(array))
	for i, value := range array {
		values[i] = fmt.Sprintf(`"%s"`, value)
	}

	return fmt.Sprintf("[%s]", strings.Join(values, ","))
}

func (array signalArray) marshalRego() string {
	values := make([]string, len(array))
	for i, value := range array {
		values[i] = fmt.Sprintf("%d", value)
	}

	return fmt.Sprintf("[%s]", strings.Join(values, ","))
}

func writeLine(builder *strings.Builder, format string, args ...interface{}) {
	builder.WriteString(fmt.Sprintf(format, args...) + "\n")
}

func writeCommand(builder *strings.Builder, command []string, indent string) {
	array := (stringArray(command)).marshalRego()
	writeLine(builder, `%s"command": %s,`, indent, array)
}

func (e EnvRuleConfig) marshalRego() string {
	return fmt.Sprintf(`{"pattern": "%s", "strategy": "%s", "required": %v}`, e.Rule, e.Strategy, e.Required)
}

type envRuleArray []EnvRuleConfig

func (array envRuleArray) marshalRego() string {
	values := make([]string, len(array))
	for i, env := range array {
		values[i] = env.marshalRego()
	}

	return fmt.Sprintf("[%s]", strings.Join(values, ","))
}

func writeEnvRules(builder *strings.Builder, envRules []EnvRuleConfig, indent string) {
	writeLine(builder, `%s"env_rules": %s,`, indent, envRuleArray(envRules).marshalRego())
}

func writeLayers(builder *strings.Builder, layers []string, indent string) {
	writeLine(builder, `%s"layers": %s,`, indent, (stringArray(layers)).marshalRego())
}

func (m mountInternal) marshalRego() string {
	options := stringArray(m.Options).marshalRego()
	return fmt.Sprintf(`{"destination": "%s", "options": %s, "source": "%s", "type": "%s"}`, m.Destination, options, m.Source, m.Type)
}

func writeMounts(builder *strings.Builder, mounts []mountInternal, indent string) {
	values := make([]string, len(mounts))
	for i, mount := range mounts {
		values[i] = mount.marshalRego()
	}

	writeLine(builder, `%s"mounts": [%s],`, indent, strings.Join(values, ","))
}

func (p containerExecProcess) marshalRego() string {
	command := stringArray(p.Command).marshalRego()
	signals := signalArray(p.Signals).marshalRego()

	return fmt.Sprintf(`{"command": %s, "signals": %s}`, command, signals)
}

func writeExecProcesses(builder *strings.Builder, execProcesses []containerExecProcess, indent string) {
	values := make([]string, len(execProcesses))
	for i, process := range execProcesses {
		values[i] = process.marshalRego()
	}
	writeLine(builder, `%s"exec_processes": [%s],`, indent, strings.Join(values, ","))
}

func writeSignals(builder *strings.Builder, signals []syscall.Signal, indent string) {
	array := (signalArray(signals)).marshalRego()
	writeLine(builder, `%s"signals": %s,`, indent, array)
}

func writeContainer(builder *strings.Builder, container *securityPolicyContainer, indent string, end string) {
	writeLine(builder, "%s{", indent)
	writeCommand(builder, container.Command, indent+indentUsing)
	writeEnvRules(builder, container.EnvRules, indent+indentUsing)
	writeLayers(builder, container.Layers, indent+indentUsing)
	writeMounts(builder, container.Mounts, indent+indentUsing)
	writeExecProcesses(builder, container.ExecProcesses, indent+indentUsing)
	writeSignals(builder, container.Signals, indent+indentUsing)
	writeLine(builder, `%s"allow_elevated": %v,`, indent+indentUsing, container.AllowElevated)
	writeLine(builder, `%s"working_dir": "%s"`, indent+indentUsing, container.WorkingDir)
	writeLine(builder, "%s}%s", indent, end)
}

func addContainers(builder *strings.Builder, containers []*securityPolicyContainer) {
	writeLine(builder, "containers := [")

	for i, container := range containers {
		end := ","
		if i == len(containers)-1 {
			end = ""
		}
		writeContainer(builder, container, indentUsing, end)
	}

	writeLine(builder, "]")
}

func (p externalProcess) marshalRego() string {
	command := stringArray(p.command).marshalRego()
	envRules := envRuleArray(p.envRules).marshalRego()
	return fmt.Sprintf(`{"command": %s, "env_rules": %s, "working_dir": "%s"}`, command, envRules, p.workingDir)
}

func addExternalProcesses(builder *strings.Builder, processes []*externalProcess) {
	writeLine(builder, "external_processes := [")

	for i, process := range processes {
		end := ","
		if i == len(processes)-1 {
			end = ""
		}
		writeLine(builder, `%s%s%s`, indentUsing, process.marshalRego(), end)
	}

	writeLine(builder, "]")
}

func (p securityPolicyInternal) marshalRego() string {
	builder := new(strings.Builder)
	addContainers(builder, p.Containers)
	addExternalProcesses(builder, p.ExternalProcesses)
	writeLine(builder, `allow_properties_access := %v`, p.AllowPropertiesAccess)
	writeLine(builder, `allow_dump_stacks := %v`, p.AllowDumpStacks)
	objects := builder.String()
	return strings.Replace(policyRegoTemplate, "##OBJECTS##", objects, 1)
}
