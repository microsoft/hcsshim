//go:build linux && rego
// +build linux,rego

package securitypolicy

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"syscall"

	"github.com/Microsoft/hcsshim/internal/guest/spec"
	"github.com/Microsoft/hcsshim/internal/guestpath"
	rpi "github.com/Microsoft/hcsshim/internal/regopolicyinterpreter"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
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

// RegoEnforcer is a stub implementation of a security policy, which will be
// based on [Rego] policy language. The detailed implementation will be
// introduced in the subsequent PRs and documentation updated accordingly.
//
// [Rego]: https://www.openpolicyagent.org/docs/latest/policy-language/
type regoEnforcer struct {
	// Base64 encoded (JSON) policy
	base64policy string
	// Rego interpreter
	rego *rpi.RegoPolicyInterpreter
	// Default mount data
	defaultMounts []oci.Mount
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

func (policy *regoEnforcer) enableLogging(path string, logLevel rpi.LogLevel) {
	policy.rego.EnableLogging(path, logLevel)
}

func newRegoPolicy(code string, defaultMounts []oci.Mount, privilegedMounts []oci.Mount) (policy *regoEnforcer, err error) {
	policy = new(regoEnforcer)

	policy.defaultMounts = make([]oci.Mount, len(defaultMounts))
	copy(policy.defaultMounts, defaultMounts)

	objectDefaults := make(map[string]interface{})
	err = json.Unmarshal([]byte(FrameworkObjects), &objectDefaults)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal framework object defaults: %w", err)
	}

	defaultMountData := make([]interface{}, 0, len(defaultMounts))
	privilegedMountData := make([]interface{}, 0, len(privilegedMounts))
	data := map[string]interface{}{
		"objectDefaults":   objectDefaults,
		"defaultMounts":    appendMountData(defaultMountData, defaultMounts),
		"privilegedMounts": appendMountData(privilegedMountData, privilegedMounts),
		"sandboxPrefix":    guestpath.SandboxMountPrefix,
		"hugePagesPrefix":  guestpath.HugePagesMountPrefix,
		"plan9Prefix":      plan9Prefix,
	}

	policy.rego, err = rpi.NewRegoPolicyInterpreter(code, data)
	if err != nil {
		return nil, err
	}
	policy.stdio = map[string]bool{}

	policy.base64policy = ""
	policy.rego.AddModule("framework.rego", &rpi.RegoModule{Namespace: "framework", Code: FrameworkCode})
	policy.rego.AddModule("api.rego", &rpi.RegoModule{Namespace: "api", Code: APICode})

	err = policy.rego.Compile()
	if err != nil {
		return nil, fmt.Errorf("rego compilation failed: %w", err)
	}

	return policy, nil
}

func (policy *regoEnforcer) applyDefaults(enforcementPoint string, results rpi.RegoQueryResult) (rpi.RegoQueryResult, error) {
	deny := rpi.RegoQueryResult{"allowed": false}
	info, err := policy.queryEnforcementPoint(enforcementPoint)
	if err != nil {
		return deny, err
	}

	if results.IsEmpty() && info.availableByPolicyVersion {
		// policy should define this rule but it is missing
		return deny, fmt.Errorf("rule for %s is missing from policy", enforcementPoint)
	}

	return info.defaultResults.Union(results), nil
}

type enforcementPointInfo struct {
	availableByPolicyVersion bool
	defaultResults           rpi.RegoQueryResult
}

func (policy *regoEnforcer) queryEnforcementPoint(enforcementPoint string) (*enforcementPointInfo, error) {
	input := inputData{
		"name": enforcementPoint,
		"rule": enforcementPoint,
	}
	result, err := policy.rego.Query("data.framework.enforcement_point_info", input)

	if err != nil {
		return nil, fmt.Errorf("error querying enforcement point information: %w", err)
	}

	unknown, err := result.Bool("unknown")
	if err != nil {
		return nil, err
	}

	if unknown {
		return nil, fmt.Errorf("enforcement point rule %s does not exist", enforcementPoint)
	}

	invalid, err := result.Bool("invalid")
	if err != nil {
		return nil, err
	}

	if invalid {
		return nil, fmt.Errorf("enforcement point rule %s is invalid", enforcementPoint)
	}

	defaultResults, err := result.Object("default_results")
	if err != nil {
		return nil, errors.New("enforcement point result missing defaults")
	}

	availableByPolicyVersion, err := result.Bool("available")
	if err != nil {
		return nil, errors.New("enforcement point result missing availability info")
	}

	return &enforcementPointInfo{
		availableByPolicyVersion: availableByPolicyVersion,
		defaultResults:           defaultResults,
	}, nil
}

