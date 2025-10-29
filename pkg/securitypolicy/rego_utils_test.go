//go:build rego
// +build rego

package securitypolicy

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	rpi "github.com/Microsoft/hcsshim/internal/regopolicyinterpreter"
	"github.com/blang/semver/v4"
	"github.com/open-policy-agent/opa/rego"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

const (
	// variables that influence generated rego-only test fixtures
	maxDiffLength                              = 64
	maxExternalProcessesInGeneratedConstraints = 16
	maxFragmentsInGeneratedConstraints         = 4
	maxGeneratedExternalProcesses              = 12
	maxGeneratedSandboxIDLength                = 32
	maxGeneratedEnforcementPointLength         = 64
	maxGeneratedPlan9Mounts                    = 8
	maxGeneratedFragmentFeedLength             = 256
	maxGeneratedFragmentIssuerLength           = 16
	maxPlan9MountTargetLength                  = 64
	maxPlan9MountIndex                         = 16

	// variables that influence generated test fixtures
	minStringLength                           = 10
	maxContainersInGeneratedConstraints       = 32
	maxLayersInGeneratedContainer             = 32
	maxGeneratedContainerID                   = 1000000
	maxGeneratedCommandLength                 = 128
	maxGeneratedCommandArgs                   = 12
	maxGeneratedEnvironmentVariables          = 16
	maxGeneratedEnvironmentVariableRuleLength = 64
	maxGeneratedEnvironmentVariableRules      = 8
	maxGeneratedFragmentNamespaceLength       = 32
	maxGeneratedMountTargetLength             = 256
	maxGeneratedVersion                       = 10
	rootHashLength                            = 64
	maxGeneratedMounts                        = 4
	maxGeneratedMountSourceLength             = 32
	maxGeneratedMountDestinationLength        = 32
	maxGeneratedMountOptions                  = 5
	maxGeneratedMountOptionLength             = 32
	maxGeneratedExecProcesses                 = 4
	maxGeneratedWorkingDirLength              = 128
	maxSignalNumber                           = 64
	maxGeneratedNameLength                    = 8
	maxGeneratedGroupNames                    = 4
	maxGeneratedCapabilities                  = 12
	maxGeneratedCapabilitesLength             = 24
	maxWindowsSignalLength                    = 64
	// additional consts
	// the standard enforcer tests don't do anything with the encoded policy
	// string. this const exists to make that explicit
	ignoredEncodedPolicyString = ""
)

var testRand *rand.Rand
var testDataGenerator *dataGenerator

func init() {
	seed := time.Now().Unix()
	if seedStr, ok := os.LookupEnv("SEED"); ok {
		if parsedSeed, err := strconv.ParseInt(seedStr, 10, 64); err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse seed: %d\n", seed)
		} else {
			seed = parsedSeed
		}
	}
	testRand = rand.New(rand.NewSource(seed))
	fmt.Fprintf(os.Stdout, "securitypolicy_test seed: %d\n", seed)
	testDataGenerator = newDataGenerator(testRand)
}

func Test_RegoTemplates(t *testing.T) {
	query := rego.New(
		rego.Query("data.api"),
		rego.Module("api.rego", APICode))

	ctx := context.Background()
	resultSet, err := query.Eval(ctx)
	if err != nil {
		t.Fatalf("unable to query API enforcement points: %s", err)
	}

	apiRules := resultSet[0].Expressions[0].Value.(map[string]interface{})
	enforcementPoints := apiRules["enforcement_points"].(map[string]interface{})

	policyCode := strings.Replace(policyRegoTemplate, "@@OBJECTS@@", "", 1)
	policyCode = strings.Replace(policyCode, "@@API_VERSION@@", apiVersion, 1)
	policyCode = strings.Replace(policyCode, "@@FRAMEWORK_VERSION@@", frameworkVersion, 1)

	err = verifyPolicyRules(apiVersion, enforcementPoints, policyCode)
	if err != nil {
		t.Errorf("Policy Rego Template is invalid: %s", err)
	}

	err = verifyPolicyRules(apiVersion, enforcementPoints, openDoorRego)
	if err != nil {
		t.Errorf("Open Door Rego Template is invalid: %s", err)
	}
}

func verifyPolicyRules(apiVersion string, enforcementPoints map[string]interface{}, policyCode string) error {
	query := rego.New(
		rego.Query("data.policy"),
		rego.Module("policy.rego", policyCode),
		rego.Module("framework.rego", FrameworkCode),
	)

	ctx := context.Background()
	resultSet, err := query.Eval(ctx)
	if err != nil {
		return fmt.Errorf("unable to query policy template rules: %w", err)
	}

	policyTemplateRules := resultSet[0].Expressions[0].Value.(map[string]interface{})
	policyTemplateAPIVersion := policyTemplateRules["api_version"].(string)

	if policyTemplateAPIVersion != apiVersion {
		return fmt.Errorf("Policy template version != api version: %s != %s", apiVersion, policyTemplateAPIVersion)
	}

	for rule := range enforcementPoints {
		if _, ok := policyTemplateRules[rule]; !ok {
			return fmt.Errorf("Rule %s in API is missing from policy template", rule)
		}
	}

	for rule := range policyTemplateRules {
		if rule == "api_version" || rule == "framework_version" || rule == "reason" {
			continue
		}

		if _, ok := enforcementPoints[rule]; !ok {
			return fmt.Errorf("Rule %s in policy template is missing from API", rule)
		}
	}

	return nil
}

func copyMounts(mounts []oci.Mount) []oci.Mount {
	bytes, err := json.Marshal(mounts)
	if err != nil {
		panic(err)
	}

	mountsCopy := make([]oci.Mount, len(mounts))
	err = json.Unmarshal(bytes, &mountsCopy)
	if err != nil {
		panic(err)
	}

	return mountsCopy
}

func copyMountsInternal(mounts []mountInternal) []mountInternal {
	var mountsCopy []mountInternal

	for _, in := range mounts {
		out := mountInternal{
			Source:      in.Source,
			Destination: in.Destination,
			Type:        in.Type,
			Options:     copyStrings(in.Options),
		}

		mountsCopy = append(mountsCopy, out)
	}

	return mountsCopy
}

func copyLinuxCapabilities(caps oci.LinuxCapabilities) oci.LinuxCapabilities {
	bytes, err := json.Marshal(caps)
	if err != nil {
		panic(err)
	}

	capsCopy := oci.LinuxCapabilities{}
	err = json.Unmarshal(bytes, &capsCopy)
	if err != nil {
		panic(err)
	}

	return capsCopy
}

func copyLinuxSeccomp(seccomp oci.LinuxSeccomp) oci.LinuxSeccomp {
	bytes, err := json.Marshal(seccomp)
	if err != nil {
		panic(err)
	}

	seccompCopy := oci.LinuxSeccomp{}
	err = json.Unmarshal(bytes, &seccompCopy)
	if err != nil {
		panic(err)
	}

	return seccompCopy
}

type regoOverlayTestConfig struct {
	layers      []string
	containerID string
	policy      *regoEnforcer
}

func setupRegoOverlayTest(gc *generatedConstraints, valid bool) (tc *regoOverlayTestConfig, err error) {
	securityPolicy := gc.toPolicy()
	policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{}, testOSType)

	if err != nil {
		return nil, err
	}

	containerID := testDataGenerator.uniqueContainerID()
	c := selectContainerFromContainerList(gc.containers, testRand)

	var layerPaths []string
	if valid {
		layerPaths, err = testDataGenerator.createValidOverlayForContainer(policy, c)
		if err != nil {
			return nil, fmt.Errorf("error creating valid overlay: %w", err)
		}
	} else {
		layerPaths, err = testDataGenerator.createInvalidOverlayForContainer(policy, c)
		if err != nil {
			return nil, fmt.Errorf("error creating invalid overlay: %w", err)
		}
	}

	// see NOTE_TESTCOPY
	return &regoOverlayTestConfig{
		layers:      copyStrings(layerPaths),
		containerID: containerID,
		policy:      policy,
	}, nil
}

func setupPlan9MountTest(gc *generatedConstraints) (tc *regoPlan9MountTestConfig, err error) {
	securityPolicy := gc.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	testContainer := selectContainerFromContainerList(gc.containers, testRand)
	mountIndex := atMost(testRand, int32(len(testContainer.Mounts)-1))
	testMount := &testContainer.Mounts[mountIndex]
	testMount.Source = plan9Prefix
	testMount.Type = "secret"

	policy, err := newRegoPolicy(securityPolicy.marshalRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts),
		testOSType)
	if err != nil {
		return nil, err
	}

	containerID, err := mountImageForContainer(policy, testContainer)
	if err != nil {
		return nil, err
	}

	uvmPathForShare := generateUVMPathForShare(testRand, containerID)

	envList := buildEnvironmentVariablesFromEnvRules(testContainer.EnvRules, testRand)
	sandboxID := testDataGenerator.uniqueSandboxID()

	mounts := testContainer.Mounts
	mounts = append(mounts, defaultMounts...)

	if testContainer.AllowElevated {
		mounts = append(mounts, privilegedMounts...)
	}
	mountSpec := buildMountSpecFromMountArray(mounts, sandboxID, testRand)
	mountSpec.Mounts = append(mountSpec.Mounts, oci.Mount{
		Source:      uvmPathForShare,
		Destination: testMount.Destination,
		Options:     testMount.Options,
		Type:        testMount.Type,
	})

	user := buildIDNameFromConfig(testContainer.User.UserIDName, testRand)
	groups := buildGroupIDNamesFromUser(testContainer.User, testRand)
	umask := testContainer.User.Umask

	capabilities := testContainer.Capabilities.toExternal()
	seccomp := testContainer.SeccompProfileSHA256

	// see NOTE_TESTCOPY
	return &regoPlan9MountTestConfig{
		envList:         copyStrings(envList),
		argList:         copyStrings(testContainer.Command),
		workingDir:      testContainer.WorkingDir,
		containerID:     containerID,
		sandboxID:       sandboxID,
		mounts:          copyMounts(mountSpec.Mounts),
		noNewPrivileges: testContainer.NoNewPrivileges,
		user:            user,
		groups:          groups,
		umask:           umask,
		uvmPathForShare: uvmPathForShare,
		policy:          policy,
		capabilities:    &capabilities,
		seccomp:         seccomp,
	}, nil
}

type regoPlan9MountTestConfig struct {
	envList         []string
	argList         []string
	workingDir      string
	containerID     string
	sandboxID       string
	mounts          []oci.Mount
	uvmPathForShare string
	noNewPrivileges bool
	user            IDName
	groups          []IDName
	umask           string
	policy          *regoEnforcer
	capabilities    *oci.LinuxCapabilities
	seccomp         string
}

func mountImageForContainer(policy *regoEnforcer, container *securityPolicyContainer) (string, error) {
	ctx := context.Background()
	containerID := testDataGenerator.uniqueContainerID()

	layerPaths, err := testDataGenerator.createValidOverlayForContainer(policy, container)
	if err != nil {
		return "", fmt.Errorf("error creating valid overlay: %w", err)
	}

	// see NOTE_TESTCOPY
	err = policy.EnforceOverlayMountPolicy(ctx, containerID, copyStrings(layerPaths), testDataGenerator.uniqueMountTarget())
	if err != nil {
		return "", fmt.Errorf("error mounting filesystem: %w", err)
	}

	return containerID, nil
}

