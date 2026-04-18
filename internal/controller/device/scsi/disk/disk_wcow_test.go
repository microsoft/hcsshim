//go:build windows && wcow

package disk

import (
	"context"
	"errors"
	"testing"

	"github.com/Microsoft/hcsshim/internal/controller/device/scsi/mount"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// --- Mock types ---

// mockGuestSCSIEjector is a no-op for WCOW guests (the interface is empty).
type mockGuestSCSIEjector struct{}

type mockGuestSCSIMounter struct {
	err       error
	scratchFn func()
}

func (m *mockGuestSCSIMounter) AddWCOWMappedVirtualDisk(_ context.Context, _ guestresource.WCOWMappedVirtualDisk) error {
	return m.err
}

func (m *mockGuestSCSIMounter) AddWCOWMappedVirtualDiskForContainerScratch(_ context.Context, _ guestresource.WCOWMappedVirtualDisk) error {
	if m.scratchFn != nil {
		m.scratchFn()
	}
	return m.err
}

type mockGuestSCSIUnmounter struct {
	err error
}

func (m *mockGuestSCSIUnmounter) RemoveWCOWMappedVirtualDisk(_ context.Context, _ guestresource.WCOWMappedVirtualDisk) error {
	return m.err
}

// --- Helpers used by shared tests ---

func newDefaultEjector() GuestSCSIEjector {
	return &mockGuestSCSIEjector{}
}

func newDefaultMounter() mount.GuestSCSIMounter {
	return &mockGuestSCSIMounter{}
}

func newDefaultUnmounter() mount.GuestSCSIUnmounter {
	return &mockGuestSCSIUnmounter{}
}

// --- WCOW-specific tests ---

func TestMountPartitionToGuest_WCOW_Success(t *testing.T) {
	d := attachedDisk(t)
	cfg := mount.Config{Partition: 1}
	_, err := d.ReservePartition(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ReservePartition: %v", err)
	}
	guestPath, err := d.MountPartitionToGuest(context.Background(), 1, &mockGuestSCSIMounter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if guestPath == "" {
		t.Error("expected non-empty guestPath")
	}
}

func TestMountPartitionToGuest_WCOW_MountError(t *testing.T) {
	d := attachedDisk(t)
	cfg := mount.Config{Partition: 1}
	_, err := d.ReservePartition(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ReservePartition: %v", err)
	}
	_, err = d.MountPartitionToGuest(context.Background(), 1, &mockGuestSCSIMounter{err: errors.New("wcow mount fail")})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestMountPartitionToGuest_WCOW_FormatWithRefs(t *testing.T) {
	d := attachedDisk(t)
	cfg := mount.Config{Partition: 1, FormatWithRefs: true}
	_, err := d.ReservePartition(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ReservePartition: %v", err)
	}
	guestPath, err := d.MountPartitionToGuest(context.Background(), 1, &mockGuestSCSIMounter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if guestPath == "" {
		t.Error("expected non-empty guestPath")
	}
}

func TestUnmountPartitionFromGuest_WCOW_Success(t *testing.T) {
	d := attachedDisk(t)
	cfg := mount.Config{Partition: 1}
	_, err := d.ReservePartition(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ReservePartition: %v", err)
	}
	_, err = d.MountPartitionToGuest(context.Background(), 1, &mockGuestSCSIMounter{})
	if err != nil {
		t.Fatalf("MountPartitionToGuest: %v", err)
	}
	err = d.UnmountPartitionFromGuest(context.Background(), 1, &mockGuestSCSIUnmounter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnmountPartitionFromGuest_WCOW_UnmountError(t *testing.T) {
	d := attachedDisk(t)
	cfg := mount.Config{Partition: 1}
	_, err := d.ReservePartition(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ReservePartition: %v", err)
	}
	_, err = d.MountPartitionToGuest(context.Background(), 1, &mockGuestSCSIMounter{})
	if err != nil {
		t.Fatalf("MountPartitionToGuest: %v", err)
	}
	unmountErr := errors.New("wcow unmount fail")
	err = d.UnmountPartitionFromGuest(context.Background(), 1, &mockGuestSCSIUnmounter{err: unmountErr})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, unmountErr) {
		t.Errorf("expected wrapped error %v, got %v", unmountErr, err)
	}
}

func TestUnmountPartitionFromGuest_WCOW_RemovesMountWhenFullyUnmounted(t *testing.T) {
	d := attachedDisk(t)
	cfg := mount.Config{Partition: 1}
	_, err := d.ReservePartition(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ReservePartition: %v", err)
	}
	_, err = d.MountPartitionToGuest(context.Background(), 1, &mockGuestSCSIMounter{})
	if err != nil {
		t.Fatalf("MountPartitionToGuest: %v", err)
	}
	err = d.UnmountPartitionFromGuest(context.Background(), 1, &mockGuestSCSIUnmounter{})
	if err != nil {
		t.Fatalf("UnmountPartitionFromGuest: %v", err)
	}
	err = d.DetachFromVM(context.Background(), &mockVMSCSIRemover{}, newDefaultEjector())
	if err != nil {
		t.Fatalf("DetachFromVM after unmount: %v", err)
	}
	if d.State() != StateDetached {
		t.Errorf("expected state %d, got %d", StateDetached, d.State())
	}
}
