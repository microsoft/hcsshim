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

	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
)

const (
	regoMarshaller = "rego"
	// jsonMarshaller is retained only so callers that still request it get an
	// actionable error: JSON policy output has been removed in favor of rego.
	jsonMarshaller = "json"
)

var (
	registeredMarshallers = map[string]OSAwareMarshalFunc{}
	defaultMarshaller     = regoMarshaller
)

func init() {
	registeredMarshallers[regoMarshaller] = osAwareMarshalRego
	registeredMarshallers[jsonMarshaller] = rejectJSONMarshaller
}

// rejectJSONMarshaller keeps "json" a recognized marshaller so callers that
// still request it get a clear message pointing at rego instead of a generic
// "unknown marshaller" error.
func rejectJSONMarshaller(
	_ bool,
	_ []*Container,
	_ []*WindowsContainer,
	_ string,
	_ []ExternalProcessConfig,
	_ []FragmentConfig,
	_ bool,
	_ bool,
	_ bool,
	_ bool,
	_ bool,
	_ bool,
	_ bool,
	_ bool,
	_ bool,
) (string, error) {
	return "", fmt.Errorf("JSON policy output is no longer supported; use the %q marshaller", regoMarshaller)
}

//go:embed policy.rego
var policyRegoTemplate string

//go:embed open_door.rego
var openDoorRegoTemplate string

var openDoorRego = strings.Replace(openDoorRegoTemplate, "@@API_VERSION@@", apiVersion, 1)

// ContainerInterface represents either Container or WindowsContainer
type ContainerInterface interface {
	ToInternalContainer() (interface{}, error)
}

// Implement ContainerInterface for Container
func (c *Container) ToInternalContainer() (interface{}, error) {
	return c.toInternal()
}

// Implement ContainerInterface for WindowsContainer
func (c *WindowsContainer) ToInternalContainer() (interface{}, error) {
	return c.toInternal()
}

// OSAwareMarshalFunc is like marshalFunc but works with mixed container types
type OSAwareMarshalFunc func(
	allowAll bool,
	linuxContainers []*Container,
	windowsContainers []*WindowsContainer,
	osType string,
	externalProcesses []ExternalProcessConfig,
	fragments []FragmentConfig,
	allowPropertiesAccess bool,
	allowDumpStacks bool,
	allowRuntimeLogging bool,
	allowHostNetwork bool,
	allowEnvironmentVariableDropping bool,
	allowUnencryptedScratch bool,
	allowCapabilityDropping bool,
	allowRegistryChangesDropping bool,
	allowLogProviderDropping bool,
) (string, error)

// osAwareMarshalRego handles both Linux and Windows containers
func osAwareMarshalRego(
	allowAll bool,
	linuxContainers []*Container,
	windowsContainers []*WindowsContainer,
	osType string,
	externalProcesses []ExternalProcessConfig,
	fragments []FragmentConfig,
	allowPropertiesAccess bool,
	allowDumpStacks bool,
	allowRuntimeLogging bool,
	allowHostNetwork bool,
	allowEnvironmentVariableDropping bool,
	allowUnencryptedScratch bool,
	allowCapabilityDropping bool,
	allowRegistryChangesDropping bool,
	allowLogProviderDropping bool,
) (string, error) {
	if allowAll {
		if len(linuxContainers) > 0 || len(windowsContainers) > 0 {
			return "", ErrInvalidOpenDoorPolicy
		}
		return openDoorRego, nil
	}

	switch osType {
	case "linux":
		if len(windowsContainers) > 0 {
			return "", fmt.Errorf("cannot marshal Windows containers on Linux OS")
		}
		return marshalRego(allowAll, linuxContainers, externalProcesses, fragments,
			allowPropertiesAccess, allowDumpStacks, allowRuntimeLogging, allowHostNetwork,
			allowEnvironmentVariableDropping, allowUnencryptedScratch, allowCapabilityDropping,
			allowRegistryChangesDropping, allowLogProviderDropping)

	case "windows":
		if len(linuxContainers) > 0 {
			return "", fmt.Errorf("cannot marshal Linux containers on Windows OS")
		}
		return marshalWindowsRego(allowAll, windowsContainers, externalProcesses, fragments,
			allowPropertiesAccess, allowDumpStacks, allowRuntimeLogging, allowHostNetwork,
			allowEnvironmentVariableDropping, allowUnencryptedScratch, allowCapabilityDropping,
			allowRegistryChangesDropping, allowLogProviderDropping)

	default:
		return "", fmt.Errorf("unsupported OS type: %s", osType)
	}
}

