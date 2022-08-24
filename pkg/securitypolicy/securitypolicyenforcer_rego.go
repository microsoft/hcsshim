//go:build linux && rego
// +build linux,rego

package securitypolicy

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
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
	defaultMarshaller = regoMarshaller
}

//go:embed framework.rego
var frameworkCode string

// regoEnforcer is a stub implementation of a security policy, which will be
// based on [Rego] policy language. The detailed implementation will be
// introduced in the subsequent PRs and documentation updated accordingly.
//
// [Rego]: https://www.openpolicyagent.org/docs/latest/policy-language/
type regoEnforcer struct {
	// Rego which describes policy behavior (see above)
	code string
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

//nolint:unused
func (sp SecurityPolicy) toInternal() (*securityPolicyInternal, error) {
	policy := new(securityPolicyInternal)
	var err error
	if policy.Containers, err = sp.Containers.toInternal(); err != nil {
		return nil, err
	}

	return policy, nil
}

func createRegoEnforcer(base64EncodedPolicy string,
	defaultMounts []oci.Mount,
	privilegedMounts []oci.Mount,
) (SecurityPolicyEnforcer, error) {
	// base64 decode the incoming policy string
	// It will either be (legacy) JSON or Rego.
	rawPolicy, err := base64.StdEncoding.DecodeString(base64EncodedPolicy)
	if err != nil {
		return nil, fmt.Errorf("unable to decode policy from Base64 format: %w", err)
	}

	// Try to unmarshal the JSON
	var code string
	securityPolicy := new(SecurityPolicy)
	err = json.Unmarshal(rawPolicy, securityPolicy)
	if err == nil {
		containers := make([]*Container, securityPolicy.Containers.Length)

		for i := 0; i < securityPolicy.Containers.Length; i++ {
			index := strconv.Itoa(i)
			cConf, ok := securityPolicy.Containers.Elements[index]
			if !ok {
				return nil, fmt.Errorf("container constraint with index %q not found", index)
			}
			containers[i] = &cConf
		}

		code, err = marshalRego(securityPolicy.AllowAll, containers)
		if err != nil {
			return nil, fmt.Errorf("error marshaling the policy to Rego: %w", err)
		}
	} else {
		// this is either a Rego policy or malformed JSON
		code = string(rawPolicy)
	}

	regoPolicy, err := newRegoPolicy(code, defaultMounts, privilegedMounts)
	if err != nil {
		return nil, fmt.Errorf("error creating Rego policy: %w", err)
	}
	regoPolicy.base64policy = base64EncodedPolicy
	return regoPolicy, nil
}

func newRegoPolicy(code string, defaultMounts []oci.Mount, privilegedMounts []oci.Mount) (*regoEnforcer, error) {
	policy := new(regoEnforcer)
	policy.code = code

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
		"policy.rego":    policy.code,
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
			"Policy": policy.code,
		}).Error("Rego policy execution error")
		return results, err
	}

	output := buf.String()
	if len(output) > 0 {
		log.G(ctx).WithFields(logrus.Fields{
			"output": output,
		}).Debug("Rego policy output")
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
		inputJSON, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("unable to marshal the Rego input data: %w", err)
		}

		return fmt.Errorf("device mount not allowed by policy.\ninput: %s", string(inputJSON))
	}

	deviceMap := policy.data["devices"].(map[string]string)
	if _, ok := deviceMap[target]; ok {
		inputJSON, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("unable to marshal the Rego input data: %w", err)
		}

		return fmt.Errorf("device %s already mounted.\ninput: %s", target, string(inputJSON))
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
		inputJSON, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("unable to marshal the Rego input data: %w", err)
		}

		return fmt.Errorf("overlay mount not allowed by policy.\ninput: %s", string(inputJSON))
	}

	// we store the mapping of container ID -> layerPaths for later
	// use in EnforceCreateContainerPolicy here.
	containerMap := policy.data["containers"].(map[string]interface{})
	if _, ok := containerMap[containerID]; ok {
		inputJSON, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("unable to marshal the Rego input data: %w", err)
		}

		return fmt.Errorf("container %s already mounted.\ninput: %s", containerID, string(inputJSON))
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
		inputJSON, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("unable to marshal the Rego input data: %w", err)
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
		return fmt.Errorf("container creation not allowed by policy. Reasons: [%s].\nInput: %s", strings.Join(reasons, ","), string(inputJSON))
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
