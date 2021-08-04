package securitypolicy

import (
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/google/go-cmp/cmp"
)

const (
	maxContainersInGeneratedPolicy = 32
	maxLayersInGeneratedContainer  = 32
	maxGeneratedContainerID        = 1000000
	maxGeneratedCommandLength      = 128
	maxGeneratedCommandArgs        = 12
	maxGeneratedMountTargetLength  = 256
	rootHashLength                 = 64
)

// Do we correctly set up the data structures that are part of creating a new
// StandardSecurityPolicyEnforcer
func Test_StandardSecurityPolicyEnforcer_Devices_Initialization(t *testing.T) {
	f := func(p *SecurityPolicy) bool {
		policy, err := NewStandardSecurityPolicyEnforcer(p)
		if err != nil {
			return false
		}

		// there should be a device entry for each container
		if len(p.Containers) != len(policy.Devices) {
			return false
		}

		// in each device entry that corresponds to a container,
		// the array should have space for all the root hashes
		for i := 0; i < len(p.Containers); i++ {
			if len(p.Containers[i].Layers) != len(policy.Devices[i]) {
				return false
			}
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000}); err != nil {
		t.Errorf("Test_StandardSecurityPolicyEnforcer_Devices_Initialization failed: %v", err)
	}
}

// Verify that StandardSecurityPolicyEnforcer.EnforcePmemMountPolicy will return
// an error when there's no matching root hash in the policy
func Test_EnforcePmemMountPolicy_No_Matches(t *testing.T) {
	f := func(p *SecurityPolicy) bool {

		policy, err := NewStandardSecurityPolicyEnforcer(p)
		if err != nil {
			return false
		}

		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		target := generateMountTarget(r)
		rootHash := generateInvalidRootHash(r)

		err = policy.EnforcePmemMountPolicy(target, rootHash)

		// we expect an error, not getting one means something is broken
		return err != nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000}); err != nil {
		t.Errorf("Test_EnforcePmemMountPolicy_No_Matches failed: %v", err)
	}
}

// Verify that StandardSecurityPolicyEnforcer.EnforcePmemMountPolicy doesn't return
// an error when there's a matching root hash in the policy
func Test_EnforcePmemMountPolicy_Matches(t *testing.T) {
	f := func(p *SecurityPolicy) bool {

		policy, err := NewStandardSecurityPolicyEnforcer(p)
		if err != nil {
			return false
		}

		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		target := generateMountTarget(r)
		rootHash := selectRootHashFromPolicy(p, r)

		err = policy.EnforcePmemMountPolicy(target, rootHash)

		// getting an error means something is broken
		return err == nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000}); err != nil {
		t.Errorf("Test_EnforcePmemMountPolicy_No_Matches failed: %v", err)
	}
}

// Verify that StandardSecurityPolicyEnforcer.EnforceOverlayMountPolicy will return
// an error when there's no matching overlay targets.
func Test_EnforceOverlayMountPolicy_No_Matches(t *testing.T) {
	f := func(p *SecurityPolicy) bool {

		policy, err := NewStandardSecurityPolicyEnforcer(p)
		if err != nil {
			return false
		}

		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		containerID := generateContainerId(r)
		container := selectContainerFromPolicy(p, r)

		layerPaths, err := createInvalidOverlayForContainer(policy, container, r)
		if err != nil {
			return false
		}

		err = policy.EnforceOverlayMountPolicy(containerID, layerPaths)

		// not getting an error means something is broken
		return err != nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000}); err != nil {
		t.Errorf("Test_EnforceOverlayMountPolicy_No_Matches failed: %v", err)
	}
}

// Verify that StandardSecurityPolicyEnforcer.EnforceOverlayMountPolicy doesn't
// return an error when there's a valid overlay target.
func Test_EnforceOverlayMountPolicy_Matches(t *testing.T) {
	f := func(p *SecurityPolicy) bool {

		policy, err := NewStandardSecurityPolicyEnforcer(p)
		if err != nil {
			return false
		}

		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		containerID := generateContainerId(r)
		container := selectContainerFromPolicy(p, r)

		layerPaths, err := createValidOverlayForContainer(policy, container, r)
		if err != nil {
			return false
		}

		err = policy.EnforceOverlayMountPolicy(containerID, layerPaths)

		// getting an error means something is broken
		return err == nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000}); err != nil {
		t.Errorf("Test_EnforceOverlayMountPolicy_Matches: %v", err)
	}
}