// marshalWindowsRego creates Rego policy for Windows containers
func marshalWindowsRego(
	allowAll bool,
	containers []*WindowsContainer,
	externalProcesses []ExternalProcessConfig,
	fragments []FragmentConfig,
	allowPropertiesAccess bool,
	allowDumpStacks bool,
	allowRuntimeLogging bool,
	allowHostNetwork bool,
	allowEnvironmentVariableDropping bool,
	allowUnencryptedScratch bool,
	allowCapabilityDropping bool,
	allowRegistryChangesDropping bool,
	allowLogProviderDropping bool,
) (string, error) {
	if allowAll {
		if len(containers) > 0 {
			return "", ErrInvalidOpenDoorPolicy
		}
		return openDoorRego, nil
	}

	// Convert WindowsContainer to internal Windows container representation
	windowsContainersInternal, err := windowsContainersToInternal(containers)
	if err != nil {
		return "", err
	}

	policy := &securityPolicyWindowsInternal{
		Containers:                       windowsContainersInternal,
		ExternalProcesses:                externalProcessToInternal(externalProcesses),
		Fragments:                        fragmentsToInternal(fragments),
		AllowPropertiesAccess:            allowPropertiesAccess,
		AllowDumpStacks:                  allowDumpStacks,
		AllowRuntimeLogging:              allowRuntimeLogging,
		AllowHostNetwork:                 allowHostNetwork,
		AllowEnvironmentVariableDropping: allowEnvironmentVariableDropping,
		AllowUnencryptedScratch:          allowUnencryptedScratch,
		AllowCapabilityDropping:          allowCapabilityDropping,
		AllowRegistryChangesDropping:     allowRegistryChangesDropping,
		AllowLogProviderDropping:         allowLogProviderDropping,
	}

	return policy.marshalWindowsRego(), nil
}

func marshalRego(
	allowAll bool,
	containers []*Container,
	externalProcesses []ExternalProcessConfig,
	fragments []FragmentConfig,
	allowPropertiesAccess bool,
	allowDumpStacks bool,
	allowRuntimeLogging bool,
	allowHostNetwork bool,
	allowEnvironmentVariableDropping bool,
	allowUnencryptedScratch bool,
	allowCapabilityDropping bool,
	allowRegistryChangesDropping bool,
	allowLogProviderDropping bool,
) (string, error) {
	if allowAll {
		if len(containers) > 0 {
			return "", ErrInvalidOpenDoorPolicy
		}

		return openDoorRego, nil
	}

	policy, err := newSecurityPolicyInternal(
		containers,
		externalProcesses,
		fragments,
		allowPropertiesAccess,
		allowDumpStacks,
		allowRuntimeLogging,
		allowHostNetwork,
		allowEnvironmentVariableDropping,
		allowUnencryptedScratch,
		allowCapabilityDropping,
		allowRegistryChangesDropping,
		allowLogProviderDropping,
	)
	if err != nil {
		return "", err
	}

	return policy.marshalRego(), nil
}

func MarshalFragment(
	namespace string,
	svn string,
	containers []*Container,
	externalProcesses []ExternalProcessConfig,
	fragments []FragmentConfig) (string, error) {
	fragment, err := newSecurityPolicyFragment(namespace, svn, containers, externalProcesses, fragments)
	if err != nil {
		return "", err
	}

	return fragment.marshalRego(), nil
}

// MarshalWindowsFragment encodes a Rego policy fragment for Windows containers.
func MarshalWindowsFragment(
	namespace string,
	svn string,
	containers []*WindowsContainer,
	externalProcesses []ExternalProcessConfig,
	fragments []FragmentConfig) (string, error) {
	fragment, err := newWindowsSecurityPolicyFragment(namespace, svn, containers, externalProcesses, fragments)
	if err != nil {
		return "", err
	}

	return fragment.marshalRego(), nil
}

