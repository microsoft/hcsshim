//go:build windows

package disk

import (
	"context"
	"errors"
	"testing"

	"github.com/Microsoft/hcsshim/internal/controller/device/scsi/mount"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// --- Mock types ---

type mockVMSCSIAdder struct {
	err error
}

func (m *mockVMSCSIAdder) AddSCSIDisk(_ context.Context, _ hcsschema.Attachment, _ uint, _ uint) error {
	return m.err
}

type mockVMSCSIRemover struct {
	err error
}

func (m *mockVMSCSIRemover) RemoveSCSIDisk(_ context.Context, _ uint, _ uint) error {
	return m.err
}

type mockLinuxGuestSCSIEjector struct {
	err error
}

func (m *mockLinuxGuestSCSIEjector) RemoveSCSIDevice(_ context.Context, _ guestresource.SCSIDevice) error {
	return m.err
}

type mockLinuxGuestSCSIMounter struct {
	err error
}

func (m *mockLinuxGuestSCSIMounter) AddLCOWMappedVirtualDisk(_ context.Context, _ guestresource.LCOWMappedVirtualDisk) error {
	return m.err
}

type mockLinuxGuestSCSIUnmounter struct {
	err error
}

func (m *mockLinuxGuestSCSIUnmounter) RemoveLCOWMappedVirtualDisk(_ context.Context, _ guestresource.LCOWMappedVirtualDisk) error {
	return m.err
}

type mockWindowsGuestSCSIMounter struct {
	err error
}

func (m *mockWindowsGuestSCSIMounter) AddWCOWMappedVirtualDisk(_ context.Context, _ guestresource.WCOWMappedVirtualDisk) error {
	return m.err
}

func (m *mockWindowsGuestSCSIMounter) AddWCOWMappedVirtualDiskForContainerScratch(_ context.Context, _ guestresource.WCOWMappedVirtualDisk) error {
	return m.err
}

type mockWindowsGuestSCSIUnmounter struct {
	err error
}

func (m *mockWindowsGuestSCSIUnmounter) RemoveWCOWMappedVirtualDisk(_ context.Context, _ guestresource.WCOWMappedVirtualDisk) error {
	return m.err
}

// --- Helpers ---

func defaultConfig() Config {
	return Config{
		HostPath: `C:\test\disk.vhdx`,
		ReadOnly: false,
		Type:     TypeVirtualDisk,
	}
}

func attachedDisk(t *testing.T) *Disk {
	t.Helper()
	d := NewReserved(0, 0, defaultConfig())
	if err := d.AttachToVM(context.Background(), &mockVMSCSIAdder{}); err != nil {
		t.Fatalf("setup AttachToVM: %v", err)
	}
	return d
}

// --- Tests ---

func TestNewReserved(t *testing.T) {
	cfg := Config{
		HostPath: `C:\test\disk.vhdx`,
		ReadOnly: true,
		Type:     TypePassThru,
		EVDType:  "evd-type",
	}
	d := NewReserved(1, 2, cfg)

	if d.State() != StateReserved {
		t.Errorf("expected state %d, got %d", StateReserved, d.State())
	}
	if d.Config() != cfg {
		t.Errorf("expected config %+v, got %+v", cfg, d.Config())
	}
	if d.HostPath() != cfg.HostPath {
		t.Errorf("expected host path %q, got %q", cfg.HostPath, d.HostPath())
	}
}

func TestConfigEquals(t *testing.T) {
	base := Config{
		HostPath: `C:\a.vhdx`,
		ReadOnly: true,
		Type:     TypeVirtualDisk,
		EVDType:  "evd",
	}
	same := base
	if !base.Equals(same) {
		t.Error("expected equal configs to be equal")
	}

	diffPath := base
	diffPath.HostPath = `C:\b.vhdx`
	if base.Equals(diffPath) {
		t.Error("expected different HostPath to be not equal")
	}

	diffReadOnly := base
	diffReadOnly.ReadOnly = false
	if base.Equals(diffReadOnly) {
		t.Error("expected different ReadOnly to be not equal")
	}

	diffType := base
	diffType.Type = TypePassThru
	if base.Equals(diffType) {
		t.Error("expected different Type to be not equal")
	}

	diffEVD := base
	diffEVD.EVDType = "other"
	if base.Equals(diffEVD) {
		t.Error("expected different EVDType to be not equal")
	}
}

func TestAttachToVM_FromReserved_Success(t *testing.T) {
	d := NewReserved(0, 1, defaultConfig())
	err := d.AttachToVM(context.Background(), &mockVMSCSIAdder{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.State() != StateAttached {
		t.Errorf("expected state %d, got %d", StateAttached, d.State())
	}
}

func TestAttachToVM_FromReserved_Error(t *testing.T) {
	addErr := errors.New("add failed")
	d := NewReserved(0, 1, defaultConfig())
	err := d.AttachToVM(context.Background(), &mockVMSCSIAdder{err: addErr})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, addErr) {
		t.Errorf("expected wrapped error %v, got %v", addErr, err)
	}
	if d.State() != StateDetached {
		t.Errorf("expected state %d after failure, got %d", StateDetached, d.State())
	}
}

func TestAttachToVM_Idempotent_WhenAttached(t *testing.T) {
	d := attachedDisk(t)
	// Second attach should be idempotent.
	if err := d.AttachToVM(context.Background(), &mockVMSCSIAdder{}); err != nil {
		t.Fatalf("unexpected error on idempotent attach: %v", err)
	}
	if d.State() != StateAttached {
		t.Errorf("expected state %d, got %d", StateAttached, d.State())
	}
}

func TestAttachToVM_ErrorWhenDetached(t *testing.T) {
	d := NewReserved(0, 0, defaultConfig())
	// Fail attachment to move to detached.
	_ = d.AttachToVM(context.Background(), &mockVMSCSIAdder{err: errors.New("fail")})
	if d.State() != StateDetached {
		t.Fatalf("expected detached state, got %d", d.State())
	}
	err := d.AttachToVM(context.Background(), &mockVMSCSIAdder{})
	if err == nil {
		t.Fatal("expected error when attaching detached disk")
	}
}

func TestDetachFromVM_FromAttached_NoMounts_NoLinuxGuest(t *testing.T) {
	d := attachedDisk(t)
	err := d.DetachFromVM(context.Background(), &mockVMSCSIRemover{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.State() != StateDetached {
		t.Errorf("expected state %d, got %d", StateDetached, d.State())
	}
}

func TestDetachFromVM_FromAttached_WithLinuxGuest(t *testing.T) {
	d := attachedDisk(t)
	err := d.DetachFromVM(context.Background(), &mockVMSCSIRemover{}, &mockLinuxGuestSCSIEjector{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.State() != StateDetached {
		t.Errorf("expected state %d, got %d", StateDetached, d.State())
	}
}

func TestDetachFromVM_LinuxEjectError(t *testing.T) {
	d := attachedDisk(t)
	ejectErr := errors.New("eject failed")
	err := d.DetachFromVM(context.Background(), &mockVMSCSIRemover{}, &mockLinuxGuestSCSIEjector{err: ejectErr})
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

func TestDetachFromVM_RemoveError(t *testing.T) {
	d := attachedDisk(t)
	removeErr := errors.New("remove failed")
	// No linux guest so eject is skipped, but RemoveSCSIDisk fails.
	err := d.DetachFromVM(context.Background(), &mockVMSCSIRemover{err: removeErr}, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, removeErr) {
		t.Errorf("expected wrapped error %v, got %v", removeErr, err)
	}
	// State should be ejected since eject succeeded but removal failed.
	if d.State() != StateEjected {
		t.Errorf("expected state %d, got %d", StateEjected, d.State())
	}
}

func TestDetachFromVM_SkipsWhenMountsExist(t *testing.T) {
	d := attachedDisk(t)
	// Reserve a partition so len(mounts) > 0.
	_, err := d.ReservePartition(context.Background(), mount.Config{Partition: 1})
	if err != nil {
		t.Fatalf("ReservePartition: %v", err)
	}
	err = d.DetachFromVM(context.Background(), &mockVMSCSIRemover{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should remain attached because there are outstanding mounts.
	if d.State() != StateAttached {
		t.Errorf("expected state %d, got %d", StateAttached, d.State())
	}
}

func TestDetachFromVM_Idempotent_WhenReserved(t *testing.T) {
	d := NewReserved(0, 0, defaultConfig())
	err := d.DetachFromVM(context.Background(), &mockVMSCSIRemover{}, nil)
	if err != nil {
		t.Fatalf("unexpected error detaching reserved disk: %v", err)
	}
}

func TestDetachFromVM_Idempotent_WhenDetached(t *testing.T) {
	d := attachedDisk(t)
	_ = d.DetachFromVM(context.Background(), &mockVMSCSIRemover{}, nil)
	// Second detach should be idempotent.
	err := d.DetachFromVM(context.Background(), &mockVMSCSIRemover{}, nil)
	if err != nil {
		t.Fatalf("unexpected error on idempotent detach: %v", err)
	}
}

func TestReservePartition_Success(t *testing.T) {
	d := attachedDisk(t)
	cfg := mount.Config{Partition: 1, ReadOnly: true}
	m, err := d.ReservePartition(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil mount")
	}
	if m.State() != mount.StateReserved {
		t.Errorf("expected mount state %d, got %d", mount.StateReserved, m.State())
	}
}

func TestReservePartition_SuccessFromReservedDisk(t *testing.T) {
	d := NewReserved(0, 0, defaultConfig())
	cfg := mount.Config{Partition: 1}
	m, err := d.ReservePartition(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil mount")
	}
}

func TestReservePartition_SamePartitionSameConfig(t *testing.T) {
	d := attachedDisk(t)
	cfg := mount.Config{Partition: 1, ReadOnly: true}
	m1, err := d.ReservePartition(context.Background(), cfg)
	if err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	m2, err := d.ReservePartition(context.Background(), cfg)
	if err != nil {
		t.Fatalf("second reserve: %v", err)
	}
	if m1 != m2 {
		t.Error("expected same mount object on duplicate reservation")
	}
}

func TestReservePartition_SamePartitionDifferentConfig(t *testing.T) {
	d := attachedDisk(t)
	cfg1 := mount.Config{Partition: 1, ReadOnly: true}
	_, err := d.ReservePartition(context.Background(), cfg1)
	if err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	cfg2 := mount.Config{Partition: 1, ReadOnly: false}
	_, err = d.ReservePartition(context.Background(), cfg2)
	if err == nil {
		t.Fatal("expected error reserving same partition with different config")
	}
}

func TestReservePartition_ErrorWhenDetached(t *testing.T) {
	d := NewReserved(0, 0, defaultConfig())
	_ = d.AttachToVM(context.Background(), &mockVMSCSIAdder{err: errors.New("fail")})
	_, err := d.ReservePartition(context.Background(), mount.Config{Partition: 1})
	if err == nil {
		t.Fatal("expected error when disk is detached")
	}
}

func TestMountPartitionToGuest_Success(t *testing.T) {
	d := attachedDisk(t)
	cfg := mount.Config{Partition: 1}
	_, err := d.ReservePartition(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ReservePartition: %v", err)
	}
	guestPath, err := d.MountPartitionToGuest(context.Background(), 1, &mockLinuxGuestSCSIMounter{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if guestPath == "" {
		t.Error("expected non-empty guestPath")
	}
}

func TestMountPartitionToGuest_ErrorWhenNotAttached(t *testing.T) {
	d := NewReserved(0, 0, defaultConfig())
	_, err := d.MountPartitionToGuest(context.Background(), 1, &mockLinuxGuestSCSIMounter{}, nil)
	if err == nil {
		t.Fatal("expected error when disk is not attached")
	}
}

func TestMountPartitionToGuest_PartitionNotFound(t *testing.T) {
	d := attachedDisk(t)
	_, err := d.MountPartitionToGuest(context.Background(), 99, &mockLinuxGuestSCSIMounter{}, nil)
	if err == nil {
		t.Fatal("expected error for unreserved partition")
	}
}

func TestMountPartitionToGuest_MountError(t *testing.T) {
	d := attachedDisk(t)
	cfg := mount.Config{Partition: 1}
	_, err := d.ReservePartition(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ReservePartition: %v", err)
	}
	_, err = d.MountPartitionToGuest(context.Background(), 1, &mockLinuxGuestSCSIMounter{err: errors.New("mount fail")}, nil)
	if err != nil {
		// This is expected - the mount error propagates.
		return
	}
}

func TestUnmountPartitionFromGuest_Success(t *testing.T) {
	d := attachedDisk(t)
	cfg := mount.Config{Partition: 1}
	_, err := d.ReservePartition(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ReservePartition: %v", err)
	}
	_, err = d.MountPartitionToGuest(context.Background(), 1, &mockLinuxGuestSCSIMounter{}, nil)
	if err != nil {
		t.Fatalf("MountPartitionToGuest: %v", err)
	}
	err = d.UnmountPartitionFromGuest(context.Background(), 1, &mockLinuxGuestSCSIUnmounter{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnmountPartitionFromGuest_SucceedsWhenNotAttached(t *testing.T) {
	d := NewReserved(0, 0, defaultConfig())
	// No partition reserved, so this is a no-op success for retry logic.
	err := d.UnmountPartitionFromGuest(context.Background(), 1, &mockLinuxGuestSCSIUnmounter{}, nil)
	if err != nil {
		t.Fatalf("expected nil error for missing partition on non-attached disk, got: %v", err)
	}
}

func TestUnmountPartitionFromGuest_PartitionNotFound_IsNoOp(t *testing.T) {
	d := attachedDisk(t)
	// Missing partition is treated as success for retry safety.
	err := d.UnmountPartitionFromGuest(context.Background(), 99, &mockLinuxGuestSCSIUnmounter{}, nil)
	if err != nil {
		t.Fatalf("expected nil error for missing partition, got: %v", err)
	}
}

func TestUnmountPartitionFromGuest_UnmountError(t *testing.T) {
	d := attachedDisk(t)
	cfg := mount.Config{Partition: 1}
	_, err := d.ReservePartition(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ReservePartition: %v", err)
	}
	_, err = d.MountPartitionToGuest(context.Background(), 1, &mockLinuxGuestSCSIMounter{}, nil)
	if err != nil {
		t.Fatalf("MountPartitionToGuest: %v", err)
	}
	unmountErr := errors.New("unmount fail")
	err = d.UnmountPartitionFromGuest(context.Background(), 1, &mockLinuxGuestSCSIUnmounter{err: unmountErr}, nil)
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
	_, err = d.MountPartitionToGuest(context.Background(), 1, &mockLinuxGuestSCSIMounter{}, nil)
	if err != nil {
		t.Fatalf("MountPartitionToGuest: %v", err)
	}
	err = d.UnmountPartitionFromGuest(context.Background(), 1, &mockLinuxGuestSCSIUnmounter{}, nil)
	if err != nil {
		t.Fatalf("UnmountPartitionFromGuest: %v", err)
	}
	// After full unmount the partition should be removed, so detach should succeed
	// (no mounts remain).
	err = d.DetachFromVM(context.Background(), &mockVMSCSIRemover{}, nil)
	if err != nil {
		t.Fatalf("DetachFromVM after unmount: %v", err)
	}
	if d.State() != StateDetached {
		t.Errorf("expected state %d, got %d", StateDetached, d.State())
	}
}

func TestUnmountPartitionFromGuest_RetryAfterDetachFailure(t *testing.T) {
	// Simulates the scenario where unmount succeeds but detach fails.
	// On retry, UnmountPartitionFromGuest should be a no-op (partition already removed)
	// so that DetachFromVM can be retried.
	d := attachedDisk(t)
	cfg := mount.Config{Partition: 1}
	_, err := d.ReservePartition(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ReservePartition: %v", err)
	}
	_, err = d.MountPartitionToGuest(context.Background(), 1, &mockLinuxGuestSCSIMounter{}, nil)
	if err != nil {
		t.Fatalf("MountPartitionToGuest: %v", err)
	}
	// Unmount succeeds and removes the partition from the mounts map.
	err = d.UnmountPartitionFromGuest(context.Background(), 1, &mockLinuxGuestSCSIUnmounter{}, nil)
	if err != nil {
		t.Fatalf("UnmountPartitionFromGuest: %v", err)
	}
	// Detach fails (e.g. transient error).
	err = d.DetachFromVM(context.Background(), &mockVMSCSIRemover{err: errors.New("transient")}, nil)
	if err == nil {
		t.Fatal("expected detach error")
	}
	// Retry: unmount is a no-op since partition is gone.
	err = d.UnmountPartitionFromGuest(context.Background(), 1, &mockLinuxGuestSCSIUnmounter{}, nil)
	if err != nil {
		t.Fatalf("retry UnmountPartitionFromGuest should be no-op, got: %v", err)
	}
	// Retry detach now succeeds.
	err = d.DetachFromVM(context.Background(), &mockVMSCSIRemover{}, nil)
	if err != nil {
		t.Fatalf("retry DetachFromVM: %v", err)
	}
	if d.State() != StateDetached {
		t.Errorf("expected state %d, got %d", StateDetached, d.State())
	}
}

// --- Windows (WCOW) guest tests ---

func TestMountPartitionToGuest_WCOW_Success(t *testing.T) {
	d := attachedDisk(t)
	cfg := mount.Config{Partition: 1}
	_, err := d.ReservePartition(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ReservePartition: %v", err)
	}
	guestPath, err := d.MountPartitionToGuest(context.Background(), 1, nil, &mockWindowsGuestSCSIMounter{})
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
	_, err = d.MountPartitionToGuest(context.Background(), 1, nil, &mockWindowsGuestSCSIMounter{err: errors.New("wcow mount fail")})
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
	guestPath, err := d.MountPartitionToGuest(context.Background(), 1, nil, &mockWindowsGuestSCSIMounter{})
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
	_, err = d.MountPartitionToGuest(context.Background(), 1, nil, &mockWindowsGuestSCSIMounter{})
	if err != nil {
		t.Fatalf("MountPartitionToGuest: %v", err)
	}
	err = d.UnmountPartitionFromGuest(context.Background(), 1, nil, &mockWindowsGuestSCSIUnmounter{})
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
	_, err = d.MountPartitionToGuest(context.Background(), 1, nil, &mockWindowsGuestSCSIMounter{})
	if err != nil {
		t.Fatalf("MountPartitionToGuest: %v", err)
	}
	unmountErr := errors.New("wcow unmount fail")
	err = d.UnmountPartitionFromGuest(context.Background(), 1, nil, &mockWindowsGuestSCSIUnmounter{err: unmountErr})
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
	_, err = d.MountPartitionToGuest(context.Background(), 1, nil, &mockWindowsGuestSCSIMounter{})
	if err != nil {
		t.Fatalf("MountPartitionToGuest: %v", err)
	}
	err = d.UnmountPartitionFromGuest(context.Background(), 1, nil, &mockWindowsGuestSCSIUnmounter{})
	if err != nil {
		t.Fatalf("UnmountPartitionFromGuest: %v", err)
	}
	err = d.DetachFromVM(context.Background(), &mockVMSCSIRemover{}, nil)
	if err != nil {
		t.Fatalf("DetachFromVM after unmount: %v", err)
	}
	if d.State() != StateDetached {
		t.Errorf("expected state %d, got %d", StateDetached, d.State())
	}
}
