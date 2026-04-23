//go:build windows && lcow

package mount

import (
	"context"
	"errors"
	"testing"

	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// --- Mock types ---

type mockMounter struct {
	err error
}

func (m *mockMounter) AddMappedVirtualDisk(_ context.Context, _ guestresource.LCOWMappedVirtualDisk) error {
	return m.err
}

type mockUnmounter struct {
	err error
}

func (m *mockUnmounter) RemoveMappedVirtualDisk(_ context.Context, _ guestresource.LCOWMappedVirtualDisk) error {
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

// --- LCOW-specific tests ---

func TestConfigEquals(t *testing.T) {
	base := Config{
		ReadOnly:         true,
		Encrypted:        true,
		EnsureFilesystem: true,
		Filesystem:       "ext4",
		BlockDev:         false,
		Options:          []string{"rw", "noatime"},
	}
	same := Config{
		ReadOnly:         true,
		Encrypted:        true,
		EnsureFilesystem: true,
		Filesystem:       "ext4",
		BlockDev:         false,
		Options:          []string{"rw", "noatime"},
	}
	if !base.Equals(same) {
		t.Error("expected equal configs to be equal")
	}

	// Options comparison should be order-insensitive.
	reordered := same
	reordered.Options = []string{"noatime", "rw"}
	if !base.Equals(reordered) {
		t.Error("expected configs with reordered options to be equal")
	}

	// Options comparison should be case-insensitive.
	uppercased := same
	uppercased.Options = []string{"RW", "Noatime"}
	if !base.Equals(uppercased) {
		t.Error("expected configs with differently-cased options to be equal")
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

func TestUnmountFromGuest_Error(t *testing.T) {
	m := mountedMount(t)
	unmountErr := errors.New("lcow unmount failed")
	err := m.UnmountFromGuest(context.Background(), &mockUnmounter{err: unmountErr})
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
