//go:build windows && wcow

package network

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/Microsoft/hcsshim/hcn"
	"github.com/Microsoft/hcsshim/internal/controller/network/mocks"
	"github.com/Microsoft/hcsshim/internal/gcs"
	"github.com/Microsoft/hcsshim/internal/hcs/schema1"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
)

var (
	errWCOWHostAdd     = errors.New("host AddNIC failed")
	errWCOWGuestPreAdd = errors.New("guest PreAdd failed")
	errWCOWGuestAdd    = errors.New("guest Add failed")
	errWCOWGuestRemove = errors.New("guest Remove failed")
)

// newWCOWController constructs a Controller wired with WCOW mocks and the
// supplied namespace-add capability flag. State starts at StateNotConfigured;
// callers must override netState before exercising guarded entry points.
func newWCOWController(
	t *testing.T,
	ctrl *gomock.Controller,
	namespaceAddSupported bool,
) (*Controller, *mocks.MockvmNetworkManager, *mocks.MockguestNetwork) {
	t.Helper()

	vm := mocks.NewMockvmNetworkManager(ctrl)
	guest := mocks.NewMockguestNetwork(ctrl)
	caps := mocks.NewMockcapabilitiesProvider(ctrl)
	caps.EXPECT().Capabilities().Return(&gcs.WCOWGuestDefinedCapabilities{
		GuestDefinedCapabilities: schema1.GuestDefinedCapabilities{
			NamespaceAddRequestSupported: namespaceAddSupported,
		},
	})

	c := New(
		&Options{NetworkNamespace: "ns-1"},
		vm,
		guest,
		caps,
	)
	return c, vm, guest
}

