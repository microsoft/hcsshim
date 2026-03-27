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

func (m *mockLinuxMounter) AddLCOWMappedVirtualDisk(_ context.Context, _ guestresource.LCOWMappedVirtualDisk) error {
	return m.err
}

type mockLinuxUnmounter struct {
	err error
}

func (m *mockLinuxUnmounter) RemoveLCOWMappedVirtualDisk(_ context.Context, _ guestresource.LCOWMappedVirtualDisk) error {
	return m.err
}

type mockWindowsMounter struct {
	err       error
	scratchFn func()
}

func (m *mockWindowsMounter) AddWCOWMappedVirtualDisk(_ context.Context, _ guestresource.WCOWMappedVirtualDisk) error {
	return m.err
}

func (m *mockWindowsMounter) AddWCOWMappedVirtualDiskForContainerScratch(_ context.Context, _ guestresource.WCOWMappedVirtualDisk) error {
	if m.scratchFn != nil {
		m.scratchFn()
	}
	return m.err
}

type mockWindowsUnmounter struct {
	err error
}

func (m *mockWindowsUnmounter) RemoveWCOWMappedVirtualDisk(_ context.Context, _ guestresource.WCOWMappedVirtualDisk) error {
	return m.err
}

// --- Helpers ---

func defaultConfig() Config {
	return Config{
		Partition: 1,
		ReadOnly:  false,
	}
}

func mountedLCOW(t *testing.T) *Mount {
	t.Helper()
	m := NewReserved(0, 0, defaultConfig())
	if _, err := m.MountToGuest(context.Background(), &mockLinuxMounter{}, nil); err != nil {
		t.Fatalf("setup MountToGuest: %v", err)
	}
	return m
}

func mountedWCOW(t *testing.T) *Mount {
	t.Helper()
	m := NewReserved(0, 0, defaultConfig())
	if _, err := m.MountToGuest(context.Background(), nil, &mockWindowsMounter{}); err != nil {
		t.Fatalf("setup MountToGuest WCOW: %v", err)
	}
	return m
}

// --- Tests ---

func TestNewReserved(t *testing.T) {
	cfg := Config{
		Partition:  2,
		ReadOnly:   true,
		Encrypted:  true,
		Filesystem: "ext4",
	}
	m := NewReserved(1, 3, cfg)
	if m.State() != StateReserved {
		t.Errorf("expected state %d, got %d", StateReserved, m.State())
	}
}

