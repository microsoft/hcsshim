//go:build windows && (lcow || wcow)

package network

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/Microsoft/hcsshim/internal/controller/network/mocks"
	"github.com/Microsoft/hcsshim/internal/gcs"
	"github.com/Microsoft/hcsshim/internal/guest/prot"
)

// ─────────────────────────────────────────────────────────────────────────────
// Test helpers
// ─────────────────────────────────────────────────────────────────────────────

// newCapsProvider returns a [mocks.MockcapabilitiesProvider] whose
// Capabilities() returns LCOW capabilities with the given namespace-add flag.
// LCOW is used here because Capabilities() returns the [gcs.GuestDefinedCapabilities]
// interface; the concrete OS does not matter for the platform-agnostic tests.
func newCapsProvider(t *testing.T, ctrl *gomock.Controller, namespaceAddSupported bool) *mocks.MockcapabilitiesProvider {
	t.Helper()
	caps := mocks.NewMockcapabilitiesProvider(ctrl)
	caps.EXPECT().Capabilities().Return(&gcs.LCOWGuestDefinedCapabilities{
		GcsGuestCapabilities: prot.GcsGuestCapabilities{
			NamespaceAddRequestSupported: namespaceAddSupported,
		},
	})
	return caps
}

// newNilCapsProvider returns a provider whose Capabilities() returns nil,
// modelling a guest that has not yet reported its capabilities.
func newNilCapsProvider(t *testing.T, ctrl *gomock.Controller) *mocks.MockcapabilitiesProvider {
	t.Helper()
	caps := mocks.NewMockcapabilitiesProvider(ctrl)
	caps.EXPECT().Capabilities().Return(nil)
	return caps
}

// ─────────────────────────────────────────────────────────────────────────────
// Construction tests
// ─────────────────────────────────────────────────────────────────────────────

// TestNew_NamespaceCapabilityCached verifies that [New] queries the guest's
// capabilities exactly once and caches the namespace-add flag, so hot paths
// do not re-query on every Setup/Teardown call.
func TestNew_NamespaceCapabilityCached(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := New(
		&Options{NetworkNamespace: "ns-1"},
		mocks.NewMockvmNetworkManager(ctrl),
		mocks.NewMockguestNetwork(ctrl),
		newCapsProvider(t, ctrl, true),
	)

	if !c.isNamespaceSupportedByGuest {
		t.Error("expected isNamespaceSupportedByGuest=true after caps say supported")
	}
	if c.netState != StateNotConfigured {
		t.Errorf("expected initial state NotConfigured, got %s", c.netState)
	}
	if c.namespaceID != "ns-1" {
		t.Errorf("expected namespaceID=ns-1, got %q", c.namespaceID)
	}
	if c.vmEndpoints == nil {
		t.Error("expected vmEndpoints map to be non-nil")
	}
}

// TestNew_NilCapabilities_NoCrash verifies that a guest which reports nil
// capabilities does not panic [New] and leaves the namespace flag unset.
// This matches the guarded read in network.go.
func TestNew_NilCapabilities_NoCrash(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := New(
		&Options{NetworkNamespace: "ns-1"},
		mocks.NewMockvmNetworkManager(ctrl),
		mocks.NewMockguestNetwork(ctrl),
		newNilCapsProvider(t, ctrl),
	)

	if c.isNamespaceSupportedByGuest {
		t.Error("expected isNamespaceSupportedByGuest=false when caps are nil")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Setup state-guard tests
// ─────────────────────────────────────────────────────────────────────────────

// TestSetup_RejectInWrongState verifies that Setup refuses to run when the
// controller is not in StateNotConfigured. The guard must reject the call
// without mutating state, since the deferred Invalid handler is registered
// after the guard.
func TestSetup_RejectInWrongState(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := New(
		&Options{NetworkNamespace: "ns-1"},
		mocks.NewMockvmNetworkManager(ctrl),
		mocks.NewMockguestNetwork(ctrl),
		newCapsProvider(t, ctrl, true),
	)
	c.netState = StateConfigured

	err := c.Setup(context.Background())
	if err == nil {
		t.Fatal("expected error from Setup in StateConfigured, got nil")
	}
	if !strings.Contains(err.Error(), "Configured") {
		t.Errorf("expected error to mention current state Configured, got: %v", err)
	}
	// State must not change on guard rejection — the deferred Invalid
	// handler is registered after the guard, so a guard miss must be a no-op.
	if c.netState != StateConfigured {
		t.Errorf("expected state to remain Configured after guard rejection, got %s", c.netState)
	}
}

// TestSetup_EmptyNamespaceID verifies that Setup fails fast when no
// namespace ID is configured and transitions the controller to Invalid.
// This is the only Setup pre-HCN failure path that we can exercise without
// a live HNS, but it also covers the deferred Invalid-on-error handler.
func TestSetup_EmptyNamespaceID(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := New(
		&Options{NetworkNamespace: ""},
		mocks.NewMockvmNetworkManager(ctrl),
		mocks.NewMockguestNetwork(ctrl),
		newCapsProvider(t, ctrl, true),
	)

	err := c.Setup(context.Background())
	if err == nil {
		t.Fatal("expected error from Setup with empty namespace ID, got nil")
	}
	// On any Setup failure after the state guard, the controller must
	// move to Invalid so subsequent Setup calls keep being rejected.
	if c.netState != StateInvalid {
		t.Errorf("expected state Invalid after Setup failure, got %s", c.netState)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Teardown idempotency tests
// ─────────────────────────────────────────────────────────────────────────────

// TestTeardown_NoOpFromNotConfigured verifies that calling Teardown before
// Setup is a no-op. The shim invokes Teardown unconditionally on pod
// removal even if Setup never ran (or failed before reaching Configured).
func TestTeardown_NoOpFromNotConfigured(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmNetworkManager(ctrl)
	guest := mocks.NewMockguestNetwork(ctrl)
	c := New(
		&Options{NetworkNamespace: "ns-1"},
		vm,
		guest,
		newCapsProvider(t, ctrl, true),
	)

	// No EXPECT() on vm or guest — any call would fail the test.
	if err := c.Teardown(context.Background()); err != nil {
		t.Fatalf("expected nil from Teardown in NotConfigured, got: %v", err)
	}
	if c.netState != StateNotConfigured {
		t.Errorf("expected state to remain NotConfigured, got %s", c.netState)
	}
}

// TestTeardown_NoOpFromTornDown verifies that calling Teardown a second time
// after a successful Teardown is a no-op. Containerd retries StopPodSandbox.
func TestTeardown_NoOpFromTornDown(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmNetworkManager(ctrl)
	guest := mocks.NewMockguestNetwork(ctrl)
	c := New(
		&Options{NetworkNamespace: "ns-1"},
		vm,
		guest,
		newCapsProvider(t, ctrl, true),
	)
	c.netState = StateTornDown

	if err := c.Teardown(context.Background()); err != nil {
		t.Fatalf("expected nil from Teardown in TornDown, got: %v", err)
	}
	if c.netState != StateTornDown {
		t.Errorf("expected state to remain TornDown, got %s", c.netState)
	}
}
