//go:build windows && (lcow || wcow)

package scsi

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Microsoft/hcsshim/internal/controller/device/scsi/disk"
	"github.com/Microsoft/hcsshim/internal/controller/device/scsi/mount"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"

	"github.com/Microsoft/go-winio/pkg/guid"
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

// --- Helpers ---

func defaultDiskConfig() disk.Config {
	return disk.Config{
		HostPath: `C:\test\disk.vhdx`,
		ReadOnly: false,
		Type:     disk.TypeVirtualDisk,
	}
}

func defaultMountConfig() mount.Config {
	return mount.Config{
		Partition: 1,
		ReadOnly:  false,
	}
}

func newController(vm *mockVMOps, guest *mockGuestOps) *Controller {
	return New(1, vm, guest)
}

func reservedController(t *testing.T) (*Controller, guid.GUID) {
	t.Helper()
	c := newController(&mockVMOps{}, newMockGuestOps())
	id, err := c.Reserve(context.Background(), defaultDiskConfig(), defaultMountConfig())
	if err != nil {
		t.Fatalf("setup Reserve: %v", err)
	}
	return c, id
}

func mappedController(t *testing.T) (*Controller, guid.GUID) {
	t.Helper()
	c, id := reservedController(t)
	if _, err := c.MapToGuest(context.Background(), id); err != nil {
		t.Fatalf("setup MapToGuest: %v", err)
	}
	return c, id
}

// --- Tests: New ---

func TestNew(t *testing.T) {
	c := New(4, &mockVMOps{}, newMockGuestOps())
	if c == nil {
		t.Fatal("expected non-nil controller")
	}
}

// --- Tests: Reserve ---

func TestReserve_Success(t *testing.T) {
	c := newController(&mockVMOps{}, newMockGuestOps())
	id, err := c.Reserve(context.Background(), defaultDiskConfig(), defaultMountConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == (guid.GUID{}) {
		t.Fatal("expected non-nil reservation ID")
	}
}

func TestReserve_SameDiskSamePartition(t *testing.T) {
	c := newController(&mockVMOps{}, newMockGuestOps())
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
	c := newController(&mockVMOps{}, newMockGuestOps())
	dc := defaultDiskConfig()
	_, err := c.Reserve(context.Background(), dc, mount.Config{Partition: 1})
	if err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	_, err = c.Reserve(context.Background(), dc, mount.Config{Partition: 2})
	if err != nil {
		t.Fatalf("second reserve different partition: %v", err)
	}
}

func TestReserve_SamePathDifferentConfig(t *testing.T) {
	c := newController(&mockVMOps{}, newMockGuestOps())
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
	c := newController(&mockVMOps{}, newMockGuestOps())
	mc := defaultMountConfig()
	_, err := c.Reserve(context.Background(), disk.Config{HostPath: `C:\a.vhdx`, Type: disk.TypeVirtualDisk}, mc)
	if err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	_, err = c.Reserve(context.Background(), disk.Config{HostPath: `C:\b.vhdx`, Type: disk.TypeVirtualDisk}, mc)
	if err != nil {
		t.Fatalf("second reserve: %v", err)
	}
}

func TestReserve_NoAvailableSlots(t *testing.T) {
	// Create with 0 controllers so there are no slots.
	c := New(0, &mockVMOps{}, newMockGuestOps())
	_, err := c.Reserve(context.Background(), defaultDiskConfig(), defaultMountConfig())
	if err == nil {
		t.Fatal("expected error when no slots available")
	}
}

func TestReserve_FillsAllSlots(t *testing.T) {
	c := New(1, &mockVMOps{}, newMockGuestOps())
	mc := defaultMountConfig()
	for i := range numLUNsPerController {
		dc := disk.Config{
			HostPath: fmt.Sprintf(`C:\disk%d.vhdx`, i),
			Type:     disk.TypeVirtualDisk,
		}
		_, err := c.Reserve(context.Background(), dc, mc)
		if err != nil {
			t.Fatalf("reserve slot %d: %v", i, err)
		}
	}
	// Next reservation should fail.
	_, err := c.Reserve(context.Background(), disk.Config{HostPath: `C:\overflow.vhdx`, Type: disk.TypeVirtualDisk}, mc)
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
	c := newController(&mockVMOps{}, newMockGuestOps())
	unknownID, err := guid.NewV4()
	if err != nil {
		t.Fatalf("NewV4: %v", err)
	}
	_, err = c.MapToGuest(context.Background(), unknownID)
	if err == nil {
		t.Fatal("expected error for unknown reservation")
	}
}

func TestMapToGuest_AttachError(t *testing.T) {
	vm := &mockVMOps{addErr: errors.New("attach failed")}
	c := New(1, vm, newMockGuestOps())
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
	c := newController(&mockVMOps{}, newMockGuestOps())
	unknownID, err := guid.NewV4()
	if err != nil {
		t.Fatalf("NewV4: %v", err)
	}
	err = c.UnmapFromGuest(context.Background(), unknownID)
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

func TestUnmapFromGuest_DetachError(t *testing.T) {
	vm := &mockVMOps{}
	c := New(1, vm, newMockGuestOps())
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
	c := newController(&mockVMOps{}, newMockGuestOps())
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

// --- Tests: Reserve + Unmap lifecycle ---

func TestReserveAfterUnmap_ReusesSlot(t *testing.T) {
	c, id := mappedController(t)
	if err := c.UnmapFromGuest(context.Background(), id); err != nil {
		t.Fatalf("UnmapFromGuest: %v", err)
	}
	// Reserve a different disk in the now-freed slot.
	dc := disk.Config{HostPath: `C:\other.vhdx`, Type: disk.TypeVirtualDisk}
	_, err := c.Reserve(context.Background(), dc, defaultMountConfig())
	if err != nil {
		t.Fatalf("reserve after unmap: %v", err)
	}
}

func TestUnmapFromGuest_AfterFailedMapToGuest(t *testing.T) {
	// MapToGuest fails at attach, then UnmapFromGuest should clean up the
	// reservation and free the slot.
	vm := &mockVMOps{addErr: errors.New("attach failed")}
	c := New(1, vm, newMockGuestOps())
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
	c := New(1, vm, newMockGuestOps())
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
