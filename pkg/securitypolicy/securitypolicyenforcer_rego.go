//go:build rego
// +build rego

package securitypolicy

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"syscall"

	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/Microsoft/hcsshim/internal/log"
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

const capabilitiesNilError = "capabilities object provided by the UVM to the policy engine is nil"
const invalidPolicyMessage = "Security policy is not valid. Please check security policy or re-generate with tooling."
const noReasonMessage = "Security policy is either not valid or did not provide a reason for denial. Please check security policy or re-generate with tooling."
const noAPIVersionError = "policy does not define api_version"

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
	// Maximum error message length
	maxErrorMessageLength int
	// OS type
	osType string
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
	maxErrorMessageLength int,
	osType string,
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
		if securityPolicy.AllowAll {
			return createOpenDoorEnforcer(base64EncodedPolicy, defaultMounts, privilegedMounts, maxErrorMessageLength, osType)
		}

		containers := make([]*Container, securityPolicy.Containers.Length)

		for i := 0; i < securityPolicy.Containers.Length; i++ {
			index := strconv.Itoa(i)
			cConf, ok := securityPolicy.Containers.Elements[index]
			if !ok {
				return nil, fmt.Errorf("container constraint with index %q not found", index)
			}
			cConf.AllowStdioAccess = true
			cConf.NoNewPrivileges = false
			cConf.User = UserConfig{
				UserIDName:   IDNameConfig{Strategy: IDNameStrategyAny},
				GroupIDNames: []IDNameConfig{{Strategy: IDNameStrategyAny}},
				Umask:        "0022",
			}
			cConf.SeccompProfileSHA256 = ""
			containers[i] = &cConf
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
			false,
		)
		if err != nil {
			return nil, fmt.Errorf("error marshaling the policy to Rego: %w", err)
		}
	} else {
		// this is either a Rego policy or malformed JSON
		code = string(rawPolicy)
	}

	regoPolicy, err := newRegoPolicy(code, defaultMounts, privilegedMounts, osType)
	if err != nil {
		return nil, fmt.Errorf("error creating Rego policy: %w", err)
	}
	regoPolicy.base64policy = base64EncodedPolicy
	regoPolicy.maxErrorMessageLength = maxErrorMessageLength
	return regoPolicy, nil
}

func (policy *regoEnforcer) enableLogging(path string, logLevel rpi.LogLevel) {
	policy.rego.EnableLogging(path, logLevel)
}

