//go:build windows && lcow

package share

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/Microsoft/hcsshim/internal/controller/device/plan9/mount"
	mountmocks "github.com/Microsoft/hcsshim/internal/controller/device/plan9/mount/mocks"
	"github.com/Microsoft/hcsshim/internal/controller/device/plan9/share/mocks"
)

var (
	errVMAdd    = errors.New("VM add failed")
	errVMRemove = errors.New("VM remove failed")
)

func newTestConfig() Config {
	return Config{
		HostPath: "/host/path",
		ReadOnly: false,
		Restrict: false,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// NewReserved
// ─────────────────────────────────────────────────────────────────────────────

// TestNewReserved_InitialState verifies that a freshly created share starts in
// StateReserved with the correct name and host path (no mount has been reserved
// yet).
func TestNewReserved_InitialState(t *testing.T) {
	s := NewReserved("share0", newTestConfig())
	if s.State() != StateReserved {
		t.Errorf("expected StateReserved, got %v", s.State())
	}
	if s.Name() != "share0" {
		t.Errorf("expected name %q, got %q", "share0", s.Name())
	}
	if s.HostPath() != "/host/path" {
		t.Errorf("expected host path %q, got %q", "/host/path", s.HostPath())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// AddToVM
// ─────────────────────────────────────────────────────────────────────────────

// TestAddToVM_HappyPath verifies that a StateReserved share transitions to
// StateAdded after a successful VM AddPlan9 call.
func TestAddToVM_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockVMPlan9Adder(ctrl)

	s := NewReserved("share0", newTestConfig())
	vm.EXPECT().AddPlan9(gomock.Any(), gomock.Any()).Return(nil)

	if err := s.AddToVM(context.Background(), vm); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.State() != StateAdded {
		t.Errorf("expected StateAdded, got %v", s.State())
	}
}

// TestAddToVM_VMFails_TransitionsToInvalid verifies that when the VM AddPlan9
// call fails, the share transitions to StateInvalid so that outstanding
// mount reservations can be drained before the share is fully removed.
func TestAddToVM_VMFails_TransitionsToInvalid(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockVMPlan9Adder(ctrl)

	s := NewReserved("share0", newTestConfig())
	vm.EXPECT().AddPlan9(gomock.Any(), gomock.Any()).Return(errVMAdd)

	err := s.AddToVM(context.Background(), vm)
	if err == nil {
		t.Fatal("expected error")
	}
	if s.State() != StateInvalid {
		t.Errorf("expected StateInvalid after VM add failure, got %v", s.State())
	}
}

// TestAddToVM_AlreadyAdded_Idempotent verifies that calling AddToVM a second
// time on a StateAdded share is a no-op that does not issue another VM call.
func TestAddToVM_AlreadyAdded_Idempotent(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockVMPlan9Adder(ctrl)

	s := NewReserved("share0", newTestConfig())
	// VM call must happen exactly once.
	vm.EXPECT().AddPlan9(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	_ = s.AddToVM(context.Background(), vm)

	// Second call must be a no-op.
	if err := s.AddToVM(context.Background(), vm); err != nil {
		t.Fatalf("second AddToVM returned unexpected error: %v", err)
	}
	if s.State() != StateAdded {
		t.Errorf("expected StateAdded, got %v", s.State())
	}
}

// TestAddToVM_OnInvalidShare_Errors verifies that calling AddToVM on a share in
// StateInvalid (previous add failed) returns an error without issuing any VM call.
func TestAddToVM_OnInvalidShare_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockVMPlan9Adder(ctrl)

	s := NewReserved("share0", newTestConfig())
	vm.EXPECT().AddPlan9(gomock.Any(), gomock.Any()).Return(errVMAdd)
	_ = s.AddToVM(context.Background(), vm)

	// Share is now StateInvalid — AddToVM must return an error.
	err := s.AddToVM(context.Background(), vm)
	if err == nil {
		t.Fatal("expected error when adding an invalid share")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RemoveFromVM
// ─────────────────────────────────────────────────────────────────────────────

// TestRemoveFromVM_OnReserved_TransitionsToRemoved verifies that removing a
// share that was never added to the VM (still StateReserved) transitions
// directly to StateRemoved without issuing a VM RemovePlan9 call.
func TestRemoveFromVM_OnReserved_TransitionsToRemoved(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockVMPlan9Remover(ctrl)

	s := NewReserved("share0", newTestConfig())

	// Share was never added — RemovePlan9 must NOT be called.
	if err := s.RemoveFromVM(context.Background(), vm); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.State() != StateRemoved {
		t.Errorf("expected StateRemoved, got %v", s.State())
	}
}

// TestRemoveFromVM_HappyPath verifies the normal AddToVM → RemoveFromVM flow:
// the VM RemovePlan9 call is issued and the share transitions to StateRemoved.
func TestRemoveFromVM_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	vmAdd := mocks.NewMockVMPlan9Adder(ctrl)
	vmRemove := mocks.NewMockVMPlan9Remover(ctrl)

	s := NewReserved("share0", newTestConfig())
	vmAdd.EXPECT().AddPlan9(gomock.Any(), gomock.Any()).Return(nil)
	_ = s.AddToVM(context.Background(), vmAdd)

	vmRemove.EXPECT().RemovePlan9(gomock.Any(), gomock.Any()).Return(nil)
	if err := s.RemoveFromVM(context.Background(), vmRemove); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.State() != StateRemoved {
		t.Errorf("expected StateRemoved, got %v", s.State())
	}
}

// TestRemoveFromVM_VMFails_StaysInAdded verifies that a failed VM RemovePlan9
// call leaves the share in StateAdded so the caller can retry.
func TestRemoveFromVM_VMFails_StaysInAdded(t *testing.T) {
	ctrl := gomock.NewController(t)
	vmAdd := mocks.NewMockVMPlan9Adder(ctrl)
	vmRemove := mocks.NewMockVMPlan9Remover(ctrl)

	s := NewReserved("share0", newTestConfig())
	vmAdd.EXPECT().AddPlan9(gomock.Any(), gomock.Any()).Return(nil)
	_ = s.AddToVM(context.Background(), vmAdd)

	vmRemove.EXPECT().RemovePlan9(gomock.Any(), gomock.Any()).Return(errVMRemove)
	err := s.RemoveFromVM(context.Background(), vmRemove)
	if err == nil {
		t.Fatal("expected error")
	}
	if s.State() != StateAdded {
		t.Errorf("expected StateAdded after failed remove, got %v", s.State())
	}
}

// TestRemoveFromVM_VMFails_ThenRetry verifies that after a failed VM remove
// (share stays StateAdded), a retry succeeds and transitions to StateRemoved.
func TestRemoveFromVM_VMFails_ThenRetry(t *testing.T) {
	ctrl := gomock.NewController(t)
	vmAdd := mocks.NewMockVMPlan9Adder(ctrl)
	vmRemove := mocks.NewMockVMPlan9Remover(ctrl)

	s := NewReserved("share0", newTestConfig())
	vmAdd.EXPECT().AddPlan9(gomock.Any(), gomock.Any()).Return(nil)
	_ = s.AddToVM(context.Background(), vmAdd)

	vmRemove.EXPECT().RemovePlan9(gomock.Any(), gomock.Any()).Return(errVMRemove)
	_ = s.RemoveFromVM(context.Background(), vmRemove)

	vmRemove.EXPECT().RemovePlan9(gomock.Any(), gomock.Any()).Return(nil)
	if err := s.RemoveFromVM(context.Background(), vmRemove); err != nil {
		t.Fatalf("retry RemoveFromVM failed: %v", err)
	}
	if s.State() != StateRemoved {
		t.Errorf("expected StateRemoved after retry, got %v", s.State())
	}
}

// TestRemoveFromVM_AlreadyRemoved_NoOp verifies that calling RemoveFromVM on a
// share already in the terminal StateRemoved is a no-op with no error and no
// duplicate VM call.
func TestRemoveFromVM_AlreadyRemoved_NoOp(t *testing.T) {
	ctrl := gomock.NewController(t)
	vmAdd := mocks.NewMockVMPlan9Adder(ctrl)
	vmRemove := mocks.NewMockVMPlan9Remover(ctrl)

	s := NewReserved("share0", newTestConfig())
	vmAdd.EXPECT().AddPlan9(gomock.Any(), gomock.Any()).Return(nil)
	_ = s.AddToVM(context.Background(), vmAdd)

	vmRemove.EXPECT().RemovePlan9(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	_ = s.RemoveFromVM(context.Background(), vmRemove)

	// Second remove on StateRemoved must be a no-op with no error.
	if err := s.RemoveFromVM(context.Background(), vmRemove); err != nil {
		t.Fatalf("unexpected error on second remove: %v", err)
	}
}

// TestRemoveFromVM_WithActiveMountSkipsRemove verifies that RemoveFromVM on a
// StateAdded share with an active mount reservation does not issue the VM
// RemovePlan9 call and keeps the share in StateAdded.
func TestRemoveFromVM_WithActiveMountSkipsRemove(t *testing.T) {
	ctrl := gomock.NewController(t)
	vmAdd := mocks.NewMockVMPlan9Adder(ctrl)
	vmRemove := mocks.NewMockVMPlan9Remover(ctrl)

	s := NewReserved("share0", newTestConfig())
	vmAdd.EXPECT().AddPlan9(gomock.Any(), gomock.Any()).Return(nil)
	_ = s.AddToVM(context.Background(), vmAdd)

	// Reserve a mount to simulate an active mount (mount != nil).
	_, _ = s.ReserveMount(context.Background(), mount.Config{})

	// RemovePlan9 must NOT be called while a mount is active.
	if err := s.RemoveFromVM(context.Background(), vmRemove); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.State() != StateAdded {
		t.Errorf("expected StateAdded (still held by mount), got %v", s.State())
	}
}

// TestRemoveFromVM_OnInvalid_WithActiveMount_NoOps verifies that RemoveFromVM
// on a StateInvalid share with an active mount reservation does not transition
// to StateRemoved and keeps the share in StateInvalid.
func TestRemoveFromVM_OnInvalid_WithActiveMount_NoOps(t *testing.T) {
	ctrl := gomock.NewController(t)
	vmAdd := mocks.NewMockVMPlan9Adder(ctrl)
	vmRemove := mocks.NewMockVMPlan9Remover(ctrl)

	s := NewReserved("share0", newTestConfig())
	// Reserve a mount before AddToVM to simulate the controller's Reserve flow.
	_, _ = s.ReserveMount(context.Background(), mount.Config{})

	// AddToVM fails → StateInvalid, but mount is still active.
	vmAdd.EXPECT().AddPlan9(gomock.Any(), gomock.Any()).Return(errVMAdd)
	_ = s.AddToVM(context.Background(), vmAdd)

	if s.State() != StateInvalid {
		t.Fatalf("expected StateInvalid, got %v", s.State())
	}

	// RemoveFromVM must not transition to Removed while mount is active.
	if err := s.RemoveFromVM(context.Background(), vmRemove); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.State() != StateInvalid {
		t.Errorf("expected StateInvalid (mount still active), got %v", s.State())
	}
}

// TestRemoveFromVM_OnInvalid_NoMount_TransitionsToRemoved verifies that
// RemoveFromVM on a StateInvalid share with no active mount transitions to
// StateRemoved.
func TestRemoveFromVM_OnInvalid_NoMount_TransitionsToRemoved(t *testing.T) {
	ctrl := gomock.NewController(t)
	vmAdd := mocks.NewMockVMPlan9Adder(ctrl)
	vmRemove := mocks.NewMockVMPlan9Remover(ctrl)
	guestUnmount := mountmocks.NewMockLinuxGuestPlan9Unmounter(ctrl)

	s := NewReserved("share0", newTestConfig())
	// Reserve a mount, then drain it so mount becomes nil.
	_, _ = s.ReserveMount(context.Background(), mount.Config{})

	// AddToVM fails → StateInvalid.
	vmAdd.EXPECT().AddPlan9(gomock.Any(), gomock.Any()).Return(errVMAdd)
	_ = s.AddToVM(context.Background(), vmAdd)

	// Drain the mount via UnmountFromGuest.
	if err := s.UnmountFromGuest(context.Background(), guestUnmount); err != nil {
		t.Fatalf("unexpected error during unmount: %v", err)
	}

	// Now RemoveFromVM should transition to Removed.
	if err := s.RemoveFromVM(context.Background(), vmRemove); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.State() != StateRemoved {
		t.Errorf("expected StateRemoved after draining mounts, got %v", s.State())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// ReserveMount
// ─────────────────────────────────────────────────────────────────────────────

// TestReserveMount_CreatesNewMount verifies that calling ReserveMount on a
// share without an existing mount creates a new mount in StateReserved.
func TestReserveMount_CreatesNewMount(t *testing.T) {
	s := NewReserved("share0", newTestConfig())

	m, err := s.ReserveMount(context.Background(), mount.Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil mount")
	}
	if m.State() != mount.StateReserved {
		t.Errorf("expected mount StateReserved, got %v", m.State())
	}
}

// TestReserveMount_SameConfig_ReturnsSameMount verifies that a second
// ReserveMount call with the same config returns the same mount object
// (bumping its refCount) instead of creating a new one.
func TestReserveMount_SameConfig_ReturnsSameMount(t *testing.T) {
	s := NewReserved("share0", newTestConfig())

	m1, _ := s.ReserveMount(context.Background(), mount.Config{ReadOnly: false})
	m2, err := s.ReserveMount(context.Background(), mount.Config{ReadOnly: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m1 != m2 {
		t.Error("expected same mount object returned for same config")
	}
}

// TestReserveMount_DifferentConfig_Errors verifies that reserving a mount with
// a different config than the existing mount returns an error.
func TestReserveMount_DifferentConfig_Errors(t *testing.T) {
	s := NewReserved("share0", newTestConfig())
	_, _ = s.ReserveMount(context.Background(), mount.Config{ReadOnly: false})

	_, err := s.ReserveMount(context.Background(), mount.Config{ReadOnly: true})
	if err == nil {
		t.Fatal("expected error when reserving mount with different config")
	}
}

// TestReserveMount_OnInvalidShare_Errors verifies that calling ReserveMount on
// a share in StateInvalid (previous add failed) returns an error.
func TestReserveMount_OnInvalidShare_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockVMPlan9Adder(ctrl)

	s := NewReserved("share0", newTestConfig())
	vm.EXPECT().AddPlan9(gomock.Any(), gomock.Any()).Return(errVMAdd)
	_ = s.AddToVM(context.Background(), vm)

	_, err := s.ReserveMount(context.Background(), mount.Config{})
	if err == nil {
		t.Fatal("expected error when reserving mount on invalid share")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// MountToGuest
// ─────────────────────────────────────────────────────────────────────────────

// TestMountToGuest_RequiresStateAdded verifies that MountToGuest fails when
// the share is still in StateReserved (not yet added to the VM).
func TestMountToGuest_RequiresStateAdded(t *testing.T) {
	ctrl := gomock.NewController(t)
	guestMount := mountmocks.NewMockLinuxGuestPlan9Mounter(ctrl)

	s := NewReserved("share0", newTestConfig())
	_, _ = s.ReserveMount(context.Background(), mount.Config{})

	// Share is in StateReserved — mount must fail.
	_, err := s.MountToGuest(context.Background(), guestMount)
	if err == nil {
		t.Fatal("expected error when mounting share not yet added to VM")
	}
}

// TestMountToGuest_RequiresReservedMount verifies that MountToGuest fails when
// no mount has been reserved on the share via ReserveMount.
func TestMountToGuest_RequiresReservedMount(t *testing.T) {
	ctrl := gomock.NewController(t)
	vmAdd := mocks.NewMockVMPlan9Adder(ctrl)
	guestMount := mountmocks.NewMockLinuxGuestPlan9Mounter(ctrl)

	s := NewReserved("share0", newTestConfig())
	vmAdd.EXPECT().AddPlan9(gomock.Any(), gomock.Any()).Return(nil)
	_ = s.AddToVM(context.Background(), vmAdd)

	// No mount reserved — must return error.
	_, err := s.MountToGuest(context.Background(), guestMount)
	if err == nil {
		t.Fatal("expected error when no mount is reserved")
	}
}

// TestMountToGuest_HappyPath verifies a full ReserveMount → AddToVM →
// MountToGuest flow: the guest AddLCOWMappedDirectory call succeeds and
// MountToGuest returns a non-empty guest path.
func TestMountToGuest_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	vmAdd := mocks.NewMockVMPlan9Adder(ctrl)
	guestMount := mountmocks.NewMockLinuxGuestPlan9Mounter(ctrl)

	s := NewReserved("share0", newTestConfig())
	vmAdd.EXPECT().AddPlan9(gomock.Any(), gomock.Any()).Return(nil)
	_ = s.AddToVM(context.Background(), vmAdd)
	_, _ = s.ReserveMount(context.Background(), mount.Config{})

	guestMount.EXPECT().AddLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil)
	guestPath, err := s.MountToGuest(context.Background(), guestMount)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if guestPath == "" {
		t.Error("expected non-empty guest path")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// UnmountFromGuest
// ─────────────────────────────────────────────────────────────────────────────

// TestUnmountFromGuest_HappyPath verifies a full mount → unmount cycle: after
// a successful guest RemoveLCOWMappedDirectory call the share's internal mount
// entry is cleared.
func TestUnmountFromGuest_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	vmAdd := mocks.NewMockVMPlan9Adder(ctrl)
	guestMount := mountmocks.NewMockLinuxGuestPlan9Mounter(ctrl)
	guestUnmount := mountmocks.NewMockLinuxGuestPlan9Unmounter(ctrl)

	s := NewReserved("share0", newTestConfig())
	vmAdd.EXPECT().AddPlan9(gomock.Any(), gomock.Any()).Return(nil)
	_ = s.AddToVM(context.Background(), vmAdd)
	_, _ = s.ReserveMount(context.Background(), mount.Config{})

	guestMount.EXPECT().AddLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil)
	_, _ = s.MountToGuest(context.Background(), guestMount)

	guestUnmount.EXPECT().RemoveLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil)
	if err := s.UnmountFromGuest(context.Background(), guestUnmount); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestUnmountFromGuest_NoMount_IsNoOp verifies that calling UnmountFromGuest on
// a share with no reserved mount is a no-op that returns no error.
func TestUnmountFromGuest_NoMount_IsNoOp(t *testing.T) {
	ctrl := gomock.NewController(t)
	guestUnmount := mountmocks.NewMockLinuxGuestPlan9Unmounter(ctrl)

	s := NewReserved("share0", newTestConfig())

	// No mount reserved — must be a no-op.
	if err := s.UnmountFromGuest(context.Background(), guestUnmount); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
