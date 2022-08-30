//go:build linux && rego
// +build linux,rego

package securitypolicy

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/Microsoft/hcsshim/internal/guest/spec"
	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/storage/inmem"
	"github.com/open-policy-agent/opa/topdown"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

const regoEnforcerName = "rego"

func init() {
	registeredEnforcers[regoEnforcerName] = createRegoEnforcer
	// Overriding the value inside init guarantees that this assignment happens
	// after the variable has been initialized in securitypolicy.go and there
	// are no race conditions. When multiple init functions are defined in a
	// single package, the order of their execution is determined by the
	// filename.
	defaultEnforcer = regoEnforcerName
}

//go:embed framework.rego
var frameworkCode string

//go:embed policy.rego
var policyCode string

var indentUsing string = "    "

// regoEnforcer is a stub implementation of a security policy, which will be
// based on [Rego] policy language. The detailed implementation will be
// introduced in the subsequent PRs and documentation updated accordingly.
//
// [Rego]: https://www.openpolicyagent.org/docs/latest/policy-language/
type regoEnforcer struct {
	// Rego which describes policy behavior (see above)
	behavior string
	// Rego which describes policy objects (containers, etc.)
	objects string
	// Mutex to prevent concurrent access to fields
	mutex sync.Mutex
	// Rego data object, used to store policy state
	data map[string]interface{}
	// Base64 encoded (JSON) policy
	base64policy string
	// Compiled modules
	compiledModules *ast.Compiler
}

var _ SecurityPolicyEnforcer = (*regoEnforcer)(nil)

type securityPolicyInternal struct {
	AllowAll   bool
	Containers []*securityPolicyContainer
}

func (sp SecurityPolicy) toInternal() (*securityPolicyInternal, error) {
	policy := new(securityPolicyInternal)
	var err error
	if policy.Containers, err = sp.Containers.toInternal(); err != nil {
		return nil, err
	}

	policy.AllowAll = sp.AllowAll

	return policy, nil
}

type stringArray []string

func (array stringArray) marshalRego() string {
	values := make([]string, len(array))
	for i, value := range array {
		values[i] = fmt.Sprintf("%q", value)
	}

	return fmt.Sprintf("[%s]", strings.Join(values, ","))
}

func writeCommand(builder *strings.Builder, command []string, indent string) error {
	array := (stringArray(command)).marshalRego()
	_, err := builder.WriteString(fmt.Sprintf("%s\"command\": %s,\n", indent, array))
	return err
}

func (e EnvRuleConfig) marshalRego() string {
	return fmt.Sprintf(`{"pattern": "%s", "strategy": "%s"}`, e.Rule, e.Strategy)
}

type envRuleArray []EnvRuleConfig

func (array envRuleArray) marshalRego() string {
	values := make([]string, len(array))
	for i, env := range array {
		values[i] = env.marshalRego()
	}

	return fmt.Sprintf("[%s]", strings.Join(values, ","))
}

func writeEnvRules(builder *strings.Builder, envRules []EnvRuleConfig, indent string) error {
	_, err := builder.WriteString(fmt.Sprintf("%s\"env_rules\": %s,\n", indent, envRuleArray(envRules).marshalRego()))
	return err
}

func writeLayers(builder *strings.Builder, layers []string, indent string) error {
	array := (stringArray(layers)).marshalRego()
	_, err := builder.WriteString(fmt.Sprintf("%s\"layers\": %s,\n", indent, array))
	return err
}

func (m mountInternal) marshalRego() string {
	options := stringArray(m.Options).marshalRego()
	return fmt.Sprintf("{\"destination\": \"%s\", \"options\": %s, \"source\": \"%s\", \"type\": \"%s\"}", m.Destination, options, m.Source, m.Type)
}

func writeMounts(builder *strings.Builder, mounts []mountInternal, indent string) error {
	values := make([]string, len(mounts))
	for i, mount := range mounts {
		values[i] = mount.marshalRego()
	}

	_, err := builder.WriteString(fmt.Sprintf("%s\"mounts\": [%s],\n", indent, strings.Join(values, ",")))
	return err
}

func writeContainer(builder *strings.Builder, container *securityPolicyContainer, indent string) error {
	if _, err := builder.WriteString(fmt.Sprintf("%s{\n", indent)); err != nil {
		return err
	}

	if err := writeCommand(builder, container.Command, indent+indentUsing); err != nil {
		return err
	}

	if err := writeEnvRules(builder, container.EnvRules, indent+indentUsing); err != nil {
		return err
	}

	if err := writeLayers(builder, container.Layers, indent+indentUsing); err != nil {
		return err
	}

	if err := writeMounts(builder, container.Mounts, indent+indentUsing); err != nil {
		return err
	}

	if _, err := builder.WriteString(fmt.Sprintf("%s\"allow_elevated\": %v,\n", indent+indentUsing, container.AllowElevated)); err != nil {
		return err
	}

	if _, err := builder.WriteString(fmt.Sprintf("%s\"working_dir\": \"%s\"\n", indent+indentUsing, container.WorkingDir)); err != nil {
		return err
	}

	if _, err := builder.WriteString(fmt.Sprintf("%s}", indent)); err != nil {
		return err
	}

	return nil
}

