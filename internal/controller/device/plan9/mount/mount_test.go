//go:build windows && lcow

package mount

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/Microsoft/hcsshim/internal/controller/device/plan9/mount/mocks"
)

var (
	errGuestMount   = errors.New("guest mount failed")
	errGuestUnmount = errors.New("guest unmount failed")
)

// ─────────────────────────────────────────────────────────────────────────────
// NewReserved
// ─────────────────────────────────────────────────────────────────────────────

// TestNewReserved_InitialState verifies that a freshly created mount starts
// in StateReserved with refCount=1 and the correct guest path derived from
// the share name.
func TestNewReserved_InitialState(t *testing.T) {
	m := NewReserved("share0", Config{ReadOnly: false})
	if m.State() != StateReserved {
		t.Errorf("expected StateReserved, got %v", m.State())
	}

	expected := fmt.Sprintf(GuestPathFmt, "share0")
	if m.GuestPath() != expected {
		t.Errorf("expected guest path %q, got %q", expected, m.GuestPath())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Reserve
// ─────────────────────────────────────────────────────────────────────────────

// TestReserve_SameConfig_IncrementsRefCount verifies that reserving with the
// same config increments the internal refCount from 1 to 2.
func TestReserve_SameConfig_IncrementsRefCount(t *testing.T) {
	m := NewReserved("share0", Config{ReadOnly: false})
	if err := m.Reserve(Config{ReadOnly: false}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.refCount != 2 {
		t.Errorf("expected refCount=2, got %d", m.refCount)
	}
}

// TestReserve_DifferentConfig_Errors verifies that reserving with a different
// config (e.g. ReadOnly mismatch) returns an error.
func TestReserve_DifferentConfig_Errors(t *testing.T) {
	m := NewReserved("share0", Config{ReadOnly: false})
	err := m.Reserve(Config{ReadOnly: true})
	if err == nil {
		t.Fatal("expected error when reserving with different config")
	}
}

// TestReserve_OnUnmountedMount_Errors verifies that calling Reserve on a mount
// that has already reached StateUnmounted returns an error; the terminal state
// cannot accept new reservations.
func TestReserve_OnUnmountedMount_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	guest := mocks.NewMockGuestPlan9Mounter(ctrl)
	guestUnmount := mocks.NewMockGuestPlan9Unmounter(ctrl)

	m := NewReserved("share0", Config{})

	// Mount then unmount to reach StateUnmounted.
	guest.EXPECT().AddLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil)
	_, _ = m.MountToGuest(context.Background(), guest)

	guestUnmount.EXPECT().RemoveLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil)
	_ = m.UnmountFromGuest(context.Background(), guestUnmount)

	if m.State() != StateUnmounted {
		t.Fatalf("expected StateUnmounted, got %v", m.State())
	}

	err := m.Reserve(Config{})
	if err == nil {
		t.Fatal("expected error when reserving on unmounted mount")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// MountToGuest
// ─────────────────────────────────────────────────────────────────────────────

// TestMountToGuest_HappyPath verifies that a mount in StateReserved transitions
// to StateMounted after a successful guest AddLCOWMappedDirectory call and
// returns the expected guest path.
func TestMountToGuest_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	guest := mocks.NewMockGuestPlan9Mounter(ctrl)

	m := NewReserved("share0", Config{ReadOnly: true})

	guest.EXPECT().AddLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil)

	guestPath, err := m.MountToGuest(context.Background(), guest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := fmt.Sprintf(GuestPathFmt, "share0")
	if guestPath != expected {
		t.Errorf("expected guest path %q, got %q", expected, guestPath)
	}
	if m.State() != StateMounted {
		t.Errorf("expected StateMounted, got %v", m.State())
	}
}

// TestMountToGuest_GuestFails_TransitionsToInvalid verifies that when the
// guest AddLCOWMappedDirectory call fails, the mount transitions directly from
// StateReserved to StateInvalid so outstanding reservations can be drained.
func TestMountToGuest_GuestFails_TransitionsToInvalid(t *testing.T) {
	ctrl := gomock.NewController(t)
	guest := mocks.NewMockGuestPlan9Mounter(ctrl)

	m := NewReserved("share0", Config{})

	guest.EXPECT().AddLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(errGuestMount)

	_, err := m.MountToGuest(context.Background(), guest)
	if err == nil {
		t.Fatal("expected error")
	}
	if m.State() != StateInvalid {
		t.Errorf("expected StateInvalid after mount failure, got %v", m.State())
	}
}

// TestMountToGuest_AlreadyMounted_Idempotent verifies that calling MountToGuest
// a second time on a StateMounted mount returns the existing guest path without
// issuing a new AddLCOWMappedDirectory call to the guest.
func TestMountToGuest_AlreadyMounted_Idempotent(t *testing.T) {
	ctrl := gomock.NewController(t)
	guest := mocks.NewMockGuestPlan9Mounter(ctrl)

	m := NewReserved("share0", Config{})

	// First mount — guest call must happen exactly once.
	guest.EXPECT().AddLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	_, _ = m.MountToGuest(context.Background(), guest)

	// Second mount on the same mount object — must NOT re-issue guest call.
	guestPath, err := m.MountToGuest(context.Background(), guest)
	if err != nil {
		t.Fatalf("unexpected error on second mount: %v", err)
	}
	if m.State() != StateMounted {
		t.Errorf("expected StateMounted, got %v", m.State())
	}
	expected := fmt.Sprintf(GuestPathFmt, "share0")
	if guestPath != expected {
		t.Errorf("expected %q, got %q", expected, guestPath)
	}
}

// TestMountToGuest_OnInvalid_Errors verifies that calling MountToGuest on a
// mount in StateInvalid (from a prior mount failure) returns an error.
func TestMountToGuest_OnInvalid_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	guest := mocks.NewMockGuestPlan9Mounter(ctrl)

	m := NewReserved("share0", Config{})
	guest.EXPECT().AddLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(errGuestMount)
	_, _ = m.MountToGuest(context.Background(), guest)

	// Try to mount again from StateInvalid.
	_, err := m.MountToGuest(context.Background(), guest)
	if err == nil {
		t.Fatal("expected error when mounting from StateInvalid")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// UnmountFromGuest
// ─────────────────────────────────────────────────────────────────────────────

// TestUnmountFromGuest_HappyPath verifies that unmounting a StateMounted mount
// with refCount=1 issues a guest RemoveLCOWMappedDirectory call and transitions
// the mount to the terminal StateUnmounted.
func TestUnmountFromGuest_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	guest := mocks.NewMockGuestPlan9Mounter(ctrl)
	guestUnmount := mocks.NewMockGuestPlan9Unmounter(ctrl)

	m := NewReserved("share0", Config{})
	guest.EXPECT().AddLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil)
	_, _ = m.MountToGuest(context.Background(), guest)

	guestUnmount.EXPECT().RemoveLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil)
	if err := m.UnmountFromGuest(context.Background(), guestUnmount); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.State() != StateUnmounted {
		t.Errorf("expected StateUnmounted, got %v", m.State())
	}
}

