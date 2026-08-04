//go:build windows && lcow

package network

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/Microsoft/hcsshim/hcn"
	"github.com/Microsoft/hcsshim/internal/controller/network/mocks"
	"github.com/Microsoft/hcsshim/internal/gcs"
	"github.com/Microsoft/hcsshim/internal/guest/prot"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	hcs "github.com/Microsoft/hcsshim/internal/hcs/v2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/vm/guestmanager"
)

var (
	errLCOWHostAdd     = errors.New("host AddNIC failed")
	errLCOWHostRemove  = errors.New("host RemoveNIC failed")
	errLCOWGuestAdd    = errors.New("guest AddNetworkInterface failed")
	errLCOWGuestRemove = errors.New("guest RemoveNetworkInterface failed")
)

// newLCOWController constructs a Controller wired with the supplied mocks
// and pre-set namespace-add capability flag. The state is left at
// StateNotConfigured; callers needing a different state must override
// netState before exercising guarded entry points.
func newLCOWController(
	t *testing.T,
	ctrl *gomock.Controller,
	namespaceAddSupported bool,
) (*Controller, *mocks.MockvmNetworkManager, *mocks.MockguestNetwork) {
	t.Helper()

	vm := mocks.NewMockvmNetworkManager(ctrl)
	guest := mocks.NewMockguestNetwork(ctrl)
	caps := mocks.NewMockcapabilitiesProvider(ctrl)
	caps.EXPECT().Capabilities().Return(&gcs.LCOWGuestDefinedCapabilities{
		GcsGuestCapabilities: prot.GcsGuestCapabilities{
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

// newLCOWEndpoint returns a synthetic HCN endpoint suitable for unit tests.
// All fields the controller reads downstream are populated so that the
// hcsschema.NetworkAdapter passed to AddNIC/RemoveNIC is fully realistic.
func newLCOWEndpoint(name string) *hcn.HostComputeEndpoint {
	return &hcn.HostComputeEndpoint{
		Id:                   "ep-" + name,
		Name:                 name,
		MacAddress:           "aa:bb:cc:dd:ee:01",
		HostComputeNamespace: "ns-1",
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Add endpoint tests
// ─────────────────────────────────────────────────────────────────────────────

// TestLCOW_AddEndpoint_Success_NamespaceSupport verifies the happy path with
// namespace support: host AddNIC runs first, then guest AddNetworkInterface
// receives an adapter built from the same endpoint, and the NIC is tracked.
func TestLCOW_AddEndpoint_Success_NamespaceSupport(t *testing.T) {
	ctrl := gomock.NewController(t)
	c, vm, guest := newLCOWController(t, ctrl, true)

	ep := newLCOWEndpoint("eth0")
	expectedAdapter, err := guestresource.BuildLCOWNetworkAdapter(ep.HostComputeNamespace, "nic-1", ep, false)
	if err != nil {
		t.Fatalf("failed to build expected adapter: %v", err)
	}

	gomock.InOrder(
		vm.EXPECT().AddNIC(gomock.Any(), "nic-1", &hcsschema.NetworkAdapter{
			EndpointId: ep.Id,
			MacAddress: ep.MacAddress,
		}).Return(nil),
		guest.EXPECT().AddNetworkInterface(gomock.Any(), expectedAdapter).Return(nil),
	)

	if err := c.addEndpointToGuestNamespace(context.Background(), ep.HostComputeNamespace, "nic-1", ep, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, ok := c.vmEndpoints["nic-1"]; !ok || got != ep {
		t.Errorf("expected nic-1 → %+v in vmEndpoints, got: %+v (present=%v)", ep, got, ok)
	}
}

// TestLCOW_AddEndpoint_Success_NoNamespaceSupport verifies that when the
// guest does not advertise namespace-add support, only the host-side NIC
// hot-add is performed; the guest call is skipped.
func TestLCOW_AddEndpoint_Success_NoNamespaceSupport(t *testing.T) {
	ctrl := gomock.NewController(t)
	c, vm, _ := newLCOWController(t, ctrl, false)

	ep := newLCOWEndpoint("eth0")

	vm.EXPECT().AddNIC(gomock.Any(), "nic-1", &hcsschema.NetworkAdapter{
		EndpointId: ep.Id,
		MacAddress: ep.MacAddress,
	}).Return(nil)
	// guest.AddNetworkInterface is intentionally not expected — gomock will
	// fail the test if the controller calls it without namespace support.

	if err := c.addEndpointToGuestNamespace(context.Background(), ep.HostComputeNamespace, "nic-1", ep, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := c.vmEndpoints["nic-1"]; !ok {
		t.Error("expected nic-1 to be tracked even when guest call is skipped")
	}
}

// TestLCOW_AddEndpoint_HostFails_NotTracked verifies that when the host-side
// AddNIC fails, the NIC is NOT tracked in vmEndpoints. Tracking a NIC the
// host does not own would lead Teardown to call RemoveNIC on a non-existent
// device, masking the real failure.
func TestLCOW_AddEndpoint_HostFails_NotTracked(t *testing.T) {
	ctrl := gomock.NewController(t)
	c, vm, _ := newLCOWController(t, ctrl, true)

	ep := newLCOWEndpoint("eth0")

	vm.EXPECT().AddNIC(gomock.Any(), "nic-1", gomock.Any()).Return(errLCOWHostAdd)
	// guest.AddNetworkInterface must not be called when host add fails.

	err := c.addEndpointToGuestNamespace(context.Background(), ep.HostComputeNamespace, "nic-1", ep, false)
	if !errors.Is(err, errLCOWHostAdd) {
		t.Fatalf("expected host add error to wrap, got: %v", err)
	}
	if _, ok := c.vmEndpoints["nic-1"]; ok {
		t.Error("expected nic-1 NOT tracked after host AddNIC failure")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Remove endpoint tests
// ─────────────────────────────────────────────────────────────────────────────

// TestLCOW_RemoveEndpoint_Success verifies the happy removal path: guest-side
// removal runs first, then host-side hot-remove.
func TestLCOW_RemoveEndpoint_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	c, vm, guest := newLCOWController(t, ctrl, true)

	ep := newLCOWEndpoint("eth0")

	gomock.InOrder(
		guest.EXPECT().RemoveNetworkInterface(gomock.Any(), &guestresource.LCOWNetworkAdapter{
			NamespaceID: "ns-1",
			ID:          "nic-1",
		}).Return(nil),
		vm.EXPECT().RemoveNIC(gomock.Any(), "nic-1", &hcsschema.NetworkAdapter{
			EndpointId: ep.Id,
			MacAddress: ep.MacAddress,
		}).Return(nil),
	)

	if err := c.removeEndpointFromGuestNamespace(context.Background(), "nic-1", ep); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestLCOW_RemoveEndpoint_GuestFails_HostNotCalled verifies that if the
// guest-side removal fails, the host-side RemoveNIC is NOT invoked. The
// caller (Teardown) marks the controller Invalid and lets a future
// Teardown retry both halves.
func TestLCOW_RemoveEndpoint_GuestFails_HostNotCalled(t *testing.T) {
	ctrl := gomock.NewController(t)
	c, _, guest := newLCOWController(t, ctrl, true)

	ep := newLCOWEndpoint("eth0")

	guest.EXPECT().RemoveNetworkInterface(gomock.Any(), gomock.Any()).Return(errLCOWGuestRemove)
	// vm.RemoveNIC must not be called.

	err := c.removeEndpointFromGuestNamespace(context.Background(), "nic-1", ep)
	if !errors.Is(err, errLCOWGuestRemove) {
		t.Fatalf("expected guest remove error to wrap, got: %v", err)
	}
}

// TestLCOW_RemoveEndpoint_BridgeClosed_HostStillCalled verifies that when the
// guest-side removal fails because the bridge is closed (the GCS is gone and
// its state with it), the controller still hot-removes the NIC from the host
// so cleanup completes instead of stalling on a doomed retry.
func TestLCOW_RemoveEndpoint_BridgeClosed_HostStillCalled(t *testing.T) {
	ctrl := gomock.NewController(t)
	c, vm, guest := newLCOWController(t, ctrl, true)

	ep := newLCOWEndpoint("eth0")

	gomock.InOrder(
		guest.EXPECT().RemoveNetworkInterface(gomock.Any(), gomock.Any()).
			Return(fmt.Errorf("transport gone: %w", gcs.ErrBridgeClosed)),
		vm.EXPECT().RemoveNIC(gomock.Any(), "nic-1", &hcsschema.NetworkAdapter{
			EndpointId: ep.Id,
			MacAddress: ep.MacAddress,
		}).Return(nil),
	)

	if err := c.removeEndpointFromGuestNamespace(context.Background(), "nic-1", ep); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestLCOW_RemoveEndpoint_GuestConnectionUnavailable_HostStillCalled mirrors
// the bridge-closed case for [guestmanager.ErrGuestConnectionUnavailable].
func TestLCOW_RemoveEndpoint_GuestConnectionUnavailable_HostStillCalled(t *testing.T) {
	ctrl := gomock.NewController(t)
	c, vm, guest := newLCOWController(t, ctrl, true)

	ep := newLCOWEndpoint("eth0")

	gomock.InOrder(
		guest.EXPECT().RemoveNetworkInterface(gomock.Any(), gomock.Any()).
			Return(fmt.Errorf("guest RPC: %w", guestmanager.ErrGuestConnectionUnavailable)),
		vm.EXPECT().RemoveNIC(gomock.Any(), "nic-1", &hcsschema.NetworkAdapter{
			EndpointId: ep.Id,
			MacAddress: ep.MacAddress,
		}).Return(nil),
	)

	if err := c.removeEndpointFromGuestNamespace(context.Background(), "nic-1", ep); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestLCOW_RemoveEndpoint_NoNamespaceSupport_HostOnly verifies that when the
// guest never received the namespace, the controller skips the guest-side
// removal and only hot-removes the NIC from the host.
func TestLCOW_RemoveEndpoint_NoNamespaceSupport_HostOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	c, vm, _ := newLCOWController(t, ctrl, false)

	ep := newLCOWEndpoint("eth0")

	vm.EXPECT().RemoveNIC(gomock.Any(), "nic-1", &hcsschema.NetworkAdapter{
		EndpointId: ep.Id,
		MacAddress: ep.MacAddress,
	}).Return(nil)
	// guest.RemoveNetworkInterface must not be called.

	if err := c.removeEndpointFromGuestNamespace(context.Background(), "nic-1", ep); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestLCOW_RemoveEndpoint_HostFails_VMGone_Tolerated verifies that when the
// host-side RemoveNIC fails because the UVM has already exited (HCS reports
// the system as gone / already stopped / invalid state / handle closed), the
// controller treats the failure as success. The NIC is destroyed alongside
// the VM, so propagating the error would only leak the cached endpoint
// mapping and block teardown — symmetric with the bridge-closed tolerance on
// the guest side.
func TestLCOW_RemoveEndpoint_HostFails_VMGone_Tolerated(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
	}{
		{"ComputeSystemDoesNotExist", fmt.Errorf("hcs::System::Modify: %w", hcs.ErrComputeSystemDoesNotExist)},
		{"VmcomputeAlreadyStopped", fmt.Errorf("hcs::System::Modify: %w", hcs.ErrVmcomputeAlreadyStopped)},
		{"VmcomputeOperationInvalidState", fmt.Errorf("hcs::System::Modify: %w", hcs.ErrVmcomputeOperationInvalidState)},
		{"AlreadyClosed", fmt.Errorf("hcs::System::Modify: %w", hcs.ErrAlreadyClosed)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			c, vm, guest := newLCOWController(t, ctrl, true)

			ep := newLCOWEndpoint("eth0")

			gomock.InOrder(
				guest.EXPECT().RemoveNetworkInterface(gomock.Any(), gomock.Any()).Return(nil),
				vm.EXPECT().RemoveNIC(gomock.Any(), "nic-1", gomock.Any()).Return(tc.err),
			)

			if err := c.removeEndpointFromGuestNamespace(context.Background(), "nic-1", ep); err != nil {
				t.Fatalf("expected VM-gone error from host RemoveNIC to be tolerated, got: %v", err)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Teardown tests (LCOW: removeNetNSInsideGuest is a no-op so full Teardown
// is exercisable end-to-end without HNS)
// ─────────────────────────────────────────────────────────────────────────────

// TestLCOW_Teardown_HappyPath verifies that Teardown removes every tracked
// endpoint, clears the tracking map, and transitions the controller to
// StateTornDown.
func TestLCOW_Teardown_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	c, vm, guest := newLCOWController(t, ctrl, true)

	c.netState = StateConfigured
	ep1 := newLCOWEndpoint("eth0")
	ep2 := newLCOWEndpoint("eth1")
	c.vmEndpoints["nic-1"] = ep1
	c.vmEndpoints["nic-2"] = ep2

	// Two endpoints tracked, so each remove API runs exactly twice. Map
	// iteration order is not asserted here; gomock matches by call count.
	guest.EXPECT().RemoveNetworkInterface(gomock.Any(), gomock.Any()).Return(nil).Times(2)
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

// TestLCOW_Teardown_PartialFailure_RemainingAttempted verifies the cleanup
// chain: if removing one NIC fails, the controller must still attempt the
// remaining NICs. A leak here means a UVM kept alive with stuck NICs.
// The controller transitions to Invalid, but the surviving NICs are gone.
func TestLCOW_Teardown_PartialFailure_RemainingAttempted(t *testing.T) {
	ctrl := gomock.NewController(t)
	c, vm, guest := newLCOWController(t, ctrl, true)

	c.netState = StateConfigured
	ep1 := newLCOWEndpoint("eth0")
	ep2 := newLCOWEndpoint("eth1")
	ep3 := newLCOWEndpoint("eth2")
	c.vmEndpoints["nic-1"] = ep1
	c.vmEndpoints["nic-2"] = ep2
	c.vmEndpoints["nic-3"] = ep3

	// Fail nic-2 specifically; nic-1 and nic-3 succeed.
	// Map iteration order is random, so use DoAndReturn to branch on nicID.
	guest.EXPECT().RemoveNetworkInterface(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, settings *guestresource.LCOWNetworkAdapter) error {
			if settings.ID == "nic-2" {
				return errLCOWGuestRemove
			}
			return nil
		}).Times(3)
	// RemoveNIC is only called when guest removal succeeds — so 2 calls.
	vm.EXPECT().RemoveNIC(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(2)

	err := c.Teardown(context.Background())
	if !errors.Is(err, errLCOWGuestRemove) {
		t.Fatalf("expected joined error to wrap guest remove failure, got: %v", err)
	}
	if c.netState != StateInvalid {
		t.Errorf("expected state Invalid after partial failure, got %s", c.netState)
	}
	// nic-2 must remain tracked (so a retry can clean it up); nic-1 and nic-3
	// must be gone.
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

// TestLCOW_Teardown_HostRemoveFails_StateInvalid verifies that a host-side
// RemoveNIC failure also surfaces as Invalid (matching the guest-side path)
// and keeps the failed NIC tracked for a future retry.
func TestLCOW_Teardown_HostRemoveFails_StateInvalid(t *testing.T) {
	ctrl := gomock.NewController(t)
	c, vm, guest := newLCOWController(t, ctrl, true)

	c.netState = StateConfigured
	ep := newLCOWEndpoint("eth0")
	c.vmEndpoints["nic-1"] = ep

	guest.EXPECT().RemoveNetworkInterface(gomock.Any(), gomock.Any()).Return(nil)
	vm.EXPECT().RemoveNIC(gomock.Any(), "nic-1", gomock.Any()).Return(errLCOWHostRemove)

	err := c.Teardown(context.Background())
	if !errors.Is(err, errLCOWHostRemove) {
		t.Fatalf("expected host remove error to wrap, got: %v", err)
	}
	if c.netState != StateInvalid {
		t.Errorf("expected state Invalid, got %s", c.netState)
	}
	if _, ok := c.vmEndpoints["nic-1"]; !ok {
		t.Error("expected nic-1 to remain tracked after host RemoveNIC failure")
	}
}

// ──────────────────────────────────────────────────────────────────────────
// Half-setup recovery: Teardown unwinds host-side state after partial failures
// ──────────────────────────────────────────────────────────────────────────

// TestLCOW_AddEndpoint_HostOK_GuestFails_TeardownUnwindsHost covers the
// half-setup recovery contract end-to-end: when the guest-side
// AddNetworkInterface fails after a successful host AddNIC, the NIC must
// remain tracked so a subsequent Teardown can remove the host-side device.
// Otherwise the host UVM leaks the NIC.
func TestLCOW_AddEndpoint_HostOK_GuestFails_TeardownUnwindsHost(t *testing.T) {
	ctrl := gomock.NewController(t)
	c, vm, guest := newLCOWController(t, ctrl, true)

	ep := newLCOWEndpoint("eth0")

	gomock.InOrder(
		vm.EXPECT().AddNIC(gomock.Any(), "nic-1", gomock.Any()).Return(nil),
		guest.EXPECT().AddNetworkInterface(gomock.Any(), gomock.Any()).Return(errLCOWGuestAdd),
	)

	if err := c.addEndpointToGuestNamespace(context.Background(), ep.HostComputeNamespace, "nic-1", ep, false); !errors.Is(err, errLCOWGuestAdd) {
		t.Fatalf("expected guest add error to wrap, got: %v", err)
	}
	if _, ok := c.vmEndpoints["nic-1"]; !ok {
		t.Fatal("expected nic-1 to be tracked after guest-side failure so Teardown can unwind the host NIC")
	}

	// Teardown must run both legs: guest remove (best-effort, may no-op) and
	// host RemoveNIC (the actual leak-recovery path).
	c.netState = StateConfigured
	gomock.InOrder(
		guest.EXPECT().RemoveNetworkInterface(gomock.Any(), gomock.Any()).Return(nil),
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

// TestLCOW_Teardown_GuestFails_RetryFromInvalid covers the half-setup,
// failure-in-teardown, then success-teardown sequence. The first Teardown
// fails on guest removal; the controller transitions to StateInvalid and
// keeps the failed NIC tracked. A second Teardown call (which production
// code is allowed to make from StateInvalid) drains the remaining endpoint
// and reaches StateTornDown.
func TestLCOW_Teardown_GuestFails_RetryFromInvalid(t *testing.T) {
	ctrl := gomock.NewController(t)
	c, vm, guest := newLCOWController(t, ctrl, true)

	c.netState = StateConfigured
	ep := newLCOWEndpoint("eth0")
	c.vmEndpoints["nic-1"] = ep

	// First Teardown: guest remove fails → state Invalid, NIC still tracked.
	guest.EXPECT().RemoveNetworkInterface(gomock.Any(), gomock.Any()).Return(errLCOWGuestRemove)

	if err := c.Teardown(context.Background()); !errors.Is(err, errLCOWGuestRemove) {
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
		guest.EXPECT().RemoveNetworkInterface(gomock.Any(), gomock.Any()).Return(nil),
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
