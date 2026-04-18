//go:build windows && wcow

package scsi

import (
	"context"
	"errors"
	"testing"

	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// --- Mock type ---

type mockGuestOps struct {
	mountErr   error
	unmountErr error
}

func (m *mockGuestOps) AddWCOWMappedVirtualDisk(_ context.Context, _ guestresource.WCOWMappedVirtualDisk) error {
	return m.mountErr
}

func (m *mockGuestOps) AddWCOWMappedVirtualDiskForContainerScratch(_ context.Context, _ guestresource.WCOWMappedVirtualDisk) error {
	return m.mountErr
}

func (m *mockGuestOps) RemoveWCOWMappedVirtualDisk(_ context.Context, _ guestresource.WCOWMappedVirtualDisk) error {
	return m.unmountErr
}

// --- Helpers used by shared tests ---

func newMockGuestOps() *mockGuestOps {
	return &mockGuestOps{}
}

// --- WCOW-specific tests ---

func TestMapToGuest_MountError(t *testing.T) {
	guest := &mockGuestOps{mountErr: errors.New("mount failed")}
	c := New(1, &mockVMOps{}, guest)
	id, err := c.Reserve(context.Background(), defaultDiskConfig(), defaultMountConfig())
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	_, err = c.MapToGuest(context.Background(), id)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUnmapFromGuest_UnmountError(t *testing.T) {
	guest := &mockGuestOps{}
	c := New(1, &mockVMOps{}, guest)
	dc := defaultDiskConfig()
	mc := defaultMountConfig()

	id, err := c.Reserve(context.Background(), dc, mc)
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if _, err := c.MapToGuest(context.Background(), id); err != nil {
		t.Fatalf("MapToGuest: %v", err)
	}
	// Now inject an unmount error.
	guest.unmountErr = errors.New("unmount failed")
	err = c.UnmapFromGuest(context.Background(), id)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUnmapFromGuest_WindowsGuest(t *testing.T) {
	guest := &mockGuestOps{}
	c := New(1, &mockVMOps{}, guest)
	dc := defaultDiskConfig()
	mc := defaultMountConfig()

	id, err := c.Reserve(context.Background(), dc, mc)
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if _, err := c.MapToGuest(context.Background(), id); err != nil {
		t.Fatalf("MapToGuest: %v", err)
	}
	if err := c.UnmapFromGuest(context.Background(), id); err != nil {
		t.Fatalf("UnmapFromGuest: %v", err)
	}
}