// TestUnmountFromGuest_GuestFails_StaysInMounted verifies that a failed guest
// RemoveLCOWMappedDirectory call leaves the mount in StateMounted so the caller
// can retry.
func TestUnmountFromGuest_GuestFails_StaysInMounted(t *testing.T) {
	ctrl := gomock.NewController(t)
	guest := mocks.NewMockGuestPlan9Mounter(ctrl)
	guestUnmount := mocks.NewMockGuestPlan9Unmounter(ctrl)

	m := NewReserved("share0", Config{})
	guest.EXPECT().AddLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil)
	_, _ = m.MountToGuest(context.Background(), guest)

	guestUnmount.EXPECT().RemoveLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(errGuestUnmount)
	err := m.UnmountFromGuest(context.Background(), guestUnmount)
	if err == nil {
		t.Fatal("expected error")
	}
	if m.State() != StateMounted {
		t.Errorf("expected state to remain StateMounted after failed unmount, got %v", m.State())
	}
}

// TestUnmountFromGuest_GuestFails_ThenRetrySucceeds verifies that after a failed
// guest unmount (mount stays StateMounted), a second UnmountFromGuest attempt
// succeeds and transitions the mount to StateUnmounted.
func TestUnmountFromGuest_GuestFails_ThenRetrySucceeds(t *testing.T) {
	ctrl := gomock.NewController(t)
	guest := mocks.NewMockGuestPlan9Mounter(ctrl)
	guestUnmount := mocks.NewMockGuestPlan9Unmounter(ctrl)

	m := NewReserved("share0", Config{})
	guest.EXPECT().AddLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil)
	_, _ = m.MountToGuest(context.Background(), guest)

	guestUnmount.EXPECT().RemoveLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(errGuestUnmount)
	_ = m.UnmountFromGuest(context.Background(), guestUnmount)

	// Retry with success.
	guestUnmount.EXPECT().RemoveLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil)
	if err := m.UnmountFromGuest(context.Background(), guestUnmount); err != nil {
		t.Fatalf("retry unmount failed: %v", err)
	}
	if m.State() != StateUnmounted {
		t.Errorf("expected StateUnmounted after retry, got %v", m.State())
	}
}

