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

type module struct {
	namespace string
	feed      string
	issuer    string
	code      string
}

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
	// Modules
	modules map[string]*module
	// Compiled modules
	compiledModules *ast.Compiler
	// Debug flag
	debug bool
	// Stdio allowed state on a per container id basis
	stdio map[string]bool
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

func toStringSet(items []string) stringSet {
	s := make(stringSet)
	for _, item := range items {
		s.add(item)
	}

	return s
}

func (s stringSet) toArray() []string {
	a := make([]string, 0, len(s))
	for item := range s {
		a = append(a, item)
	}

	return a
}

func (a stringSet) intersect(b stringSet) stringSet {
	s := make(stringSet)
	for item := range a {
		if b.contains(item) {
			s.add(item)
		}
	}

	return s
}

type policyResults map[string]interface{}
type metadataObject map[string]interface{}
type metadataStore map[string]metadataObject
type inputData map[string]interface{}

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

		code, err = marshalRego(
			securityPolicy.AllowAll,
			containers,
			[]ExternalProcessConfig{},
			[]FragmentConfig{},
			true,
			true,
			true,
			false,
			true,
		)
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
		"metadata":         make(metadataStore),
		"defaultMounts":    appendMountData(defaultMountData, defaultMounts),
		"privilegedMounts": appendMountData(privilegedMountData, privilegedMounts),
		"sandboxPrefix":    guestpath.SandboxMountPrefix,
		"hugePagesPrefix":  guestpath.HugePagesMountPrefix,
		"plan9Prefix":      plan9Prefix,
	}
	policy.base64policy = ""
	policy.debug = false
	policy.modules = map[string]*module{
		"policy.rego":    {namespace: "policy", code: policy.code},
		"api.rego":       {namespace: "api", code: apiCode},
		"framework.rego": {namespace: "framework", code: frameworkCode},
	}
	policy.stdio = map[string]bool{}

	err := policy.compile()
	if err != nil {
		return nil, fmt.Errorf("rego compilation failed: %w", err)
	}

	return policy, nil
}

func (policy *regoEnforcer) compile() error {
	if policy.compiledModules != nil {
		return nil
	}

	modules := make(map[string]string)
	for _, module := range policy.modules {
		modules[module.namespace+".rego"] = module.code
	}

	// TODO temporary hack for debugging policies until GCS logging design
	// and implementation is finalized. This option should be changed to
	// "true" if debugging is desired.
	options := ast.CompileOpts{
		EnablePrintStatements: policy.debug,
	}

	if compiled, err := ast.CompileModulesWithOpt(modules, options); err == nil {
		policy.compiledModules = compiled
		return nil
	} else {
		return fmt.Errorf("rego compilation failed: %w", err)
	}
}

