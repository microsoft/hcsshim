//go:build windows && lcow

package vpci

import (
	"context"
	"errors"
	"testing"

	"github.com/Microsoft/go-winio/pkg/guid"
	"go.uber.org/mock/gomock"

	"github.com/Microsoft/hcsshim/internal/controller/device/vpci/mocks"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

func newTestDevice() Device {
	return Device{
		DeviceInstanceID:     "PCI\\VEN_1234&DEV_5678\\0",
		VirtualFunctionIndex: 0,
	}
}

var (
	errHostAdd    = errors.New("host add failed")
	errHostRemove = errors.New("host remove failed")
	errGuestAdd   = errors.New("guest add failed")
)

// ─────────────────────────────────────────────────────────────────────────────
// Reserve tests
// ─────────────────────────────────────────────────────────────────────────────

// TestReserve_NewDevice verifies that reserving a device returns a non-zero GUID
// and that the device appears in both tracking maps.
func TestReserve_NewDevice(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := New(mocks.NewMockvmVPCI(ctrl), mocks.NewMocklinuxGuestVPCI(ctrl))
	ctx := context.Background()

	dev := newTestDevice()
	g, err := c.Reserve(ctx, dev)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g == (guid.GUID{}) {
		t.Fatal("expected non-zero GUID")
	}
	if _, ok := c.devices[g]; !ok {
		t.Error("device not in c.devices after Reserve")
	}
	if storedGUID, ok := c.deviceToGUID[dev]; !ok || storedGUID != g {
		t.Error("device not in c.deviceToGUID after Reserve")
	}
	if c.devices[g].state != StateReserved {
		t.Errorf("expected StateReserved, got %v", c.devices[g].state)
	}
}

// TestReserve_SameDeviceTwice verifies idempotency: a second Reserve for the
// same device returns the exact same GUID without creating a new entry.
func TestReserve_SameDeviceTwice(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := New(mocks.NewMockvmVPCI(ctrl), mocks.NewMocklinuxGuestVPCI(ctrl))
	ctx := context.Background()

	dev := newTestDevice()
	g1, err := c.Reserve(ctx, dev)
	if err != nil {
		t.Fatalf("first Reserve: %v", err)
	}
	g2, err := c.Reserve(ctx, dev)
	if err != nil {
		t.Fatalf("second Reserve: %v", err)
	}
	if g1 != g2 {
		t.Errorf("expected same GUID, got %v vs %v", g1, g2)
	}
	if len(c.devices) != 1 {
		t.Errorf("expected 1 device entry, got %d", len(c.devices))
	}
}

// TestReserve_DifferentVF verifies that two VFs on the same physical device are
// treated as independent reservations.
func TestReserve_DifferentVF(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := New(mocks.NewMockvmVPCI(ctrl), mocks.NewMocklinuxGuestVPCI(ctrl))
	ctx := context.Background()

	dev0 := Device{DeviceInstanceID: "PCI\\VEN_1234&DEV_5678\\0", VirtualFunctionIndex: 0}
	dev1 := Device{DeviceInstanceID: "PCI\\VEN_1234&DEV_5678\\0", VirtualFunctionIndex: 1}

	g0, _ := c.Reserve(ctx, dev0)
	g1, _ := c.Reserve(ctx, dev1)

	if g0 == g1 {
		t.Error("different VFs should get different GUIDs")
	}
	if len(c.devices) != 2 {
		t.Errorf("expected 2 device entries, got %d", len(c.devices))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// AddToVM tests
// ─────────────────────────────────────────────────────────────────────────────

// TestAddToVM_NoReservation verifies that AddToVM returns an error when called
// with an unknown GUID.
func TestAddToVM_NoReservation(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := New(mocks.NewMockvmVPCI(ctrl), mocks.NewMocklinuxGuestVPCI(ctrl))
	ctx := context.Background()

	unknownGUID, _ := guid.NewV4()
	err := c.AddToVM(ctx, unknownGUID)
	if err == nil {
		t.Fatal("expected error for unknown GUID")
	}
}

// TestAddToVM_HappyPath verifies a normal Reserve → AddToVM flow.
func TestAddToVM_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmVPCI(ctrl)
	guest := mocks.NewMocklinuxGuestVPCI(ctrl)
	c := New(vm, guest)
	ctx := context.Background()

	dev := newTestDevice()
	g, _ := c.Reserve(ctx, dev)

	vm.EXPECT().AddDevice(gomock.Any(), g, gomock.Any()).Return(nil)
	guest.EXPECT().AddVPCIDevice(gomock.Any(), gomock.Any()).Return(nil)

	if err := c.AddToVM(ctx, g); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	di := c.devices[g]
	if di.state != StateReady {
		t.Errorf("expected StateReady, got %v", di.state)
	}
	if di.refCount != 1 {
		t.Errorf("expected refCount=1, got %d", di.refCount)
	}
}

// TestAddToVM_Idempotent verifies that calling AddToVM twice on the same device
// increments the refCount but does not make a second host-side call.
func TestAddToVM_Idempotent(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmVPCI(ctrl)
	guest := mocks.NewMocklinuxGuestVPCI(ctrl)
	c := New(vm, guest)
	ctx := context.Background()

	dev := newTestDevice()
	g, _ := c.Reserve(ctx, dev)

	// Host and guest calls must happen exactly once.
	vm.EXPECT().AddDevice(gomock.Any(), g, gomock.Any()).Return(nil).Times(1)
	guest.EXPECT().AddVPCIDevice(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	_ = c.AddToVM(ctx, g)
	if err := c.AddToVM(ctx, g); err != nil {
		t.Fatalf("second AddToVM: %v", err)
	}

	di := c.devices[g]
	if di.refCount != 2 {
		t.Errorf("expected refCount=2, got %d", di.refCount)
	}
	if di.state != StateReady {
		t.Errorf("expected StateReady, got %v", di.state)
	}
}

// TestAddToVM_HostFails verifies that a host-side failure transitions the device
// to StateRemoved (still tracked in the map) without bumping the refCount.
// The device must be cleaned up via RemoveFromVM.
func TestAddToVM_HostFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmVPCI(ctrl)
	guest := mocks.NewMocklinuxGuestVPCI(ctrl)
	c := New(vm, guest)
	ctx := context.Background()

	dev := newTestDevice()
	g, _ := c.Reserve(ctx, dev)

	vm.EXPECT().AddDevice(gomock.Any(), g, gomock.Any()).Return(errHostAdd)

	err := c.AddToVM(ctx, g)
	if err == nil {
		t.Fatal("expected error")
	}

	// Device must still be tracked (StateRemoved, awaiting RemoveFromVM cleanup).
	di, ok := c.devices[g]
	if !ok {
		t.Fatal("expected device to still be tracked after host failure")
	}
	if di.state != StateRemoved {
		t.Errorf("expected StateRemoved after host failure, got %v", di.state)
	}
	if di.refCount != 0 {
		t.Errorf("expected refCount=0 after host failure, got %d", di.refCount)
	}
}

// TestAddToVM_StateRemoved verifies that calling AddToVM on a StateRemoved device
// returns an error and does not make any host or guest calls.
func TestAddToVM_StateRemoved(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmVPCI(ctrl)
	guest := mocks.NewMocklinuxGuestVPCI(ctrl)
	c := New(vm, guest)
	ctx := context.Background()

	dev := newTestDevice()
	g, _ := c.Reserve(ctx, dev)

	// Host-side add fails → StateRemoved.
	vm.EXPECT().AddDevice(gomock.Any(), g, gomock.Any()).Return(errHostAdd)
	_ = c.AddToVM(ctx, g)

	// Second AddToVM: must NOT call host or guest again.
	err := c.AddToVM(ctx, g)
	if err == nil {
		t.Fatal("expected error for StateRemoved device")
	}
}

// TestAddToVM_GuestFails verifies that a guest-side failure marks the device
// StateAssignedInvalid and does not bump the refCount.
func TestAddToVM_GuestFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmVPCI(ctrl)
	guest := mocks.NewMocklinuxGuestVPCI(ctrl)
	c := New(vm, guest)
	ctx := context.Background()

	dev := newTestDevice()
	g, _ := c.Reserve(ctx, dev)

	vm.EXPECT().AddDevice(gomock.Any(), g, gomock.Any()).Return(nil)
	guest.EXPECT().AddVPCIDevice(gomock.Any(), gomock.Any()).Return(errGuestAdd)

	err := c.AddToVM(ctx, g)
	if err == nil {
		t.Fatal("expected error")
	}

	di := c.devices[g]
	if di.state != StateAssignedInvalid {
		t.Errorf("expected StateAssignedInvalid after guest failure, got %v", di.state)
	}
	if di.refCount != 0 {
		t.Errorf("expected refCount=0 after guest failure, got %d", di.refCount)
	}
}

// TestAddToVM_InvalidDevice verifies that AddToVM on a StateAssignedInvalid device
// returns an error and does not attempt a new host/guest call.
func TestAddToVM_InvalidDevice(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmVPCI(ctrl)
	guest := mocks.NewMocklinuxGuestVPCI(ctrl)
	c := New(vm, guest)
	ctx := context.Background()

	dev := newTestDevice()
	g, _ := c.Reserve(ctx, dev)

	// First AddToVM: host succeeds, guest fails → StateAssignedInvalid.
	vm.EXPECT().AddDevice(gomock.Any(), g, gomock.Any()).Return(nil)
	guest.EXPECT().AddVPCIDevice(gomock.Any(), gomock.Any()).Return(errGuestAdd)
	_ = c.AddToVM(ctx, g)

	// Second AddToVM: must NOT call host or guest again.
	err := c.AddToVM(ctx, g)
	if err == nil {
		t.Fatal("expected error for StateAssignedInvalid device")
	}
}

// TestAddToVM_ReservedTwice_ThenAdd exercises the scenario where the same
// device is reserved (getting the same GUID back), then AddToVM is called.
// The reservation itself is idempotent, so AddToVM should be called only once
// for the underlying host/guest pair.
func TestAddToVM_ReservedTwice_ThenAdd(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmVPCI(ctrl)
	guest := mocks.NewMocklinuxGuestVPCI(ctrl)
	c := New(vm, guest)
	ctx := context.Background()

	dev := newTestDevice()
	g1, _ := c.Reserve(ctx, dev)
	g2, _ := c.Reserve(ctx, dev)

	if g1 != g2 {
		t.Fatal("expected same GUID from double Reserve")
	}

	vm.EXPECT().AddDevice(gomock.Any(), g1, gomock.Any()).Return(nil).Times(1)
	guest.EXPECT().AddVPCIDevice(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	if err := c.AddToVM(ctx, g1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RemoveFromVM tests
// ─────────────────────────────────────────────────────────────────────────────

// TestRemoveFromVM_NoDevice verifies that removing an unknown GUID returns an error.
func TestRemoveFromVM_NoDevice(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := New(mocks.NewMockvmVPCI(ctrl), mocks.NewMocklinuxGuestVPCI(ctrl))
	ctx := context.Background()

	unknownGUID, _ := guid.NewV4()
	err := c.RemoveFromVM(ctx, unknownGUID)
	if err == nil {
		t.Fatal("expected error for unknown GUID")
	}
}

// TestRemoveFromVM_ReservedButNeverAdded verifies that removing a device that
// was reserved but never added cleans up the maps without touching the host.
func TestRemoveFromVM_ReservedButNeverAdded(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmVPCI(ctrl)
	guest := mocks.NewMocklinuxGuestVPCI(ctrl)
	c := New(vm, guest)
	ctx := context.Background()

	dev := newTestDevice()
	g, _ := c.Reserve(ctx, dev)

	// RemoveDevice must NOT be called.
	if err := c.RemoveFromVM(ctx, g); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := c.devices[g]; ok {
		t.Error("device still tracked after removal of reserved-only device")
	}
	if _, ok := c.deviceToGUID[dev]; ok {
		t.Error("deviceToGUID still has entry after removal of reserved-only device")
	}
}

// TestRemoveFromVM_AfterHostAddFails verifies that a device in StateRemoved (due to
// a failed host-side add in AddToVM) can be cleaned up via RemoveFromVM without
// making any host call.
func TestRemoveFromVM_AfterHostAddFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmVPCI(ctrl)
	guest := mocks.NewMocklinuxGuestVPCI(ctrl)
	c := New(vm, guest)
	ctx := context.Background()

	dev := newTestDevice()
	g, _ := c.Reserve(ctx, dev)

	// Host-side add fails → StateRemoved.
	vm.EXPECT().AddDevice(gomock.Any(), g, gomock.Any()).Return(errHostAdd)
	_ = c.AddToVM(ctx, g)

	// RemoveFromVM must NOT call RemoveDevice (no host-side state to clean up).
	if err := c.RemoveFromVM(ctx, g); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := c.devices[g]; ok {
		t.Error("device still tracked after RemoveFromVM on StateRemoved device")
	}
	if _, ok := c.deviceToGUID[dev]; ok {
		t.Error("deviceToGUID still has entry after RemoveFromVM on StateRemoved device")
	}
}

// TestRemoveFromVM_HappyPath verifies a full Reserve → AddToVM → RemoveFromVM cycle.
func TestRemoveFromVM_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmVPCI(ctrl)
	guest := mocks.NewMocklinuxGuestVPCI(ctrl)
	c := New(vm, guest)
	ctx := context.Background()

	dev := newTestDevice()
	g, _ := c.Reserve(ctx, dev)

	vm.EXPECT().AddDevice(gomock.Any(), g, gomock.Any()).Return(nil)
	guest.EXPECT().AddVPCIDevice(gomock.Any(), gomock.Any()).Return(nil)
	_ = c.AddToVM(ctx, g)

	vm.EXPECT().RemoveDevice(gomock.Any(), g).Return(nil)
	if err := c.RemoveFromVM(ctx, g); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := c.devices[g]; ok {
		t.Error("device still tracked after successful removal")
	}
}

// TestRemoveFromVM_RefCounting verifies that the device is only removed from
// the host when the last reference is dropped.
func TestRemoveFromVM_RefCounting(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmVPCI(ctrl)
	guest := mocks.NewMocklinuxGuestVPCI(ctrl)
	c := New(vm, guest)
	ctx := context.Background()

	dev := newTestDevice()
	g, _ := c.Reserve(ctx, dev)

	vm.EXPECT().AddDevice(gomock.Any(), g, gomock.Any()).Return(nil).Times(1)
	guest.EXPECT().AddVPCIDevice(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	_ = c.AddToVM(ctx, g)
	_ = c.AddToVM(ctx, g) // refCount → 2

	// First remove: decrements ref to 1, no host call.
	if err := c.RemoveFromVM(ctx, g); err != nil {
		t.Fatalf("first remove error: %v", err)
	}
	if c.devices[g].refCount != 1 {
		t.Errorf("expected refCount=1 after first remove, got %d", c.devices[g].refCount)
	}

	// Second remove: drops to 0, triggers host remove.
	vm.EXPECT().RemoveDevice(gomock.Any(), g).Return(nil).Times(1)
	if err := c.RemoveFromVM(ctx, g); err != nil {
		t.Fatalf("second remove error: %v", err)
	}
	if _, ok := c.devices[g]; ok {
		t.Error("device still tracked after last ref dropped")
	}
}

// TestRemoveFromVM_HostFails verifies that a failed host-side remove marks the
// device StateAssignedInvalid so it can be retried.
func TestRemoveFromVM_HostFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmVPCI(ctrl)
	guest := mocks.NewMocklinuxGuestVPCI(ctrl)
	c := New(vm, guest)
	ctx := context.Background()

	dev := newTestDevice()
	g, _ := c.Reserve(ctx, dev)

	vm.EXPECT().AddDevice(gomock.Any(), g, gomock.Any()).Return(nil)
	guest.EXPECT().AddVPCIDevice(gomock.Any(), gomock.Any()).Return(nil)
	_ = c.AddToVM(ctx, g)

	// First remove attempt fails.
	vm.EXPECT().RemoveDevice(gomock.Any(), g).Return(errHostRemove)
	err := c.RemoveFromVM(ctx, g)
	if err == nil {
		t.Fatal("expected error on host remove failure")
	}

	di := c.devices[g]
	if di.state != StateAssignedInvalid {
		t.Errorf("expected StateAssignedInvalid after failed remove, got %v", di.state)
	}
	if di.refCount != 0 {
		t.Errorf("expected refCount=0 after failed remove, got %d", di.refCount)
	}
}

// TestRemoveFromVM_HostFails_ThenRetry verifies that after a failed host remove
// (device is now StateAssignedInvalid with refCount=0), a retry via RemoveFromVM
// succeeds and cleans up the maps.
func TestRemoveFromVM_HostFails_ThenRetry(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmVPCI(ctrl)
	guest := mocks.NewMocklinuxGuestVPCI(ctrl)
	c := New(vm, guest)
	ctx := context.Background()

	dev := newTestDevice()
	g, _ := c.Reserve(ctx, dev)

	vm.EXPECT().AddDevice(gomock.Any(), g, gomock.Any()).Return(nil)
	guest.EXPECT().AddVPCIDevice(gomock.Any(), gomock.Any()).Return(nil)
	_ = c.AddToVM(ctx, g)

	// First remove: host fails.
	vm.EXPECT().RemoveDevice(gomock.Any(), g).Return(errHostRemove)
	_ = c.RemoveFromVM(ctx, g)

	// Retry: host succeeds.
	vm.EXPECT().RemoveDevice(gomock.Any(), g).Return(nil)
	if err := c.RemoveFromVM(ctx, g); err != nil {
		t.Fatalf("retry RemoveFromVM failed: %v", err)
	}

	if _, ok := c.devices[g]; ok {
		t.Error("device still tracked after successful retry removal")
	}
}

// TestRemoveFromVM_InvalidDevice_AfterGuestFail verifies that a device stuck in
// StateAssignedInvalid (due to guest failure in AddToVM) can be cleaned up via RemoveFromVM.
func TestRemoveFromVM_InvalidDevice_AfterGuestFail(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmVPCI(ctrl)
	guest := mocks.NewMocklinuxGuestVPCI(ctrl)
	c := New(vm, guest)
	ctx := context.Background()

	dev := newTestDevice()
	g, _ := c.Reserve(ctx, dev)

	// AddToVM: host succeeds, guest fails.
	vm.EXPECT().AddDevice(gomock.Any(), g, gomock.Any()).Return(nil)
	guest.EXPECT().AddVPCIDevice(gomock.Any(), gomock.Any()).Return(errGuestAdd)
	_ = c.AddToVM(ctx, g)

	// RemoveFromVM should issue a host remove and clean up.
	vm.EXPECT().RemoveDevice(gomock.Any(), g).Return(nil)
	if err := c.RemoveFromVM(ctx, g); err != nil {
		t.Fatalf("RemoveFromVM on invalid device failed: %v", err)
	}

	if _, ok := c.devices[g]; ok {
		t.Error("device still tracked after cleanup of invalid device")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Scenario: Reserve deduplication vs. state transitions
// ─────────────────────────────────────────────────────────────────────────────

// TestReserve_AfterRemove verifies what happens when Reserve is called for a
// device that was previously removed.
// After RemoveFromVM the device is untracked from deviceToGUID, so Reserve
// should treat it as a brand-new device and return a fresh GUID.
func TestReserve_AfterRemove(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmVPCI(ctrl)
	guest := mocks.NewMocklinuxGuestVPCI(ctrl)
	c := New(vm, guest)
	ctx := context.Background()

	dev := newTestDevice()
	g1, _ := c.Reserve(ctx, dev)

	vm.EXPECT().AddDevice(gomock.Any(), g1, gomock.Any()).Return(nil)
	guest.EXPECT().AddVPCIDevice(gomock.Any(), gomock.Any()).Return(nil)
	_ = c.AddToVM(ctx, g1)

	vm.EXPECT().RemoveDevice(gomock.Any(), g1).Return(nil)
	_ = c.RemoveFromVM(ctx, g1)

	// Reserve again after full removal: should get a new GUID.
	g2, err := c.Reserve(ctx, dev)
	if err != nil {
		t.Fatalf("Reserve after remove failed: %v", err)
	}
	if g2 == g1 {
		t.Error("expected a new GUID after re-reserving a fully removed device")
	}
}

// TestReserve_AfterGuestFailure verifies what Reserve returns for a device that
// is currently StateAssignedInvalid (guest failed, host succeeded).
// Since the device is still in deviceToGUID, Reserve should return the SAME GUID.
func TestReserve_AfterGuestFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmVPCI(ctrl)
	guest := mocks.NewMocklinuxGuestVPCI(ctrl)
	c := New(vm, guest)
	ctx := context.Background()

	dev := newTestDevice()
	g1, _ := c.Reserve(ctx, dev)

	vm.EXPECT().AddDevice(gomock.Any(), g1, gomock.Any()).Return(nil)
	guest.EXPECT().AddVPCIDevice(gomock.Any(), gomock.Any()).Return(errGuestAdd)
	_ = c.AddToVM(ctx, g1)

	// Device is now StateAssignedInvalid and still in deviceToGUID.
	g2, err := c.Reserve(ctx, dev)
	if err != nil {
		t.Fatalf("Reserve after guest failure: %v", err)
	}
	if g2 != g1 {
		t.Errorf("expected same GUID for invalid device on re-reserve, got different")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Scenario: RemoveFromVM on a StateRemoved device (already untracked)
// ─────────────────────────────────────────────────────────────────────────────

// TestRemoveFromVM_AlreadyRemoved verifies that calling RemoveFromVM twice
// returns an error on the second call (device no longer in map).
func TestRemoveFromVM_AlreadyRemoved(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmVPCI(ctrl)
	guest := mocks.NewMocklinuxGuestVPCI(ctrl)
	c := New(vm, guest)
	ctx := context.Background()

	dev := newTestDevice()
	g, _ := c.Reserve(ctx, dev)

	vm.EXPECT().AddDevice(gomock.Any(), g, gomock.Any()).Return(nil)
	guest.EXPECT().AddVPCIDevice(gomock.Any(), gomock.Any()).Return(nil)
	_ = c.AddToVM(ctx, g)

	vm.EXPECT().RemoveDevice(gomock.Any(), g).Return(nil)
	_ = c.RemoveFromVM(ctx, g)

	// Second call: device is no longer tracked.
	err := c.RemoveFromVM(ctx, g)
	if err == nil {
		t.Fatal("expected error when removing an already-removed device")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Scenario: AddToVM propagates correct HCS settings
// ─────────────────────────────────────────────────────────────────────────────

// TestAddToVM_HCSSettings verifies that AddToVM passes the correct VirtualPciDevice
// settings including PropagateNumaAffinity=true and the device instance path.
func TestAddToVM_HCSSettings(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmVPCI(ctrl)
	guest := mocks.NewMocklinuxGuestVPCI(ctrl)
	c := New(vm, guest)
	ctx := context.Background()

	dev := Device{
		DeviceInstanceID:     "PCI\\VEN_ABCD&DEV_1234\\99",
		VirtualFunctionIndex: 3,
	}
	g, _ := c.Reserve(ctx, dev)

	vm.EXPECT().
		AddDevice(gomock.Any(), g, gomock.AssignableToTypeOf(hcsschema.VirtualPciDevice{})).
		DoAndReturn(func(_ context.Context, _ guid.GUID, settings hcsschema.VirtualPciDevice) error {
			if settings.PropagateNumaAffinity == nil || !*settings.PropagateNumaAffinity {
				t.Error("expected PropagateNumaAffinity=true")
			}
			if len(settings.Functions) != 1 {
				t.Errorf("expected 1 function, got %d", len(settings.Functions))
			}
			fn := settings.Functions[0]
			if fn.DeviceInstancePath != dev.DeviceInstanceID {
				t.Errorf("expected DeviceInstancePath=%q, got %q", dev.DeviceInstanceID, fn.DeviceInstancePath)
			}
			if fn.VirtualFunction != dev.VirtualFunctionIndex {
				t.Errorf("expected VirtualFunction=%d, got %d", dev.VirtualFunctionIndex, fn.VirtualFunction)
			}
			return nil
		})
	guest.EXPECT().AddVPCIDevice(gomock.Any(), gomock.Any()).Return(nil)

	if err := c.AddToVM(ctx, g); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestAddToVM_GuestVMBusGUIDForwarded verifies that the correct vmBusGUID string
// is forwarded to the guest-side AddVPCIDevice call.
func TestAddToVM_GuestVMBusGUIDForwarded(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmVPCI(ctrl)
	guest := mocks.NewMocklinuxGuestVPCI(ctrl)
	c := New(vm, guest)
	ctx := context.Background()

	dev := newTestDevice()
	g, _ := c.Reserve(ctx, dev)

	vm.EXPECT().AddDevice(gomock.Any(), g, gomock.Any()).Return(nil)
	guest.EXPECT().
		AddVPCIDevice(gomock.Any(), gomock.AssignableToTypeOf(guestresource.LCOWMappedVPCIDevice{})).
		DoAndReturn(func(_ context.Context, req guestresource.LCOWMappedVPCIDevice) error {
			if req.VMBusGUID != g.String() {
				t.Errorf("expected VMBusGUID=%q, got %q", g.String(), req.VMBusGUID)
			}
			return nil
		})

	_ = c.AddToVM(ctx, g)
}
