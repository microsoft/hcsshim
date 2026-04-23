//go:build windows && wcow

package mount

import (
	"context"
	"errors"
	"testing"

	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// --- Mock types ---

type mockMounter struct {
	err       error
	scratchFn func()
}

func (m *mockMounter) AddMappedVirtualDisk(_ context.Context, _ guestresource.WCOWMappedVirtualDisk) error {
	return m.err
}

func (m *mockMounter) AddMappedVirtualDiskForContainerScratch(_ context.Context, _ guestresource.WCOWMappedVirtualDisk) error {
	if m.scratchFn != nil {
		m.scratchFn()
	}
	return m.err
}

type mockUnmounter struct {
	err error
}

func (m *mockUnmounter) RemoveMappedVirtualDisk(_ context.Context, _ guestresource.WCOWMappedVirtualDisk) error {
	return m.err
}

// --- Helpers used by shared tests ---

func newDefaultMounter() GuestSCSIMounter {
	return &mockMounter{}
}

func newDefaultUnmounter() GuestSCSIUnmounter {
	return &mockUnmounter{}
}

func newFailingMounter(err error) GuestSCSIMounter {
	return &mockMounter{err: err}
}

func mountedMount(t *testing.T) *Mount {
	t.Helper()
	m := NewReserved(0, 0, defaultConfig())
	if _, err := m.MountToGuest(context.Background(), newDefaultMounter()); err != nil {
		t.Fatalf("setup MountToGuest: %v", err)
	}
	return m
}

// --- WCOW-specific tests ---

func TestConfigEquals(t *testing.T) {
	base := Config{
		ReadOnly:       true,
		FormatWithRefs: false,
	}
	same := Config{
		ReadOnly:       true,
		FormatWithRefs: false,
	}
	if !base.Equals(same) {
		t.Error("expected equal configs to be equal")
	}

	tests := []struct {
		name   string
		modify func(c *Config)
	}{
		{"ReadOnly", func(c *Config) { c.ReadOnly = false }},
		{"FormatWithRefs", func(c *Config) { c.FormatWithRefs = true }},
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

func TestMountToGuest_Success(t *testing.T) {
	m := NewReserved(0, 0, defaultConfig())
	guestPath, err := m.MountToGuest(context.Background(), &mockMounter{})
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

func TestMountToGuest_FormatWithRefs(t *testing.T) {
	scratchCalled := false
	m := NewReserved(0, 0, Config{Partition: 1, FormatWithRefs: true})
	wm := &mockMounter{scratchFn: func() { scratchCalled = true }}
	_, err := m.MountToGuest(context.Background(), wm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !scratchCalled {
		t.Error("expected AddMappedVirtualDiskForContainerScratch to be called")
	}
}

func TestUnmountFromGuest_Error(t *testing.T) {
	m := mountedMount(t)
	unmountErr := errors.New("wcow unmount failed")
	err := m.UnmountFromGuest(context.Background(), &mockUnmounter{err: unmountErr})
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
