//go:build windows

package mount

import (
	"context"
	"errors"
	"testing"

	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// --- Mock types ---

type mockLinuxMounter struct {
	err error
}

func (m *mockLinuxMounter) AddLCOWMappedVPMemDevice(_ context.Context, _ guestresource.LCOWMappedVPMemDevice) error {
	return m.err
}

type mockLinuxUnmounter struct {
	err error
}

func (m *mockLinuxUnmounter) RemoveLCOWMappedVPMemDevice(_ context.Context, _ guestresource.LCOWMappedVPMemDevice) error {
	return m.err
}

// --- Helpers ---

func defaultMountConfig() MountConfig {
	return MountConfig{}
}

func mountedMount(t *testing.T) *Mount {
	t.Helper()
	m := NewReserved(0, defaultMountConfig())
	if _, err := m.MountToGuest(context.Background(), &mockLinuxMounter{}); err != nil {
		t.Fatalf("setup MountToGuest: %v", err)
	}
	return m
}

// --- Tests ---

func TestNewReserved(t *testing.T) {
	cfg := MountConfig{}
	m := NewReserved(5, cfg)
	if m.State() != MountStateReserved {
		t.Errorf("expected state %d, got %d", MountStateReserved, m.State())
	}
}

func TestMountConfigEquals(t *testing.T) {
	a := MountConfig{}
	b := MountConfig{}
	if !a.Equals(b) {
		t.Error("expected equal configs to be equal")
	}
}

func TestReserve_SameConfig(t *testing.T) {
	cfg := defaultMountConfig()
	m := NewReserved(0, cfg)
	if err := m.Reserve(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReserve_WhenMounted(t *testing.T) {
	m := mountedMount(t)
	cfg := defaultMountConfig()
	if err := m.Reserve(cfg); err != nil {
		t.Fatalf("unexpected error reserving mounted mount: %v", err)
	}
}

func TestReserve_WhenUnmounted(t *testing.T) {
	m := mountedMount(t)
	_ = m.UnmountFromGuest(context.Background(), &mockLinuxUnmounter{})
	if m.State() != MountStateUnmounted {
		t.Fatalf("expected unmounted, got %d", m.State())
	}
	err := m.Reserve(defaultMountConfig())
	if err == nil {
		t.Fatal("expected error reserving unmounted mount")
	}
}

func TestMountToGuest_Success(t *testing.T) {
	m := NewReserved(0, defaultMountConfig())
	guestPath, err := m.MountToGuest(context.Background(), &mockLinuxMounter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if guestPath == "" {
		t.Error("expected non-empty guestPath")
	}
	if m.State() != MountStateMounted {
		t.Errorf("expected state %d, got %d", MountStateMounted, m.State())
	}
}

func TestMountToGuest_Error(t *testing.T) {
	mountErr := errors.New("mount failed")
	m := NewReserved(0, defaultMountConfig())
	_, err := m.MountToGuest(context.Background(), &mockLinuxMounter{err: mountErr})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, mountErr) {
		t.Errorf("expected wrapped error %v, got %v", mountErr, err)
	}
	if m.State() != MountStateUnmounted {
		t.Errorf("expected state %d after failure, got %d", MountStateUnmounted, m.State())
	}
}

func TestMountToGuest_Idempotent_WhenMounted(t *testing.T) {
	m := mountedMount(t)
	guestPath, err := m.MountToGuest(context.Background(), &mockLinuxMounter{})
	if err != nil {
		t.Fatalf("unexpected error on idempotent mount: %v", err)
	}
	if guestPath == "" {
		t.Error("expected non-empty guestPath on idempotent mount")
	}
	if m.State() != MountStateMounted {
		t.Errorf("expected state %d, got %d", MountStateMounted, m.State())
	}
}

func TestMountToGuest_ErrorWhenUnmounted(t *testing.T) {
	m := mountedMount(t)
	_ = m.UnmountFromGuest(context.Background(), &mockLinuxUnmounter{})
	_, err := m.MountToGuest(context.Background(), &mockLinuxMounter{})
	if err == nil {
		t.Fatal("expected error mounting an unmounted mount")
	}
}

func TestUnmountFromGuest_Success(t *testing.T) {
	m := mountedMount(t)
	err := m.UnmountFromGuest(context.Background(), &mockLinuxUnmounter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.State() != MountStateUnmounted {
		t.Errorf("expected state %d, got %d", MountStateUnmounted, m.State())
	}
}

func TestUnmountFromGuest_Error(t *testing.T) {
	m := mountedMount(t)
	unmountErr := errors.New("unmount failed")
	err := m.UnmountFromGuest(context.Background(), &mockLinuxUnmounter{err: unmountErr})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, unmountErr) {
		t.Errorf("expected wrapped error %v, got %v", unmountErr, err)
	}
	// State should remain mounted since unmount failed.
	if m.State() != MountStateMounted {
		t.Errorf("expected state %d, got %d", MountStateMounted, m.State())
	}
}

func TestUnmountFromGuest_FromReserved_DecrementsRefCount(t *testing.T) {
	m := NewReserved(0, defaultMountConfig())
	err := m.UnmountFromGuest(context.Background(), &mockLinuxUnmounter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should still be reserved since no guest mount was done.
	if m.State() != MountStateReserved {
		t.Errorf("expected state %d, got %d", MountStateReserved, m.State())
	}
}

func TestUnmountFromGuest_ErrorWhenUnmounted(t *testing.T) {
	m := mountedMount(t)
	_ = m.UnmountFromGuest(context.Background(), &mockLinuxUnmounter{})
	err := m.UnmountFromGuest(context.Background(), &mockLinuxUnmounter{})
	if err == nil {
		t.Fatal("expected error unmounting already-unmounted mount")
	}
}

func TestUnmountFromGuest_MultipleRefs_DoesNotUnmountUntilLastRef(t *testing.T) {
	cfg := defaultMountConfig()
	m := NewReserved(0, cfg)
	// Add a second reservation.
	if err := m.Reserve(cfg); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	// Mount the guest.
	if _, err := m.MountToGuest(context.Background(), &mockLinuxMounter{}); err != nil {
		t.Fatalf("MountToGuest: %v", err)
	}
	// First unmount should decrement ref but stay mounted.
	if err := m.UnmountFromGuest(context.Background(), &mockLinuxUnmounter{}); err != nil {
		t.Fatalf("first UnmountFromGuest: %v", err)
	}
	if m.State() != MountStateMounted {
		t.Errorf("expected state %d after first unmount, got %d", MountStateMounted, m.State())
	}
	// Second unmount should actually unmount.
	if err := m.UnmountFromGuest(context.Background(), &mockLinuxUnmounter{}); err != nil {
		t.Fatalf("second UnmountFromGuest: %v", err)
	}
	if m.State() != MountStateUnmounted {
		t.Errorf("expected state %d after final unmount, got %d", MountStateUnmounted, m.State())
	}
}