func MarshalPolicy(
	marshaller string,
	allowAll bool,
	containers []*Container,
	externalProcesses []ExternalProcessConfig,
	fragments []FragmentConfig,
	allowPropertiesAccess bool,
	allowDumpStacks bool,
	allowRuntimeLogging bool,
	allowHostNetwork bool,
	allowEnvironmentVariableDropping bool,
	allowUnencryptedScratch bool,
	allowCapbilitiesDropping bool,
	allowRegistryChangesDropping bool,
	allowLogProviderDropping bool,
) (string, error) {
	if marshaller == "" {
		marshaller = defaultMarshaller
	}

	if marshal, ok := registeredMarshallers[marshaller]; !ok {
		return "", fmt.Errorf("unknown marshaller: %q", marshaller)
	} else {
		return marshal(
			allowAll,
			containers,
			nil,
			"linux",
			externalProcesses,
			fragments,
			allowPropertiesAccess,
			allowDumpStacks,
			allowRuntimeLogging,
			allowHostNetwork,
			allowEnvironmentVariableDropping,
			allowUnencryptedScratch,
			allowCapbilitiesDropping,
			allowRegistryChangesDropping,
			allowLogProviderDropping,
		)
	}
}

// MarshalWindowsPolicy encodes a security policy for Windows containers.
func MarshalWindowsPolicy(
	marshaller string,
	allowAll bool,
	containers []*WindowsContainer,
	externalProcesses []ExternalProcessConfig,
	fragments []FragmentConfig,
	allowPropertiesAccess bool,
	allowDumpStacks bool,
	allowRuntimeLogging bool,
	allowHostNetwork bool,
	allowEnvironmentVariableDropping bool,
	allowUnencryptedScratch bool,
	allowCapabilitiesDropping bool,
	allowRegistryChangesDropping bool,
	allowLogProviderDropping bool,
) (string, error) {
	if marshaller == "" {
		marshaller = regoMarshaller
	}
	if marshaller != regoMarshaller {
		return "", fmt.Errorf("marshaller %q is not supported for Windows policies", marshaller)
	}

	marshal, ok := registeredMarshallers[marshaller]
	if !ok {
		return "", fmt.Errorf("unknown marshaller: %q", marshaller)
	}

	return marshal(
		allowAll,
		nil,
		containers,
		"windows",
		externalProcesses,
		fragments,
		allowPropertiesAccess,
		allowDumpStacks,
		allowRuntimeLogging,
		allowHostNetwork,
		allowEnvironmentVariableDropping,
		allowUnencryptedScratch,
		allowCapabilitiesDropping,
		allowRegistryChangesDropping,
		allowLogProviderDropping,
	)
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

// regoString returns s as a JSON-compatible Rego double-quoted string literal,
// escaping backslashes and quotes (required for Windows paths and commands).
func regoString(s string) string {
	// json.Marshal of a string never returns an error.
	encoded, _ := json.Marshal(s)
	return string(encoded)
}

// mustMarshalJSON encodes v as JSON for embedding in a Rego policy. Encoding via
// the JSON marshaller guarantees every string value is safely quoted/escaped.
func mustMarshalJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Errorf("failed to marshal Rego object to JSON: %w", err))
	}
	return string(b)
}