func buildMountSpecFromMountArray(mounts []mountInternal, sandboxID string, r *rand.Rand) *oci.Spec {
	mountSpec := new(oci.Spec)

	// Select some number of the valid, matching rules to be environment
	// variable
	numberOfMounts := int32(len(mounts))
	numberOfMatches := randMinMax(r, 1, numberOfMounts)
	usedIndexes := map[int]struct{}{}
	for numberOfMatches > 0 {
		anIndex := -1
		if (numberOfMatches * 2) > numberOfMounts {
			// if we have a lot of matches, randomly select
			exists := true

			for exists {
				anIndex = int(randMinMax(r, 0, numberOfMounts-1))
				_, exists = usedIndexes[anIndex]
			}
		} else {
			// we have a "smaller set of rules. we'll just iterate and select from
			// available
			exists := true

			for exists {
				anIndex++
				_, exists = usedIndexes[anIndex]
			}
		}

		mount := mounts[anIndex]

		source := substituteUVMPath(sandboxID, mount).Source
		mountSpec.Mounts = append(mountSpec.Mounts, oci.Mount{
			Source:      source,
			Destination: mount.Destination,
			Options:     mount.Options,
			Type:        mount.Type,
		})
		usedIndexes[anIndex] = struct{}{}

		numberOfMatches--
	}

	return mountSpec
}

func selectExecProcess(processes []containerExecProcess, r *rand.Rand) containerExecProcess {
	numProcesses := len(processes)
	return processes[r.Intn(numProcesses)]
}

func selectWindowsExecProcess(processes []windowsContainerExecProcess, r *rand.Rand) windowsContainerExecProcess {
	numProcesses := len(processes)
	return processes[r.Intn(numProcesses)]
}

func selectSignalFromSignals(r *rand.Rand, signals []syscall.Signal) syscall.Signal {
	numSignals := len(signals)
	return signals[r.Intn(numSignals)]
}

func selectSignalFromWindowsSignals(r *rand.Rand, signals []guestrequest.SignalValueWCOW) guestrequest.SignalValueWCOW {
	numSignals := len(signals)
	return signals[r.Intn(numSignals)]
}

func generateUVMPathForShare(r *rand.Rand, containerID string) string {
	return fmt.Sprintf("%s/%s%s",
		guestpath.LCOWRootPrefixInUVM,
		containerID,
		fmt.Sprintf(guestpath.LCOWMountPathPrefixFmt, atMost(r, maxPlan9MountIndex)))
}

func generateLinuxID(r *rand.Rand) uint32 {
	return r.Uint32()
}

type regoScratchMountPolicyTestConfig struct {
	policy *regoEnforcer
}

func setupRegoScratchMountTest(
	gc *generatedConstraints,
	unencryptedScratch bool,
) (tc *regoScratchMountPolicyTestConfig, err error) {
	securityPolicy := gc.toPolicy()
	securityPolicy.AllowUnencryptedScratch = unencryptedScratch

	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)
	policy, err := newRegoPolicy(securityPolicy.marshalRego(), toOCIMounts(defaultMounts), toOCIMounts(privilegedMounts), testOSType)

	if err != nil {
		return nil, err
	}
	return &regoScratchMountPolicyTestConfig{
		policy: policy,
	}, nil
}

func generateCapabilities(r *rand.Rand) *oci.LinuxCapabilities {
	return &oci.LinuxCapabilities{
		Bounding:    generateCapabilitiesSet(r, 0),
		Effective:   generateCapabilitiesSet(r, 0),
		Inheritable: generateCapabilitiesSet(r, 0),
		Permitted:   generateCapabilitiesSet(r, 0),
		Ambient:     generateCapabilitiesSet(r, 0),
	}
}

func alterCapabilitySet(r *rand.Rand, set []string) []string {
	newSet := copyStrings(set)

	if len(newSet) == 0 {
		return generateCapabilitiesSet(r, 1)
	}

	alterations := atLeastNAtMostM(r, 1, 4)
	for i := alterations; i > 0; i-- {
		if len(newSet) == 0 {
			newSet = generateCapabilitiesSet(r, 1)
		} else {
			action := atMost(r, 2)
			if action == 0 {
				newSet = superCapabilitySet(r, newSet)
			} else if action == 1 {
				newSet = subsetCapabilitySet(r, newSet)
			} else {
				replace := atMost(r, int32((len(newSet) - 1)))
				newSet[replace] = generateCapability(r)
			}
		}
	}

	return newSet
}

func subsetCapabilitySet(r *rand.Rand, set []string) []string {
	newSet := make([]string, 0)

	setSize := int32(len(set))
	if setSize == 0 {
		// no subset is possible
		return newSet
	} else if setSize == 1 {
		// only one possibility
		return newSet
	}

	// We need to remove at least 1 item, potentially all
	numberOfMatches := randMinMax(r, 0, setSize-1)
	usedIndexes := map[int]struct{}{}
	for i := numberOfMatches; i > 0; i-- {
		anIndex := -1
		if ((setSize - int32(len(usedIndexes))) * 2) > i {
			// the set is pretty large compared to our number to select,
			// we will gran randomly
			exists := true

			for exists {
				anIndex = int(randMinMax(r, 0, setSize-1))
				_, exists = usedIndexes[anIndex]
			}
		} else {
			// we have a "smaller set of capabilities. we'll just iterate and
			// select from available
			exists := true

			for exists {
				anIndex++
				_, exists = usedIndexes[anIndex]
			}
		}

		newSet = append(newSet, set[anIndex])
		usedIndexes[anIndex] = struct{}{}
	}

	return newSet
}

func superCapabilitySet(r *rand.Rand, set []string) []string {
	newSet := copyStrings(set)

	additions := atLeastNAtMostM(r, 1, 12)
	for i := additions; i > 0; i-- {
		newSet = append(newSet, generateCapability(r))
	}

	return newSet
}

func (c capabilitiesInternal) toExternal() oci.LinuxCapabilities {
	return oci.LinuxCapabilities{
		Bounding:    c.Bounding,
		Effective:   c.Effective,
		Inheritable: c.Inheritable,
		Permitted:   c.Permitted,
		Ambient:     c.Ambient,
	}
}

func buildIDNameFromConfig(config IDNameConfig, r *rand.Rand) IDName {
	switch config.Strategy {
	case IDNameStrategyName:
		return IDName{
			ID:   generateIDNameID(r),
			Name: config.Rule,
		}

	case IDNameStrategyID:
		return IDName{
			ID:   config.Rule,
			Name: generateIDNameName(r),
		}

	case IDNameStrategyAny:
		return generateIDName(r)

	default:
		panic(fmt.Sprintf("unsupported ID Name strategy: %v", config.Strategy))
	}
}

func buildGroupIDNamesFromUser(user UserConfig, r *rand.Rand) []IDName {
	groupIDNames := make([]IDName, 0)

	// Select some number of the valid, matching rules to be groups
	numberOfGroups := int32(len(user.GroupIDNames))
	numberOfMatches := randMinMax(r, 1, numberOfGroups)
	usedIndexes := map[int]struct{}{}
	for numberOfMatches > 0 {
		anIndex := -1
		if (numberOfMatches * 2) > numberOfGroups {
			// if we have a lot of matches, randomly select
			exists := true

			for exists {
				anIndex = int(randMinMax(r, 0, numberOfGroups-1))
				_, exists = usedIndexes[anIndex]
			}
		} else {
			// we have a "smaller set of rules. we'll just iterate and select from
			// available
			exists := true

			for exists {
				anIndex++
				_, exists = usedIndexes[anIndex]
			}
		}

		if user.GroupIDNames[anIndex].Strategy == IDNameStrategyRegex {
			// we don't match from regex groups or any groups
			numberOfMatches--
			continue
		}

		groupIDName := buildIDNameFromConfig(user.GroupIDNames[anIndex], r)
		groupIDNames = append(groupIDNames, groupIDName)
		usedIndexes[anIndex] = struct{}{}

		numberOfMatches--
	}

	return groupIDNames
}

func generateIDNameName(r *rand.Rand) string {
	return randVariableString(r, maxGeneratedNameLength)
}

func generateIDNameID(r *rand.Rand) string {
	id := r.Uint32()
	return strconv.FormatUint(uint64(id), 10)
}

func generateIDName(r *rand.Rand) IDName {
	return IDName{
		ID:   generateIDNameID(r),
		Name: generateIDNameName(r),
	}
}

func toOCIMounts(mounts []mountInternal) []oci.Mount {
	result := make([]oci.Mount, len(mounts))
	for i, mount := range mounts {
		result[i] = oci.Mount{
			Source:      mount.Source,
			Destination: mount.Destination,
			Options:     mount.Options,
			Type:        mount.Type,
		}
	}
	return result
}

func setupExternalProcessTest(gc *generatedConstraints) (tc *regoExternalPolicyTestConfig, err error) {
	gc.externalProcesses = generateExternalProcesses(testRand)
	securityPolicy := gc.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	policy, err := newRegoPolicy(securityPolicy.marshalRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts),
		testOSType)
	if err != nil {
		return nil, err
	}

	return &regoExternalPolicyTestConfig{
		policy: policy,
	}, nil
}

func setupWindowsExternalProcessTest(gc *generatedWindowsConstraints) (tc *regoExternalPolicyTestConfig, err error) {
	gc.externalProcesses = generateExternalProcesses(testRand)
	securityPolicy := gc.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	policy, err := newRegoPolicy(securityPolicy.marshalWindowsRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts),
		testOSType)
	if err != nil {
		return nil, err
	}

	return &regoExternalPolicyTestConfig{
		policy: policy,
	}, nil
}

type regoExternalPolicyTestConfig struct {
	policy *regoEnforcer
}

func setupGetPropertiesTest(gc *generatedConstraints, allowPropertiesAccess bool) (tc *regoGetPropertiesTestConfig, err error) {
	gc.allowGetProperties = allowPropertiesAccess

	securityPolicy := gc.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	policy, err := newRegoPolicy(securityPolicy.marshalRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts),
		testOSType)
	if err != nil {
		return nil, err
	}

	return &regoGetPropertiesTestConfig{
		policy: policy,
	}, nil
}

func setupGetPropertiesTestWindows(gc *generatedWindowsConstraints, allowPropertiesAccess bool) (tc *regoGetPropertiesTestConfig, err error) {
	gc.allowGetProperties = allowPropertiesAccess

	securityPolicy := gc.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	policy, err := newRegoPolicy(securityPolicy.marshalWindowsRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts),
		testOSType)
	if err != nil {
		return nil, err
	}

	return &regoGetPropertiesTestConfig{
		policy: policy,
	}, nil
}

type regoGetPropertiesTestConfig struct {
	policy *regoEnforcer
}

func setupDumpStacksTest(constraints *generatedConstraints, allowDumpStacks bool) (tc *regoGetPropertiesTestConfig, err error) {
	constraints.allowDumpStacks = allowDumpStacks

	securityPolicy := constraints.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	policy, err := newRegoPolicy(securityPolicy.marshalRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts),
		testOSType)
	if err != nil {
		return nil, err
	}

	return &regoGetPropertiesTestConfig{
		policy: policy,
	}, nil
}

func setupDumpStacksTestWindows(constraints *generatedWindowsConstraints, allowDumpStacks bool) (tc *regoGetPropertiesTestConfig, err error) {
	constraints.allowDumpStacks = allowDumpStacks

	securityPolicy := constraints.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	policy, err := newRegoPolicy(securityPolicy.marshalWindowsRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts),
		testOSType)
	if err != nil {
		return nil, err
	}

	return &regoGetPropertiesTestConfig{
		policy: policy,
	}, nil
}

type regoDumpStacksTestConfig struct {
	policy *regoEnforcer
}

type regoPolicyOnlyTestConfig struct {
	policy *regoEnforcer
}

