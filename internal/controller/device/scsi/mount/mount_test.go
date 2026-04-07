//go:build (windows && lcow) || (windows && wcow)

package mount

import (
	"context"
	"errors"
	"testing"
)

// --- Helpers ---

func defaultConfig() Config {
	return Config{
		Partition: 1,
		ReadOnly:  false,
	}
}

// --- Tests ---

func TestNewReserved(t *testing.T) {
	cfg := Config{
		Partition: 2,
		ReadOnly:  true,
	}
	m := NewReserved(1, 3, cfg)
	if m.State() != StateReserved {
		t.Errorf("expected state %d, got %d", StateReserved, m.State())
	}
}

func TestReserve_SameConfig(t *testing.T) {
	cfg := defaultConfig()
	m := NewReserved(0, 0, cfg)
	if err := m.Reserve(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReserve_DifferentConfig(t *testing.T) {
	m := NewReserved(0, 0, Config{ReadOnly: true})
	err := m.Reserve(Config{ReadOnly: false})
	if err == nil {
		t.Fatal("expected error for different config")
	}
}

func TestReserve_WhenMounted(t *testing.T) {
	m := mountedMount(t)
	cfg := defaultConfig()
	if err := m.Reserve(cfg); err != nil {
		t.Fatalf("unexpected error reserving mounted mount: %v", err)
	}
}

func TestReserve_WhenUnmounted(t *testing.T) {
	m := mountedMount(t)
	_ = m.UnmountFromGuest(context.Background(), newDefaultUnmounter())
	if m.State() != StateUnmounted {
		t.Fatalf("expected unmounted, got %d", m.State())
	}
	err := m.Reserve(defaultConfig())
	if err == nil {
		t.Fatal("expected error reserving unmounted mount")
	}
}

func TestMountToGuest_Idempotent_WhenMounted(t *testing.T) {
	m := mountedMount(t)
	guestPath, err := m.MountToGuest(context.Background(), newDefaultMounter())
	if err != nil {
		t.Fatalf("unexpected error on idempotent mount: %v", err)
	}
	if guestPath == "" {
		t.Error("expected non-empty guestPath on idempotent mount")
	}
	if m.State() != StateMounted {
		t.Errorf("expected state %d, got %d", StateMounted, m.State())
	}
}

func TestMountToGuest_ErrorWhenUnmounted(t *testing.T) {
	m := mountedMount(t)
	_ = m.UnmountFromGuest(context.Background(), newDefaultUnmounter())
	_, err := m.MountToGuest(context.Background(), newDefaultMounter())
	if err == nil {
		t.Fatal("expected error mounting an unmounted mount")
	}
}

func TestMountToGuest_Error(t *testing.T) {
	mountErr := errors.New("mount failed")
	m := NewReserved(0, 0, defaultConfig())
	_, err := m.MountToGuest(context.Background(), newFailingMounter(mountErr))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, mountErr) {
		t.Errorf("expected wrapped error %v, got %v", mountErr, err)
	}
	if m.State() != StateUnmounted {
		t.Errorf("expected state %d after failure, got %d", StateUnmounted, m.State())
	}
}

func TestUnmountFromGuest_FromReserved_DecrementsRefCount(t *testing.T) {
	m := NewReserved(0, 0, defaultConfig())
	err := m.UnmountFromGuest(context.Background(), newDefaultUnmounter())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should still be reserved since no guest mount was done.
	if m.State() != StateReserved {
		t.Errorf("expected state %d, got %d", StateReserved, m.State())
	}
}

func TestUnmountFromGuest_ErrorWhenUnmounted(t *testing.T) {
	m := mountedMount(t)
	_ = m.UnmountFromGuest(context.Background(), newDefaultUnmounter())
	err := m.UnmountFromGuest(context.Background(), newDefaultUnmounter())
	if err == nil {
		t.Fatal("expected error unmounting already-unmounted mount")
	}
}

func TestUnmountFromGuest_MultipleRefs_DoesNotUnmountUntilLastRef(t *testing.T) {
	cfg := defaultConfig()
	m := NewReserved(0, 0, cfg)
	// Add a second reservation.
	if err := m.Reserve(cfg); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	// Mount the guest.
	if _, err := m.MountToGuest(context.Background(), newDefaultMounter()); err != nil {
		t.Fatalf("MountToGuest: %v", err)
	}
	// First unmount should decrement ref but stay mounted.
	if err := m.UnmountFromGuest(context.Background(), newDefaultUnmounter()); err != nil {
		t.Fatalf("first UnmountFromGuest: %v", err)
	}
	if m.State() != StateMounted {
		t.Errorf("expected state %d after first unmount, got %d", StateMounted, m.State())
	}
	// Second unmount should actually unmount.
	if err := m.UnmountFromGuest(context.Background(), newDefaultUnmounter()); err != nil {
		t.Fatalf("second UnmountFromGuest: %v", err)
	}
	if m.State() != StateUnmounted {
		t.Errorf("expected state %d after final unmount, got %d", StateUnmounted, m.State())
	}
}
