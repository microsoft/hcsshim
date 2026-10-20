//go:build windows
// +build windows

package securitypolicy

import (
	"context"
	"strings"
	"testing"

	"github.com/Microsoft/hcsshim/internal/vm/vmutils/etw"
	oci "github.com/opencontainers/runtime-spec/specs-go"
)

type trackingWindowsEnforcer struct {
	SecurityPolicyEnforcer
	encodedPolicy string
	createCalls   int
	logCalls      int
}

func (e *trackingWindowsEnforcer) EncodedSecurityPolicy() string {
	return e.encodedPolicy
}

func (e *trackingWindowsEnforcer) EnforceCreateContainerPolicyV2(
	_ context.Context,
	_ string,
	_ []string,
	envList []string,
	_ string,
	_ []oci.Mount,
	_ IDName,
	opts *CreateContainerOptions,
) (EnvList, *oci.LinuxCapabilities, bool, error) {
	e.createCalls++
	if opts == nil {
		panic("CreateContainerOptions must not be nil")
	}
	return envList, nil, true, nil
}

func (e *trackingWindowsEnforcer) EnforceLogProviderPolicy(
	_ context.Context,
	providerNames []string,
) ([]string, error) {
	e.logCalls++
	return providerNames, nil
}

func newTrackingWindowsEnforcer(encodedPolicy string) *trackingWindowsEnforcer {
	return &trackingWindowsEnforcer{
		SecurityPolicyEnforcer: &OpenDoorSecurityPolicyEnforcer{},
		encodedPolicy:          encodedPolicy,
	}
}

func TestHasSecurityPolicyWindows(t *testing.T) {
	if HasSecurityPolicy(&OpenDoorSecurityPolicyEnforcer{}) {
		t.Fatal("default open-door enforcer should not report a policy")
	}
	if HasSecurityPolicy(&ClosedDoorSecurityPolicyEnforcer{}) {
		t.Fatal("closed-door enforcer should not report an encoded policy")
	}
	if !HasSecurityPolicy(newTrackingWindowsEnforcer("policy")) {
		t.Fatal("encoded policy was not detected")
	}
}

func TestEnforceWCOWCreateContainerPolicy_PolicyPresenceGate(t *testing.T) {
	newSpec := func() *oci.Spec {
		return &oci.Spec{Process: &oci.Process{}}
	}

	t.Run("no policy skips supplemental validation but calls enforcer", func(t *testing.T) {
		enforcer := newTrackingWindowsEnforcer("")
		if _, err := EnforceWCOWCreateContainerPolicy(
			context.Background(),
			enforcer,
			"invalid container ID",
			newSpec(),
			nil,
			WCOWContainerPolicyState{},
		); err != nil {
			t.Fatalf("create policy returned error without encoded policy: %v", err)
		}
		if enforcer.createCalls != 1 {
			t.Fatalf("create enforcer calls = %d, want 1", enforcer.createCalls)
		}
	})

	t.Run("policy enables supplemental validation", func(t *testing.T) {
		enforcer := newTrackingWindowsEnforcer("policy")
		if _, err := EnforceWCOWCreateContainerPolicy(
			context.Background(),
			enforcer,
			"invalid container ID",
			newSpec(),
			nil,
			WCOWContainerPolicyState{},
		); err == nil || !strings.Contains(err.Error(), "invalid container ID") {
			t.Fatalf("expected container ID validation error, got %v", err)
		}
		if enforcer.createCalls != 0 {
			t.Fatalf("create enforcer calls = %d, want 0 after validation failure", enforcer.createCalls)
		}
	})

	t.Run("closed door remains unconditional", func(t *testing.T) {
		if _, err := EnforceWCOWCreateContainerPolicy(
			context.Background(),
			&ClosedDoorSecurityPolicyEnforcer{},
			"container",
			newSpec(),
			nil,
			WCOWContainerPolicyState{},
		); err == nil || !strings.Contains(err.Error(), "running commands is denied by policy") {
			t.Fatalf("expected closed-door denial, got %v", err)
		}
	})
}

func TestEnforceWCOWLogProviders_PolicyPresenceGate(t *testing.T) {
	sources := etw.LogSourcesInfo{
		LogConfig: etw.LogConfig{
			Sources: []etw.Source{{
				Providers: []etw.EtwProvider{{ProviderGUID: "not-a-guid"}},
			}},
		},
	}

	t.Run("no policy skips supplemental validation but calls enforcer", func(t *testing.T) {
		enforcer := newTrackingWindowsEnforcer("")
		if _, err := EnforceWCOWLogProviders(context.Background(), enforcer, sources); err != nil {
			t.Fatalf("log provider policy returned error without encoded policy: %v", err)
		}
		if enforcer.logCalls != 1 {
			t.Fatalf("log provider enforcer calls = %d, want 1", enforcer.logCalls)
		}
	})

	t.Run("policy enables supplemental validation", func(t *testing.T) {
		enforcer := newTrackingWindowsEnforcer("policy")
		if _, err := EnforceWCOWLogProviders(context.Background(), enforcer, sources); err == nil ||
			!strings.Contains(err.Error(), "provider with no name") {
			t.Fatalf("expected provider-model validation error, got %v", err)
		}
		if enforcer.logCalls != 0 {
			t.Fatalf("log provider enforcer calls = %d, want 0 after validation failure", enforcer.logCalls)
		}
	})

	t.Run("closed door remains unconditional", func(t *testing.T) {
		validSources := etw.LogSourcesInfo{
			LogConfig: etw.LogConfig{
				Sources: []etw.Source{{
					Providers: []etw.EtwProvider{{ProviderName: "provider"}},
				}},
			},
		}
		if _, err := EnforceWCOWLogProviders(
			context.Background(),
			&ClosedDoorSecurityPolicyEnforcer{},
			validSources,
		); err == nil || !strings.Contains(err.Error(), "log provider is denied by policy") {
			t.Fatalf("expected closed-door denial, got %v", err)
		}
	})
}