func TestConfigEquals(t *testing.T) {
	base := Config{
		ReadOnly:         true,
		Encrypted:        true,
		EnsureFilesystem: true,
		Filesystem:       "ext4",
		BlockDev:         false,
		FormatWithRefs:   false,
		Options:          []string{"rw", "noatime"},
	}
	same := Config{
		ReadOnly:         true,
		Encrypted:        true,
		EnsureFilesystem: true,
		Filesystem:       "ext4",
		BlockDev:         false,
		FormatWithRefs:   false,
		Options:          []string{"rw", "noatime"},
	}
	if !base.Equals(same) {
		t.Error("expected equal configs to be equal")
	}

	tests := []struct {
		name   string
		modify func(c *Config)
	}{
		{"ReadOnly", func(c *Config) { c.ReadOnly = false }},
		{"Encrypted", func(c *Config) { c.Encrypted = false }},
		{"EnsureFilesystem", func(c *Config) { c.EnsureFilesystem = false }},
		{"Filesystem", func(c *Config) { c.Filesystem = "xfs" }},
		{"BlockDev", func(c *Config) { c.BlockDev = true }},
		{"FormatWithRefs", func(c *Config) { c.FormatWithRefs = true }},
		{"Options", func(c *Config) { c.Options = []string{"ro"} }},
		{"OptionsNil", func(c *Config) { c.Options = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modified := same
			tt.modify(&modified)
			if base.Equals(modified) {
				t.Errorf("expected configs to differ on %s", tt.name)
			}
		})
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
	m := mountedLCOW(t)
	cfg := defaultConfig()
	if err := m.Reserve(cfg); err != nil {
		t.Fatalf("unexpected error reserving mounted mount: %v", err)
	}
}

func TestReserve_WhenUnmounted(t *testing.T) {
	m := mountedLCOW(t)
	_ = m.UnmountFromGuest(context.Background(), &mockLinuxUnmounter{}, nil)
	if m.State() != StateUnmounted {
		t.Fatalf("expected unmounted, got %d", m.State())
	}
	err := m.Reserve(defaultConfig())
	if err == nil {
		t.Fatal("expected error reserving unmounted mount")
	}
}

func TestMountToGuest_LCOW_Success(t *testing.T) {
	m := NewReserved(0, 0, defaultConfig())
	guestPath, err := m.MountToGuest(context.Background(), &mockLinuxMounter{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if guestPath == "" {
		t.Error("expected non-empty guestPath")
	}
	if m.State() != StateMounted {
		t.Errorf("expected state %d, got %d", StateMounted, m.State())
	}
}

func TestMountToGuest_WCOW_Success(t *testing.T) {
	m := NewReserved(0, 0, defaultConfig())
	guestPath, err := m.MountToGuest(context.Background(), nil, &mockWindowsMounter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if guestPath == "" {
		t.Error("expected non-empty guestPath")
	}
	if m.State() != StateMounted {
		t.Errorf("expected state %d, got %d", StateMounted, m.State())
	}
}

func TestMountToGuest_LCOW_Error(t *testing.T) {
	mountErr := errors.New("lcow mount failed")
	m := NewReserved(0, 0, defaultConfig())
	_, err := m.MountToGuest(context.Background(), &mockLinuxMounter{err: mountErr}, nil)
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

func TestMountToGuest_WCOW_Error(t *testing.T) {
	mountErr := errors.New("wcow mount failed")
	m := NewReserved(0, 0, defaultConfig())
	_, err := m.MountToGuest(context.Background(), nil, &mockWindowsMounter{err: mountErr})
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

func TestMountToGuest_WCOW_FormatWithRefs(t *testing.T) {
	scratchCalled := false
	m := NewReserved(0, 0, Config{Partition: 1, FormatWithRefs: true})
	wm := &mockWindowsMounter{scratchFn: func() { scratchCalled = true }}
	_, err := m.MountToGuest(context.Background(), nil, wm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !scratchCalled {
		t.Error("expected AddWCOWMappedVirtualDiskForContainerScratch to be called")
	}
}

func TestMountToGuest_Idempotent_WhenMounted(t *testing.T) {
	m := mountedLCOW(t)
	guestPath, err := m.MountToGuest(context.Background(), &mockLinuxMounter{}, nil)
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
	m := mountedLCOW(t)
	_ = m.UnmountFromGuest(context.Background(), &mockLinuxUnmounter{}, nil)
	_, err := m.MountToGuest(context.Background(), &mockLinuxMounter{}, nil)
	if err == nil {
		t.Fatal("expected error mounting an unmounted mount")
	}
}

func TestMountToGuest_ErrorWhenBothGuestsNil(t *testing.T) {
	m := NewReserved(0, 0, defaultConfig())
	_, err := m.MountToGuest(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error when both guests are nil")
	}
}

func TestUnmountFromGuest_LCOW_Success(t *testing.T) {
	m := mountedLCOW(t)
	err := m.UnmountFromGuest(context.Background(), &mockLinuxUnmounter{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.State() != StateUnmounted {
		t.Errorf("expected state %d, got %d", StateUnmounted, m.State())
	}
}

func TestUnmountFromGuest_WCOW_Success(t *testing.T) {
	m := mountedWCOW(t)
	err := m.UnmountFromGuest(context.Background(), nil, &mockWindowsUnmounter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.State() != StateUnmounted {
		t.Errorf("expected state %d, got %d", StateUnmounted, m.State())
	}
}

func TestUnmountFromGuest_LCOW_Error(t *testing.T) {
	m := mountedLCOW(t)
	unmountErr := errors.New("lcow unmount failed")
	err := m.UnmountFromGuest(context.Background(), &mockLinuxUnmounter{err: unmountErr}, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, unmountErr) {
		t.Errorf("expected wrapped error %v, got %v", unmountErr, err)
	}
	// State should remain mounted since unmount failed.
	if m.State() != StateMounted {
		t.Errorf("expected state %d, got %d", StateMounted, m.State())
	}
}

func TestUnmountFromGuest_WCOW_Error(t *testing.T) {
	m := mountedWCOW(t)
	unmountErr := errors.New("wcow unmount failed")
	err := m.UnmountFromGuest(context.Background(), nil, &mockWindowsUnmounter{err: unmountErr})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, unmountErr) {
		t.Errorf("expected wrapped error %v, got %v", unmountErr, err)
	}
	if m.State() != StateMounted {
		t.Errorf("expected state %d, got %d", StateMounted, m.State())
	}
}

func TestUnmountFromGuest_FromReserved_DecrementsRefCount(t *testing.T) {
	m := NewReserved(0, 0, defaultConfig())
	err := m.UnmountFromGuest(context.Background(), &mockLinuxUnmounter{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should still be reserved since no guest mount was done.
	if m.State() != StateReserved {
		t.Errorf("expected state %d, got %d", StateReserved, m.State())
	}
}

func TestUnmountFromGuest_ErrorWhenUnmounted(t *testing.T) {
	m := mountedLCOW(t)
	_ = m.UnmountFromGuest(context.Background(), &mockLinuxUnmounter{}, nil)
	err := m.UnmountFromGuest(context.Background(), &mockLinuxUnmounter{}, nil)
	if err == nil {
		t.Fatal("expected error unmounting already-unmounted mount")
	}
}

func TestUnmountFromGuest_ErrorWhenBothGuestsNil(t *testing.T) {
	m := mountedLCOW(t)
	err := m.UnmountFromGuest(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error when both guests are nil")
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
	if _, err := m.MountToGuest(context.Background(), &mockLinuxMounter{}, nil); err != nil {
		t.Fatalf("MountToGuest: %v", err)
	}
	// First unmount should decrement ref but stay mounted.
	if err := m.UnmountFromGuest(context.Background(), &mockLinuxUnmounter{}, nil); err != nil {
		t.Fatalf("first UnmountFromGuest: %v", err)
	}
	if m.State() != StateMounted {
		t.Errorf("expected state %d after first unmount, got %d", StateMounted, m.State())
	}
	// Second unmount should actually unmount.
	if err := m.UnmountFromGuest(context.Background(), &mockLinuxUnmounter{}, nil); err != nil {
		t.Fatalf("second UnmountFromGuest: %v", err)
	}
	if m.State() != StateUnmounted {
		t.Errorf("expected state %d after final unmount, got %d", StateUnmounted, m.State())
	}
}
