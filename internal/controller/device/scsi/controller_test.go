//go:build windows

package scsi

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Microsoft/hcsshim/internal/controller/device/scsi/disk"
	"github.com/Microsoft/hcsshim/internal/controller/device/scsi/mount"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/google/uuid"
)

// --- Mock types ---

type mockVMOps struct {
	addErr    error
	removeErr error
}

func (m *mockVMOps) AddSCSIDisk(_ context.Context, _ hcsschema.Attachment, _ uint, _ uint) error {
	return m.addErr
}

func (m *mockVMOps) RemoveSCSIDisk(_ context.Context, _ uint, _ uint) error {
	return m.removeErr
}

type mockLinuxGuestOps struct {
	mountErr   error
	unmountErr error
	ejectErr   error
}

func (m *mockLinuxGuestOps) AddLCOWMappedVirtualDisk(_ context.Context, _ guestresource.LCOWMappedVirtualDisk) error {
	return m.mountErr
}

func (m *mockLinuxGuestOps) RemoveLCOWMappedVirtualDisk(_ context.Context, _ guestresource.LCOWMappedVirtualDisk) error {
	return m.unmountErr
}

func (m *mockLinuxGuestOps) RemoveSCSIDevice(_ context.Context, _ guestresource.SCSIDevice) error {
	return m.ejectErr
}

type mockWindowsGuestOps struct {
	mountErr   error
	unmountErr error
}

func (m *mockWindowsGuestOps) AddWCOWMappedVirtualDisk(_ context.Context, _ guestresource.WCOWMappedVirtualDisk) error {
	return m.mountErr
}

func (m *mockWindowsGuestOps) AddWCOWMappedVirtualDiskForContainerScratch(_ context.Context, _ guestresource.WCOWMappedVirtualDisk) error {
	return m.mountErr
}

func (m *mockWindowsGuestOps) RemoveWCOWMappedVirtualDisk(_ context.Context, _ guestresource.WCOWMappedVirtualDisk) error {
	return m.unmountErr
}

// --- Helpers ---

func defaultDiskConfig() disk.DiskConfig {
	return disk.DiskConfig{
		HostPath: `C:\test\disk.vhdx`,
		ReadOnly: false,
		Type:     disk.DiskTypeVirtualDisk,
	}
}

func defaultMountConfig() mount.MountConfig {
	return mount.MountConfig{
		Partition: 1,
		ReadOnly:  false,
	}
}

func newController(vm *mockVMOps, lg *mockLinuxGuestOps, wg *mockWindowsGuestOps) *Controller {
	return New(1, vm, lg, wg)
}

func reservedController(t *testing.T) (*Controller, uuid.UUID) {
	t.Helper()
	c := newController(&mockVMOps{}, &mockLinuxGuestOps{}, nil)
	id, err := c.Reserve(context.Background(), defaultDiskConfig(), defaultMountConfig())
	if err != nil {
		t.Fatalf("setup Reserve: %v", err)
	}
	return c, id
}

func mappedController(t *testing.T) (*Controller, uuid.UUID) {
	t.Helper()
	c, id := reservedController(t)
	if _, err := c.MapToGuest(context.Background(), id); err != nil {
		t.Fatalf("setup MapToGuest: %v", err)
	}
	return c, id
}

// --- Tests: New ---

func TestNew(t *testing.T) {
	c := New(4, &mockVMOps{}, &mockLinuxGuestOps{}, nil)
	if c == nil {
		t.Fatal("expected non-nil controller")
	}
}

// --- Tests: Reserve ---