func setupRegoPolicyOnlyTest(gc *generatedConstraints) (tc *regoPolicyOnlyTestConfig, err error) {
	securityPolicy := gc.toPolicy()
	policy, err := newRegoPolicy(securityPolicy.marshalRego(), []oci.Mount{}, []oci.Mount{}, testOSType)

	if err != nil {
		return nil, err
	}

	// see NOTE_TESTCOPY
	return &regoPolicyOnlyTestConfig{
		policy: policy,
	}, nil
}

type regoFragmentTestConfig struct {
	fragments         []*regoFragment
	containers        []*regoFragmentContainer
	externalProcesses []*externalProcess
	subFragments      []*regoFragment
	plan9Mounts       []string
	mountSpec         []string
	policy            *regoEnforcer
}

type regoFragmentContainer struct {
	container    *securityPolicyContainer
	envList      []string
	sandboxID    string
	mounts       []oci.Mount
	user         IDName
	groups       []IDName
	capabilities *oci.LinuxCapabilities
	seccomp      string
}

// Fragment tests set up for Linux
func setupSimpleRegoFragmentTestConfig(gc *generatedConstraints) (*regoFragmentTestConfig, error) {
	return setupRegoFragmentTestConfig(gc, 1, []string{"containers"}, []string{}, false, false, false, false)
}

func setupRegoFragmentTestConfigWithIncludes(gc *generatedConstraints, includes []string) (*regoFragmentTestConfig, error) {
	return setupRegoFragmentTestConfig(gc, 1, includes, []string{}, false, false, false, false)
}

func setupRegoFragmentTestConfigWithExcludes(gc *generatedConstraints, excludes []string) (*regoFragmentTestConfig, error) {
	return setupRegoFragmentTestConfig(gc, 1, []string{}, excludes, false, false, false, false)
}

func setupRegoFragmentSVNErrorTestConfig(gc *generatedConstraints) (*regoFragmentTestConfig, error) {
	return setupRegoFragmentTestConfig(gc, 1, []string{"containers"}, []string{}, true, false, false, false)
}

func setupRegoSubfragmentSVNErrorTestConfig(gc *generatedConstraints) (*regoFragmentTestConfig, error) {
	return setupRegoFragmentTestConfig(gc, 1, []string{"fragments"}, []string{}, true, false, false, false)
}

func setupRegoFragmentTwoFeedTestConfig(gc *generatedConstraints, sameIssuer bool, sameFeed bool) (*regoFragmentTestConfig, error) {
	return setupRegoFragmentTestConfig(gc, 2, []string{"containers"}, []string{}, false, sameIssuer, sameFeed, false)
}

func setupRegoFragmentSVNMismatchTestConfig(gc *generatedConstraints) (*regoFragmentTestConfig, error) {
	return setupRegoFragmentTestConfig(gc, 2, []string{"containers"}, []string{}, false, false, false, true)
}

func setupRegoFragmentTestConfig(gc *generatedConstraints, numFragments int, includes []string, excludes []string, svnError bool, sameIssuer bool, sameFeed bool, svnMismatch bool) (tc *regoFragmentTestConfig, err error) {
	gc.fragments = generateFragments(testRand, int32(numFragments))

	if sameIssuer {
		for _, fragment := range gc.fragments {
			fragment.issuer = gc.fragments[0].issuer
			if sameFeed {
				fragment.feed = gc.fragments[0].feed
			}
		}
	}

	subSVNError := svnError
	if len(includes) > 0 && includes[0] == "fragments" {
		svnError = false
	}
	fragments := selectFragmentsFromConstraints(gc, numFragments, includes, excludes, svnError, frameworkVersion, svnMismatch)

	containers := make([]*regoFragmentContainer, numFragments)
	subFragments := make([]*regoFragment, numFragments)
	externalProcesses := make([]*externalProcess, numFragments)
	plan9Mounts := make([]string, numFragments)
	for i, fragment := range fragments {
		container := fragment.selectContainer()

		envList := buildEnvironmentVariablesFromEnvRules(container.EnvRules, testRand)
		sandboxID := testDataGenerator.uniqueSandboxID()
		user := buildIDNameFromConfig(container.User.UserIDName, testRand)
		groups := buildGroupIDNamesFromUser(container.User, testRand)
		capabilities := copyLinuxCapabilities(container.Capabilities.toExternal())
		seccomp := container.SeccompProfileSHA256

		mounts := container.Mounts
		mountSpec := buildMountSpecFromMountArray(mounts, sandboxID, testRand)
		containers[i] = &regoFragmentContainer{
			container:    container,
			envList:      envList,
			sandboxID:    sandboxID,
			mounts:       mountSpec.Mounts,
			user:         user,
			groups:       groups,
			capabilities: &capabilities,
			seccomp:      seccomp,
		}

		for _, include := range fragment.info.includes {
			switch include {
			case "fragments":
				subFragments[i] = selectFragmentsFromConstraints(fragment.constraints, 1, []string{"containers"}, []string{}, subSVNError, frameworkVersion, false)[0]
				break

			case "external_processes":
				externalProcesses[i] = selectExternalProcessFromConstraints(fragment.constraints, testRand)
				break
			}
		}

		// now that we've explicitly added the excluded items to the fragment
		// we remove the include string so that the generated policy
		// does not include them.
		fragment.info.includes = removeStringsFromArray(fragment.info.includes, excludes)

		code := fragment.constraints.toFragment().marshalRego()
		fragment.code = setFrameworkVersion(code, frameworkVersion)
	}

	if sameFeed {
		includeSet := make(map[string]bool)
		minSVN := strconv.Itoa(maxGeneratedVersion)
		for _, fragment := range gc.fragments {
			svn := fragment.minimumSVN
			if compareSVNs(svn, minSVN) < 0 {
				minSVN = svn
			}
			for _, include := range fragment.includes {
				includeSet[include] = true
			}
		}
		frag := gc.fragments[0]
		frag.minimumSVN = minSVN
		frag.includes = make([]string, 0, len(includeSet))
		for include := range includeSet {
			frag.includes = append(frag.includes, include)
		}

		gc.fragments = []*fragment{frag}

	}

	securityPolicy := gc.toPolicy()
	defaultMounts := toOCIMounts(generateMounts(testRand))
	privilegedMounts := toOCIMounts(generateMounts(testRand))
	policy, err := newRegoPolicy(securityPolicy.marshalRego(), defaultMounts, privilegedMounts, testOSType)

	if err != nil {
		return nil, err
	}

	return &regoFragmentTestConfig{
		fragments:         fragments,
		containers:        containers,
		subFragments:      subFragments,
		externalProcesses: externalProcesses,
		plan9Mounts:       plan9Mounts,
		policy:            policy,
	}, nil
}

func compareSVNs(lhs string, rhs string) int {
	lhs_int, err := strconv.Atoi(lhs)
	if err == nil {
		rhs_int, err := strconv.Atoi(rhs)
		if err == nil {
			return lhs_int - rhs_int
		}
	}

	panic("unable to compare SVNs")
}

type regoDropEnvsTestConfig struct {
	envList      []string
	expected     []string
	argList      []string
	workingDir   string
	containerID  string
	sandboxID    string
	mounts       []oci.Mount
	policy       *regoEnforcer
	capabilities oci.LinuxCapabilities
}

func setupEnvRuleSets(count int) [][]EnvRuleConfig {
	numEnvRules := []int{int(randMinMax(testRand, 1, 4)),
		int(randMinMax(testRand, 1, 4)),
		int(randMinMax(testRand, 1, 4))}
	envRuleLookup := make(stringSet)
	envRules := make([][]EnvRuleConfig, count)

	for i := 0; i < count; i++ {
		rules := envRuleLookup.randUniqueArray(testRand, func(r *rand.Rand) string {
			return randVariableString(r, 10)
		}, int32(numEnvRules[i]))

		envRules[i] = make([]EnvRuleConfig, numEnvRules[i])
		for j, rule := range rules {
			envRules[i][j] = EnvRuleConfig{
				Strategy: "string",
				Rule:     rule,
			}
		}
	}

	return envRules
}

func setupRegoDropEnvsTest(disjoint bool) (*regoContainerTestConfig, error) {
	gc := generateConstraints(testRand, 1)
	gc.allowEnvironmentVariableDropping = true

	const numContainers int = 3
	envRules := setupEnvRuleSets(numContainers)
	containers := make([]*securityPolicyContainer, numContainers)
	envs := make([][]string, numContainers)

	for i := 0; i < numContainers; i++ {
		c, err := gc.containers[0].clone()
		if err != nil {
			return nil, err
		}
		containers[i] = c
		envs[i] = buildEnvironmentVariablesFromEnvRules(envRules[i], testRand)
		if i == 0 {
			c.EnvRules = envRules[i]
		} else if disjoint {
			c.EnvRules = append(envRules[0], envRules[i]...)
		} else {
			c.EnvRules = append(containers[i-1].EnvRules, envRules[i]...)
		}
	}

	gc.containers = containers
	securityPolicy := gc.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	policy, err := newRegoPolicy(securityPolicy.marshalRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts),
		testOSType)

	if err != nil {
		return nil, err
	}

	containerIDs := make([]string, numContainers)
	for i, c := range gc.containers {
		containerID, err := mountImageForContainer(policy, c)
		if err != nil {
			return nil, err
		}

		containerIDs[i] = containerID
	}

	var envList []string
	if disjoint {
		var extraLen int
		if len(envs[1]) < len(envs[2]) {
			extraLen = len(envs[1])
		} else {
			extraLen = len(envs[2])
		}
		envList = append(envs[0], envs[1][:extraLen]...)
		envList = append(envList, envs[2][:extraLen]...)
	} else {
		envList = append(envs[0], envs[1]...)
		envList = append(envList, envs[2]...)
	}

	user := buildIDNameFromConfig(containers[2].User.UserIDName, testRand)
	groups := buildGroupIDNamesFromUser(containers[2].User, testRand)
	umask := containers[2].User.Umask

	sandboxID := testDataGenerator.uniqueSandboxID()

	mounts := containers[2].Mounts
	mounts = append(mounts, defaultMounts...)
	if containers[2].AllowElevated {
		mounts = append(mounts, privilegedMounts...)
	}

	mountSpec := buildMountSpecFromMountArray(mounts, sandboxID, testRand)
	capabilities := copyLinuxCapabilities(containers[2].Capabilities.toExternal())
	seccomp := containers[2].SeccompProfileSHA256

	// see NOTE_TESTCOPY
	return &regoContainerTestConfig{
		envList:         copyStrings(envList),
		argList:         copyStrings(containers[2].Command),
		workingDir:      containers[2].WorkingDir,
		containerID:     containerIDs[2],
		sandboxID:       sandboxID,
		mounts:          copyMounts(mountSpec.Mounts),
		noNewPrivileges: containers[2].NoNewPrivileges,
		user:            user,
		groups:          groups,
		umask:           umask,
		policy:          policy,
		capabilities:    &capabilities,
		seccomp:         seccomp,
		ctx:             gc.ctx,
	}, nil
}

