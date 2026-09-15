package main

import (
	"testing"

	"github.com/Microsoft/hcsshim/internal/tools/securitypolicy/helpers"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

func TestLinuxPolicyContainerConfigs_AllowAllEnumeratesNoContainers(t *testing.T) {
	config := &securitypolicy.PolicyConfig{AllowAll: true}

	if got := linuxPolicyContainerConfigs(config); len(got) != 0 {
		t.Fatalf("open-door policy must enumerate no containers, got %d", len(got))
	}
}

// An open-door policy is rejected outright if it enumerates any container, so
// the guard above is what makes `-t rego` with allow_all work at all.
func TestMarshalPolicy_AllowAllRejectsContainers(t *testing.T) {
	withPause, err := helpers.PolicyContainersFromConfigs(helpers.DefaultContainerConfigs())
	if err != nil {
		t.Skipf("cannot resolve default containers offline: %v", err)
	}

	if _, err := marshalOpenDoor(withPause); err == nil {
		t.Fatal("expected allow_all combined with a container list to be rejected")
	}

	if _, err := marshalOpenDoor(nil); err != nil {
		t.Fatalf("open-door policy with no containers should marshal, got %v", err)
	}
}

func marshalOpenDoor(containers []*securitypolicy.Container) (string, error) {
	return securitypolicy.MarshalPolicy(
		"rego",
		true, // allowAll
		containers,
		nil,
		nil,
		false,
		false,
		false,
		false,
		false,
		false,
		false,
		false,
	)
}

func TestLinuxPolicyContainerConfigs_DefaultsAddedWhenNotAllowAll(t *testing.T) {
	config := &securitypolicy.PolicyConfig{}

	got := linuxPolicyContainerConfigs(config)
	if want := len(helpers.DefaultContainerConfigs()); len(got) != want {
		t.Fatalf("expected %d default containers, got %d", want, len(got))
	}
}