func TestReserve_Success(t *testing.T) {
	c := newController(&mockVMOps{}, &mockLinuxGuestOps{}, nil)
	id, err := c.Reserve(context.Background(), defaultDiskConfig(), defaultMountConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == uuid.Nil {
		t.Fatal("expected non-nil reservation ID")
	}
}

func TestReserve_SameDiskSamePartition(t *testing.T) {
	c := newController(&mockVMOps{}, &mockLinuxGuestOps{}, nil)
	dc := defaultDiskConfig()
	mc := defaultMountConfig()
	id1, err := c.Reserve(context.Background(), dc, mc)
	if err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	id2, err := c.Reserve(context.Background(), dc, mc)
	if err != nil {
		t.Fatalf("second reserve: %v", err)
	}
	if id1 == id2 {
		t.Error("expected different reservation IDs")
	}
}

func TestReserve_SameDiskDifferentPartition(t *testing.T) {
	c := newController(&mockVMOps{}, &mockLinuxGuestOps{}, nil)
	dc := defaultDiskConfig()
	_, err := c.Reserve(context.Background(), dc, mount.MountConfig{Partition: 1})
	if err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	_, err = c.Reserve(context.Background(), dc, mount.MountConfig{Partition: 2})
	if err != nil {
		t.Fatalf("second reserve different partition: %v", err)
	}
}

func TestReserve_SamePathDifferentDiskConfig(t *testing.T) {
	c := newController(&mockVMOps{}, &mockLinuxGuestOps{}, nil)
	dc := defaultDiskConfig()
	mc := defaultMountConfig()
	_, err := c.Reserve(context.Background(), dc, mc)
	if err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	dc2 := dc
	dc2.ReadOnly = true
	_, err = c.Reserve(context.Background(), dc2, mc)
	if err == nil {
		t.Fatal("expected error for same path with different disk config")
	}
}

func TestReserve_DifferentDisks(t *testing.T) {
	c := newController(&mockVMOps{}, &mockLinuxGuestOps{}, nil)
	mc := defaultMountConfig()
	_, err := c.Reserve(context.Background(), disk.DiskConfig{HostPath: `C:\a.vhdx`, Type: disk.DiskTypeVirtualDisk}, mc)
	if err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	_, err = c.Reserve(context.Background(), disk.DiskConfig{HostPath: `C:\b.vhdx`, Type: disk.DiskTypeVirtualDisk}, mc)
	if err != nil {
		t.Fatalf("second reserve: %v", err)
	}
}

func TestReserve_NoAvailableSlots(t *testing.T) {
	// Create with 0 controllers so there are no slots.
	c := New(0, &mockVMOps{}, &mockLinuxGuestOps{}, nil)
	_, err := c.Reserve(context.Background(), defaultDiskConfig(), defaultMountConfig())
	if err == nil {
		t.Fatal("expected error when no slots available")
	}
}

func TestReserve_FillsAllSlots(t *testing.T) {
	c := New(1, &mockVMOps{}, &mockLinuxGuestOps{}, nil)
	mc := defaultMountConfig()
	for i := range numLUNsPerController {
		dc := disk.DiskConfig{
			HostPath: fmt.Sprintf(`C:\disk%d.vhdx`, i),
			Type:     disk.DiskTypeVirtualDisk,
		}
		_, err := c.Reserve(context.Background(), dc, mc)
		if err != nil {
			t.Fatalf("reserve slot %d: %v", i, err)
		}
	}
	// Next reservation should fail.
	_, err := c.Reserve(context.Background(), disk.DiskConfig{HostPath: `C:\overflow.vhdx`, Type: disk.DiskTypeVirtualDisk}, mc)
	if err == nil {
		t.Fatal("expected error when all slots are full")
	}
}

// --- Tests: MapToGuest ---

func TestMapToGuest_Success(t *testing.T) {
	c, id := reservedController(t)
	guestPath, err := c.MapToGuest(context.Background(), id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if guestPath == "" {
		t.Error("expected non-empty guestPath")
	}
}

func TestMapToGuest_UnknownReservation(t *testing.T) {
	c := newController(&mockVMOps{}, &mockLinuxGuestOps{}, nil)
	_, err := c.MapToGuest(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error for unknown reservation")
	}
}

func TestMapToGuest_AttachError(t *testing.T) {
	vm := &mockVMOps{addErr: errors.New("attach failed")}
	c := New(1, vm, &mockLinuxGuestOps{}, nil)
	id, err := c.Reserve(context.Background(), defaultDiskConfig(), defaultMountConfig())
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	_, err = c.MapToGuest(context.Background(), id)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestMapToGuest_MountError(t *testing.T) {
	lg := &mockLinuxGuestOps{mountErr: errors.New("mount failed")}
	c := New(1, &mockVMOps{}, lg, nil)
	id, err := c.Reserve(context.Background(), defaultDiskConfig(), defaultMountConfig())
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	_, err = c.MapToGuest(context.Background(), id)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestMapToGuest_Idempotent(t *testing.T) {
	c, id := reservedController(t)
	if _, err := c.MapToGuest(context.Background(), id); err != nil {
		t.Fatalf("first map: %v", err)
	}
	// Second call should be idempotent (disk already attached, mount already mounted).
	if _, err := c.MapToGuest(context.Background(), id); err != nil {
		t.Fatalf("second map (idempotent): %v", err)
	}
}

// --- Tests: UnmapFromGuest ---

func TestUnmapFromGuest_Success(t *testing.T) {
	c, id := mappedController(t)
	err := c.UnmapFromGuest(context.Background(), id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnmapFromGuest_UnknownReservation(t *testing.T) {
	c := newController(&mockVMOps{}, &mockLinuxGuestOps{}, nil)
	err := c.UnmapFromGuest(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error for unknown reservation")
	}
}

func TestUnmapFromGuest_CleansUpSlotWhenFullyDetached(t *testing.T) {
	c, id := mappedController(t)
	err := c.UnmapFromGuest(context.Background(), id)
	if err != nil {
		t.Fatalf("UnmapFromGuest: %v", err)
	}
	// The slot should be freed, so we can reserve the same path again.
	_, err = c.Reserve(context.Background(), defaultDiskConfig(), defaultMountConfig())
	if err != nil {
		t.Fatalf("re-reserve after unmap: %v", err)
	}
}

func TestUnmapFromGuest_UnmountError(t *testing.T) {
	lg := &mockLinuxGuestOps{}
	c := New(1, &mockVMOps{}, lg, nil)
	id, err := c.Reserve(context.Background(), defaultDiskConfig(), defaultMountConfig())
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if _, err := c.MapToGuest(context.Background(), id); err != nil {
		t.Fatalf("MapToGuest: %v", err)
	}
	// Now inject an unmount error.
	lg.unmountErr = errors.New("unmount failed")
	err = c.UnmapFromGuest(context.Background(), id)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUnmapFromGuest_DetachError(t *testing.T) {
	vm := &mockVMOps{}
	c := New(1, vm, &mockLinuxGuestOps{}, nil)
	id, err := c.Reserve(context.Background(), defaultDiskConfig(), defaultMountConfig())
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if _, err := c.MapToGuest(context.Background(), id); err != nil {
		t.Fatalf("MapToGuest: %v", err)
	}
	// Now inject a remove error. Unmount succeeds but detach fails.
	vm.removeErr = errors.New("remove failed")
	err = c.UnmapFromGuest(context.Background(), id)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUnmapFromGuest_MultipleReservationsSameDisk(t *testing.T) {
	c := newController(&mockVMOps{}, &mockLinuxGuestOps{}, nil)
	dc := defaultDiskConfig()
	mc := defaultMountConfig()

	id1, err := c.Reserve(context.Background(), dc, mc)
	if err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	id2, err := c.Reserve(context.Background(), dc, mc)
	if err != nil {
		t.Fatalf("second reserve: %v", err)
	}

	// Map both.
	if _, err := c.MapToGuest(context.Background(), id1); err != nil {
		t.Fatalf("MapToGuest id1: %v", err)
	}
	if _, err := c.MapToGuest(context.Background(), id2); err != nil {
		t.Fatalf("MapToGuest id2: %v", err)
	}

	// Unmap first. Disk should still be attached because id2 holds a ref.
	if err := c.UnmapFromGuest(context.Background(), id1); err != nil {
		t.Fatalf("UnmapFromGuest id1: %v", err)
	}

	// Unmap second. Now disk should be fully detached and slot freed.
	if err := c.UnmapFromGuest(context.Background(), id2); err != nil {
		t.Fatalf("UnmapFromGuest id2: %v", err)
	}

	// Slot should be free for reuse.
	_, err = c.Reserve(context.Background(), dc, mc)
	if err != nil {
		t.Fatalf("re-reserve after full unmap: %v", err)
	}
}

func TestUnmapFromGuest_WindowsGuest(t *testing.T) {
	wg := &mockWindowsGuestOps{}
	c := New(1, &mockVMOps{}, nil, wg)
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

// --- Tests: Reserve + Unmap lifecycle ---

func TestReserveAfterUnmap_ReusesSlot(t *testing.T) {
	c, id := mappedController(t)
	if err := c.UnmapFromGuest(context.Background(), id); err != nil {
		t.Fatalf("UnmapFromGuest: %v", err)
	}
	// Reserve a different disk in the now-freed slot.
	dc := disk.DiskConfig{HostPath: `C:\other.vhdx`, Type: disk.DiskTypeVirtualDisk}
	_, err := c.Reserve(context.Background(), dc, defaultMountConfig())
	if err != nil {
		t.Fatalf("reserve after unmap: %v", err)
	}
}

func TestUnmapFromGuest_AfterFailedMapToGuest(t *testing.T) {
	// MapToGuest fails at attach, then UnmapFromGuest should clean up the
	// reservation and free the slot.
	vm := &mockVMOps{addErr: errors.New("attach failed")}
	c := New(1, vm, &mockLinuxGuestOps{}, nil)
	dc := defaultDiskConfig()
	mc := defaultMountConfig()

	id, err := c.Reserve(context.Background(), dc, mc)
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	_, err = c.MapToGuest(context.Background(), id)
	if err == nil {
		t.Fatal("expected MapToGuest to fail")
	}
	// UnmapFromGuest should succeed and clean up.
	err = c.UnmapFromGuest(context.Background(), id)
	if err != nil {
		t.Fatalf("UnmapFromGuest after failed MapToGuest: %v", err)
	}
	// Slot should be freed for reuse.
	_, err = c.Reserve(context.Background(), dc, mc)
	if err != nil {
		t.Fatalf("re-reserve after cleanup: %v", err)
	}
}

func TestUnmapFromGuest_RetryAfterDetachFailure(t *testing.T) {
	// UnmapFromGuest fails at detach. Retry should succeed because
	// UnmountPartitionFromGuest is a no-op when the partition is already gone.
	vm := &mockVMOps{}
	c := New(1, vm, &mockLinuxGuestOps{}, nil)
	dc := defaultDiskConfig()
	mc := defaultMountConfig()

	id, err := c.Reserve(context.Background(), dc, mc)
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if _, err := c.MapToGuest(context.Background(), id); err != nil {
		t.Fatalf("MapToGuest: %v", err)
	}
	// Inject remove error for first attempt.
	vm.removeErr = errors.New("transient remove failure")
	err = c.UnmapFromGuest(context.Background(), id)
	if err == nil {
		t.Fatal("expected detach error")
	}
	// Fix the error and retry.
	vm.removeErr = nil
	err = c.UnmapFromGuest(context.Background(), id)
	if err != nil {
		t.Fatalf("retry UnmapFromGuest: %v", err)
	}
	// Slot should be freed.
	_, err = c.Reserve(context.Background(), dc, mc)
	if err != nil {
		t.Fatalf("re-reserve after retry: %v", err)
	}
}