func setupRegoDropEnvsTestWindows(disjoint bool) (*regoContainerTestConfig, error) {
	gc := generateWindowsConstraints(testRand, 1)
	gc.allowEnvironmentVariableDropping = true

	const numContainers int = 3
	envRules := setupEnvRuleSets(numContainers)
	containers := make([]*securityPolicyWindowsContainer, numContainers)
	envs := make([][]string, numContainers)

	for i := 0; i < numContainers; i++ {
		c, err := gc.containers[0].clone()
		if err != nil {
			return nil, err
		}
		containers[i] = c
		envs[i] = buildEnvironmentVariablesFromEnvRules(envRules[i], testRand)
		if i == 0 {
			c.EnvRules = envRules[i]
		} else if disjoint {
			c.EnvRules = append(envRules[0], envRules[i]...)
		} else {
			c.EnvRules = append(containers[i-1].EnvRules, envRules[i]...)
		}
	}

	gc.containers = containers
	securityPolicy := gc.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	policy, err := newRegoPolicy(securityPolicy.marshalWindowsRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts),
		testOSType)

	if err != nil {
		return nil, err
	}

	containerIDs := make([]string, numContainers)
	for i, c := range gc.containers {
		containerID, err := mountImageForWindowsContainer(policy, c)
		if err != nil {
			return nil, err
		}

		containerIDs[i] = containerID
	}

	var envList []string
	if disjoint {
		var extraLen int
		if len(envs[1]) < len(envs[2]) {
			extraLen = len(envs[1])
		} else {
			extraLen = len(envs[2])
		}
		envList = append(envs[0], envs[1][:extraLen]...)
		envList = append(envList, envs[2][:extraLen]...)
	} else {
		envList = append(envs[0], envs[1]...)
		envList = append(envList, envs[2]...)
	}

	// Handle Windows user configuration
	user := IDName{}
	if containers[2].User != "" {
		user = IDName{Name: containers[2].User}
	} else {
		user = IDName{Name: generateIDNameName(testRand)}
	}

	sandboxID := testDataGenerator.uniqueSandboxID()

	// see NOTE_TESTCOPY
	return &regoContainerTestConfig{
		envList:         copyStrings(envList),
		argList:         copyStrings(containers[2].Command),
		workingDir:      containers[2].WorkingDir,
		containerID:     containerIDs[2],
		sandboxID:       sandboxID,
		mounts:          []oci.Mount{},
		noNewPrivileges: false,
		user:            user,
		groups:          []IDName{},
		umask:           "",
		capabilities:    nil,
		seccomp:         "",
		policy:          policy,
		ctx:             gc.ctx,
	}, nil
}

type regoFrameworkVersionTestConfig struct {
	policy    *regoEnforcer
	fragments []*regoFragment
}

func setFrameworkVersion(code string, version string) string {
	template := `framework_version := "%s"`
	old := fmt.Sprintf(template, frameworkVersion)
	if version == "" {
		return strings.Replace(code, old, "", 1)
	}

	new := fmt.Sprintf(template, version)
	return strings.Replace(code, old, new, 1)
}

func setupFrameworkVersionSimpleTest(gc *generatedConstraints, policyVersion string, version string) (*regoFrameworkVersionTestConfig, error) {
	return setupFrameworkVersionTest(gc, policyVersion, version, 0, "", []string{})
}

func setupFrameworkVersionTest(gc *generatedConstraints, policyVersion string, version string, numFragments int, fragmentVersion string, includes []string) (*regoFrameworkVersionTestConfig, error) {
	fragments := make([]*regoFragment, 0, numFragments)
	if numFragments > 0 {
		gc.fragments = generateFragments(testRand, int32(numFragments))
		fragments = selectFragmentsFromConstraints(gc, numFragments, includes, []string{}, false, fragmentVersion, false)
	}

	securityPolicy := gc.toPolicy()
	policy, err := newRegoPolicy(setFrameworkVersion(securityPolicy.marshalRego(), policyVersion), []oci.Mount{}, []oci.Mount{}, testOSType)

	if err != nil {
		return nil, err
	}

	code := strings.Replace(frameworkCodeTemplate, "@@FRAMEWORK_VERSION@@", version, 1)
	policy.rego.RemoveModule("framework.rego")
	policy.rego.AddModule("framework.rego", &rpi.RegoModule{Namespace: "framework", Code: code})
	err = policy.rego.Compile()
	if err != nil {
		return nil, err
	}

	return &regoFrameworkVersionTestConfig{policy: policy, fragments: fragments}, nil
}

type regoFragment struct {
	info        *fragment
	constraints *generatedConstraints
	code        string
}

func (f *regoFragment) selectContainer() *securityPolicyContainer {
	return selectContainerFromContainerList(f.constraints.containers, testRand)
}

func mustIncrementSVN(svn string) string {
	svn_semver, err := semver.Parse(svn)

	if err == nil {
		svn_semver.IncrementMajor()
		return svn_semver.String()
	}

	svn_int, err := strconv.Atoi(svn)

	if err == nil {
		return strconv.Itoa(svn_int + 1)
	}

	panic("Could not increment SVN")
}

func selectFragmentsFromConstraints(gc *generatedConstraints, numFragments int, includes []string, excludes []string, svnError bool, frameworkVersion string, svnMismatch bool) []*regoFragment {
	choices := randChoices(testRand, numFragments, len(gc.fragments))
	fragments := make([]*regoFragment, numFragments)
	for i, choice := range choices {
		config := gc.fragments[choice]
		config.includes = addStringsToArray(config.includes, includes)
		// since we want to test that the policy cannot include an excluded
		// quantity, we must first ensure they are in the fragment
		config.includes = addStringsToArray(config.includes, excludes)

		constraints := generateConstraints(testRand, maxContainersInGeneratedConstraints)
		for _, include := range config.includes {
			switch include {
			case "fragments":
				constraints.fragments = generateFragments(testRand, 1)
				for _, fragment := range constraints.fragments {
					fragment.includes = addStringsToArray(fragment.includes, []string{"containers"})
				}
				break

			case "external_processes":
				constraints.externalProcesses = generateExternalProcesses(testRand)
				break
			}
		}

		svn := config.minimumSVN
		if svnMismatch {
			if randBool(testRand) {
				svn = generateSemver(testRand)
			} else {
				config.minimumSVN = generateSemver(testRand)
			}
		}

		constraints.svn = svn
		if svnError {
			config.minimumSVN = mustIncrementSVN(config.minimumSVN)
		}

		code := constraints.toFragment().marshalRego()
		code = setFrameworkVersion(code, frameworkVersion)

		fragments[i] = &regoFragment{
			info:        config,
			constraints: constraints,
			code:        code,
		}
	}

	return fragments
}

func generateSandboxID(r *rand.Rand) string {
	return randVariableString(r, maxGeneratedSandboxIDLength)
}

func generateEnforcementPoint(r *rand.Rand) string {
	first := randChar(r)
	return first + randString(r, atMost(r, maxGeneratedEnforcementPointLength))
}

func (gen *dataGenerator) uniqueSandboxID() string {
	return gen.sandboxIDs.randUnique(gen.rng, generateSandboxID)
}

func (gen *dataGenerator) uniqueEnforcementPoint() string {
	return gen.enforcementPoints.randUnique(gen.rng, generateEnforcementPoint)
}

type regoContainerTestConfig struct {
	envList         []string
	argList         []string
	workingDir      string
	containerID     string
	sandboxID       string
	mounts          []oci.Mount
	noNewPrivileges bool
	user            IDName
	groups          []IDName
	umask           string
	capabilities    *oci.LinuxCapabilities
	seccomp         string
	policy          *regoEnforcer
	ctx             context.Context
}

func setupSimpleRegoCreateContainerTest(gc *generatedConstraints) (tc *regoContainerTestConfig, err error) {
	c := selectContainerFromContainerList(gc.containers, testRand)
	return setupRegoCreateContainerTest(gc, c, false)
}

func setupRegoPrivilegedMountTest(gc *generatedConstraints) (tc *regoContainerTestConfig, err error) {
	c := selectContainerFromContainerList(gc.containers, testRand)
	return setupRegoCreateContainerTest(gc, c, true)
}

func setupRegoCreateContainerTest(gc *generatedConstraints, testContainer *securityPolicyContainer, privilegedError bool) (tc *regoContainerTestConfig, err error) {
	securityPolicy := gc.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	policy, err := newRegoPolicy(securityPolicy.marshalRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts),
		testOSType)
	if err != nil {
		return nil, err
	}

	containerID, err := mountImageForContainer(policy, testContainer)
	if err != nil {
		return nil, err
	}

	envList := buildEnvironmentVariablesFromEnvRules(testContainer.EnvRules, testRand)
	sandboxID := testDataGenerator.uniqueSandboxID()

	mounts := testContainer.Mounts
	mounts = append(mounts, defaultMounts...)
	if privilegedError {
		testContainer.AllowElevated = false
	}

	if testContainer.AllowElevated || privilegedError {
		mounts = append(mounts, privilegedMounts...)
	}
	mountSpec := buildMountSpecFromMountArray(mounts, sandboxID, testRand)

	// Handle user configuration based on OS type
	user := IDName{}
	if testContainer.User.UserIDName.Strategy != IDNameStrategyRegex {
		user = buildIDNameFromConfig(testContainer.User.UserIDName, testRand)
	}
	groups := buildGroupIDNamesFromUser(testContainer.User, testRand)
	umask := testContainer.User.Umask

	var capabilities *oci.LinuxCapabilities
	if testContainer.Capabilities != nil {
		capsExternal := copyLinuxCapabilities(testContainer.Capabilities.toExternal())
		capabilities = &capsExternal
	} else {
		capabilities = nil
	}

	seccomp := testContainer.SeccompProfileSHA256

	// Return full config for Linux
	return &regoContainerTestConfig{
		envList:         copyStrings(envList),
		argList:         copyStrings(testContainer.Command),
		workingDir:      testContainer.WorkingDir,
		containerID:     containerID,
		sandboxID:       sandboxID,
		mounts:          copyMounts(mountSpec.Mounts),
		noNewPrivileges: testContainer.NoNewPrivileges,
		user:            user,
		groups:          groups,
		umask:           umask,
		capabilities:    capabilities,
		seccomp:         seccomp,
		policy:          policy,
		ctx:             gc.ctx,
	}, nil

}

func setupRegoRunningContainerTest(gc *generatedConstraints, privileged bool) (tc *regoRunningContainerTestConfig, err error) {
	securityPolicy := gc.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	policy, err := newRegoPolicy(securityPolicy.marshalRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts),
		testOSType)
	if err != nil {
		return nil, err
	}

	var runningContainers []regoRunningContainer
	numOfRunningContainers := int(atLeastOneAtMost(testRand, int32(len(gc.containers))))
	containersToRun := randChoicesWithReplacement(testRand, numOfRunningContainers, len(gc.containers))
	for _, i := range containersToRun {
		containerToStart := gc.containers[i]
		r, err := runContainer(policy, containerToStart, defaultMounts, privilegedMounts, privileged)
		if err != nil {
			return nil, err
		}
		runningContainers = append(runningContainers, *r)
	}

	return &regoRunningContainerTestConfig{
		runningContainers: runningContainers,
		policy:            policy,
		defaultMounts:     copyMountsInternal(defaultMounts),
		privilegedMounts:  copyMountsInternal(privilegedMounts),
	}, nil
}

func runContainer(enforcer *regoEnforcer, container *securityPolicyContainer, defaultMounts []mountInternal, privilegedMounts []mountInternal, privileged bool) (*regoRunningContainer, error) {
	ctx := context.Background()
	containerID, err := mountImageForContainer(enforcer, container)
	if err != nil {
		return nil, err
	}

	envList := buildEnvironmentVariablesFromEnvRules(container.EnvRules, testRand)
	user := buildIDNameFromConfig(container.User.UserIDName, testRand)
	groups := buildGroupIDNamesFromUser(container.User, testRand)
	umask := container.User.Umask
	sandboxID := generateSandboxID(testRand)

	mounts := container.Mounts
	mounts = append(mounts, defaultMounts...)
	if container.AllowElevated {
		mounts = append(mounts, privilegedMounts...)
	}
	mountSpec := buildMountSpecFromMountArray(mounts, sandboxID, testRand)
	var capabilities oci.LinuxCapabilities
	if container.Capabilities == nil {
		if privileged {
			capabilities = capabilitiesInternal{
				Bounding:    DefaultPrivilegedCapabilities(),
				Inheritable: DefaultPrivilegedCapabilities(),
				Effective:   DefaultPrivilegedCapabilities(),
				Permitted:   DefaultPrivilegedCapabilities(),
				Ambient:     []string{},
			}.toExternal()
		} else {
			capabilities = capabilitiesInternal{
				Bounding:    DefaultUnprivilegedCapabilities(),
				Inheritable: []string{},
				Effective:   DefaultUnprivilegedCapabilities(),
				Permitted:   DefaultUnprivilegedCapabilities(),
				Ambient:     []string{},
			}.toExternal()
		}
	} else {
		capabilities = container.Capabilities.toExternal()
	}
	seccomp := container.SeccompProfileSHA256

	_, _, _, err = enforcer.EnforceCreateContainerPolicy(ctx, sandboxID, containerID, container.Command, envList, container.WorkingDir, mountSpec.Mounts, privileged, container.NoNewPrivileges, user, groups, umask, &capabilities, seccomp)
	if err != nil {
		return nil, err
	}

	return &regoRunningContainer{
		container:   container,
		envList:     envList,
		containerID: containerID,
	}, nil
}