func addContainers(builder *strings.Builder, containers []*securityPolicyContainer) error {
	if _, err := builder.WriteString("containers := [\n"); err != nil {
		return err
	}

	for i, container := range containers {
		if err := writeContainer(builder, container, indentUsing); err != nil {
			return err
		}

		var end string
		if i < len(containers)-1 {
			end = ",\n"
		} else {
			end = "\n"
		}

		if _, err := builder.WriteString(end); err != nil {
			return err
		}
	}

	if _, err := builder.WriteString("]\n"); err != nil {
		return err
	}

	return nil
}

func (p securityPolicyInternal) marshalRego() (string, error) {
	builder := new(strings.Builder)
	if _, err := builder.WriteString(fmt.Sprintf("package policy\nallow_all := %v\n", p.AllowAll)); err != nil {
		return "", err
	}

	if err := addContainers(builder, p.Containers); err != nil {
		return "", err
	}

	return builder.String(), nil
}

func createRegoEnforcer(state SecurityPolicyState, defaultMounts []oci.Mount, privilegedMounts []oci.Mount) (SecurityPolicyEnforcer, error) {
	policy, err := state.SecurityPolicy.toInternal()
	if err != nil {
		return nil, fmt.Errorf("error converting to internal format: %w", err)
	}

	regoPolicy, err := newRegoPolicyFromInternal(policy, defaultMounts, privilegedMounts)
	if err != nil {
		return nil, fmt.Errorf("error converting to Rego: %w", err)
	}

	regoPolicy.base64policy = state.EncodedSecurityPolicy.SecurityPolicy
	return regoPolicy, nil
}

func newRegoPolicyFromInternal(securityPolicy *securityPolicyInternal, defaultMounts []oci.Mount, privilegedMounts []oci.Mount) (*regoEnforcer, error) {
	policy := new(regoEnforcer)
	policy.behavior = policyCode
	var err error
	policy.objects, err = securityPolicy.marshalRego()
	if err != nil {
		return nil, fmt.Errorf("failed to convert json to rego: %w", err)
	}

	defaultMountData := make([]interface{}, 0, len(defaultMounts))
	privilegedMountData := make([]interface{}, 0, len(privilegedMounts))
	policy.data = map[string]interface{}{
		"started":          []string{},
		"devices":          map[string]string{},
		"containers":       map[string]interface{}{},
		"defaultMounts":    appendMountData(defaultMountData, defaultMounts),
		"privilegedMounts": appendMountData(privilegedMountData, privilegedMounts),
		"sandboxPrefix":    guestpath.SandboxMountPrefix,
		"hugePagesPrefix":  guestpath.HugePagesMountPrefix,
	}
	policy.base64policy = ""

	modules := map[string]string{
		"behavior.rego":  policy.behavior,
		"objects.rego":   policy.objects,
		"framework.rego": frameworkCode,
	}

	// TODO temporary hack for debugging policies until GCS logging design
	// and implementation is finalized. This option should be changed to
	// "true" if debugging is desired.
	options := ast.CompileOpts{
		EnablePrintStatements: false,
	}

	if compiled, err := ast.CompileModulesWithOpt(modules, options); err == nil {
		policy.compiledModules = compiled
	} else {
		return nil, fmt.Errorf("rego compilation failed: %w", err)
	}

	return policy, nil
}

func (policy *regoEnforcer) Query(input map[string]interface{}) (rego.ResultSet, error) {
	store := inmem.NewFromObject(policy.data)

	var buf bytes.Buffer
	rule := input["name"].(string)
	query := rego.New(
		rego.Query(fmt.Sprintf("data.policy.%s", rule)),
		rego.Compiler(policy.compiledModules),
		rego.Input(input),
		rego.Store(store),
		rego.PrintHook(topdown.NewPrintHook(&buf)))

	ctx := context.Background()
	results, err := query.Eval(ctx)
	if err != nil {
		log.G(ctx).WithError(err).WithFields(logrus.Fields{
			"Policy": policy.objects,
		})
		return results, err
	}

	output := buf.String()
	if len(output) > 0 {
		log.G(ctx).Debug(output)
	}

	return results, nil
}

