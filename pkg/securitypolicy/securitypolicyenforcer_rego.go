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
	"syscall"

	"github.com/Microsoft/hcsshim/internal/guest/spec"
	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/storage/inmem"
	"github.com/open-policy-agent/opa/topdown"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
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

//go:embed api.rego
var apiCode string

const plan9Prefix = "plan9://"

// RegoEnforcer is a stub implementation of a security policy, which will be
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

		if securityPolicy.AllowAll {
			return createOpenDoorEnforcer(base64EncodedPolicy, defaultMounts, privilegedMounts)
		}

		code, err = marshalRego(securityPolicy.AllowAll, containers, []ExternalProcessConfig{})
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
		// for more information on metadata, see the `updateMetadata` method
		"metadata":         map[string]map[string]interface{}{},
		"defaultMounts":    appendMountData(defaultMountData, defaultMounts),
		"privilegedMounts": appendMountData(privilegedMountData, privilegedMounts),
		"sandboxPrefix":    guestpath.SandboxMountPrefix,
		"hugePagesPrefix":  guestpath.HugePagesMountPrefix,
		"plan9Prefix":      plan9Prefix,
	}
	policy.base64policy = ""

	modules := map[string]string{
		"policy.rego":    policy.code,
		"api.rego":       apiCode,
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

func (policy *regoEnforcer) allowed(enforcementPoint string, results map[string]interface{}) (bool, error) {
	if len(results) == 0 {
		info, err := policy.queryEnforcementPoint(enforcementPoint)
		if err != nil {
			return false, err
		}

		if info.availableByPolicyVersion {
			// policy should define this rule but it is missing
			return false, fmt.Errorf("rule for %s is missing from policy", enforcementPoint)
		} else {
			// rule added after policy was authored
			return info.allowedByDefault, nil
		}
	}

	if allowed, ok := results["allowed"].(bool); ok {
		return allowed, nil
	} else {
		return false, fmt.Errorf("unable to load 'allowed' from object returned by policy for %s", enforcementPoint)
	}
}

type enforcementPointInfo struct {
	availableByPolicyVersion bool
	allowedByDefault         bool
}

func (policy *regoEnforcer) queryEnforcementPoint(enforcementPoint string) (enforcementPointInfo, error) {
	input := map[string]interface{}{"name": enforcementPoint}
	input["rule"] = enforcementPoint
	query := rego.New(
		rego.Query("data.framework.enforcement_point_info"),
		rego.Input(input),
		rego.Compiler(policy.compiledModules))

	var info enforcementPointInfo

	ctx := context.Background()
	resultSet, err := query.Eval(ctx)
	if err != nil {
		return info, err
	}

	results := resultSet[0].Expressions[0].Value.(map[string]interface{})

	if results["unknown"].(bool) {
		return info, fmt.Errorf("enforcement point rule %s does not exist", enforcementPoint)
	}

	if results["invalid"].(bool) {
		return info, fmt.Errorf("enforcement point rule %s is invalid", enforcementPoint)
	}

	info.availableByPolicyVersion = results["available"].(bool)
	info.allowedByDefault = results["allowed"].(bool)
	return info, nil
}

func (policy *regoEnforcer) query(enforcementPoint string, input map[string]interface{}) (map[string]interface{}, error) {
	store := inmem.NewFromObject(policy.data)

	input["name"] = enforcementPoint
	var buf bytes.Buffer
	query := rego.New(
		rego.Query(fmt.Sprintf("data.policy.%s", enforcementPoint)),
		rego.Compiler(policy.compiledModules),
		rego.Input(input),
		rego.Store(store),
		rego.PrintHook(topdown.NewPrintHook(&buf)))

	ctx := context.Background()
	resultSet, err := query.Eval(ctx)
	if err != nil {
		log.G(ctx).WithError(err).WithFields(logrus.Fields{
			"Policy": policy.code,
		}).Error("Rego policy execution error")
		return nil, err
	}

	output := buf.String()
	if len(output) > 0 {
		log.G(ctx).WithFields(logrus.Fields{
			"output": output,
		}).Debug("Rego policy output")
	}

	if len(resultSet) == 0 {
		return map[string]interface{}{}, nil
	}

	results, ok := resultSet[0].Expressions[0].Value.(map[string]interface{})
	if !ok {
		return nil, errors.New("unable to load results object from Rego query")
	}
	return results, nil
}

/**
Each rule can optionally return a series of metadata commands in addition to
`allowed` which will then be made available in the `data.metadata` namespace
for use by the policy in future rule evaluations. A metadata command has the
following format:

``` json
{
    "<name>": {
        "action": "<add|update|remove>",
        "key": "<key>",
        "value": "<optional value>"
    }
}
```

Metadata objects can be any Rego object, *i.e.* arbitrary JSON. Importantly,
the Go code does not need to understand what they are or what they contain, just
place them in the specified point in the hierarchy such that the policy can find
them in later rule evaluations. To give a sense of how this works, here are a
sequence of rule results and the resulting metadata state:

**Initial State**
``` json
{
    "metadata": {}
}
```

**Result 1**
``` json
{
    "allowed": true,
    "devices": {
        "action": "add",
        "key": "/dev/layer0",
        "value": "5c5d1ae1aff5e1f36d5300de46592efe4ccb7889e60a4b82bbaf003c2248f2a7"
    }
}
```

**State 1**
``` json
{
    "metadata": {
        "devices": {
            "/dev/layer0": "5c5d1ae1aff5e1f36d5300de46592efe4ccb7889e60a4b82bbaf003c2248f2a7"
        }
    }
}
```

**Result 2**
``` json
{
    "allowed": true,
    "matches": {
        "action": "add",
        "key": "container1",
        "value": [{<container>}, {<container>}, {<container>}]
    }
}
```

**State 2**
``` json
{
    "metadata": {
        "devices": {
            "/dev/layer0": "5c5d1ae1aff5e1f36d5300de46592efe4ccb7889e60a4b82bbaf003c2248f2a7"
        },
        "matches": {
            "container1": [{<container>}, {<container>}, {<container>}]
        }
    }
}
```

**Result 3**
``` json
{
    "allowed": true,
    "matches": {
        "action": "update",
        "key": "container1",
        "value": [{<container>}]
    }
}
```

**State 3**
``` json
{
    "metadata": {
        "devices": {
            "/dev/layer0": "5c5d1ae1aff5e1f36d5300de46592efe4ccb7889e60a4b82bbaf003c2248f2a7"
        },
        "matches": {
            "container1": [{<container>}]
        }
    }
}
```

**Result 4**
``` json
{
    "allowed": true,
    "devices": {
        "action": "remove",
        "key": "/dev/layer0"
    }
}
```

**State 4**
``` json
{
    "metadata": {
        "devices": {},
        "matches": {
            "container1": [{<container>}]
        }
    }
}
```
*/

type metadataAction string

const (
	Add    metadataAction = "add"
	Update metadataAction = "update"
	Remove metadataAction = "remove"
)

type metadataOperation struct {
	action metadataAction
	key    string
	value  interface{}
}

func newMetadataOperation(operation interface{}) (*metadataOperation, error) {
	data, ok := operation.(map[string]interface{})
	var metadataOp metadataOperation
	if !ok {
		return nil, errors.New("unable to load metadata object")
	}
	action, ok := data["action"].(string)
	if !ok {
		return nil, errors.New("unable to load metadata action")
	}
	metadataOp.action = metadataAction(action)
	metadataOp.key, ok = data["key"].(string)
	if !ok {
		return nil, errors.New("unable to load metadata key")
	}

	if metadataOp.action != Remove {
		metadataOp.value, ok = data["value"]
		if !ok {
			return nil, errors.New("unable to load metadata value")
		}
	}

	return &metadataOp, nil
}

func (policy *regoEnforcer) updateMetadata(results map[string]interface{}) error {
	policy.mutex.Lock()
	defer policy.mutex.Unlock()

	// this is the top-level data namespace for metadata
	metadata := policy.data["metadata"].(map[string]map[string]interface{})
	for name, value := range results {
		if name == "allowed" {
			continue
		}

		if _, ok := metadata[name]; !ok {
			// this adds the metadata object if it does not already exist
			metadata[name] = make(map[string]interface{})
		}

		op, err := newMetadataOperation(value)
		if err != nil {
			return err
		}

		switch op.action {
		case Add:
			_, ok := metadata[name][op.key]
			if ok {
				return fmt.Errorf("cannot add metadata value, key %s[%s] already exists", name, op.key)
			}

			metadata[name][op.key] = op.value
			break

		case Update:
			metadata[name][op.key] = op.value
			break

		case Remove:
			delete(metadata[name], op.key)
			break

		default:
			return fmt.Errorf("unrecognized metadata action: %s", op.action)
		}
	}

	return nil
}

func (policy *regoEnforcer) enforce(enforcementPoint string, input map[string]interface{}) error {
	results, err := policy.query(enforcementPoint, input)
	if err != nil {
		return err
	}

	allowed, err := policy.allowed(enforcementPoint, results)
	if err != nil {
		return err
	}

	if allowed {
		err = policy.updateMetadata(results)
		if err != nil {
			return fmt.Errorf("unable to update metadata: %w", err)
		}
		return nil
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("unable to marshal the Rego input data: %w", err)
	}

	input["rule"] = enforcementPoint
	results, err = policy.query("reason", input)
	if err != nil {
		return fmt.Errorf("%s not allowed by policy.\nInput: %s", enforcementPoint, string(inputJSON))
	}

	if errors, ok := results["errors"]; ok {
		return fmt.Errorf("%s not allowed by policy. Errors: %v.\nInput: %s", enforcementPoint, errors, string(inputJSON))
	} else {
		return fmt.Errorf("%s not allowed by policy.\nInput: %s", enforcementPoint, string(inputJSON))
	}
}

func (policy *regoEnforcer) EnforceDeviceMountPolicy(target string, deviceHash string) error {
	input := map[string]interface{}{
		"target":     target,
		"deviceHash": deviceHash,
	}

	return policy.enforce("mount_device", input)
}

func (policy *regoEnforcer) EnforceOverlayMountPolicy(containerID string, layerPaths []string, target string) error {
	input := map[string]interface{}{
		"containerID": containerID,
		"layerPaths":  layerPaths,
		"target":      target,
	}

	return policy.enforce("mount_overlay", input)
}

func (policy *regoEnforcer) EnforceOverlayUnmountPolicy(target string) error {
	input := map[string]interface{}{
		"unmountTarget": target,
	}

	return policy.enforce("unmount_overlay", input)
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
	input := map[string]interface{}{
		"containerID":  containerID,
		"argList":      argList,
		"envList":      envList,
		"workingDir":   workingDir,
		"sandboxDir":   sandboxMountsDir(sandboxID),
		"hugePagesDir": hugePagesMountsDir(sandboxID),
		"mounts":       appendMountData([]interface{}{}, mounts),
	}

	return policy.enforce("create_container", input)
}

func (policy *regoEnforcer) EnforceDeviceUnmountPolicy(unmountTarget string) error {
	input := map[string]interface{}{
		"unmountTarget": unmountTarget,
	}

	return policy.enforce("unmount_device", input)
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

func (policy *regoEnforcer) EnforceExecInContainerPolicy(containerID string, argList []string, envList []string, workingDir string) error {
	input := map[string]interface{}{
		"containerID": containerID,
		"argList":     argList,
		"envList":     envList,
		"workingDir":  workingDir,
	}

	return policy.enforce("exec_in_container", input)
}

func (policy *regoEnforcer) EnforceExecExternalProcessPolicy(argList []string, envList []string, workingDir string) error {
	input := map[string]interface{}{
		"argList":    argList,
		"envList":    envList,
		"workingDir": workingDir,
	}

	return policy.enforce("exec_external", input)
}

func (policy *regoEnforcer) EnforceShutdownContainerPolicy(containerID string) error {
	input := map[string]interface{}{
		"containerID": containerID,
	}

	return policy.enforce("shutdown_container", input)
}

func (policy *regoEnforcer) EnforceSignalContainerProcessPolicy(containerID string, signal syscall.Signal, isInitProcess bool, startupArgList []string) error {
	input := map[string]interface{}{
		"containerID":   containerID,
		"signal":        signal,
		"isInitProcess": isInitProcess,
		"argList":       startupArgList,
	}

	return policy.enforce("signal_container_process", input)
}

func (policy *regoEnforcer) EnforcePlan9MountPolicy(target string) error {
	mountPathPrefix := strings.Replace(guestpath.LCOWMountPathPrefixFmt, "%d", "[0-9]+", 1)
	input := map[string]interface{}{
		"rootPrefix":      guestpath.LCOWRootPrefixInUVM,
		"mountPathPrefix": mountPathPrefix,
		"target":          target,
	}

	return policy.enforce("plan9_mount", input)
}

func (policy *regoEnforcer) EnforcePlan9UnmountPolicy(target string) error {
	input := map[string]interface{}{
		"target": target,
	}

	return policy.enforce("plan9_unmount", input)
}