type regoRunningContainerTestConfig struct {
	runningContainers []regoRunningContainer
	policy            *regoEnforcer
	defaultMounts     []mountInternal
	privilegedMounts  []mountInternal
}

type regoRunningContainer struct {
	container        *securityPolicyContainer
	windowsContainer *securityPolicyWindowsContainer
	envList          []string
	containerID      string
}

func setupRegoRunningWindowsContainerTest(gc *generatedWindowsConstraints) (tc *regoRunningContainerTestConfig, err error) {
	securityPolicy := gc.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	//fmt.Printf("Generated Rego policy:\n%s\n", securityPolicy.marshalWindowsRego())

	policy, err := newRegoPolicy(securityPolicy.marshalWindowsRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts),
		testOSType)
	if err != nil {
		return nil, err
	}

	var runningContainers []regoRunningContainer
	numOfRunningContainers := int(atLeastOneAtMost(testRand, int32(len(gc.containers))))
	containersToRun := randChoicesWithReplacement(testRand, numOfRunningContainers, len(gc.containers))
	for _, i := range containersToRun {
		containerToStart := gc.containers[i]
		r, err := runWindowsContainer(policy, containerToStart)
		if err != nil {
			return nil, err
		}
		runningContainers = append(runningContainers, *r)
	}

	return &regoRunningContainerTestConfig{
		runningContainers: runningContainers,
		policy:            policy,
		defaultMounts:     copyMountsInternal(defaultMounts),
		privilegedMounts:  copyMountsInternal(privilegedMounts),
	}, nil
}

func runWindowsContainer(enforcer *regoEnforcer, container *securityPolicyWindowsContainer) (*regoRunningContainer, error) {
	ctx := context.Background()
	containerID, err := mountImageForWindowsContainer(enforcer, container)
	if err != nil {
		return nil, err
	}

	envList := buildEnvironmentVariablesFromEnvRules(container.EnvRules, testRand)
	user := IDName{Name: container.User}

	_, _, _, err = enforcer.EnforceCreateContainerPolicyV2(ctx, containerID, container.Command, envList, container.WorkingDir, nil, user, nil)

	if err != nil {
		return nil, err
	}

	return &regoRunningContainer{
		windowsContainer: container,
		envList:          envList,
		containerID:      containerID,
	}, nil
}

func copyStrings(values []string) []string {
	valuesCopy := make([]string, len(values))
	copy(valuesCopy, values)
	return valuesCopy
}

//go:embed api_test.rego
var apiTestCode string

func (p *regoEnforcer) injectTestAPI() error {
	p.rego.RemoveModule("api.rego")
	p.rego.AddModule("api.rego", &rpi.RegoModule{Namespace: "api", Code: apiTestCode})

	return p.rego.Compile()
}

func selectContainerFromRunningContainers(containers []regoRunningContainer, r *rand.Rand) regoRunningContainer {
	numContainers := len(containers)
	return containers[r.Intn(numContainers)]
}

func idForRunningContainer(container *securityPolicyContainer, running []regoRunningContainer) (string, error) {
	for _, c := range running {
		if c.container == container {
			return c.containerID, nil
		}
	}

	return "", errors.New("Container isn't running")
}

func idForRunningWindowsContainer(container *securityPolicyWindowsContainer, running []regoRunningContainer) (string, error) {
	for _, c := range running {
		if c.windowsContainer == container {
			return c.containerID, nil
		}
	}

	return "", errors.New("Container isn't running")
}

func generateFragments(r *rand.Rand, minFragments int32) []*fragment {
	numFragments := randMinMax(r, minFragments, maxFragmentsInGeneratedConstraints)

	fragments := make([]*fragment, numFragments)
	for i := 0; i < int(numFragments); i++ {
		fragments[i] = generateFragment(r)
	}

	return fragments
}

func generateFragmentIssuer(r *rand.Rand) string {
	return randString(r, maxGeneratedFragmentIssuerLength)
}

func generateFragmentFeed(r *rand.Rand) string {
	return randString(r, maxGeneratedFragmentFeedLength)
}

func (gen *dataGenerator) uniqueFragmentNamespace() string {
	return gen.fragmentNamespaces.randUnique(gen.rng, generateFragmentNamespace)
}

func (gen *dataGenerator) uniqueFragmentIssuer() string {
	return gen.fragmentIssuers.randUnique(gen.rng, generateFragmentIssuer)
}

func (gen *dataGenerator) uniqueFragmentFeed() string {
	return gen.fragmentFeeds.randUnique(gen.rng, generateFragmentFeed)
}

func generateFragment(r *rand.Rand) *fragment {
	possibleIncludes := []string{"containers", "fragments", "external_processes"}
	numChoices := int(atLeastOneAtMost(r, int32(len(possibleIncludes))))
	includes := randChooseStrings(r, possibleIncludes, numChoices)
	return &fragment{
		issuer:     testDataGenerator.uniqueFragmentIssuer(),
		feed:       testDataGenerator.uniqueFragmentFeed(),
		minimumSVN: generateSVN(r),
		includes:   includes,
	}
}

func addStringsToArray(values []string, valuesToAdd []string) []string {
	toAdd := []string{}
	for _, valueToAdd := range valuesToAdd {
		add := true
		for _, value := range values {
			if value == valueToAdd {
				add = false
				break
			}
		}
		if add {
			toAdd = append(toAdd, valueToAdd)
		}
	}

	return append(values, toAdd...)
}

func removeStringsFromArray(values []string, valuesToRemove []string) []string {
	remain := make([]string, 0, len(values))
	for _, value := range values {
		keep := true
		for _, toRemove := range valuesToRemove {
			if value == toRemove {
				keep = false
				break
			}
		}
		if keep {
			remain = append(remain, value)
		}
	}

	return remain
}

func areStringArraysEqual(lhs []string, rhs []string) bool {
	if len(lhs) != len(rhs) {
		return false
	}

	sort.Strings(lhs)
	sort.Strings(rhs)

	for i, a := range lhs {
		if a != rhs[i] {
			return false
		}
	}

	return true
}

func (c securityPolicyContainer) clone() (*securityPolicyContainer, error) {
	contents, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}

	var clone securityPolicyContainer
	err = json.Unmarshal(contents, &clone)
	if err != nil {
		return nil, err
	}

	return &clone, nil
}

func (c securityPolicyWindowsContainer) clone() (*securityPolicyWindowsContainer, error) {
	contents, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}

	var clone securityPolicyWindowsContainer
	err = json.Unmarshal(contents, &clone)
	if err != nil {
		return nil, err
	}

	return &clone, nil
}

func (p externalProcess) clone() *externalProcess {
	envRules := make([]EnvRuleConfig, len(p.envRules))
	copy(envRules, p.envRules)

	return &externalProcess{
		command:          copyStrings(p.command),
		envRules:         envRules,
		workingDir:       p.workingDir,
		allowStdioAccess: p.allowStdioAccess,
	}
}

func (p containerExecProcess) clone() containerExecProcess {
	return containerExecProcess{
		Command: copyStrings(p.Command),
		Signals: p.Signals,
	}
}

func (c *securityPolicyContainer) toContainer() *Container {
	execProcesses := make([]ExecProcessConfig, len(c.ExecProcesses))
	for i, ep := range c.ExecProcesses {
		execProcesses[i] = ExecProcessConfig(ep)
	}

	capabilities := CapabilitiesConfig{
		Bounding:    c.Capabilities.Bounding,
		Effective:   c.Capabilities.Effective,
		Inheritable: c.Capabilities.Inheritable,
		Permitted:   c.Capabilities.Permitted,
		Ambient:     c.Capabilities.Ambient,
	}

	return &Container{
		Command:              CommandArgs(stringArrayToStringMap(c.Command)),
		EnvRules:             envRuleArrayToEnvRules(c.EnvRules),
		Layers:               Layers(stringArrayToStringMap(c.Layers)),
		WorkingDir:           c.WorkingDir,
		Mounts:               mountArrayToMounts(c.Mounts),
		AllowElevated:        c.AllowElevated,
		ExecProcesses:        execProcesses,
		Signals:              c.Signals,
		AllowStdioAccess:     c.AllowStdioAccess,
		NoNewPrivileges:      c.NoNewPrivileges,
		User:                 c.User,
		Capabilities:         &capabilities,
		SeccompProfileSHA256: c.SeccompProfileSHA256,
	}
}

func (c *securityPolicyWindowsContainer) toWindowsContainer() *WindowsContainer {
	execProcesses := make([]WindowsExecProcessConfig, len(c.ExecProcesses))
	for i, ep := range c.ExecProcesses {
		execProcesses[i] = WindowsExecProcessConfig(ep)
	}

	return &WindowsContainer{
		Command:       CommandArgs(stringArrayToStringMap(c.Command)),
		EnvRules:      envRuleArrayToEnvRules(c.EnvRules),
		Layers:        Layers(stringArrayToStringMap(c.Layers)),
		WorkingDir:    c.WorkingDir,
		ExecProcesses: execProcesses,
		Signals:       c.Signals,
		User:          c.User,
	}
}

func envRuleArrayToEnvRules(envRules []EnvRuleConfig) EnvRules {
	elements := make(map[string]EnvRuleConfig)
	for i, envRule := range envRules {
		elements[strconv.Itoa(i)] = envRule
	}
	return EnvRules{
		Elements: elements,
		Length:   len(envRules),
	}
}

func mountArrayToMounts(mounts []mountInternal) Mounts {
	elements := make(map[string]Mount)
	for i, mount := range mounts {
		elements[strconv.Itoa(i)] = Mount{
			Source:      mount.Source,
			Destination: mount.Destination,
			Type:        mount.Type,
			Options:     Options(stringArrayToStringMap(mount.Options)),
		}
	}

	return Mounts{
		Elements: elements,
		Length:   len(mounts),
	}
}

func (p externalProcess) toConfig() ExternalProcessConfig {
	return ExternalProcessConfig{
		Command:          p.command,
		WorkingDir:       p.workingDir,
		AllowStdioAccess: p.allowStdioAccess,
	}
}

func (f fragment) toConfig() FragmentConfig {
	return FragmentConfig{
		Issuer:     f.issuer,
		Feed:       f.feed,
		MinimumSVN: f.minimumSVN,
		Includes:   f.includes,
	}
}

func stringArrayToStringMap(values []string) StringArrayMap {
	elements := make(map[string]string)
	for i, value := range values {
		elements[strconv.Itoa(i)] = value
	}

	return StringArrayMap{
		Elements: elements,
		Length:   len(values),
	}
}

func (s *stringSet) randUniqueArray(r *rand.Rand, generator func(*rand.Rand) string, numItems int32) []string {
	items := make([]string, numItems)
	for i := 0; i < int(numItems); i++ {
		items[i] = s.randUnique(r, generator)
	}
	return items
}