// Tests the specific case of trying to mount the same overlay twice using the /// same container id. This should be disallowed.
func Test_EnforceOverlayMountPolicy_Overlay_Single_Container_Twice(t *testing.T) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	p := generateSecurityPolicy(r, 1)

	policy, err := NewStandardSecurityPolicyEnforcer(p)
	if err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}

	containerID := generateContainerId(r)
	container := selectContainerFromPolicy(p, r)

	layerPaths, err := createValidOverlayForContainer(policy, container, r)
	if err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}

	err = policy.EnforceOverlayMountPolicy(containerID, layerPaths)
	if err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}

	err = policy.EnforceOverlayMountPolicy(containerID, layerPaths)
	if err == nil {
		t.Fatalf("able to create overlay for the same container twice")
	}
}

// Test that if more than 1 instance of the same image is started, that we can
// create all the overlays that are required. So for example, if there are
// 13 instances of image X that all share the same overlay of root hashes,
// all 13 should be allowed.
func Test_EnforceOverlayMountPolicy_Multiple_Instances_Same_Container(t *testing.T) {
	for containersToCreate := 2; containersToCreate <= maxContainersInGeneratedPolicy; containersToCreate++ {
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		var containers []SecurityPolicyContainer

		for i := 1; i <= int(containersToCreate); i++ {
			arg := "command " + strconv.Itoa(i)
			c := SecurityPolicyContainer{
				Command: []string{arg},
				Layers:  []string{"1", "2"},
			}

			containers = append(containers, c)
		}

		p := &SecurityPolicy{
			AllowAll:   false,
			Containers: containers,
		}

		policy, err := NewStandardSecurityPolicyEnforcer(p)
		if err != nil {
			t.Fatal("unexpected error on test setup")
		}

		idsUsed := map[string]bool{}
		for i := 0; i < len(p.Containers); i++ {
			layerPaths, err := createValidOverlayForContainer(policy, p.Containers[i], r)
			if err != nil {
				t.Fatal("unexpected error on test setup")
			}

			idUnique := false
			var id string
			for idUnique == false {
				id = generateContainerId(r)
				_, found := idsUsed[id]
				idUnique = !found
				idsUsed[id] = true
			}
			err = policy.EnforceOverlayMountPolicy(id, layerPaths)
			if err != nil {
				t.Fatalf("failed with %d containers", containersToCreate)
			}
		}

		t.Logf("ok for %d\n", containersToCreate)
	}
}

// Verify that can't create more containers using an overlay than exists in the
// policy. For example, if there is a single instance of image Foo in the
// policy, we should be able to create a single container for that overlay
// but no more than that one.
func Test_EnforceOverlayMountPolicy_Overlay_Single_Container_Twice_With_Different_IDs(t *testing.T) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	p := generateSecurityPolicy(r, 1)

	policy, err := NewStandardSecurityPolicyEnforcer(p)
	if err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}

	var containerIDOne, containerIDTwo string

	for containerIDOne == containerIDTwo {
		containerIDOne = generateContainerId(r)
		containerIDTwo = generateContainerId(r)
	}
	container := selectContainerFromPolicy(p, r)

	layerPaths, err := createValidOverlayForContainer(policy, container, r)
	if err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}

	err = policy.EnforceOverlayMountPolicy(containerIDOne, layerPaths)
	if err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}

	err = policy.EnforceOverlayMountPolicy(containerIDTwo, layerPaths)
	if err == nil {
		t.Fatalf("able to reuse an overlay across containers")
	}
}

func Test_EnforceCommandPolicy_Matches(t *testing.T) {
	f := func(p *SecurityPolicy) bool {
		policy, err := NewStandardSecurityPolicyEnforcer(p)
		if err != nil {
			return false
		}

		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		containerID := generateContainerId(r)
		container := selectContainerFromPolicy(p, r)

		layerPaths, err := createValidOverlayForContainer(policy, container, r)
		if err != nil {
			return false
		}

		err = policy.EnforceOverlayMountPolicy(containerID, layerPaths)
		if err != nil {
			return false
		}

		err = policy.EnforceCommandPolicy(containerID, container.Command)

		// getting an error means something is broken
		return err == nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000}); err != nil {
		t.Errorf("Test_EnforceCommandPolicy_Matches: %v", err)
	}
}

func Test_EnforceCommandPolicy_NoMatches(t *testing.T) {
	f := func(p *SecurityPolicy) bool {
		policy, err := NewStandardSecurityPolicyEnforcer(p)
		if err != nil {
			return false
		}

		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		containerID := generateContainerId(r)
		container := selectContainerFromPolicy(p, r)

		layerPaths, err := createValidOverlayForContainer(policy, container, r)
		if err != nil {
			return false
		}

		err = policy.EnforceOverlayMountPolicy(containerID, layerPaths)
		if err != nil {
			return false
		}

		err = policy.EnforceCommandPolicy(containerID, generateCommand(r))

		// not getting an error means something is broken
		return err != nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000}); err != nil {
		t.Errorf("Test_EnforceCommandPolicy_NoMatches: %v", err)
	}
}