// TestUnmountFromGuest_RefCounting_NoGuestCallUntilLastRef verifies that with
// refCount=2, the first UnmountFromGuest only decrements the refCount (no guest
// call), and the second UnmountFromGuest issues the physical guest unmount and
// transitions the mount to StateUnmounted.
func TestUnmountFromGuest_RefCounting_NoGuestCallUntilLastRef(t *testing.T) {
	ctrl := gomock.NewController(t)
	guest := mocks.NewMockGuestPlan9Mounter(ctrl)
	guestUnmount := mocks.NewMockGuestPlan9Unmounter(ctrl)

	m := NewReserved("share0", Config{})
	// Add a second reservation.
	_ = m.Reserve(Config{})

	guest.EXPECT().AddLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil)
	_, _ = m.MountToGuest(context.Background(), guest)

	// First unmount: drops refCount to 1, no guest call.
	if err := m.UnmountFromGuest(context.Background(), guestUnmount); err != nil {
		t.Fatalf("first unmount error: %v", err)
	}
	if m.State() != StateMounted {
		t.Errorf("expected StateMounted after first unmount, got %v", m.State())
	}
	if m.refCount != 1 {
		t.Errorf("expected refCount=1 after first unmount, got %d", m.refCount)
	}

	// Second unmount: drops refCount to 0, guest call issued.
	guestUnmount.EXPECT().RemoveLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil)
	if err := m.UnmountFromGuest(context.Background(), guestUnmount); err != nil {
		t.Fatalf("second unmount error: %v", err)
	}
	if m.State() != StateUnmounted {
		t.Errorf("expected StateUnmounted after last unmount, got %v", m.State())
	}
}

// TestUnmountFromGuest_OnReserved_NoGuestCall verifies that unmounting a mount
// that was never mounted (still in StateReserved) transitions directly to
// StateUnmounted without issuing any guest call.
func TestUnmountFromGuest_OnReserved_NoGuestCall(t *testing.T) {
	ctrl := gomock.NewController(t)
	guestUnmount := mocks.NewMockGuestPlan9Unmounter(ctrl)

	// Never mounted — never calls guest unmount.
	m := NewReserved("share0", Config{})
	if err := m.UnmountFromGuest(context.Background(), guestUnmount); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.State() != StateUnmounted {
		t.Errorf("expected StateUnmounted, got %v", m.State())
	}
}

// TestUnmountFromGuest_OnUnmounted_Errors verifies that calling UnmountFromGuest
// on a mount already in the terminal StateUnmounted returns an error.
func TestUnmountFromGuest_OnUnmounted_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	guest := mocks.NewMockGuestPlan9Mounter(ctrl)
	guestUnmount := mocks.NewMockGuestPlan9Unmounter(ctrl)

	m := NewReserved("share0", Config{})
	guest.EXPECT().AddLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil)
	_, _ = m.MountToGuest(context.Background(), guest)

	guestUnmount.EXPECT().RemoveLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil)
	_ = m.UnmountFromGuest(context.Background(), guestUnmount)

	// Second unmount on StateUnmounted — must return an error.
	if err := m.UnmountFromGuest(context.Background(), guestUnmount); err == nil {
		t.Fatal("expected error when unmounting an already-unmounted mount")
	}
}