func newRegoPolicy(code string, defaultMounts []oci.Mount, privilegedMounts []oci.Mount, osType string) (policy *regoEnforcer, err error) {
	policy = new(regoEnforcer)

	policy.osType = osType
	policy.defaultMounts = make([]oci.Mount, len(defaultMounts))
	copy(policy.defaultMounts, defaultMounts)

	defaultMountData := make([]interface{}, 0, len(defaultMounts))
	privilegedMountData := make([]interface{}, 0, len(privilegedMounts))
	data := map[string]interface{}{
		"defaultMounts":                   appendMountData(defaultMountData, defaultMounts),
		"privilegedMounts":                appendMountData(privilegedMountData, privilegedMounts),
		"sandboxPrefix":                   guestpath.SandboxMountPrefix,
		"hugePagesPrefix":                 guestpath.HugePagesMountPrefix,
		"plan9Prefix":                     plan9Prefix,
		"defaultUnprivilegedCapabilities": DefaultUnprivilegedCapabilities(),
		"defaultPrivilegedCapabilities":   DefaultPrivilegedCapabilities(),
	}

	policy.rego, err = rpi.NewRegoPolicyInterpreter(code, data)
	policy.rego.UpdateOSType(osType)
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

	// by default we do not perform message truncation
	policy.maxErrorMessageLength = 0

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

	versionMissing, err := result.Bool("version_missing")
	if err != nil {
		return nil, err
	}

	if versionMissing {
		return nil, errors.New(noAPIVersionError)
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

func (policy *regoEnforcer) enforce(ctx context.Context, enforcementPoint string, input inputData) (rpi.RegoQueryResult, error) {
	rule := "data.policy." + enforcementPoint
	result, err := policy.rego.Query(rule, input)
	if err != nil {
		return nil, policy.denyWithError(ctx, err, input)
	}

	result, err = policy.applyDefaults(enforcementPoint, result)
	if err != nil {
		return result, policy.denyWithError(ctx, err, input)
	}

	allowed, err := result.Bool("allowed")
	if err != nil {
		return nil, policy.denyWithError(ctx, err, input)
	}

	if !allowed {
		return nil, policy.denyWithReason(ctx, enforcementPoint, input)
	}

	return result, nil
}

type decisionTruncator func(map[string]interface{})

func truncateErrorObjects(decision map[string]interface{}) {
	if rawReason, ok := decision["reason"]; ok {
		// check if it is a framework reason object
		if reason, ok := rawReason.(rpi.RegoQueryResult); ok {
			// check if we can remove error_objects
			if _, ok := reason["error_objects"]; ok {
				decision["truncated"] = append(decision["truncated"].([]string), "reason.error_objects")
				delete(reason, "error_objects")
				decision["reason"] = reason
			}
		}
	}
}

func truncateInput(decision map[string]interface{}) {
	if _, ok := decision["input"]; ok {
		// remove the input
		decision["truncated"] = append(decision["truncated"].([]string), "input")
		delete(decision, "input")
	}
}

func truncateReason(decision map[string]interface{}) {
	decision["truncated"] = append(decision["truncated"].([]string), "reason")
	delete(decision, "reason")
}

func (policy *regoEnforcer) policyDecisionToError(ctx context.Context, decision map[string]interface{}) error {
	decisionJSON, err := json.Marshal(decision)
	if err != nil {
		log.G(ctx).WithError(err).Error("unable to marshal error object")
		decisionJSON = []byte(`"Unable to marshal error object"`)
	}

	log.G(ctx).WithField("policyDecision", string(decisionJSON))

	base64EncodedDecisionJSON := base64.StdEncoding.EncodeToString(decisionJSON)
	errorMessage := fmt.Errorf(policyDecisionPattern, base64EncodedDecisionJSON)
	if policy.maxErrorMessageLength == 0 {
		// indicates no message truncation
		return fmt.Errorf(policyDecisionPattern, base64EncodedDecisionJSON)
	}

	if len(errorMessage.Error()) <= policy.maxErrorMessageLength {
		return errorMessage
	}

	decision["truncated"] = []string{}
	truncators := []decisionTruncator{truncateErrorObjects, truncateInput, truncateReason}
	for _, truncate := range truncators {
		truncate(decision)

		decisionJSON, err := json.Marshal(decision)
		if err != nil {
			log.G(ctx).WithError(err).Error("unable to marshal error object")
			decisionJSON = []byte(`"Unable to marshal error object"`)
		}
		base64EncodedDecisionJSON = base64.StdEncoding.EncodeToString(decisionJSON)
		errorMessage = fmt.Errorf(policyDecisionPattern, base64EncodedDecisionJSON)

		if len(errorMessage.Error()) <= policy.maxErrorMessageLength {
			break
		}
	}

	return errorMessage
}

func (policy *regoEnforcer) denyWithError(ctx context.Context, policyError error, input inputData) error {
	input = policy.redactSensitiveData(input)
	input = replaceCapabilitiesWithPlaceholders(input)
	policyDecision := map[string]interface{}{
		"input":       input,
		"decision":    "deny",
		"reason":      invalidPolicyMessage,
		"policyError": policyError.Error(),
	}

	return policy.policyDecisionToError(ctx, policyDecision)
}

func (policy *regoEnforcer) denyWithReason(ctx context.Context, enforcementPoint string, input inputData) error {
	cleaned_input := policy.redactSensitiveData(input)
	cleaned_input = replaceCapabilitiesWithPlaceholders(cleaned_input)
	input["rule"] = enforcementPoint
	policyDecision := map[string]interface{}{
		"input":    cleaned_input,
		"decision": "deny",
	}

	result, err := policy.rego.Query("data.policy.reason", input)
	if err == nil {
		if result.IsEmpty() {
			policyDecision["reason"] = noReasonMessage
		} else {
			policyDecision["reason"] = replaceCapabilitiesWithPlaceholdersInReason(result)
		}
	} else {
		log.G(ctx).WithError(err).Warn("unable to obtain reason for policy decision")
		policyDecision["reason"] = noReasonMessage
	}

	return policy.policyDecisionToError(ctx, policyDecision)
}

func areCapsEqual(actual map[string]interface{}, expected map[string][]string) bool {
	for key, caps := range expected {
		values, ok := actual[key].([]interface{})
		if !ok {
			return false
		}

		if len(values) != len(caps) {
			return false
		}

		for i, value := range values {
			cap, ok := value.(string)
			if !ok {
				return false
			}

			if cap != caps[i] {
				return false
			}
		}
	}

	return true
}

var privilegedCapabilities = map[string][]string{
	"bounding":    DefaultPrivilegedCapabilities(),
	"effective":   DefaultPrivilegedCapabilities(),
	"inheritable": DefaultPrivilegedCapabilities(),
	"permitted":   DefaultPrivilegedCapabilities(),
	"ambient":     EmptyCapabiltiesSet(),
}

var unprivilegedCapabilities = map[string][]string{
	"bounding":    DefaultUnprivilegedCapabilities(),
	"effective":   DefaultUnprivilegedCapabilities(),
	"inheritable": EmptyCapabiltiesSet(),
	"permitted":   DefaultUnprivilegedCapabilities(),
	"ambient":     EmptyCapabiltiesSet(),
}

// as capability lists are repetitive and take up a lot of room in the error
// message, we can replace the defaults with placeholders to save space
func replaceCapabilitiesWithPlaceholders(object map[string]interface{}) map[string]interface{} {
	capabilities, ok := object["capabilities"].(map[string]interface{})
	if !ok {
		return object
	}

	if areCapsEqual(capabilities, privilegedCapabilities) {
		object["capabilities"] = "[privileged]"
	} else if areCapsEqual(capabilities, unprivilegedCapabilities) {
		object["capabilities"] = "[unprivileged]"
	}

	return object
}

func replaceCapabilitiesWithPlaceholdersInReason(reason rpi.RegoQueryResult) rpi.RegoQueryResult {
	errorObjectsRaw, err := reason.Value("error_objects")
	if err != nil {
		return reason
	}

	errorObjects, ok := errorObjectsRaw.([]interface{})
	if !ok {
		return reason
	}

	objects := make([]interface{}, len(errorObjects))
	for i, objectRaw := range errorObjects {
		object, ok := objectRaw.(map[string]interface{})
		if !ok {
			objects[i] = objectRaw
			continue
		}

		objects[i] = replaceCapabilitiesWithPlaceholders(object)
	}

	reason["error_objects"] = objects
	return reason
}

func (policy *regoEnforcer) redactSensitiveData(input inputData) inputData {
	if v, k := input["envList"]; k {
		newInput := make(inputData)
		for k, v := range input {
			newInput[k] = v
		}

		newEnvList := make([]string, 0)
		cast, ok := v.([]string)
		if ok {
			for _, env := range cast {
				parts := strings.Split(env, "=")
				redacted := parts[0] + "=<<redacted>>"
				newEnvList = append(newEnvList, redacted)
			}
		}

		newInput["envList"] = newEnvList

		return newInput
	}

	return input
}

func (policy *regoEnforcer) EnforceDeviceMountPolicy(ctx context.Context, target string, deviceHash string) error {
	input := inputData{
		"target":     target,
		"deviceHash": deviceHash,
	}

	_, err := policy.enforce(ctx, "mount_device", input)
	return err
}

func (policy *regoEnforcer) EnforceOverlayMountPolicy(ctx context.Context, containerID string, layerPaths []string, target string) error {
	input := inputData{
		"containerID": containerID,
		"layerPaths":  layerPaths,
		"target":      target,
	}

	_, err := policy.enforce(ctx, "mount_overlay", input)
	return err
}

func (policy *regoEnforcer) EnforceOverlayUnmountPolicy(ctx context.Context, target string) error {
	input := inputData{
		"unmountTarget": target,
	}

	_, err := policy.enforce(ctx, "unmount_overlay", input)
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

func getCapsToKeep(capsList *oci.LinuxCapabilities, results rpi.RegoQueryResult) (*oci.LinuxCapabilities, error) {
	value, err := results.Value("caps_list")
	if err != nil || value == nil {
		// policy did not return an 'caps_list'. This is interpreted
		// as "proceed with provided caps list".
		return capsList, nil
	}

	capsMap, ok := value.(map[string]interface{})

	if !ok {
		return nil, fmt.Errorf("policy returned incorrect type for 'caps_list', expected map[string]interface{}, received %T", value)
	}

	bounding, err := filterCapabilities(capsList.Bounding, capsMap["bounding"])
	if err != nil {
		return nil, err
	}
	effective, err := filterCapabilities(capsList.Effective, capsMap["effective"])
	if err != nil {
		return nil, err
	}
	inheritable, err := filterCapabilities(capsList.Inheritable, capsMap["inheritable"])
	if err != nil {
		return nil, err
	}
	permitted, err := filterCapabilities(capsList.Permitted, capsMap["permitted"])
	if err != nil {
		return nil, err
	}
	ambient, err := filterCapabilities(capsList.Ambient, capsMap["ambient"])
	if err != nil {
		return nil, err
	}

	return &oci.LinuxCapabilities{
		Bounding:    bounding,
		Effective:   effective,
		Inheritable: inheritable,
		Permitted:   permitted,
		Ambient:     ambient,
	}, nil
}

func filterCapabilities(suppliedList []string, fromRegoCapsList interface{}) ([]string, error) {
	keepSet := make(stringSet)
	if capsList, ok := fromRegoCapsList.([]interface{}); ok {
		for _, capAsInterface := range capsList {
			if cap, ok := capAsInterface.(string); ok {
				keepSet.add(cap)
			} else {
				return nil, fmt.Errorf("members of capability sets from policy must be strings, received %T", capAsInterface)
			}
		}
	} else {
		return nil, fmt.Errorf("capability sets of caps_list from policy must be an array of interface{}, received %T", fromRegoCapsList)
	}

	keepSet = keepSet.intersect(toStringSet(suppliedList))
	return keepSet.toArray(), nil
}

func (idName IDName) toInput() interface{} {
	return map[string]interface{}{
		"id":   idName.ID,
		"name": idName.Name,
	}
}

func groupsToInputs(groups []IDName) []interface{} {
	inputs := []interface{}{}
	for _, group := range groups {
		inputs = append(inputs, group.toInput())
	}
	return inputs
}

func handleNilOrEmptyCaps(caps []string) interface{} {
	if len(caps) > 0 {
		result := make([]interface{}, len(caps))
		for i, cap := range caps {
			result[i] = cap
		}

		return result
	}

	// caps is either nil or empty.
	// In either case, we want to return an empty array.
	return make([]interface{}, 0)
}

func mapifyCapabilities(caps *oci.LinuxCapabilities) map[string]interface{} {
	out := make(map[string]interface{})

	out["bounding"] = handleNilOrEmptyCaps(caps.Bounding)
	out["effective"] = handleNilOrEmptyCaps(caps.Effective)
	out["inheritable"] = handleNilOrEmptyCaps(caps.Inheritable)
	out["permitted"] = handleNilOrEmptyCaps(caps.Permitted)
	out["ambient"] = handleNilOrEmptyCaps(caps.Ambient)
	return out
}

func (policy *regoEnforcer) EnforceCreateContainerPolicy(
	ctx context.Context,
	sandboxID string,
	containerID string,
	argList []string,
	envList []string,
	workingDir string,
	mounts []oci.Mount,
	privileged bool,
	noNewPrivileges bool,
	user IDName,
	groups []IDName,
	umask string,
	capabilities *oci.LinuxCapabilities,
	seccompProfileSHA256 string,
) (envToKeep EnvList,
	capsToKeep *oci.LinuxCapabilities,
	stdioAccessAllowed bool,
	err error) {
	opts := &CreateContainerOptions{
		SandboxID:            sandboxID,
		Privileged:           &privileged,
		NoNewPrivileges:      &noNewPrivileges,
		Groups:               groups,
		Umask:                umask,
		Capabilities:         capabilities,
		SeccompProfileSHA256: seccompProfileSHA256,
	}
	return policy.EnforceCreateContainerPolicyV2(ctx, containerID, argList, envList, workingDir, mounts, user, opts)
}

func (policy *regoEnforcer) EnforceCreateContainerPolicyV2(
	ctx context.Context,
	containerID string,
	argList []string,
	envList []string,
	workingDir string,
	mounts []oci.Mount,
	user IDName,
	opts *CreateContainerOptions,
) (envToKeep EnvList,
	capsToKeep *oci.LinuxCapabilities,
	stdioAccessAllowed bool,
	err error) {

	if policy.osType == "linux" && opts.Capabilities == nil {
		return nil, nil, false, errors.New(capabilitiesNilError)
	}

	var input inputData

	switch policy.osType {
	case "linux":
		input = inputData{
			"containerID":          containerID,
			"argList":              argList,
			"envList":              envList,
			"workingDir":           workingDir,
			"sandboxDir":           SandboxMountsDir(opts.SandboxID),
			"hugePagesDir":         HugePagesMountsDir(opts.SandboxID),
			"mounts":               appendMountData([]interface{}{}, mounts),
			"privileged":           opts.Privileged,
			"noNewPrivileges":      opts.NoNewPrivileges,
			"user":                 user.toInput(),
			"groups":               groupsToInputs(opts.Groups),
			"umask":                opts.Umask,
			"capabilities":         mapifyCapabilities(opts.Capabilities),
			"seccompProfileSHA256": opts.SeccompProfileSHA256,
		}
	case "windows":
		if envList == nil {
			envList = []string{}
		}
		input = inputData{
			"containerID": containerID,
			"argList":     argList,
			"envList":     envList,
			"workingDir":  workingDir,
			"privileged":  true,
			"user":        user.Name,
		}
	default:
		return nil, nil, false, errors.Errorf("unsupported OS value in options: %q", policy.osType)
	}

	results, err := policy.enforce(ctx, "create_container", input)
	if err != nil {
		return nil, nil, false, err
	}

	envToKeep, err = getEnvsToKeep(envList, results)
	if err != nil {
		return nil, nil, false, err
	}

	if policy.osType == "linux" {
		capsToKeep, err = getCapsToKeep(opts.Capabilities, results)
		if err != nil {
			return nil, nil, false, err
		}
	}

	stdioAccessAllowed, err = results.Bool("allow_stdio_access")
	if err != nil {
		return nil, nil, false, err
	}

	// Store the result of stdio access allowed for this container so we can use
	// it if we get queried about allowing exec in container access. Stdio access
	// is on a per-container, not per-process basis.
	policy.stdio[containerID] = stdioAccessAllowed

	return envToKeep, capsToKeep, stdioAccessAllowed, nil
}

func (policy *regoEnforcer) EnforceDeviceUnmountPolicy(ctx context.Context, unmountTarget string) error {
	input := inputData{
		"unmountTarget": unmountTarget,
	}

	_, err := policy.enforce(ctx, "unmount_device", input)
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

func (policy *regoEnforcer) EnforceExecInContainerPolicy(
	ctx context.Context,
	containerID string,
	argList []string,
	envList []string,
	workingDir string,
	noNewPrivileges bool,
	user IDName,
	groups []IDName,
	umask string,
	capabilities *oci.LinuxCapabilities,
) (envToKeep EnvList,
	capsToKeep *oci.LinuxCapabilities,
	stdioAccessAllowed bool,
	err error) {
	opts := &ExecOptions{
		User:            &user,
		Groups:          groups,
		Umask:           umask,
		Capabilities:    capabilities,
		NoNewPrivileges: &noNewPrivileges,
	}
	return policy.EnforceExecInContainerPolicyV2(ctx, containerID, argList, envList, workingDir, opts)
}

func (policy *regoEnforcer) EnforceExecInContainerPolicyV2(
	ctx context.Context,
	containerID string,
	argList []string,
	envList []string,
	workingDir string,
	opts *ExecOptions,
) (envToKeep EnvList,
	capsToKeep *oci.LinuxCapabilities,
	stdioAccessAllowed bool,
	err error) {

	if policy.osType == "linux" && opts.Capabilities == nil {
		return nil, nil, false, errors.New(capabilitiesNilError)
	}

	var input inputData

	switch policy.osType {
	case "linux":
		input = inputData{
			"containerID":     containerID,
			"argList":         argList,
			"envList":         envList,
			"workingDir":      workingDir,
			"noNewPrivileges": opts.NoNewPrivileges,
			"user":            opts.User.toInput(),
			"groups":          groupsToInputs(opts.Groups),
			"umask":           opts.Umask,
			"capabilities":    mapifyCapabilities(opts.Capabilities),
		}
	case "windows":
		input = inputData{
			"containerID": containerID,
			"argList":     argList,
			"envList":     envList,
			"workingDir":  workingDir,
			"user":        opts.User.Name,
		}
	default:
		return nil, nil, false, errors.Errorf("unsupported OS value in options: %q", policy.osType)
	}

	results, err := policy.enforce(ctx, "exec_in_container", input)
	if err != nil {
		return nil, nil, false, err
	}

	envToKeep, err = getEnvsToKeep(envList, results)
	if err != nil {
		return nil, nil, false, err
	}

	if policy.osType == "linux" {
		capsToKeep, err = getCapsToKeep(opts.Capabilities, results)
		if err != nil {
			return nil, nil, false, err
		}
	}
	return envToKeep, capsToKeep, policy.stdio[containerID], nil
}

func (policy *regoEnforcer) EnforceExecExternalProcessPolicy(ctx context.Context, argList []string, envList []string, workingDir string) (toKeep EnvList, stdioAccessAllowed bool, err error) {
	input := map[string]interface{}{
		"argList":    argList,
		"envList":    envList,
		"workingDir": workingDir,
	}

	results, err := policy.enforce(ctx, "exec_external", input)
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

func (policy *regoEnforcer) EnforceShutdownContainerPolicy(ctx context.Context, containerID string) error {
	input := inputData{
		"containerID": containerID,
	}

	_, err := policy.enforce(ctx, "shutdown_container", input)
	return err
}

func (policy *regoEnforcer) EnforceSignalContainerProcessPolicy(ctx context.Context, containerID string, signal syscall.Signal, isInitProcess bool, startupArgList []string) error {
	input := inputData{
		"containerID":   containerID,
		"signal":        signal,
		"isInitProcess": isInitProcess,
		"argList":       startupArgList,
	}

	_, err := policy.enforce(ctx, "signal_container_process", input)
	return err
}

func (policy *regoEnforcer) EnforceSignalContainerProcessPolicyV2(ctx context.Context, containerID string, opts *SignalContainerOptions) error {
	var input inputData

	switch policy.osType {
	case "linux":
		input = inputData{
			"containerID":   containerID,
			"signal":        opts.LinuxSignal,
			"isInitProcess": opts.IsInitProcess,
			"argList":       opts.LinuxStartupArgs,
		}
	case "windows":
		input = inputData{
			"containerID":   containerID,
			"signal":        opts.WindowsSignal,
			"isInitProcess": opts.IsInitProcess,
			"cmdLine":       opts.WindowsCommand,
		}
	default:
		return errors.Errorf("unsupported OS value in options: %q", policy.osType)
	}

	_, err := policy.enforce(ctx, "signal_container_process", input)
	return err
}

func (policy *regoEnforcer) EnforcePlan9MountPolicy(ctx context.Context, target string) error {
	mountPathPrefix := strings.Replace(guestpath.LCOWMountPathPrefixFmt, "%d", "[0-9]+", 1)
	input := inputData{
		"rootPrefix":      guestpath.LCOWRootPrefixInUVM,
		"mountPathPrefix": mountPathPrefix,
		"target":          target,
	}

	_, err := policy.enforce(ctx, "plan9_mount", input)
	return err
}

func (policy *regoEnforcer) EnforcePlan9UnmountPolicy(ctx context.Context, target string) error {
	input := map[string]interface{}{
		"unmountTarget": target,
	}

	_, err := policy.enforce(ctx, "plan9_unmount", input)
	return err
}

func (policy *regoEnforcer) EnforceGetPropertiesPolicy(ctx context.Context) error {
	input := make(inputData)

	_, err := policy.enforce(ctx, "get_properties", input)
	return err
}

func (policy *regoEnforcer) EnforceDumpStacksPolicy(ctx context.Context) error {
	input := make(inputData)

	_, err := policy.enforce(ctx, "dump_stacks", input)
	return err
}

func (policy *regoEnforcer) EnforceRuntimeLoggingPolicy(ctx context.Context) error {
	input := make(inputData)
	_, err := policy.enforce(ctx, "runtime_logging", input)
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

func (policy *regoEnforcer) LoadFragment(ctx context.Context, issuer string, feed string, rego string) error {
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

	results, err := policy.enforce(ctx, "load_fragment", input)

	addModule, _ := results.Bool("add_module")
	if !addModule {
		policy.rego.RemoveModule(fragment.ID())
	}

	return err
}

func (policy *regoEnforcer) EnforceScratchMountPolicy(ctx context.Context, scratchPath string, encrypted bool) error {
	input := map[string]interface{}{
		"target":    scratchPath,
		"encrypted": encrypted,
	}
	_, err := policy.enforce(ctx, "scratch_mount", input)
	if err != nil {
		return err
	}
	return nil
}

func (policy *regoEnforcer) EnforceScratchUnmountPolicy(ctx context.Context, scratchPath string) error {
	input := map[string]interface{}{
		"unmountTarget": scratchPath,
	}
	_, err := policy.enforce(ctx, "scratch_unmount", input)
	if err != nil {
		return err
	}
	return nil
}

func (policy *regoEnforcer) EnforceVerifiedCIMsPolicy(ctx context.Context, containerID string, layerHashes []string) error {
	log.G(ctx).Tracef("Enforcing verified cims in securitypolicy pkg %+v", layerHashes)
	input := inputData{
		"containerID": containerID,
		"layerHashes": layerHashes,
	}

	_, err := policy.enforce(ctx, "mount_cims", input)
	return err
}

func (policy *regoEnforcer) GetUserInfo(containerID string, process *oci.Process) (IDName, []IDName, string, error) {
	return GetAllUserInfo(containerID, process)
}