func generateExternalProcesses(r *rand.Rand) []*externalProcess {
	var processes []*externalProcess

	numProcesses := atLeastOneAtMost(r, maxExternalProcessesInGeneratedConstraints)
	for i := 0; i < int(numProcesses); i++ {
		processes = append(processes, generateExternalProcess(r))
	}

	return processes
}

func generateExternalProcess(r *rand.Rand) *externalProcess {
	return &externalProcess{
		command:          generateCommand(r),
		envRules:         generateEnvironmentVariableRules(r),
		workingDir:       generateWorkingDir(r),
		allowStdioAccess: randBool(r),
	}
}

func randChoices(r *rand.Rand, numChoices int, numItems int) []int {
	shuffle := r.Perm(numItems)
	if numChoices > numItems {
		return shuffle
	}

	return shuffle[:numChoices]
}

func randChoicesWithReplacement(r *rand.Rand, numChoices int, numItems int) []int {
	choices := make([]int, numChoices)
	for i := 0; i < numChoices; i++ {
		choices[i] = r.Intn(numItems)
	}

	return choices
}

func randChooseStrings(r *rand.Rand, items []string, numChoices int) []string {
	numItems := len(items)
	choiceIndices := randChoices(r, numChoices, numItems)
	choices := make([]string, numChoices)
	for i, index := range choiceIndices {
		choices[i] = items[index]
	}
	return choices
}

func randChooseStringsWithReplacement(r *rand.Rand, items []string, numChoices int) []string {
	numItems := len(items)
	choiceIndices := randChoicesWithReplacement(r, numChoices, numItems)
	choices := make([]string, numChoices)
	for i, index := range choiceIndices {
		choices[i] = items[index]
	}
	return choices
}

func selectExternalProcessFromConstraints(constraints *generatedConstraints, r *rand.Rand) *externalProcess {
	numberOfProcessesInConstraints := len(constraints.externalProcesses)
	return constraints.externalProcesses[r.Intn(numberOfProcessesInConstraints)]
}

func selectWindowsExternalProcessFromConstraints(constraints *generatedWindowsConstraints, r *rand.Rand) *externalProcess {
	numberOfProcessesInConstraints := len(constraints.externalProcesses)
	return constraints.externalProcesses[r.Intn(numberOfProcessesInConstraints)]
}

func (constraints *generatedConstraints) toPolicy() *securityPolicyInternal {
	return &securityPolicyInternal{
		Containers:                       constraints.containers,
		ExternalProcesses:                constraints.externalProcesses,
		Fragments:                        constraints.fragments,
		AllowPropertiesAccess:            constraints.allowGetProperties,
		AllowDumpStacks:                  constraints.allowDumpStacks,
		AllowRuntimeLogging:              constraints.allowRuntimeLogging,
		AllowEnvironmentVariableDropping: constraints.allowEnvironmentVariableDropping,
		AllowUnencryptedScratch:          constraints.allowUnencryptedScratch,
		AllowCapabilityDropping:          constraints.allowCapabilityDropping,
	}
}

func (constraints *generatedConstraints) toFragment() *securityPolicyFragment {
	return &securityPolicyFragment{
		Namespace:         constraints.namespace,
		SVN:               constraints.svn,
		Containers:        constraints.containers,
		ExternalProcesses: constraints.externalProcesses,
		Fragments:         constraints.fragments,
	}
}

func generateSemver(r *rand.Rand) string {
	major := randMinMax(r, 0, maxGeneratedVersion)
	minor := randMinMax(r, 0, maxGeneratedVersion)
	patch := randMinMax(r, 0, maxGeneratedVersion)
	return fmt.Sprintf("%d.%d.%d", major, minor, patch)
}

func assertKeyValue(object map[string]interface{}, key string, expectedValue interface{}) error {
	if actualValue, ok := object[key]; ok {
		if actualValue != expectedValue {
			return fmt.Errorf("incorrect value for no_new_privileges: %t != %t (expected)", actualValue, expectedValue)
		}
	} else {
		return fmt.Errorf("missing value for %s", key)
	}

	return nil
}

func assertDecisionJSONContains(t *testing.T, err error, expectedValues ...string) bool {
	if err == nil {
		t.Errorf("expected error to contain %v but got nil", expectedValues)
		return false
	}

	policyDecision, err := ExtractPolicyDecision(err.Error())
	if err != nil {
		t.Errorf("unable to extract policy decision from error: %v", err)
		return false
	}

	for _, expected := range expectedValues {
		if !strings.Contains(policyDecision, expected) {
			t.Errorf("expected error to contain %q", expected)
			return false
		}
	}

	return true
}

func assertDecisionJSONDoesNotContain(t *testing.T, err error, expectedValues ...string) bool {
	if err == nil {
		t.Errorf("expected error to contain %v but got nil", expectedValues)
		return false
	}

	policyDecision, err := ExtractPolicyDecision(err.Error())
	if err != nil {
		t.Errorf("unable to extract policy decision from error: %v", err)
		return false
	}

	for _, expected := range expectedValues {
		if strings.Contains(policyDecision, expected) {
			t.Errorf("expected error to not contain %q", expected)
			return false
		}
	}

	return true
}

// Windows-specific container selection function
func selectWindowsContainerFromContainerList(containers []*securityPolicyWindowsContainer, r *rand.Rand) *securityPolicyWindowsContainer {
	return containers[r.Intn(len(containers))]
}

// Windows-specific simple setup function
func setupSimpleRegoCreateContainerTestWindows(gc *generatedWindowsConstraints) (tc *regoContainerTestConfig, err error) {
	c := selectWindowsContainerFromContainerList(gc.containers, testRand)
	return setupRegoCreateContainerTestWindows(gc, c, false)
}

// Windows-specific container test setup
func setupRegoCreateContainerTestWindows(gc *generatedWindowsConstraints, testContainer *securityPolicyWindowsContainer, privilegedError bool) (tc *regoContainerTestConfig, err error) {
	securityPolicy := gc.toPolicy()
	defaultMounts := generateMounts(testRand)
	privilegedMounts := generateMounts(testRand)

	policy, err := newRegoPolicy(securityPolicy.marshalWindowsRego(),
		toOCIMounts(defaultMounts),
		toOCIMounts(privilegedMounts),
		testOSType)
	if err != nil {
		return nil, err
	}

	// Debug: print the OS type being used
	//fmt.Printf("OS type being used: %s\n", testOSType)

	// Debug: print the generated Rego policy
	//fmt.Printf("Generated Rego policy:\n%s\n", securityPolicy.marshalWindowsRego())

	containerID, err := mountImageForWindowsContainer(policy, testContainer)
	if err != nil {
		return nil, err
	}

	envList := buildEnvironmentVariablesFromEnvRules(testContainer.EnvRules, testRand)
	sandboxID := testDataGenerator.uniqueSandboxID()

	// Handle Windows user configuration
	user := IDName{}
	if testContainer.User != "" {
		user = IDName{Name: testContainer.User}
	} else {
		user = IDName{Name: generateIDNameName(testRand)}
	}

	return &regoContainerTestConfig{
		envList:         copyStrings(envList),
		argList:         copyStrings(testContainer.Command),
		workingDir:      testContainer.WorkingDir,
		containerID:     containerID,
		sandboxID:       sandboxID,
		mounts:          []oci.Mount{},
		noNewPrivileges: false,
		user:            user,
		groups:          []IDName{},
		umask:           "",
		capabilities:    nil,
		seccomp:         "",
		policy:          policy,
		ctx:             gc.ctx,
	}, nil
}

//nolint:unused
func mountImageForWindowsContainer(policy *regoEnforcer, container *securityPolicyWindowsContainer) (string, error) {
	ctx := context.Background()
	containerID := testDataGenerator.uniqueContainerID()

	// For Windows containers, we need to mount using CIMFS (container image mount)
	// The layerHashes_ok function expects hashes in reverse order compared to how they're stored
	layerHashes := make([]string, len(container.Layers))
	for i, layer := range container.Layers {
		// Reverse the order: last layer becomes first in the input
		layerHashes[len(container.Layers)-1-i] = layer
	}

	// Mount the CIMFS for the Windows container
	err := policy.EnforceVerifiedCIMsPolicy(ctx, containerID, layerHashes)
	if err != nil {
		return "", fmt.Errorf("error mounting CIMFS: %w", err)
	}

	//fmt.Printf("CIMFS mounted successfully for container %s with layers %v\n", containerID, layerHashes)

	return containerID, nil
}

//
// Setup and "fixtures" follow...
//

func (*SecurityPolicy) Generate(r *rand.Rand, _ int) reflect.Value {
	// This fixture setup is used from 1 test. Given the limited scope it is
	// used from, all functionality is in this single function. That saves having
	// confusing fixture name functions where we have generate* for both internal
	// and external versions
	p := &SecurityPolicy{
		Containers: Containers{
			Elements: map[string]Container{},
		},
	}
	p.AllowAll = false
	numContainers := int(atLeastOneAtMost(r, maxContainersInGeneratedConstraints))
	for i := 0; i < numContainers; i++ {
		c := Container{
			Command: CommandArgs{
				Elements: map[string]string{},
			},
			EnvRules: EnvRules{
				Elements: map[string]EnvRuleConfig{},
			},
			Layers: Layers{
				Elements: map[string]string{},
			},
		}

		// command
		numArgs := int(atLeastOneAtMost(r, maxGeneratedCommandArgs))
		for j := 0; j < numArgs; j++ {
			c.Command.Elements[strconv.Itoa(j)] = randVariableString(r, maxGeneratedCommandLength)
		}
		c.Command.Length = numArgs

		// layers
		numLayers := int(atLeastOneAtMost(r, maxLayersInGeneratedContainer))
		for j := 0; j < numLayers; j++ {
			c.Layers.Elements[strconv.Itoa(j)] = generateRootHash(r)
		}
		c.Layers.Length = numLayers

		// env variable rules
		numEnvRules := int(atMost(r, maxGeneratedEnvironmentVariableRules))
		for j := 0; j < numEnvRules; j++ {
			rule := EnvRuleConfig{
				Strategy: "string",
				Rule:     randVariableString(r, maxGeneratedEnvironmentVariableRuleLength),
				Required: false,
			}
			c.EnvRules.Elements[strconv.Itoa(j)] = rule
		}
		c.EnvRules.Length = numEnvRules

		p.Containers.Elements[strconv.Itoa(i)] = c
	}

	p.Containers.Length = numContainers

	return reflect.ValueOf(p)
}

func (*generatedConstraints) Generate(r *rand.Rand, _ int) reflect.Value {
	//c := generateConstraints(r, maxContainersInGeneratedConstraints)
	c := generateConstraints(r, maxContainersInGeneratedConstraints)
	return reflect.ValueOf(c)
}

func generateConstraints(r *rand.Rand, maxContainers int32) *generatedConstraints {
	var containers []*securityPolicyContainer

	numContainers := (int)(atLeastOneAtMost(r, maxContainers))
	if testOSType == "windows" {
		// Windows containers
		//for i := 0; i < numContainers; i++ {
		//		containers = append(containers, generateConstraintsWindowsContainer(r, 1, 5))
		//	}
	} else if testOSType == "linux" {
		// Linux containers
		for i := 0; i < numContainers; i++ {
			containers = append(containers, generateConstraintsContainer(r, 1, maxLayersInGeneratedContainer))
		}
	}

	return &generatedConstraints{
		containers:                       containers,
		externalProcesses:                make([]*externalProcess, 0),
		fragments:                        make([]*fragment, 0),
		allowGetProperties:               randBool(r),
		allowDumpStacks:                  randBool(r),
		allowRuntimeLogging:              false,
		allowEnvironmentVariableDropping: false,
		allowUnencryptedScratch:          randBool(r),
		namespace:                        generateFragmentNamespace(testRand),
		svn:                              generateSVN(testRand),
		allowCapabilityDropping:          false,
		ctx:                              context.Background(),
	}
}