// newWCOWEndpoint returns a synthetic HCN endpoint suitable for unit tests.
func newWCOWEndpoint(name string) *hcn.HostComputeEndpoint {
	return &hcn.HostComputeEndpoint{
		Id:                   "ep-" + name,
		Name:                 name,
		MacAddress:           "aa:bb:cc:dd:ee:01",
		HostComputeNamespace: "ns-1",
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Add endpoint tests (WCOW: PreAdd → host AddNIC → guest Add)
// ─────────────────────────────────────────────────────────────────────────────

// TestWCOW_AddEndpoint_3PhaseSequence_Success verifies the WCOW three-phase
// add sequence: guest PreAdd, then host AddNIC, then guest Add (finalize).
// The order matters — WCOW expects the guest to be informed BEFORE the NIC
// arrives on the bus.
func TestWCOW_AddEndpoint_3PhaseSequence_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	c, vm, guest := newWCOWController(t, ctrl, true)

	ep := newWCOWEndpoint("eth0")

	gomock.InOrder(
		// 1. PreAdd: tells the WCOW guest a NIC is about to arrive.
		guest.EXPECT().AddNetworkInterface(
			gomock.Any(), "nic-1", guestrequest.RequestTypePreAdd, ep,
		).Return(nil),
		// 2. Host hot-add.
		vm.EXPECT().AddNIC(gomock.Any(), "nic-1", &hcsschema.NetworkAdapter{
			EndpointId: ep.Id,
			MacAddress: ep.MacAddress,
		}).Return(nil),
		// 3. Guest finalize add.
		guest.EXPECT().AddNetworkInterface(
			gomock.Any(), "nic-1", guestrequest.RequestTypeAdd, gomock.Nil(),
		).Return(nil),
	)

	if err := c.addEndpointToGuestNamespace(context.Background(), "nic-1", ep, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, ok := c.vmEndpoints["nic-1"]; !ok || got != ep {
		t.Errorf("expected nic-1 → %+v in vmEndpoints, got: %+v (present=%v)", ep, got, ok)
	}
}

// TestWCOW_AddEndpoint_PreAddFails_NotTracked verifies that when the guest
// pre-add fails, neither the host AddNIC nor the guest finalize is invoked,
// and the NIC is not tracked. PreAdd is the WCOW-specific guard against
// presenting an unwanted device to the guest.
func TestWCOW_AddEndpoint_PreAddFails_NotTracked(t *testing.T) {
	ctrl := gomock.NewController(t)
	c, _, guest := newWCOWController(t, ctrl, true)

	ep := newWCOWEndpoint("eth0")

	guest.EXPECT().AddNetworkInterface(
		gomock.Any(), "nic-1", guestrequest.RequestTypePreAdd, ep,
	).Return(errWCOWGuestPreAdd)
	// No vm.AddNIC, no second guest.AddNetworkInterface — gomock fails if either is called.

	err := c.addEndpointToGuestNamespace(context.Background(), "nic-1", ep, false)
	if !errors.Is(err, errWCOWGuestPreAdd) {
		t.Fatalf("expected pre-add error to wrap, got: %v", err)
	}
	if _, ok := c.vmEndpoints["nic-1"]; ok {
		t.Error("expected nic-1 NOT tracked after pre-add failure")
	}
}

// TestWCOW_AddEndpoint_HostFails_NotTracked verifies that a host-side AddNIC
// failure (after a successful PreAdd) leaves the NIC untracked. Tracking
// would lead Teardown to RemoveNIC a device the host does not own.
func TestWCOW_AddEndpoint_HostFails_NotTracked(t *testing.T) {
	ctrl := gomock.NewController(t)
	c, vm, guest := newWCOWController(t, ctrl, true)

	ep := newWCOWEndpoint("eth0")

	gomock.InOrder(
		guest.EXPECT().AddNetworkInterface(
			gomock.Any(), "nic-1", guestrequest.RequestTypePreAdd, ep,
		).Return(nil),
		vm.EXPECT().AddNIC(gomock.Any(), "nic-1", gomock.Any()).Return(errWCOWHostAdd),
	)

	err := c.addEndpointToGuestNamespace(context.Background(), "nic-1", ep, false)
	if !errors.Is(err, errWCOWHostAdd) {
		t.Fatalf("expected host add error to wrap, got: %v", err)
	}
	if _, ok := c.vmEndpoints["nic-1"]; ok {
		t.Error("expected nic-1 NOT tracked after host AddNIC failure")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Remove endpoint tests
// ─────────────────────────────────────────────────────────────────────────────

// TestWCOW_RemoveEndpoint_Success verifies the happy removal path: guest-side
// remove first, then host RemoveNIC.
func TestWCOW_RemoveEndpoint_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	c, vm, guest := newWCOWController(t, ctrl, true)

	ep := newWCOWEndpoint("eth0")

	gomock.InOrder(
		guest.EXPECT().RemoveNetworkInterface(
			gomock.Any(), "nic-1", guestrequest.RequestTypeRemove, gomock.Nil(),
		).Return(nil),
		vm.EXPECT().RemoveNIC(gomock.Any(), "nic-1", &hcsschema.NetworkAdapter{
			EndpointId: ep.Id,
			MacAddress: ep.MacAddress,
		}).Return(nil),
	)

	if err := c.removeEndpointFromGuestNamespace(context.Background(), "nic-1", ep); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestWCOW_RemoveEndpoint_GuestFails_HostNotCalled verifies that a guest-side
// removal failure short-circuits the host hot-remove. Caller (Teardown)
// marks the controller Invalid and lets a future retry attempt both halves.
func TestWCOW_RemoveEndpoint_GuestFails_HostNotCalled(t *testing.T) {
	ctrl := gomock.NewController(t)
	c, _, guest := newWCOWController(t, ctrl, true)

	ep := newWCOWEndpoint("eth0")

	guest.EXPECT().RemoveNetworkInterface(
		gomock.Any(), "nic-1", guestrequest.RequestTypeRemove, gomock.Nil(),
	).Return(errWCOWGuestRemove)
	// vm.RemoveNIC must not be called.

	err := c.removeEndpointFromGuestNamespace(context.Background(), "nic-1", ep)
	if !errors.Is(err, errWCOWGuestRemove) {
		t.Fatalf("expected guest remove error to wrap, got: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Teardown tests
//
// WCOW's removeNetNSInsideGuest calls hcn.GetNamespaceByID when the guest
// supports namespace add. To exercise full Teardown without a live HNS,
// these tests construct controllers with namespace support DISABLED;
// removeNetNSInsideGuest then becomes a no-op. The endpoint loop logic
// — the high-value cleanup path — is fully covered.
// ─────────────────────────────────────────────────────────────────────────────

// TestWCOW_Teardown_HappyPath_NoNamespaceSupport verifies that Teardown
// removes every tracked endpoint, clears the tracking map, and transitions
// to StateTornDown.
func TestWCOW_Teardown_HappyPath_NoNamespaceSupport(t *testing.T) {
	ctrl := gomock.NewController(t)
	c, vm, guest := newWCOWController(t, ctrl, false)

	c.netState = StateConfigured
	ep1 := newWCOWEndpoint("eth0")
	ep2 := newWCOWEndpoint("eth1")
	c.vmEndpoints["nic-1"] = ep1
	c.vmEndpoints["nic-2"] = ep2

	guest.EXPECT().RemoveNetworkInterface(
		gomock.Any(), gomock.Any(), guestrequest.RequestTypeRemove, gomock.Nil(),
	).Return(nil).Times(2)
	vm.EXPECT().RemoveNIC(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(2)

	if err := c.Teardown(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.netState != StateTornDown {
		t.Errorf("expected state TornDown, got %s", c.netState)
	}
	if len(c.vmEndpoints) != 0 {
		t.Errorf("expected vmEndpoints to be empty, got %d entries", len(c.vmEndpoints))
	}
}

// TestWCOW_Teardown_PartialFailure_RemainingAttempted verifies the cleanup
// chain: if removing one NIC fails, the controller must still attempt the
// remaining NICs. Surviving NICs leak the host UVM device.
func TestWCOW_Teardown_PartialFailure_RemainingAttempted(t *testing.T) {
	ctrl := gomock.NewController(t)
	c, vm, guest := newWCOWController(t, ctrl, false)

	c.netState = StateConfigured
	ep1 := newWCOWEndpoint("eth0")
	ep2 := newWCOWEndpoint("eth1")
	ep3 := newWCOWEndpoint("eth2")
	c.vmEndpoints["nic-1"] = ep1
	c.vmEndpoints["nic-2"] = ep2
	c.vmEndpoints["nic-3"] = ep3

	// Fail nic-2; nic-1 and nic-3 succeed.
	guest.EXPECT().RemoveNetworkInterface(
		gomock.Any(), gomock.Any(), guestrequest.RequestTypeRemove, gomock.Nil(),
	).DoAndReturn(
		func(_ context.Context, adapterID string, _ guestrequest.RequestType, _ *hcn.HostComputeEndpoint) error {
			if adapterID == "nic-2" {
				return errWCOWGuestRemove
			}
			return nil
		}).Times(3)
	// vm.RemoveNIC only runs when guest removal succeeds → 2 calls.
	vm.EXPECT().RemoveNIC(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(2)

	err := c.Teardown(context.Background())
	if !errors.Is(err, errWCOWGuestRemove) {
		t.Fatalf("expected joined error to wrap guest remove failure, got: %v", err)
	}
	if c.netState != StateInvalid {
		t.Errorf("expected state Invalid after partial failure, got %s", c.netState)
	}
	if _, ok := c.vmEndpoints["nic-2"]; !ok {
		t.Error("expected nic-2 to remain tracked after its removal failed")
	}
	if _, ok := c.vmEndpoints["nic-1"]; ok {
		t.Error("expected nic-1 to be removed from tracking")
	}
	if _, ok := c.vmEndpoints["nic-3"]; ok {
		t.Error("expected nic-3 to be removed from tracking")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Half-setup recovery: Teardown unwinds host-side state after partial failures
// ─────────────────────────────────────────────────────────────────────────────

// TestWCOW_AddEndpoint_FinalAddFails_TeardownUnwindsHost covers the
// half-setup recovery contract end-to-end: PreAdd ok, host AddNIC ok, guest
// Add fails. The NIC must remain tracked so a subsequent Teardown can
// unwind the host-side device. Otherwise the UVM leaks the NIC.
func TestWCOW_AddEndpoint_FinalAddFails_TeardownUnwindsHost(t *testing.T) {
	ctrl := gomock.NewController(t)
	c, vm, guest := newWCOWController(t, ctrl, false)

	ep := newWCOWEndpoint("eth0")

	gomock.InOrder(
		guest.EXPECT().AddNetworkInterface(
			gomock.Any(), "nic-1", guestrequest.RequestTypePreAdd, ep,
		).Return(nil),
		vm.EXPECT().AddNIC(gomock.Any(), "nic-1", gomock.Any()).Return(nil),
		guest.EXPECT().AddNetworkInterface(
			gomock.Any(), "nic-1", guestrequest.RequestTypeAdd, gomock.Nil(),
		).Return(errWCOWGuestAdd),
	)

	if err := c.addEndpointToGuestNamespace(context.Background(), "nic-1", ep, false); !errors.Is(err, errWCOWGuestAdd) {
		t.Fatalf("expected guest add error to wrap, got: %v", err)
	}
	if _, ok := c.vmEndpoints["nic-1"]; !ok {
		t.Fatal("expected nic-1 to be tracked after guest-side failure so Teardown can unwind the host NIC")
	}

	// Teardown: guest Remove (best-effort), then host RemoveNIC.
	c.netState = StateConfigured
	gomock.InOrder(
		guest.EXPECT().RemoveNetworkInterface(
			gomock.Any(), "nic-1", guestrequest.RequestTypeRemove, gomock.Nil(),
		).Return(nil),
		vm.EXPECT().RemoveNIC(gomock.Any(), "nic-1", gomock.Any()).Return(nil),
	)

	if err := c.Teardown(context.Background()); err != nil {
		t.Fatalf("Teardown after half-setup: %v", err)
	}
	if c.netState != StateTornDown {
		t.Errorf("expected state TornDown after recovery, got %s", c.netState)
	}
	if _, ok := c.vmEndpoints["nic-1"]; ok {
		t.Error("expected nic-1 to be cleared after successful Teardown")
	}
}

// TestWCOW_Teardown_GuestFails_RetryFromInvalid covers the half-setup,
// failure-in-teardown, then success-teardown sequence for WCOW. First
// Teardown fails on guest removal -> state Invalid, NIC stays tracked. Second
// Teardown call (allowed from StateInvalid) drains the endpoint and reaches
// StateTornDown.
func TestWCOW_Teardown_GuestFails_RetryFromInvalid(t *testing.T) {
	ctrl := gomock.NewController(t)
	c, vm, guest := newWCOWController(t, ctrl, false)

	c.netState = StateConfigured
	ep := newWCOWEndpoint("eth0")
	c.vmEndpoints["nic-1"] = ep

	// First Teardown: guest remove fails → state Invalid, NIC still tracked.
	guest.EXPECT().RemoveNetworkInterface(
		gomock.Any(), "nic-1", guestrequest.RequestTypeRemove, gomock.Nil(),
	).Return(errWCOWGuestRemove)

	if err := c.Teardown(context.Background()); !errors.Is(err, errWCOWGuestRemove) {
		t.Fatalf("first Teardown: expected guest remove error, got: %v", err)
	}
	if c.netState != StateInvalid {
		t.Fatalf("first Teardown: expected StateInvalid, got %s", c.netState)
	}
	if _, ok := c.vmEndpoints["nic-1"]; !ok {
		t.Fatal("first Teardown: nic-1 must remain tracked for retry")
	}

	// Second Teardown: both legs succeed → state TornDown, map cleared.
	gomock.InOrder(
		guest.EXPECT().RemoveNetworkInterface(
			gomock.Any(), "nic-1", guestrequest.RequestTypeRemove, gomock.Nil(),
		).Return(nil),
		vm.EXPECT().RemoveNIC(gomock.Any(), "nic-1", gomock.Any()).Return(nil),
	)

	if err := c.Teardown(context.Background()); err != nil {
		t.Fatalf("retry Teardown: %v", err)
	}
	if c.netState != StateTornDown {
		t.Errorf("retry Teardown: expected StateTornDown, got %s", c.netState)
	}
	if _, ok := c.vmEndpoints["nic-1"]; ok {
		t.Error("retry Teardown: expected nic-1 to be cleared")
	}
}