// This is a tricky test.
// The key to understanding it is, that when we have multiple containers
// with the same base aka same mounts and overlay, then we don't know at the
// time of overlay which container from policy is a given container id refers
// to. Instead we have a list of possible container ids for the so far matching
// containers in policy. We can narrow down the list of possible containers
// at the time that we enforce commands.
//
// This test verifies the "narrowing possible container ids that could be
// the container in our policy" functionality works correctly.
func Test_EnforceCommandPolicy_NarrowingMatches(t *testing.T) {
	f := func(p *SecurityPolicy) bool {
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		// create two additional containers that "share everything"
		// except that they have different commands
		testContainerOne := generateSecurityPolicyContainer(r, 5)
		testContainerTwo := testContainerOne
		testContainerTwo.Command = generateCommand(r)
		// add new containers to policy before creating enforcer
		p.Containers = append(p.Containers, testContainerOne, testContainerTwo)

		policy, err := NewStandardSecurityPolicyEnforcer(p)
		if err != nil {
			return false
		}

		testContainerOneId := ""
		testContainerTwoId := ""
		indexForContainerOne := -1
		indexForContainerTwo := -1

		// mount and overlay all our containers
		for index, container := range p.Containers {
			containerID := generateContainerId(r)

			layerPaths, err := createValidOverlayForContainer(policy, container, r)
			if err != nil {
				return false
			}

			err = policy.EnforceOverlayMountPolicy(containerID, layerPaths)
			if err != nil {
				return false
			}

			if cmp.Equal(container, testContainerOne) {
				testContainerOneId = containerID
				indexForContainerOne = index
			}
			if cmp.Equal(container, testContainerTwo) {
				testContainerTwoId = containerID
				indexForContainerTwo = index
			}
		}

		// validate our expectations prior to enforcing command policy
		containerOneMapping := policy.ContainerIndexToContainerIds[indexForContainerOne]
		if len(containerOneMapping) != 2 {
			return false
		}
		for _, id := range containerOneMapping {
			if (id != testContainerOneId) && (id != testContainerTwoId) {
				return false
			}
		}

		containerTwoMapping := policy.ContainerIndexToContainerIds[indexForContainerTwo]
		if len(containerTwoMapping) != 2 {
			return false
		}
		for _, id := range containerTwoMapping {
			if (id != testContainerOneId) && (id != testContainerTwoId) {
				return false
			}
		}

		// enforce command policy for containerOne
		// this will narrow our list of possible ids down
		err = policy.EnforceCommandPolicy(testContainerOneId, testContainerOne.Command)
		if err != nil {
			return false
		}

		// Ok, we have full setup and we can now verify that when we enforced
		// command policy above that it correctly narrowed down containerTwo
		updatedMapping := policy.ContainerIndexToContainerIds[indexForContainerTwo]
		if len(updatedMapping) != 1 {
			return false
		}
		for _, id := range updatedMapping {
			if id != testContainerTwoId {
				return false
			}
		}

		return true
	}

	// This is a more expensive test to run than others, so we run fewer times
	// for each run,
	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Test_EnforceCommandPolicy_NarrowingMatches: %v", err)
	}
}

//
// Setup and "fixtures" follow...
//

func (*SecurityPolicy) Generate(r *rand.Rand, size int) reflect.Value {
	p := generateSecurityPolicy(r, maxContainersInGeneratedPolicy)
	return reflect.ValueOf(p)
}

func generateSecurityPolicy(r *rand.Rand, numContainers int32) *SecurityPolicy {
	p := &SecurityPolicy{}
	p.AllowAll = false
	containers := atLeastOneAtMost(r, numContainers)
	for i := 0; i < (int)(containers); i++ {
		p.Containers = append(p.Containers, generateSecurityPolicyContainer(r, maxLayersInGeneratedContainer))
	}

	return p
}

func generateSecurityPolicyContainer(r *rand.Rand, size int32) SecurityPolicyContainer {
	c := SecurityPolicyContainer{}
	c.Command = generateCommand(r)
	layers := int(atLeastOneAtMost(r, size))
	for i := 0; i < layers; i++ {
		c.Layers = append(c.Layers, generateRootHash(r))
	}

	return c
}

func generateRootHash(r *rand.Rand) string {
	return randString(r, rootHashLength)
}