func (policy *regoEnforcer) EnforceDeviceMountPolicy(target string, deviceHash string) error {
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

	if !result.Allowed() {
		input_json, err := json.Marshal(input)
		if err != nil {
			return errors.New("unable to marshal the Rego input data")
		}

		return fmt.Errorf("device mount not allowed by policy.\ninput: %s", string(input_json))
	}

	deviceMap := policy.data["devices"].(map[string]string)
	if _, ok := deviceMap[target]; ok {
		input_json, err := json.Marshal(input)
		if err != nil {
			return errors.New("unable to marshal the Rego input data")
		}

		return fmt.Errorf("device %s already mounted.\ninput: %s", target, string(input_json))
	}

	deviceMap[target] = deviceHash
	return nil
}

func (policy *regoEnforcer) EnforceOverlayMountPolicy(containerID string, layerPaths []string) error {
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

	if !result.Allowed() {
		input_json, err := json.Marshal(input)
		if err != nil {
			return errors.New("unable to marshal the Rego input data")
		}

		return fmt.Errorf("overlay mount not allowed by policy.\ninput: %s", string(input_json))
	}

	// we store the mapping of container ID -> layerPaths for later
	// use in EnforceCreateContainerPolicy here.
	containerMap := policy.data["containers"].(map[string]interface{})
	if _, ok := containerMap[containerID]; ok {
		input_json, err := json.Marshal(input)
		if err != nil {
			return errors.New("unable to marshal the Rego input data")
		}

		return fmt.Errorf("container %s already mounted.\ninput: %s", containerID, string(input_json))
	}

	containerMap[containerID] = map[string]interface{}{
		"containerID": containerID,
		"layerPaths":  layerPaths,
	}
	return nil
}

// Rego does not have a way to determine the OS path separator
// so we append it via these methods.
func sandboxMountsDir(sandboxID string) string {
	return fmt.Sprintf("%s%c", spec.SandboxMountsDir(sandboxID), os.PathSeparator)
}

func hugePagesMountsDir(sandboxID string) string {
	return fmt.Sprintf("%s%c", spec.HugePagesMountsDir(sandboxID), os.PathSeparator)
}

func (policy *regoEnforcer) EnforceCreateContainerPolicy(
	sandboxID string,
	containerID string,
	argList []string,
	envList []string,
	workingDir string,
	mounts []oci.Mount,
) error {
	policy.mutex.Lock()
	defer policy.mutex.Unlock()

	// first, we need to obtain the overlay filestytem information
	// which was stored in EnforceOverlayMountPolicy
	var containerInfo map[string]interface{}
	containerMap := policy.data["containers"].(map[string]interface{})
	if container, ok := containerMap[containerID]; ok {
		containerInfo = container.(map[string]interface{})
	} else {
		return fmt.Errorf("container %s does not have a filesystem", containerID)
	}

	input := map[string]interface{}{
		"name":         "create_container",
		"argList":      argList,
		"envList":      envList,
		"workingDir":   workingDir,
		"sandboxDir":   sandboxMountsDir(sandboxID),
		"hugePagesDir": hugePagesMountsDir(sandboxID),
		"mounts":       mounts,
	}

	// this adds the overlay layerPaths array to the input
	for key, value := range containerInfo {
		input[key] = value
	}

	result, err := policy.Query(input)
	if err != nil {
		return err
	}

	if result.Allowed() {
		started := policy.data["started"].([]string)
		policy.data["started"] = append(started, containerID)
		containerInfo["argList"] = argList
		containerInfo["envList"] = envList
		containerInfo["workingDir"] = workingDir
		return nil
	} else {
		input_json, err := json.Marshal(input)
		if err != nil {
			return errors.New("unable to marshal the Rego input data")
		}

		input["name"] = "reason"
		input["rule"] = "create_container"
		result, err := policy.Query(input)
		if err != nil {
			return err
		}

		reasons := []string{}
		for _, reason := range result[0].Expressions[0].Value.([]interface{}) {
			reasons = append(reasons, reason.(string))
		}
		return fmt.Errorf("container creation not allowed by policy. Reasons: [%s].\nInput: %s", strings.Join(reasons, ","), string(input_json))
	}
}

func (policy *regoEnforcer) EnforceDeviceUnmountPolicy(unmountTarget string) error {
	policy.mutex.Lock()
	defer policy.mutex.Unlock()

	devices := policy.data["devices"].(map[string]string)
	delete(devices, unmountTarget)

	return nil
}

func appendMountData(mountData []interface{}, mounts []oci.Mount) []interface{} {
	for _, mount := range mounts {
		mountData = append(mountData, map[string]interface{}{
			"destination": mount.Destination,
			"source":      mount.Source,
			"options":     mount.Options,
			"type":        mount.Type,
		})
	}

	return mountData
}

func (policy *regoEnforcer) ExtendDefaultMounts(mounts []oci.Mount) error {
	policy.mutex.Lock()
	defer policy.mutex.Unlock()

	defaultMounts := policy.data["defaultMounts"].([]interface{})
	policy.data["defaultMounts"] = appendMountData(defaultMounts, mounts)
	return nil
}

func (policy *regoEnforcer) EncodedSecurityPolicy() string {
	return policy.base64policy
}