func (policy *regoEnforcer) allowed(enforcementPoint string, results policyResults) (bool, error) {
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

func (policy *regoEnforcer) queryEnforcementPoint(enforcementPoint string) (*enforcementPointInfo, error) {
	input := inputData{"name": enforcementPoint}
	input["rule"] = enforcementPoint
	query := rego.New(
		rego.Query("data.framework.enforcement_point_info"),
		rego.Input(input),
		rego.Compiler(policy.compiledModules))

	ctx := context.Background()
	resultSet, err := query.Eval(ctx)
	if err != nil {
		return nil, err
	}

	results, ok := resultSet[0].Expressions[0].Value.(map[string]interface{})
	if !ok {
		return nil, errors.New("unable to load results object from Rego query")
	}

	if results["unknown"].(bool) {
		return nil, fmt.Errorf("enforcement point rule %s does not exist", enforcementPoint)
	}

	if results["invalid"].(bool) {
		return nil, fmt.Errorf("enforcement point rule %s is invalid", enforcementPoint)
	}

	return &enforcementPointInfo{
		availableByPolicyVersion: results["available"].(bool),
		allowedByDefault:         results["allowed"].(bool),
	}, nil
}

func (policy *regoEnforcer) query(enforcementPoint string, input inputData) (policyResults, error) {
	store := inmem.NewFromObject(policy.data)

	input["name"] = enforcementPoint
	var buf bytes.Buffer
	query := rego.New(
		rego.Query(fmt.Sprintf("data.policy.%s", enforcementPoint)),
		rego.Input(input),
		rego.Store(store),
		rego.EnablePrintStatements(policy.debug),
		rego.PrintHook(topdown.NewPrintHook(&buf)))

	if policy.compiledModules == nil {
		for _, module := range policy.modules {
			rego.Module(module.namespace, module.code)(query)
		}
	} else {
		rego.Compiler(policy.compiledModules)(query)
	}

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
		return make(policyResults), nil
	}

	results, ok := resultSet[0].Expressions[0].Value.(map[string]interface{})
	if !ok {
		return nil, errors.New("unable to load results object from Rego query")
	}
	return policyResults(results), nil
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
	if !ok {
		return nil, errors.New("unable to load metadata object")
	}
	action, ok := data["action"].(string)
	if !ok {
		return nil, errors.New("unable to load metadata action")
	}

	var metadataOp metadataOperation
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

var reservedResultKeys = map[string]struct{}{
	"allowed":            {},
	"add_module":         {},
	"env_list":           {},
	"allow_stdio_access": {},
}

func (policy *regoEnforcer) getMetadata(name string) (metadataObject, error) {
	metadata := policy.data["metadata"].(metadataStore)
	if store, ok := metadata[name]; ok {
		return store, nil
	}

	return nil, fmt.Errorf("unable to retrieve metadata store for %s", name)
}

func (policy *regoEnforcer) updateMetadata(results policyResults) error {
	policy.mutex.Lock()
	defer policy.mutex.Unlock()

	// this is the top-level data namespace for metadata
	metadata := policy.data["metadata"].(metadataStore)
	for name, value := range results {
		if _, ok := reservedResultKeys[name]; ok {
			continue
		}

		if _, ok := metadata[name]; !ok {
			// this adds the metadata object if it does not already exist
			metadata[name] = make(metadataObject)
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

func (policy *regoEnforcer) enforce(enforcementPoint string, input inputData) (policyResults, error) {
	results, err := policy.query(enforcementPoint, input)
	if err != nil {
		return nil, err
	}

	allowed, err := policy.allowed(enforcementPoint, results)
	if err != nil {
		return nil, err
	}

	if allowed {
		err = policy.updateMetadata(results)
	} else {
		err = policy.getReasonNotAllowed(enforcementPoint, input)
	}

	if err != nil {
		return nil, err
	}

	return results, nil
}

func errorString(errors interface{}) string {
	errorArray := errors.([]interface{})
	output := make([]string, len(errorArray))
	for i, err := range errorArray {
		output[i] = fmt.Sprintf("%v", err)
	}
	return strings.Join(output, ",")
}

func (policy *regoEnforcer) getReasonNotAllowed(enforcementPoint string, input inputData) error {
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("unable to marshal the Rego input data: %w", err)
	}

	input["rule"] = enforcementPoint
	results, err := policy.query("reason", input)
	if err != nil {
		return fmt.Errorf("%s not allowed by policy.\nInput: %s", enforcementPoint, string(inputJSON))
	}

	if errors, ok := results["errors"]; ok {
		return fmt.Errorf("%s not allowed by policy. Errors: %v.\nInput: %s", enforcementPoint, errorString(errors), string(inputJSON))
	} else {
		return fmt.Errorf("%s not allowed by policy.\nInput: %s", enforcementPoint, string(inputJSON))
	}
}

func (policy *regoEnforcer) EnforceDeviceMountPolicy(target string, deviceHash string) error {
	input := inputData{
		"target":     target,
		"deviceHash": deviceHash,
	}

	_, err := policy.enforce("mount_device", input)
	return err
}

func (policy *regoEnforcer) EnforceOverlayMountPolicy(containerID string, layerPaths []string, target string) error {
	input := inputData{
		"containerID": containerID,
		"layerPaths":  layerPaths,
		"target":      target,
	}

	_, err := policy.enforce("mount_overlay", input)
	return err
}

func (policy *regoEnforcer) EnforceOverlayUnmountPolicy(target string) error {
	input := inputData{
		"unmountTarget": target,
	}

	_, err := policy.enforce("unmount_overlay", input)
	return err
}

// Rego does not have a way to determine the OS path separator
// so we append it via these methods.
func sandboxMountsDir(sandboxID string) string {
	return fmt.Sprintf("%s%c", spec.SandboxMountsDir(sandboxID), os.PathSeparator)
}

func hugePagesMountsDir(sandboxID string) string {
	return fmt.Sprintf("%s%c", spec.HugePagesMountsDir(sandboxID), os.PathSeparator)
}

func getEnvsToKeep(envList []string, results policyResults) ([]string, error) {
	value, ok := results["env_list"]
	if !ok {
		// policy did not return an 'env_list'. This is interpreted
		// as "proceed with provided env list".
		return envList, nil
	}

	envsAsInterfaces, ok := value.([]interface{})

	if !ok {
		return nil, fmt.Errorf("policy returned incorrect type for 'env_list', expected []interface{}, received %T", value)
	}

	keepSet := make(stringSet)
	for _, envAsInterface := range envsAsInterfaces {
		if env, ok := envAsInterface.(string); ok {
			keepSet.add(env)
		} else {
			return nil, fmt.Errorf("members of env_list from policy must be strings, received %T", envAsInterface)
		}
	}

	keepSet = keepSet.intersect(toStringSet(envList))
	return keepSet.toArray(), nil
}

func (policy *regoEnforcer) EnforceCreateContainerPolicy(
	sandboxID string,
	containerID string,
	argList []string,
	envList []string,
	workingDir string,
	mounts []oci.Mount,
) (toKeep EnvList, stdioAccessAllowed bool, err error) {
	input := inputData{
		"containerID":  containerID,
		"argList":      argList,
		"envList":      envList,
		"workingDir":   workingDir,
		"sandboxDir":   sandboxMountsDir(sandboxID),
		"hugePagesDir": hugePagesMountsDir(sandboxID),
		"mounts":       appendMountData([]interface{}{}, mounts),
	}

	results, err := policy.enforce("create_container", input)
	if err != nil {
		return nil, false, err
	}

	toKeep, err = getEnvsToKeep(envList, results)
	if err != nil {
		return nil, false, err
	}

	if value, ok := results["allow_stdio_access"]; ok {
		if value, ok := value.(bool); ok {
			stdioAccessAllowed = value
		} else {
			// we got a non-boolean. that's a clear error
			// alert that we got an error rather than setting a default value
			return nil, false, errors.New("`allow_stdio_access` needs to be a boolean")
		}
	} else {
		// Policy writer didn't specify an `allow_studio_access` value.
		// We have two options, return an error or set a default value.
		// We are setting a default value: do not allow
		stdioAccessAllowed = false
	}

	// Store the result of stdio access allowed for this container so we can use
	// it if we get queried about allowing exec in container access. Stdio access
	// is on a per-container, not per-process basis.
	policy.stdio[containerID] = stdioAccessAllowed

	return toKeep, stdioAccessAllowed, nil
}

func (policy *regoEnforcer) EnforceDeviceUnmountPolicy(unmountTarget string) error {
	input := inputData{
		"unmountTarget": unmountTarget,
	}

	_, err := policy.enforce("unmount_device", input)
	return err
}

func appendMountData(mountData []interface{}, mounts []oci.Mount) []interface{} {
	for _, mount := range mounts {
		mountData = append(mountData, inputData{
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

func (policy *regoEnforcer) EnforceExecInContainerPolicy(containerID string, argList []string, envList []string, workingDir string) (toKeep EnvList, stdioAccessAllowed bool, err error) {
	input := inputData{
		"containerID": containerID,
		"argList":     argList,
		"envList":     envList,
		"workingDir":  workingDir,
	}

	results, err := policy.enforce("exec_in_container", input)
	if err != nil {
		return nil, false, err
	}

	toKeep, err = getEnvsToKeep(envList, results)
	if err != nil {
		return nil, false, err
	}

	return toKeep, policy.stdio[containerID], nil
}

func (policy *regoEnforcer) EnforceExecExternalProcessPolicy(argList []string, envList []string, workingDir string) (toKeep EnvList, stdioAccessAllowed bool, err error) {
	input := map[string]interface{}{
		"argList":    argList,
		"envList":    envList,
		"workingDir": workingDir,
	}

	results, err := policy.enforce("exec_external", input)
	if err != nil {
		return nil, false, err
	}

	toKeep, err = getEnvsToKeep(envList, results)
	if err != nil {
		return nil, false, err
	}

	if value, ok := results["allow_stdio_access"]; ok {
		if value, ok := value.(bool); ok {
			stdioAccessAllowed = value
		} else {
			// we got a non-boolean. that's a clear error
			// alert that we got an error rather than setting a default value
			return nil, false, errors.New("`allow_stdio_access` needs to be a boolean")
		}
	} else {
		// Policy writer didn't specify an `allow_studio_access` value.
		// We have two options, return an error or set a default value.
		// We are setting a default value: do not allow
		stdioAccessAllowed = false
	}

	return toKeep, stdioAccessAllowed, nil
}

func (policy *regoEnforcer) EnforceShutdownContainerPolicy(containerID string) error {
	input := inputData{
		"containerID": containerID,
	}

	_, err := policy.enforce("shutdown_container", input)
	return err
}

func (policy *regoEnforcer) EnforceSignalContainerProcessPolicy(containerID string, signal syscall.Signal, isInitProcess bool, startupArgList []string) error {
	input := inputData{
		"containerID":   containerID,
		"signal":        signal,
		"isInitProcess": isInitProcess,
		"argList":       startupArgList,
	}

	_, err := policy.enforce("signal_container_process", input)
	return err
}

func (policy *regoEnforcer) EnforcePlan9MountPolicy(target string) error {
	mountPathPrefix := strings.Replace(guestpath.LCOWMountPathPrefixFmt, "%d", "[0-9]+", 1)
	input := inputData{
		"rootPrefix":      guestpath.LCOWRootPrefixInUVM,
		"mountPathPrefix": mountPathPrefix,
		"target":          target,
	}

	_, err := policy.enforce("plan9_mount", input)
	return err
}

func (policy *regoEnforcer) EnforcePlan9UnmountPolicy(target string) error {
	input := map[string]interface{}{
		"unmountTarget": target,
	}

	_, err := policy.enforce("plan9_unmount", input)
	return err
}

func (policy *regoEnforcer) EnforceGetPropertiesPolicy() error {
	input := make(inputData)

	_, err := policy.enforce("get_properties", input)
	return err
}

func (policy *regoEnforcer) EnforceDumpStacksPolicy() error {
	input := make(inputData)

	_, err := policy.enforce("dump_stacks", input)
	return err
}

func (policy *regoEnforcer) EnforceRuntimeLoggingPolicy() error {
	input := make(inputData)
	_, err := policy.enforce("runtime_logging", input)
	return err
}

func moduleID(issuer string, feed string) string {
	return fmt.Sprintf("%s>%s", issuer, feed)
}

func (f module) id() string {
	return moduleID(f.issuer, f.feed)
}

func parseNamespace(rego string) (string, error) {
	lines := strings.Split(rego, "\n")
	parts := strings.Split(lines[0], " ")
	if parts[0] != "package" {
		return "", errors.New("package definition required on first line")
	}

	namespace := parts[1]
	return namespace, nil
}

func (policy *regoEnforcer) LoadFragment(issuer string, feed string, rego string) error {
	namespace, err := parseNamespace(rego)
	if err != nil {
		return fmt.Errorf("unable to load fragment: %w", err)
	}

	fragment := &module{
		issuer:    issuer,
		feed:      feed,
		code:      rego,
		namespace: namespace,
	}

	policy.modules[fragment.id()] = fragment
	policy.compiledModules = nil

	input := inputData{
		"issuer":    issuer,
		"feed":      feed,
		"namespace": namespace,
	}

	results, err := policy.enforce("load_fragment", input)

	removeModule := true
	if err == nil {
		if addModule, ok := results["add_module"].(bool); ok {
			if addModule {
				removeModule = false
			}
		}
	}

	if removeModule {
		delete(policy.modules, fragment.id())
	}

	return err
}

func (policy *regoEnforcer) EnforceScratchMountPolicy(scratchPath string, encrypted bool) error {
	input := map[string]interface{}{
		"target":    scratchPath,
		"encrypted": encrypted,
	}
	_, err := policy.enforce("scratch_mount", input)
	if err != nil {
		return err
	}
	return nil
}

func (policy *regoEnforcer) EnforceScratchUnmountPolicy(scratchPath string) error {
	input := map[string]interface{}{
		"unmountTarget": scratchPath,
	}
	_, err := policy.enforce("scratch_unmount", input)
	if err != nil {
		return err
	}
	return nil
}