func generateCommand(r *rand.Rand) []string {
	args := []string{}

	numArgs := atLeastOneAtMost(r, maxGeneratedCommandArgs)
	for i := 0; i < int(numArgs); i++ {
		args = append(args, randVariableString(r, maxGeneratedCommandLength))
	}

	return args
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

func selectRootHashFromPolicy(policy *SecurityPolicy, r *rand.Rand) string {

	numberOfContainersInPolicy := len(policy.Containers)
	container := policy.Containers[r.Intn(numberOfContainersInPolicy)]
	numberOfLayersInContainer := len(container.Layers)

	return container.Layers[r.Intn(numberOfLayersInContainer)]
}

func generateContainerId(r *rand.Rand) string {
	id := atLeastOneAtMost(r, maxGeneratedContainerID)
	return strconv.FormatInt(int64(id), 10)
}

func selectContainerFromPolicy(policy *SecurityPolicy, r *rand.Rand) SecurityPolicyContainer {
	numberOfContainersInPolicy := len(policy.Containers)
	return policy.Containers[r.Intn(numberOfContainersInPolicy)]
}

func createValidOverlayForContainer(enforcer SecurityPolicyEnforcer, container SecurityPolicyContainer, r *rand.Rand) ([]string, error) {
	// storage for our mount paths
	overlay := make([]string, len(container.Layers))

	for i := 0; i < len(container.Layers); i++ {
		mount := generateMountTarget(r)
		err := enforcer.EnforcePmemMountPolicy(mount, container.Layers[i])
		if err != nil {
			return overlay, err
		}

		overlay[len(overlay)-i-1] = mount
	}

	return overlay, nil
}

func createInvalidOverlayForContainer(enforcer SecurityPolicyEnforcer, container SecurityPolicyContainer, r *rand.Rand) ([]string, error) {
	method := r.Intn(3)
	if method == 0 {
		return invalidOverlaySameSizeWrongMounts(enforcer, container, r)
	} else if method == 1 {
		return invalidOverlayCorrectDevicesWrongOrderSomeMissing(enforcer, container, r)
	} else {
		return invalidOverlayRandomJunk(enforcer, container, r)
	}
}

func invalidOverlaySameSizeWrongMounts(enforcer SecurityPolicyEnforcer, container SecurityPolicyContainer, r *rand.Rand) ([]string, error) {
	// storage for our mount paths
	overlay := make([]string, len(container.Layers))

	for i := 0; i < len(container.Layers); i++ {
		mount := generateMountTarget(r)
		err := enforcer.EnforcePmemMountPolicy(mount, container.Layers[i])
		if err != nil {
			return overlay, err
		}

		// generate a random new mount point to cause an error
		overlay[len(overlay)-i-1] = generateMountTarget(r)
	}

	return overlay, nil
}

func invalidOverlayCorrectDevicesWrongOrderSomeMissing(enforcer SecurityPolicyEnforcer, container SecurityPolicyContainer, r *rand.Rand) ([]string, error) {
	if len(container.Layers) == 1 {
		// won't work with only 1, we need to bail out to another method
		return invalidOverlayRandomJunk(enforcer, container, r)
	}
	// storage for our mount paths
	var overlay []string

	for i := 0; i < len(container.Layers); i++ {
		mount := generateMountTarget(r)
		err := enforcer.EnforcePmemMountPolicy(mount, container.Layers[i])
		if err != nil {
			return overlay, err
		}

		if r.Intn(10) != 0 {
			overlay = append(overlay, mount)
		}
	}

	return overlay, nil
}

func invalidOverlayRandomJunk(enforcer SecurityPolicyEnforcer, container SecurityPolicyContainer, r *rand.Rand) ([]string, error) {
	// create "junk" for entry
	layersToCreate := r.Int31n(maxLayersInGeneratedContainer)
	overlay := make([]string, layersToCreate)

	for i := 0; i < int(layersToCreate); i++ {
		overlay[i] = generateMountTarget(r)
	}

	// setup entirely different and "correct" expected mounting
	for i := 0; i < len(container.Layers); i++ {
		mount := generateMountTarget(r)
		err := enforcer.EnforcePmemMountPolicy(mount, container.Layers[i])
		if err != nil {
			return overlay, err
		}
	}

	return overlay, nil
}

func randVariableString(r *rand.Rand, maxLen int32) string {
	return randString(r, atLeastOneAtMost(r, maxLen))
}

func randString(r *rand.Rand, len int32) string {
	var s strings.Builder
	for i := 0; i < (int)(len); i++ {
		s.WriteRune((rune)(0x00ff & r.Int31n(256)))
	}

	return s.String()
}

func randMinMax(r *rand.Rand, min int32, max int32) int32 {
	return r.Int31n(max-min+1) + min
}

func atLeastOneAtMost(r *rand.Rand, most int32) int32 {
	return randMinMax(r, 1, most)
}