func generateConstraintsContainer(r *rand.Rand, minNumberOfLayers, maxNumberOfLayers int32) *securityPolicyContainer {
	c := securityPolicyContainer{}
	p := generateContainerInitProcess(r)
	c.Command = p.Command
	c.EnvRules = p.EnvRules
	c.WorkingDir = p.WorkingDir
	c.Mounts = generateMounts(r)
	numLayers := int(atLeastNAtMostM(r, minNumberOfLayers, maxNumberOfLayers))
	for i := 0; i < numLayers; i++ {
		c.Layers = append(c.Layers, generateRootHash(r))
	}
	c.ExecProcesses = generateExecProcesses(r)
	c.Signals = generateListOfSignals(r, 0, maxSignalNumber)
	c.AllowStdioAccess = randBool(r)
	c.NoNewPrivileges = randBool(r)
	c.User = generateUser(r)
	c.Capabilities = generateInternalCapabilities(r)
	c.SeccompProfileSHA256 = generateSeccomp(r)

	return &c
}

func generateConstraintsWindowsContainer(r *rand.Rand, minNumberOfLayers, maxNumberOfLayers int32) *securityPolicyWindowsContainer {
	c := securityPolicyWindowsContainer{}
	p := generateContainerInitProcess(r)
	c.Command = p.Command
	c.EnvRules = p.EnvRules
	c.WorkingDir = p.WorkingDir
	numLayers := int(atLeastNAtMostM(r, minNumberOfLayers, maxNumberOfLayers))
	for i := 0; i < numLayers; i++ {
		c.Layers = append(c.Layers, generateRootHash(r))
	}
	c.ExecProcesses = generateWindowsExecProcesses(r)
	c.Signals = generateWindowsSignals(r)
	c.AllowStdioAccess = randBool(r)
	c.User = generateWindowsUser(r)

	return &c
}

func generateSeccomp(r *rand.Rand) string {
	if randBool(r) {
		// 50% chance of no seccomp profile
		return ""
	}

	return generateRootHash(r)
}

func generateInternalCapabilities(r *rand.Rand) *capabilitiesInternal {
	return &capabilitiesInternal{
		Bounding:    generateCapabilitiesSet(r, 0),
		Effective:   generateCapabilitiesSet(r, 0),
		Inheritable: generateCapabilitiesSet(r, 0),
		Permitted:   generateCapabilitiesSet(r, 0),
		Ambient:     generateCapabilitiesSet(r, 0),
	}
}

func generateCapabilitiesSet(r *rand.Rand, minSize int32) []string {
	capabilities := make([]string, 0)

	numArgs := atLeastNAtMostM(r, minSize, maxGeneratedCapabilities)
	for i := 0; i < int(numArgs); i++ {
		capabilities = append(capabilities, generateCapability(r))
	}

	return capabilities
}

func generateCapability(r *rand.Rand) string {
	return randVariableString(r, maxGeneratedCapabilitesLength)
}

func generateContainerInitProcess(r *rand.Rand) containerInitProcess {
	return containerInitProcess{
		Command:          generateCommand(r),
		EnvRules:         generateEnvironmentVariableRules(r),
		WorkingDir:       generateWorkingDir(r),
		AllowStdioAccess: randBool(r),
	}
}

func generateContainerExecProcess(r *rand.Rand) containerExecProcess {
	return containerExecProcess{
		Command: generateCommand(r),
		Signals: generateListOfSignals(r, 0, maxSignalNumber),
	}
}

func generateWindowsContainerExecProcess(r *rand.Rand) windowsContainerExecProcess {
	return windowsContainerExecProcess{
		Command: generateWindowsUser(r),
		Signals: generateWindowsSignals(r),
	}
}

func generateRootHash(r *rand.Rand) string {
	return randString(r, rootHashLength)
}

func generateWorkingDir(r *rand.Rand) string {
	return randVariableString(r, maxGeneratedWorkingDirLength)
}

func generateWindowsUser(r *rand.Rand) string {
	return randVariableString(r, maxGeneratedWorkingDirLength)
}

func generateCommand(r *rand.Rand) []string {
	var args []string

	numArgs := atLeastOneAtMost(r, maxGeneratedCommandArgs)
	for i := 0; i < int(numArgs); i++ {
		args = append(args, randVariableString(r, maxGeneratedCommandLength))
	}

	return args
}

func generateWindowsSignals(r *rand.Rand) []guestrequest.SignalValueWCOW {
	var args []guestrequest.SignalValueWCOW

	numArgs := atLeastOneAtMost(r, maxGeneratedCommandArgs)
	for i := 0; i < int(numArgs); i++ {
		var str string = randVariableString(r, maxGeneratedCommandLength)
		var sig guestrequest.SignalValueWCOW = guestrequest.SignalValueWCOW(str)
		args = append(args, sig)
	}

	return args
}

func generateEnvironmentVariableRules(r *rand.Rand) []EnvRuleConfig {
	var rules []EnvRuleConfig

	numArgs := atLeastOneAtMost(r, maxGeneratedEnvironmentVariableRules)
	for i := 0; i < int(numArgs); i++ {
		rule := EnvRuleConfig{
			Strategy: "string",
			Rule:     randVariableString(r, maxGeneratedEnvironmentVariableRuleLength),
		}
		rules = append(rules, rule)
	}

	return rules
}

func generateExecProcesses(r *rand.Rand) []containerExecProcess {
	var processes []containerExecProcess

	numProcesses := atLeastOneAtMost(r, maxGeneratedExecProcesses)

	for i := 0; i < int(numProcesses); i++ {
		processes = append(processes, generateContainerExecProcess(r))
	}

	return processes
}

func generateWindowsExecProcesses(r *rand.Rand) []windowsContainerExecProcess {
	var processes []windowsContainerExecProcess

	numProcesses := atLeastOneAtMost(r, maxGeneratedExecProcesses)
	for i := 0; i < int(numProcesses); i++ {
		processes = append(processes, generateWindowsContainerExecProcess(r))
	}

	return processes
}

func generateUmask(r *rand.Rand) string {
	// we are generating values from 000 to 777 as decimal values
	// to ensure we cover the full range of umask values
	// and so the resulting string will be a 4 digit octal representation
	// even though we are using decimal values
	return fmt.Sprintf("%04d", randMinMax(r, 0, 777))
}

func generateIDNameConfig(r *rand.Rand) IDNameConfig {
	strategies := []IDNameStrategy{IDNameStrategyName, IDNameStrategyID}
	strategy := strategies[randMinMax(r, 0, int32(len(strategies)-1))]
	switch strategy {
	case IDNameStrategyName:
		return IDNameConfig{
			Strategy: IDNameStrategyName,
			Rule:     randVariableString(r, maxGeneratedNameLength),
		}

	case IDNameStrategyID:
		return IDNameConfig{
			Strategy: IDNameStrategyID,
			Rule:     fmt.Sprintf("%d", r.Uint32()),
		}
	}
	panic("unreachable")
}

func generateUser(r *rand.Rand) UserConfig {
	numGroups := int(atLeastOneAtMost(r, maxGeneratedGroupNames))
	groupIDs := make([]IDNameConfig, numGroups)
	for i := 0; i < numGroups; i++ {
		groupIDs[i] = generateIDNameConfig(r)
	}

	return UserConfig{
		UserIDName:   generateIDNameConfig(r),
		GroupIDNames: groupIDs,
		Umask:        generateUmask(r),
	}
}

func generateEnvironmentVariables(r *rand.Rand) []string {
	var envVars []string

	numVars := atLeastOneAtMost(r, maxGeneratedEnvironmentVariables)
	for i := 0; i < int(numVars); i++ {
		variable := randVariableString(r, maxGeneratedEnvironmentVariableRuleLength)
		envVars = append(envVars, variable)
	}

	return envVars
}

func generateNeverMatchingEnvironmentVariable(r *rand.Rand) string {
	return randString(r, maxGeneratedEnvironmentVariableRuleLength+1)
}

func buildEnvironmentVariablesFromEnvRules(rules []EnvRuleConfig, r *rand.Rand) []string {
	vars := make([]string, 0)

	// Select some number of the valid, matching rules to be environment
	// variable
	numberOfRules := int32(len(rules))
	if numberOfRules == 0 {
		return vars
	}
	numberOfMatches := randMinMax(r, 1, numberOfRules)

	// Build in all required rules, this isn't a setup method of "missing item"
	// tests
	for _, rule := range rules {

		if rule.Required {
			if rule.Strategy != EnvVarRuleRegex {
				vars = append(vars, rule.Rule)
			}
			numberOfMatches--
		}
	}

	usedIndexes := map[int]struct{}{}
	for numberOfMatches > 0 {
		anIndex := -1
		if (numberOfMatches * 2) > numberOfRules {
			// if we have a lot of matches, randomly select
			exists := true

			for exists {
				anIndex = int(randMinMax(r, 0, numberOfRules-1))
				_, exists = usedIndexes[anIndex]
			}
		} else {
			// we have a "smaller set of rules. we'll just iterate and select from
			// available
			exists := true

			for exists {
				anIndex++
				_, exists = usedIndexes[anIndex]
			}
		}

		// include it if it's not regex
		if rules[anIndex].Strategy != EnvVarRuleRegex {
			vars = append(vars, rules[anIndex].Rule)
			usedIndexes[anIndex] = struct{}{}
		}
		numberOfMatches--

	}

	return vars
}

func generateMountTarget(r *rand.Rand) string {
	return randVariableString(r, maxGeneratedMountTargetLength)
}

func generateInvalidRootHash(r *rand.Rand) string {
	// Guaranteed to be an incorrect size as it maxes out in size at one less
	// than the correct length. If this ever creates a hash that passes, we
	// have a seriously weird bug
	return randVariableString(r, rootHashLength-1)
}

func generateFragmentNamespace(r *rand.Rand) string {
	return randChar(r) + randVariableString(r, maxGeneratedFragmentNamespaceLength)
}

func generateSVN(r *rand.Rand) string {
	return strconv.FormatInt(int64(randMinMax(r, 0, maxGeneratedVersion)), 10)
}

func selectRootHashFromConstraints(constraints *generatedConstraints, r *rand.Rand) string {
	numberOfContainersInConstraints := len(constraints.containers)
	container := constraints.containers[r.Intn(numberOfContainersInConstraints)]
	numberOfLayersInContainer := len(container.Layers)

	return container.Layers[r.Intn(numberOfLayersInContainer)]
}

func generateContainerID(r *rand.Rand) string {
	id := atLeastOneAtMost(r, maxGeneratedContainerID)
	return strconv.FormatInt(int64(id), 10)
}

func generateMounts(r *rand.Rand) []mountInternal {
	numberOfMounts := atLeastOneAtMost(r, maxGeneratedMounts)
	mounts := make([]mountInternal, numberOfMounts)

	for i := 0; i < int(numberOfMounts); i++ {
		numberOfOptions := atLeastOneAtMost(r, maxGeneratedMountOptions)
		options := make([]string, numberOfOptions)
		for j := 0; j < int(numberOfOptions); j++ {
			options[j] = randVariableString(r, maxGeneratedMountOptionLength)
		}

		sourcePrefix := ""
		// select a "source type". our default is "no special prefix" ie a
		// "standard source".
		prefixType := randMinMax(r, 1, 3)
		switch prefixType {
		case 2:
			// sandbox mount, gets special handling
			sourcePrefix = guestpath.SandboxMountPrefix
		case 3:
			// huge page mount, gets special handling
			sourcePrefix = guestpath.HugePagesMountPrefix
		}

		source := path.Join(sourcePrefix, randVariableString(r, maxGeneratedMountSourceLength))
		destination := randVariableString(r, maxGeneratedMountDestinationLength)

		mounts[i] = mountInternal{
			Source:      source,
			Destination: destination,
			Options:     options,
			Type:        "bind",
		}
	}

	return mounts
}