func (array stringArray) marshalRego() string {
	values := make([]string, len(array))
	for i, value := range array {
		values[i] = regoString(value)
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
	var v any
	if e.UseNameValue {
		v = struct {
			Name          string `json:"name"`
			NameStrategy  string `json:"name_strategy"`
			Value         string `json:"value"`
			ValueStrategy string `json:"value_strategy"`
			Required      bool   `json:"required"`
		}{e.Name, string(e.NameStrategy), e.Value, string(e.ValueStrategy), e.Required}
	} else {
		v = struct {
			Pattern  string `json:"pattern"`
			Strategy string `json:"strategy"`
			Required bool   `json:"required"`
		}{e.Rule, string(e.Strategy), e.Required}
	}
	return mustMarshalJSON(v)
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

func writeMountedCim(builder *strings.Builder, mountedCim []string, indent string) {
	writeLine(builder, `%s"mounted_cim": %s,`, indent, (stringArray(mountedCim)).marshalRego())
}

func writeCapabilities(builder *strings.Builder, capabilities *capabilitiesInternal, indent string) {
	if capabilities != nil {
		writeLine(builder, `%s"capabilities": {`, indent)
		writeLine(builder, `%s"bounding": %s,`, indent+indentUsing, (stringArray(capabilities.Bounding)).marshalRego())
		writeLine(builder, `%s"effective": %s,`, indent+indentUsing, (stringArray(capabilities.Effective)).marshalRego())
		writeLine(builder, `%s"inheritable": %s,`, indent+indentUsing, (stringArray(capabilities.Inheritable)).marshalRego())
		writeLine(builder, `%s"permitted": %s,`, indent+indentUsing, (stringArray(capabilities.Permitted)).marshalRego())
		writeLine(builder, `%s"ambient": %s,`, indent+indentUsing, (stringArray(capabilities.Ambient)).marshalRego())
		writeLine(builder, `%s},`, indent)
	} else {
		writeLine(builder, `%s"capabilities": null,`, indent)
	}
}

func (m mountInternal) marshalRego() string {
	options := stringArray(m.Options).marshalRego()
	return mustMarshalJSON(struct {
		Destination string          `json:"destination"`
		Options     json.RawMessage `json:"options"`
		Source      string          `json:"source"`
		Type        string          `json:"type"`
	}{m.Destination, json.RawMessage(options), m.Source, m.Type})
}

// escapeRegoString escapes a Go string so it is a valid double-quoted Rego
// string literal. This matters for Windows registry keys and values, which
// contain backslashes that would otherwise be interpreted as escape sequences.
func escapeRegoString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func writeMounts(builder *strings.Builder, mounts []mountInternal, indent string) {
	values := make([]string, len(mounts))
	for i, mount := range mounts {
		values[i] = mount.marshalRego()
	}

	writeLine(builder, `%s"mounts": [%s],`, indent, strings.Join(values, ","))
}

func (k registryKeyInternal) marshalRego() string {
	return fmt.Sprintf(`{"hive": "%s", "name": "%s", "volatile": %t}`,
		escapeRegoString(k.Hive), escapeRegoString(k.Name), k.Volatile)
}

func (v registryValueInternal) marshalRego() string {
	fields := []string{
		fmt.Sprintf(`"key": %s`, v.Key.marshalRego()),
		fmt.Sprintf(`"name": "%s"`, escapeRegoString(v.Name)),
		fmt.Sprintf(`"type": "%s"`, escapeRegoString(v.Type)),
	}
	// Type selects which value field is significant; emit only that one so the
	// policy value matches the shape registry_value_matches compares against.
	switch v.Type {
	case "String", "ExpandedString", "MultiString":
		fields = append(fields, fmt.Sprintf(`"string_value": "%s"`, escapeRegoString(v.StringValue)))
	case "DWord":
		fields = append(fields, fmt.Sprintf(`"dword_value": %d`, v.DWordValue))
	case "QWord":
		fields = append(fields, fmt.Sprintf(`"qword_value": %d`, v.QWordValue))
	case "Binary":
		fields = append(fields, fmt.Sprintf(`"binary_value": "%s"`, escapeRegoString(v.BinaryValue)))
	case "CustomType":
		fields = append(fields, fmt.Sprintf(`"custom_type": %d`, v.CustomType))
		fields = append(fields, fmt.Sprintf(`"binary_value": "%s"`, escapeRegoString(v.BinaryValue)))
	case "None":
		// No value to compare, just key, name and type.
	}
	return fmt.Sprintf("{%s}", strings.Join(fields, ", "))
}

func writeRegistryChanges(builder *strings.Builder, registryChanges registryChangesInternal, indent string) {
	addValues := make([]string, len(registryChanges.AddValues))
	for i, value := range registryChanges.AddValues {
		addValues[i] = value.marshalRego()
	}
	deleteKeys := make([]string, len(registryChanges.DeleteKeys))
	for i, key := range registryChanges.DeleteKeys {
		deleteKeys[i] = key.marshalRego()
	}

	writeLine(builder, `%s"registry_changes": {"add_values": [%s], "delete_keys": [%s]},`,
		indent, strings.Join(addValues, ", "), strings.Join(deleteKeys, ", "))
}

// Windows-specific marshal functions
func writeWindowsSignals(builder *strings.Builder, signals []guestrequest.SignalValueWCOW, indent string) {
	signalsArray := make([]string, len(signals))
	for i, s := range signals {
		signalsArray[i] = string(s)
	}
	array := (stringArray(signalsArray)).marshalRego()
	writeLine(builder, `%s"signals": %s,`, indent, array)
}

func writeWindowsUser(builder *strings.Builder, user string, indent string) {
	writeLine(builder, `%s"user": %s,`, indent, regoString(user))
}

func (p windowsContainerExecProcess) marshalRego() string {
	commandLine := []string{p.Command}
	command := stringArray(commandLine).marshalRego()
	signalsArray := make([]string, len(p.Signals))
	for i, s := range p.Signals {
		signalsArray[i] = string(s)
	}
	signals := stringArray(signalsArray).marshalRego()
	return mustMarshalJSON(struct {
		Command json.RawMessage `json:"command"`
		Signals json.RawMessage `json:"signals"`
	}{json.RawMessage(command), json.RawMessage(signals)})
}

func writeWindowsExecProcesses(builder *strings.Builder, execProcesses []windowsContainerExecProcess, indent string) {
	values := make([]string, len(execProcesses))
	for i, process := range execProcesses {
		values[i] = process.marshalRego()
	}
	writeLine(builder, `%s"exec_processes": [%s],`, indent, strings.Join(values, ","))
}

func writeWindowsContainer(builder *strings.Builder, container *securityPolicyWindowsContainer, indent string) {
	writeLine(builder, "%s{", indent)
	writeCommand(builder, container.Command, indent+indentUsing)
	writeEnvRules(builder, container.EnvRules, indent+indentUsing)
	writeLayers(builder, container.Layers, indent+indentUsing)
	writeMountedCim(builder, container.MountedCim, indent+indentUsing)
	writeMounts(builder, container.Mounts, indent+indentUsing)
	if len(container.RegistryChanges.AddValues) > 0 || len(container.RegistryChanges.DeleteKeys) > 0 {
		writeRegistryChanges(builder, container.RegistryChanges, indent+indentUsing)
	}
	writeWindowsExecProcesses(builder, container.ExecProcesses, indent+indentUsing)
	writeWindowsSignals(builder, container.Signals, indent+indentUsing)
	writeWindowsUser(builder, container.User, indent+indentUsing)
	writeLine(builder, `%s"working_dir": %s,`, indent+indentUsing, regoString(container.WorkingDir))
	writeLine(builder, `%s"allow_stdio_access": %t,`, indent+indentUsing, container.AllowStdioAccess)
	writeLine(builder, "%s},", indent)
}

func (p containerExecProcess) marshalRego() string {
	command := stringArray(p.Command).marshalRego()
	signals := signalArray(p.Signals).marshalRego()

	return mustMarshalJSON(struct {
		Command json.RawMessage `json:"command"`
		Signals json.RawMessage `json:"signals"`
	}{json.RawMessage(command), json.RawMessage(signals)})
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

func (n IDNameConfig) marshalRego() string {
	return mustMarshalJSON(struct {
		Pattern  string `json:"pattern"`
		Strategy string `json:"strategy"`
	}{n.Rule, string(n.Strategy)})
}

type idConfigArray []IDNameConfig

func (array idConfigArray) marshalRego() string {
	values := make([]string, len(array))
	for i, name := range array {
		values[i] = name.marshalRego()
	}

	return fmt.Sprintf("[%s]", strings.Join(values, ","))
}

func writeUser(builder *strings.Builder, user UserConfig, indent string) {
	groupIDNames := idConfigArray(user.GroupIDNames).marshalRego()
	writeLine(builder, `%s"user": {`, indent)
	writeLine(builder, `%s"user_idname": %s,`, indent+indentUsing, user.UserIDName.marshalRego())
	writeLine(builder, `%s"group_idnames": %s,`, indent+indentUsing, groupIDNames)
	writeLine(builder, `%s"umask": %s`, indent+indentUsing, regoString(user.Umask))
	writeLine(builder, `%s},`, indent)
}

func writeContainer(builder *strings.Builder, container *securityPolicyContainer, indent string) {
	writeLine(builder, "%s{", indent)
	writeCommand(builder, container.Command, indent+indentUsing)
	writeEnvRules(builder, container.EnvRules, indent+indentUsing)
	writeLayers(builder, container.Layers, indent+indentUsing)
	writeMounts(builder, container.Mounts, indent+indentUsing)
	writeExecProcesses(builder, container.ExecProcesses, indent+indentUsing)
	writeSignals(builder, container.Signals, indent+indentUsing)
	writeUser(builder, container.User, indent+indentUsing)
	writeCapabilities(builder, container.Capabilities, indent+indentUsing)
	writeLine(builder, `%s"seccomp_profile_sha256": %s,`, indent+indentUsing, regoString(container.SeccompProfileSHA256))
	writeLine(builder, `%s"allow_elevated": %t,`, indent+indentUsing, container.AllowElevated)
	writeLine(builder, `%s"working_dir": %s,`, indent+indentUsing, regoString(container.WorkingDir))
	writeLine(builder, `%s"allow_stdio_access": %t,`, indent+indentUsing, container.AllowStdioAccess)
	writeLine(builder, `%s"no_new_privileges": %t,`, indent+indentUsing, container.NoNewPrivileges)
	writeLine(builder, "%s},", indent)
}

func addContainers(builder *strings.Builder, containers []*securityPolicyContainer) {
	if len(containers) == 0 {
		return
	}

	writeLine(builder, "containers := [")
	for _, container := range containers {
		writeContainer(builder, container, indentUsing)
	}
	writeLine(builder, "]")
}

func (p externalProcess) marshalRego() string {
	command := stringArray(p.command).marshalRego()
	envRules := envRuleArray(p.envRules).marshalRego()
	return mustMarshalJSON(struct {
		Command          json.RawMessage `json:"command"`
		EnvRules         json.RawMessage `json:"env_rules"`
		WorkingDir       string          `json:"working_dir"`
		AllowStdioAccess bool            `json:"allow_stdio_access"`
	}{json.RawMessage(command), json.RawMessage(envRules), p.workingDir, p.allowStdioAccess})
}

func addExternalProcesses(builder *strings.Builder, processes []*externalProcess) {
	if len(processes) == 0 {
		return
	}

	writeLine(builder, "external_processes := [")

	for _, process := range processes {
		writeLine(builder, `%s%s,`, indentUsing, process.marshalRego())
	}

	writeLine(builder, "]")
}

func (f fragment) marshalRego() string {
	includes := stringArray(f.includes).marshalRego()
	obj := struct {
		Issuer     string          `json:"issuer"`
		Feed       string          `json:"feed"`
		MinimumSVN string          `json:"minimum_svn"`
		Includes   json.RawMessage `json:"includes"`
		Parameters json.RawMessage `json:"parameters,omitempty"`
	}{
		Issuer:     f.issuer,
		Feed:       f.feed,
		MinimumSVN: f.minimumSVN,
		Includes:   json.RawMessage(includes),
	}
	if len(f.parameters) > 0 {
		paramsJSON, err := json.Marshal(f.parameters)
		if err != nil {
			panic(fmt.Errorf("failed to marshal fragment parameters object to JSON: %w", err))
		}
		obj.Parameters = json.RawMessage(paramsJSON)
	}
	return mustMarshalJSON(obj)
}

func addFragments(builder *strings.Builder, fragments []*fragment) {
	if len(fragments) == 0 {
		return
	}

	writeLine(builder, "fragments := [")

	for _, fragment := range fragments {
		writeLine(builder, "%s%s,", indentUsing, fragment.marshalRego())
	}

	writeLine(builder, "]")
}

func addWindowsMappedDirectories(builder *strings.Builder, rules []WindowsMappedDirectoryRule) {
	if len(rules) == 0 {
		return
	}

	writeLine(builder, "mapped_directories := [")

	for _, rule := range rules {
		writeLine(builder, `%s{"container_path": %q, "read_only": %t},`, indentUsing, rule.ContainerPath, rule.ReadOnly)
	}

	writeLine(builder, "]")
}

func (p securityPolicyInternal) marshalRego() string {
	builder := new(strings.Builder)
	addFragments(builder, p.Fragments)
	addContainers(builder, p.Containers)
	addExternalProcesses(builder, p.ExternalProcesses)
	writeLine(builder, `allow_properties_access := %t`, p.AllowPropertiesAccess)
	writeLine(builder, `allow_dump_stacks := %t`, p.AllowDumpStacks)
	writeLine(builder, `allow_runtime_logging := %t`, p.AllowRuntimeLogging)
	writeLine(builder, `allow_host_network := %t`, p.AllowHostNetwork)
	writeLine(builder, "allow_environment_variable_dropping := %t", p.AllowEnvironmentVariableDropping)
	writeLine(builder, "allow_unencrypted_scratch := %t", p.AllowUnencryptedScratch)
	writeLine(builder, "allow_capability_dropping := %t", p.AllowCapabilityDropping)
	writeLine(builder, "allow_registry_changes_dropping := %t", p.AllowRegistryChangesDropping)
	writeLine(builder, "allow_log_provider_dropping := %t", p.AllowLogProviderDropping)
	result := strings.Replace(policyRegoTemplate, "@@OBJECTS@@", builder.String(), 1)
	result = strings.Replace(result, "@@API_VERSION@@", apiVersion, 1)
	result = strings.Replace(result, "@@FRAMEWORK_VERSION@@", frameworkVersion, 1)
	return result
}

func (p securityPolicyFragment) marshalRego() string {
	builder := new(strings.Builder)
	addFragments(builder, p.Fragments)
	addContainers(builder, p.Containers)
	addWindowsContainers(builder, p.WindowsContainers)
	addExternalProcesses(builder, p.ExternalProcesses)
	return fmt.Sprintf("package %s\n\nsvn := \"%s\"\nframework_version := \"%s\"\n\n%s", p.Namespace, p.SVN, frameworkVersion, builder.String())
}

func (p securityPolicyWindowsInternal) marshalWindowsRego() string {
	builder := new(strings.Builder)
	addFragments(builder, p.Fragments)
	addWindowsContainers(builder, p.Containers)
	addExternalProcesses(builder, p.ExternalProcesses)
	addWindowsMappedDirectories(builder, p.MappedDirectories)
	writeLine(builder, `allow_properties_access := %t`, p.AllowPropertiesAccess)
	writeLine(builder, `allow_dump_stacks := %t`, p.AllowDumpStacks)
	writeLine(builder, `allow_runtime_logging := %t`, p.AllowRuntimeLogging)
	writeLine(builder, `allow_host_network := %t`, p.AllowHostNetwork)
	writeLine(builder, "allow_environment_variable_dropping := %t", p.AllowEnvironmentVariableDropping)
	writeLine(builder, "allow_unencrypted_scratch := %t", p.AllowUnencryptedScratch)
	writeLine(builder, "allow_capability_dropping := %t", p.AllowCapabilityDropping)
	writeLine(builder, "allow_registry_changes_dropping := %t", p.AllowRegistryChangesDropping)
	writeLine(builder, "allow_log_provider_dropping := %t", p.AllowLogProviderDropping)
	result := strings.Replace(policyRegoTemplate, "@@OBJECTS@@", builder.String(), 1)
	result = strings.Replace(result, "@@API_VERSION@@", apiVersion, 1)
	result = strings.Replace(result, "@@FRAMEWORK_VERSION@@", frameworkVersion, 1)
	return result
}

func addWindowsContainers(builder *strings.Builder, containers []*securityPolicyWindowsContainer) {
	if len(containers) == 0 {
		return
	}

	writeLine(builder, "containers := [")
	for _, container := range containers {
		writeWindowsContainer(builder, container, indentUsing)
	}
	writeLine(builder, "]")
}
