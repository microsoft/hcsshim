//go:build windows && lcow

package disk

import (
	"context"
	"errors"
	"testing"

	"github.com/Microsoft/hcsshim/internal/controller/device/scsi/mount"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// --- Mock types ---

type mockGuestSCSIEjector struct {
	err error
}

func (m *mockGuestSCSIEjector) RemoveSCSIDevice(_ context.Context, _ guestresource.SCSIDevice) error {
	return m.err
}

type mockGuestSCSIMounter struct {
	err error
}

func (m *mockGuestSCSIMounter) AddMappedVirtualDisk(_ context.Context, _ guestresource.LCOWMappedVirtualDisk) error {
	return m.err
}

type mockGuestSCSIUnmounter struct {
	err error
}

func (m *mockGuestSCSIUnmounter) RemoveMappedVirtualDisk(_ context.Context, _ guestresource.LCOWMappedVirtualDisk) error {
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

// --- LCOW-specific tests ---

func TestDetachFromVM_FromAttached_WithGuest(t *testing.T) {
	d := attachedDisk(t)
	err := d.DetachFromVM(context.Background(), &mockVMSCSIRemover{}, &mockGuestSCSIEjector{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.State() != StateDetached {
		t.Errorf("expected state %d, got %d", StateDetached, d.State())
	}
}

func TestDetachFromVM_EjectError(t *testing.T) {
	d := attachedDisk(t)
	ejectErr := errors.New("eject failed")
	err := d.DetachFromVM(context.Background(), &mockVMSCSIRemover{}, &mockGuestSCSIEjector{err: ejectErr})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ejectErr) {
		t.Errorf("expected wrapped error %v, got %v", ejectErr, err)
	}
	// State should remain attached since eject failed before state transition.
	if d.State() != StateAttached {
		t.Errorf("expected state %d, got %d", StateAttached, d.State())
	}
}

func TestMountPartitionToGuest_Success(t *testing.T) {
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

func TestMountPartitionToGuest_MountError(t *testing.T) {
	d := attachedDisk(t)
	cfg := mount.Config{Partition: 1}
	_, err := d.ReservePartition(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ReservePartition: %v", err)
	}
	_, err = d.MountPartitionToGuest(context.Background(), 1, &mockGuestSCSIMounter{err: errors.New("mount fail")})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUnmountPartitionFromGuest_Success(t *testing.T) {
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

func TestUnmountPartitionFromGuest_UnmountError(t *testing.T) {
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
	unmountErr := errors.New("unmount fail")
	err = d.UnmountPartitionFromGuest(context.Background(), 1, &mockGuestSCSIUnmounter{err: unmountErr})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, unmountErr) {
		t.Errorf("expected wrapped error %v, got %v", unmountErr, err)
	}
}

func TestUnmountPartitionFromGuest_RemovesMountWhenFullyUnmounted(t *testing.T) {
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
	// After full unmount the partition should be removed, so detach should succeed
	// (no mounts remain).
	err = d.DetachFromVM(context.Background(), &mockVMSCSIRemover{}, newDefaultEjector())
	if err != nil {
		t.Fatalf("DetachFromVM after unmount: %v", err)
	}
	if d.State() != StateDetached {
		t.Errorf("expected state %d, got %d", StateDetached, d.State())
	}
}

func TestUnmountPartitionFromGuest_RetryAfterDetachFailure(t *testing.T) {
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
	// Unmount succeeds and removes the partition from the mounts map.
	err = d.UnmountPartitionFromGuest(context.Background(), 1, &mockGuestSCSIUnmounter{})
	if err != nil {
		t.Fatalf("UnmountPartitionFromGuest: %v", err)
	}
	// Detach fails (e.g. transient error).
	err = d.DetachFromVM(context.Background(), &mockVMSCSIRemover{err: errors.New("transient")}, newDefaultEjector())
	if err == nil {
		t.Fatal("expected detach error")
	}
	// Retry: unmount is a no-op since partition is gone.
	err = d.UnmountPartitionFromGuest(context.Background(), 1, &mockGuestSCSIUnmounter{})
	if err != nil {
		t.Fatalf("retry UnmountPartitionFromGuest should be no-op, got: %v", err)
	}
	// Retry detach now succeeds.
	err = d.DetachFromVM(context.Background(), &mockVMSCSIRemover{}, newDefaultEjector())
	if err != nil {
		t.Fatalf("retry DetachFromVM: %v", err)
	}
	if d.State() != StateDetached {
		t.Errorf("expected state %d, got %d", StateDetached, d.State())
	}
}