func generateListOfSignals(r *rand.Rand, atLeast int32, atMost int32) []syscall.Signal {
	numSignals := int(atLeastNAtMostM(r, atLeast, atMost))
	signalSet := make(map[syscall.Signal]struct{})

	for i := 0; i < numSignals; i++ {
		signal := generateSignal(r)
		signalSet[signal] = struct{}{}
	}

	var signals []syscall.Signal
	for k := range signalSet {
		signals = append(signals, k)
	}

	return signals
}

func generateListOfWindowsSignals(r *rand.Rand, atLeast int32, atMost int32) []guestrequest.SignalValueWCOW {
	numSignals := int(atLeastNAtMostM(r, atLeast, atMost))
	signalSet := make(map[string]struct{})

	for i := 0; i < numSignals; i++ {
		signal := randVariableString(r, maxWindowsSignalLength)
		signalSet[signal] = struct{}{}
	}

	var signals []guestrequest.SignalValueWCOW
	for k := range signalSet {
		signals = append(signals, guestrequest.SignalValueWCOW(k))
	}

	return signals
}

func generateWindowsSignal(r *rand.Rand) guestrequest.SignalValueWCOW {
	return guestrequest.SignalValueWCOW(randVariableString(r, maxWindowsSignalLength))
}
func generateSignal(r *rand.Rand) syscall.Signal {
	return syscall.Signal(atLeastOneAtMost(r, maxSignalNumber))
}

func selectContainerFromContainerList(containers []*securityPolicyContainer, r *rand.Rand) *securityPolicyContainer {
	if len(containers) == 0 {
		panic("selectContainerFromContainerList: no containers available to select from")
	}
	return containers[r.Intn(len(containers))]
}

type dataGenerator struct {
	rng                *rand.Rand
	mountTargets       stringSet
	containerIDs       stringSet
	sandboxIDs         stringSet
	enforcementPoints  stringSet
	fragmentIssuers    stringSet
	fragmentFeeds      stringSet
	fragmentNamespaces stringSet
}

func newDataGenerator(rng *rand.Rand) *dataGenerator {
	return &dataGenerator{
		rng:                rng,
		mountTargets:       make(stringSet),
		containerIDs:       make(stringSet),
		sandboxIDs:         make(stringSet),
		enforcementPoints:  make(stringSet),
		fragmentIssuers:    make(stringSet),
		fragmentFeeds:      make(stringSet),
		fragmentNamespaces: make(stringSet),
	}
}

func (s *stringSet) randUnique(r *rand.Rand, generator func(*rand.Rand) string) string {
	for {
		item := generator(r)
		if !s.contains(item) {
			s.add(item)
			return item
		}
	}
}

func (gen *dataGenerator) uniqueMountTarget() string {
	return gen.mountTargets.randUnique(gen.rng, generateMountTarget)
}

func (gen *dataGenerator) uniqueContainerID() string {
	return gen.containerIDs.randUnique(gen.rng, generateContainerID)
}

func (gen *dataGenerator) createValidOverlayForContainer(enforcer SecurityPolicyEnforcer, container *securityPolicyContainer) ([]string, error) {
	ctx := context.Background()
	// storage for our mount paths
	overlay := make([]string, len(container.Layers))

	for i := 0; i < len(container.Layers); i++ {
		mount := gen.uniqueMountTarget()
		err := enforcer.EnforceDeviceMountPolicy(ctx, mount, container.Layers[i])
		if err != nil {
			return overlay, err
		}

		overlay[len(overlay)-i-1] = mount
	}

	return overlay, nil
}

func (gen *dataGenerator) createInvalidOverlayForContainer(enforcer SecurityPolicyEnforcer, container *securityPolicyContainer) ([]string, error) {
	method := gen.rng.Intn(3)
	switch method {
	case 0:
		return gen.invalidOverlaySameSizeWrongMounts(enforcer, container)
	case 1:
		return gen.invalidOverlayCorrectDevicesWrongOrderSomeMissing(enforcer, container)
	default:
		return gen.invalidOverlayRandomJunk(enforcer, container)
	}
}

func (gen *dataGenerator) invalidOverlaySameSizeWrongMounts(enforcer SecurityPolicyEnforcer, container *securityPolicyContainer) ([]string, error) {
	ctx := context.Background()
	// storage for our mount paths
	overlay := make([]string, len(container.Layers))

	for i := 0; i < len(container.Layers); i++ {
		mount := gen.uniqueMountTarget()
		err := enforcer.EnforceDeviceMountPolicy(ctx, mount, container.Layers[i])
		if err != nil {
			return overlay, err
		}

		// generate a random new mount point to cause an error
		overlay[len(overlay)-i-1] = gen.uniqueMountTarget()
	}

	return overlay, nil
}

func (gen *dataGenerator) invalidOverlayCorrectDevicesWrongOrderSomeMissing(enforcer SecurityPolicyEnforcer, container *securityPolicyContainer) ([]string, error) {
	ctx := context.Background()
	if len(container.Layers) == 1 {
		// won't work with only 1, we need to bail out to another method
		return gen.invalidOverlayRandomJunk(enforcer, container)
	}
	// storage for our mount paths
	var overlay []string

	for i := 0; i < len(container.Layers); i++ {
		mount := gen.uniqueMountTarget()
		err := enforcer.EnforceDeviceMountPolicy(ctx, mount, container.Layers[i])
		if err != nil {
			return overlay, err
		}

		if gen.rng.Intn(10) != 0 {
			overlay = append(overlay, mount)
		}
	}

	return overlay, nil
}

func (gen *dataGenerator) invalidOverlayRandomJunk(enforcer SecurityPolicyEnforcer, container *securityPolicyContainer) ([]string, error) {
	ctx := context.Background()
	// create "junk" for entry
	layersToCreate := gen.rng.Int31n(maxLayersInGeneratedContainer)
	overlay := make([]string, layersToCreate)

	for i := 0; i < int(layersToCreate); i++ {
		overlay[i] = gen.uniqueMountTarget()
	}

	// setup entirely different and "correct" expected mounting
	for i := 0; i < len(container.Layers); i++ {
		mount := gen.uniqueMountTarget()
		err := enforcer.EnforceDeviceMountPolicy(ctx, mount, container.Layers[i])
		if err != nil {
			return overlay, err
		}
	}

	return overlay, nil
}

func randVariableString(r *rand.Rand, maxLen int32) string {
	return randString(r, atLeastOneAtMost(r, maxLen))
}

func randString(r *rand.Rand, length int32) string {
	if length < minStringLength {
		length = minStringLength
	}
	charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	sb := strings.Builder{}
	sb.Grow(int(length))
	for i := 0; i < (int)(length); i++ {
		sb.WriteByte(charset[r.Intn(len(charset))])
	}

	return sb.String()
}

func randChar(r *rand.Rand) string {
	charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	return string(charset[r.Intn(len(charset))])
}

func randBool(r *rand.Rand) bool {
	return randMinMax(r, 0, 1) == 0
}

func randMinMax(r *rand.Rand, min int32, max int32) int32 {
	return r.Int31n(max-min+1) + min
}

func atLeastNAtMostM(r *rand.Rand, min, max int32) int32 {
	return randMinMax(r, min, max)
}

func atLeastOneAtMost(r *rand.Rand, most int32) int32 {
	return atLeastNAtMostM(r, 1, most)
}

func atMost(r *rand.Rand, most int32) int32 {
	return randMinMax(r, 0, most)
}

type generatedConstraints struct {
	containers                       []*securityPolicyContainer
	externalProcesses                []*externalProcess
	fragments                        []*fragment
	allowGetProperties               bool
	allowDumpStacks                  bool
	allowRuntimeLogging              bool
	allowEnvironmentVariableDropping bool
	allowUnencryptedScratch          bool
	namespace                        string
	svn                              string
	allowCapabilityDropping          bool
	ctx                              context.Context
}

type generatedWindowsConstraints struct {
	containers                       []*securityPolicyWindowsContainer
	externalProcesses                []*externalProcess
	fragments                        []*fragment
	allowGetProperties               bool
	allowDumpStacks                  bool
	allowRuntimeLogging              bool
	allowEnvironmentVariableDropping bool
	allowUnencryptedScratch          bool
	namespace                        string
	svn                              string
	allowCapabilityDropping          bool
	ctx                              context.Context
}

func (constraints *generatedWindowsConstraints) toPolicy() *securityPolicyWindowsInternal {
	return &securityPolicyWindowsInternal{
		Containers:                       constraints.containers,
		ExternalProcesses:                constraints.externalProcesses,
		Fragments:                        constraints.fragments,
		AllowPropertiesAccess:            constraints.allowGetProperties,
		AllowDumpStacks:                  constraints.allowDumpStacks,
		AllowRuntimeLogging:              constraints.allowRuntimeLogging,
		AllowEnvironmentVariableDropping: constraints.allowEnvironmentVariableDropping,
		AllowUnencryptedScratch:          constraints.allowUnencryptedScratch,
		AllowCapabilityDropping:          constraints.allowCapabilityDropping,
	}
}

func (constraints *generatedWindowsConstraints) toFragment() *securityPolicyFragment {
	// Convert Windows containers to regular containers for fragment compatibility
	linuxContainers := make([]*securityPolicyContainer, len(constraints.containers))
	for i, winContainer := range constraints.containers {
		// This is a placeholder conversion - you may need to implement proper conversion
		linuxContainers[i] = &securityPolicyContainer{
			Command:    winContainer.Command,
			EnvRules:   winContainer.EnvRules,
			WorkingDir: winContainer.WorkingDir,
			Layers:     winContainer.Layers,
			// Note: Some Windows-specific fields may not have Linux equivalents
		}
	}

	return &securityPolicyFragment{
		Namespace:         constraints.namespace,
		SVN:               constraints.svn,
		Containers:        linuxContainers,
		ExternalProcesses: constraints.externalProcesses,
		Fragments:         constraints.fragments,
	}
}

func generateWindowsConstraints(r *rand.Rand, maxContainers int32) *generatedWindowsConstraints {
	var containers []*securityPolicyWindowsContainer

	numContainers := (int)(atLeastOneAtMost(r, maxContainers))
	for i := 0; i < numContainers; i++ {
		containers = append(containers, generateConstraintsWindowsContainer(r, 1, maxLayersInGeneratedContainer))
	}

	return &generatedWindowsConstraints{
		containers:                       containers,
		externalProcesses:                make([]*externalProcess, 0),
		fragments:                        make([]*fragment, 0),
		allowGetProperties:               randBool(r),
		allowDumpStacks:                  randBool(r),
		allowRuntimeLogging:              false,
		allowEnvironmentVariableDropping: false,
		allowUnencryptedScratch:          false,
		allowCapabilityDropping:          false,
		namespace:                        generateFragmentNamespace(r),
		svn:                              generateSVN(r),
		ctx:                              context.Background(),
	}
}

func (*generatedWindowsConstraints) Generate(r *rand.Rand, _ int) reflect.Value {
	c := generateWindowsConstraints(r, maxContainersInGeneratedConstraints)
	return reflect.ValueOf(c)
}

type containerInitProcess struct {
	Command          []string
	EnvRules         []EnvRuleConfig
	WorkingDir       string
	AllowStdioAccess bool
}