func (policy *regoEnforcer) enforce(enforcementPoint string, input inputData) (rpi.RegoQueryResult, error) {
	rule := "data.policy." + enforcementPoint
	result, err := policy.rego.Query(rule, input)
	if err != nil {
		return nil, err
	}

	result, err = policy.applyDefaults(enforcementPoint, result)
	if err != nil {
		return result, err
	}

	allowed, err := result.Bool("allowed")
	if err != nil {
		return nil, err
	}

	if !allowed {
		err = policy.getReasonNotAllowed(enforcementPoint, input)
		return nil, err
	}

	return result, nil
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
	inputJSON, err := json.Marshal(policy.redactSensitiveData(input))
	if err != nil {
		return fmt.Errorf("%s not allowed by policy. Input unavailable due to marshalling error", enforcementPoint)
	}

	input["rule"] = enforcementPoint
	result, err := policy.rego.Query("data.policy.reason", input)
	if err == nil {
		errors, _ := result.Value("errors")
		if errors != nil {
			if len(errors.([]interface{})) > 0 {
				return fmt.Errorf("%s not allowed by policy. Errors: %v. Input: %s", enforcementPoint, errors, string(inputJSON))
			} else {
				return fmt.Errorf("%s not allowed by policy. Security policy is not valid. Please check security policy or re-generate with tooling. Input: %s", enforcementPoint, string(inputJSON))
			}
		}
	}

	return fmt.Errorf("%s not allowed by policy. Security policy is not valid. Please check security policy or re-generate with tooling. Input: %s", enforcementPoint, string(inputJSON))
}

func (policy *regoEnforcer) redactSensitiveData(input inputData) inputData {
	if v, k := input["envList"]; k {
		newEnvList := make([]string, 0)
		cast, ok := v.([]string)
		if ok {
			for _, env := range cast {
				parts := strings.Split(env, "=")
				redacted := parts[0] + "=<<redacted>>"
				newEnvList = append(newEnvList, redacted)
			}
		}

		input["envList"] = newEnvList
	}

	return input
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

func getEnvsToKeep(envList []string, results rpi.RegoQueryResult) ([]string, error) {
	value, err := results.Value("env_list")
	if err != nil || value == nil {
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
	privileged bool,
) (toKeep EnvList, stdioAccessAllowed bool, err error) {
	input := inputData{
		"containerID":  containerID,
		"argList":      argList,
		"envList":      envList,
		"workingDir":   workingDir,
		"sandboxDir":   spec.SandboxMountsDir(sandboxID),
		"hugePagesDir": spec.HugePagesMountsDir(sandboxID),
		"mounts":       appendMountData([]interface{}{}, mounts),
		"privileged":   privileged,
	}

	results, err := policy.enforce("create_container", input)
	if err != nil {
		return nil, false, err
	}

	toKeep, err = getEnvsToKeep(envList, results)
	if err != nil {
		return nil, false, err
	}

	stdioAccessAllowed, err = results.Bool("allow_stdio_access")
	if err != nil {
		return nil, false, err
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
	policy.defaultMounts = append(policy.defaultMounts, mounts...)
	defaultMounts := appendMountData([]interface{}{}, policy.defaultMounts)
	return policy.rego.UpdateData("defaultMounts", defaultMounts)
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

	stdioAccessAllowed, err = results.Bool("allow_stdio_access")
	if err != nil {
		return nil, false, err
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

func parseNamespace(rego string) (string, error) {
	lines := strings.Split(rego, "\n")
	parts := strings.Split(lines[0], " ")
	if parts[0] != "package" {
		return "", errors.New("package definition required on first line")
	}

	return strings.TrimSpace(parts[1]), nil
}

func (policy *regoEnforcer) LoadFragment(issuer string, feed string, rego string) error {
	namespace, err := parseNamespace(rego)
	if err != nil {
		return fmt.Errorf("unable to load fragment: %w", err)
	}

	fragment := &rpi.RegoModule{
		Issuer:    issuer,
		Feed:      feed,
		Code:      rego,
		Namespace: namespace,
	}

	policy.rego.AddModule(fragment.ID(), fragment)

	input := inputData{
		"issuer":    issuer,
		"feed":      feed,
		"namespace": namespace,
	}

	results, err := policy.enforce("load_fragment", input)

	addModule, _ := results.Bool("add_module")
	if !addModule {
		policy.rego.RemoveModule(fragment.ID())
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
