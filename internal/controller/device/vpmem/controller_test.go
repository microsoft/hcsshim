//go:build windows

package vpmem

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Microsoft/hcsshim/internal/controller/device/vpmem/device"
	"github.com/Microsoft/hcsshim/internal/controller/device/vpmem/mount"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/google/uuid"
)

// --- Mock types ---

type mockVMOps struct {
	addErr    error
	removeErr error
}

func (m *mockVMOps) AddVPMemDevice(_ context.Context, _ hcsschema.VirtualPMemDevice, _ uint32) error {
	return m.addErr
}

func (m *mockVMOps) RemoveVPMemDevice(_ context.Context, _ uint32) error {
	return m.removeErr
}

type mockGuestOps struct {
	mountErr   error
	unmountErr error
}

func (m *mockGuestOps) AddLCOWMappedVPMemDevice(_ context.Context, _ guestresource.LCOWMappedVPMemDevice) error {
	return m.mountErr
}

func (m *mockGuestOps) RemoveLCOWMappedVPMemDevice(_ context.Context, _ guestresource.LCOWMappedVPMemDevice) error {
	return m.unmountErr
}

// --- Helpers ---

func defaultDeviceConfig() device.DeviceConfig {
	return device.DeviceConfig{
		HostPath:    `C:\test\layer.vhd`,
		ReadOnly:    true,
		ImageFormat: "Vhd1",
	}
}

func defaultMountConfig() mount.MountConfig {
	return mount.MountConfig{}
}

func newController(vm *mockVMOps, guest *mockGuestOps) *Controller {
	return New(64, vm, guest)
}

func reservedController(t *testing.T) (*Controller, uuid.UUID) {
	t.Helper()
	c := newController(&mockVMOps{}, &mockGuestOps{})
	id, err := c.Reserve(context.Background(), defaultDeviceConfig(), defaultMountConfig())
	if err != nil {
		t.Fatalf("setup Reserve: %v", err)
	}
	return c, id
}

func mountedController(t *testing.T) (*Controller, uuid.UUID) {
	t.Helper()
	c, id := reservedController(t)
	if _, err := c.Mount(context.Background(), id); err != nil {
		t.Fatalf("setup Mount: %v", err)
	}
	return c, id
}

// --- Tests: New ---

func TestNew(t *testing.T) {
	c := New(64, &mockVMOps{}, &mockGuestOps{})
	if c == nil {
		t.Fatal("expected non-nil controller")
	}
}

// --- Tests: Reserve ---

func TestReserve_Success(t *testing.T) {
	c := newController(&mockVMOps{}, &mockGuestOps{})
	id, err := c.Reserve(context.Background(), defaultDeviceConfig(), defaultMountConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == uuid.Nil {
		t.Fatal("expected non-nil reservation ID")
	}
}

func TestReserve_SameDeviceSameMount(t *testing.T) {
	c := newController(&mockVMOps{}, &mockGuestOps{})
	dc := defaultDeviceConfig()
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

func TestReserve_SamePathDifferentDeviceConfig(t *testing.T) {
	c := newController(&mockVMOps{}, &mockGuestOps{})
	dc := defaultDeviceConfig()
	mc := defaultMountConfig()
	_, err := c.Reserve(context.Background(), dc, mc)
	if err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	dc2 := dc
	dc2.ReadOnly = false
	_, err = c.Reserve(context.Background(), dc2, mc)
	if err == nil {
		t.Fatal("expected error for same path with different device config")
	}
}

func TestReserve_DifferentDevices(t *testing.T) {
	c := newController(&mockVMOps{}, &mockGuestOps{})
	mc := defaultMountConfig()
	_, err := c.Reserve(context.Background(), device.DeviceConfig{HostPath: `C:\a.vhd`, ReadOnly: true, ImageFormat: "Vhd1"}, mc)
	if err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	_, err = c.Reserve(context.Background(), device.DeviceConfig{HostPath: `C:\b.vhd`, ReadOnly: true, ImageFormat: "Vhd1"}, mc)
	if err != nil {
		t.Fatalf("second reserve: %v", err)
	}
}

func TestReserve_NoAvailableSlots(t *testing.T) {
	c := New(0, &mockVMOps{}, &mockGuestOps{})
	_, err := c.Reserve(context.Background(), defaultDeviceConfig(), defaultMountConfig())
	if err == nil {
		t.Fatal("expected error when no slots available")
	}
}

func TestReserve_FillsAllSlots(t *testing.T) {
	c := New(64, &mockVMOps{}, &mockGuestOps{})
	mc := defaultMountConfig()
	for i := range 64 {
		dc := device.DeviceConfig{
			HostPath:    fmt.Sprintf(`C:\layer%d.vhd`, i),
			ReadOnly:    true,
			ImageFormat: "Vhd1",
		}
		_, err := c.Reserve(context.Background(), dc, mc)
		if err != nil {
			t.Fatalf("reserve slot %d: %v", i, err)
		}
	}
	// Next reservation should fail.
	_, err := c.Reserve(context.Background(), device.DeviceConfig{HostPath: `C:\overflow.vhd`, ReadOnly: true, ImageFormat: "Vhd1"}, mc)
	if err == nil {
		t.Fatal("expected error when all slots are full")
	}
}

// --- Tests: Mount ---

func TestMount_Success(t *testing.T) {
	c, id := reservedController(t)
	guestPath, err := c.Mount(context.Background(), id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if guestPath == "" {
		t.Error("expected non-empty guestPath")
	}
}

func TestMount_UnknownReservation(t *testing.T) {
	c := newController(&mockVMOps{}, &mockGuestOps{})
	_, err := c.Mount(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error for unknown reservation")
	}
}

func TestMount_AttachError(t *testing.T) {
	vm := &mockVMOps{addErr: errors.New("attach failed")}
	c := New(64, vm, &mockGuestOps{})
	id, err := c.Reserve(context.Background(), defaultDeviceConfig(), defaultMountConfig())
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	_, err = c.Mount(context.Background(), id)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestMount_MountError(t *testing.T) {
	guest := &mockGuestOps{mountErr: errors.New("mount failed")}
	c := New(64, &mockVMOps{}, guest)
	id, err := c.Reserve(context.Background(), defaultDeviceConfig(), defaultMountConfig())
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	_, err = c.Mount(context.Background(), id)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestMount_Idempotent(t *testing.T) {
	c, id := reservedController(t)
	if _, err := c.Mount(context.Background(), id); err != nil {
		t.Fatalf("first mount: %v", err)
	}
	if _, err := c.Mount(context.Background(), id); err != nil {
		t.Fatalf("second mount (idempotent): %v", err)
	}
}

// --- Tests: Unmount ---

func TestUnmount_Success(t *testing.T) {
	c, id := mountedController(t)
	err := c.Unmount(context.Background(), id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnmount_UnknownReservation(t *testing.T) {
	c := newController(&mockVMOps{}, &mockGuestOps{})
	err := c.Unmount(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error for unknown reservation")
	}
}

func TestUnmount_CleansUpSlotWhenFullyDetached(t *testing.T) {
	c, id := mountedController(t)
	err := c.Unmount(context.Background(), id)
	if err != nil {
		t.Fatalf("Unmount: %v", err)
	}
	// The slot should be freed, so we can reserve the same path again.
	_, err = c.Reserve(context.Background(), defaultDeviceConfig(), defaultMountConfig())
	if err != nil {
		t.Fatalf("re-reserve after unmount: %v", err)
	}
}

func TestUnmount_UnmountError(t *testing.T) {
	guest := &mockGuestOps{}
	c := New(64, &mockVMOps{}, guest)
	id, err := c.Reserve(context.Background(), defaultDeviceConfig(), defaultMountConfig())
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if _, err := c.Mount(context.Background(), id); err != nil {
		t.Fatalf("Mount: %v", err)
	}
	// Now inject an unmount error.
	guest.unmountErr = errors.New("unmount failed")
	err = c.Unmount(context.Background(), id)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUnmount_DetachError(t *testing.T) {
	vm := &mockVMOps{}
	c := New(64, vm, &mockGuestOps{})
	id, err := c.Reserve(context.Background(), defaultDeviceConfig(), defaultMountConfig())
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if _, err := c.Mount(context.Background(), id); err != nil {
		t.Fatalf("Mount: %v", err)
	}
	// Now inject a remove error.
	vm.removeErr = errors.New("remove failed")
	err = c.Unmount(context.Background(), id)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUnmount_MultipleReservationsSameDevice(t *testing.T) {
	c := newController(&mockVMOps{}, &mockGuestOps{})
	dc := defaultDeviceConfig()
	mc := defaultMountConfig()

	id1, err := c.Reserve(context.Background(), dc, mc)
	if err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	id2, err := c.Reserve(context.Background(), dc, mc)
	if err != nil {
		t.Fatalf("second reserve: %v", err)
	}

	// Mount both.
	if _, err := c.Mount(context.Background(), id1); err != nil {
		t.Fatalf("Mount id1: %v", err)
	}
	if _, err := c.Mount(context.Background(), id2); err != nil {
		t.Fatalf("Mount id2: %v", err)
	}

	// Unmount first. Device should still be attached because id2 holds a ref.
	if err := c.Unmount(context.Background(), id1); err != nil {
		t.Fatalf("Unmount id1: %v", err)
	}

	// Unmount second. Now device should be fully detached and slot freed.
	if err := c.Unmount(context.Background(), id2); err != nil {
		t.Fatalf("Unmount id2: %v", err)
	}

	// Slot should be free for reuse.
	_, err = c.Reserve(context.Background(), dc, mc)
	if err != nil {
		t.Fatalf("re-reserve after full unmount: %v", err)
	}
}

// --- Tests: Reserve + Unmount lifecycle ---

func TestReserveAfterUnmount_ReusesSlot(t *testing.T) {
	c, id := mountedController(t)
	if err := c.Unmount(context.Background(), id); err != nil {
		t.Fatalf("Unmount: %v", err)
	}
	// Reserve a different device in the now-freed slot.
	dc := device.DeviceConfig{HostPath: `C:\other.vhd`, ReadOnly: true, ImageFormat: "Vhd1"}
	_, err := c.Reserve(context.Background(), dc, defaultMountConfig())
	if err != nil {
		t.Fatalf("reserve after unmount: %v", err)
	}
}

func TestUnmount_AfterFailedMount(t *testing.T) {
	vm := &mockVMOps{addErr: errors.New("attach failed")}
	c := New(64, vm, &mockGuestOps{})
	dc := defaultDeviceConfig()
	mc := defaultMountConfig()

	id, err := c.Reserve(context.Background(), dc, mc)
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	_, err = c.Mount(context.Background(), id)
	if err == nil {
		t.Fatal("expected Mount to fail")
	}
	// Unmount should succeed and clean up.
	err = c.Unmount(context.Background(), id)
	if err != nil {
		t.Fatalf("Unmount after failed Mount: %v", err)
	}
	// Slot should be freed for reuse.
	_, err = c.Reserve(context.Background(), dc, mc)
	if err != nil {
		t.Fatalf("re-reserve after cleanup: %v", err)
	}
}

func TestUnmount_RetryAfterDetachFailure(t *testing.T) {
	vm := &mockVMOps{}
	c := New(64, vm, &mockGuestOps{})
	dc := defaultDeviceConfig()
	mc := defaultMountConfig()

	id, err := c.Reserve(context.Background(), dc, mc)
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if _, err := c.Mount(context.Background(), id); err != nil {
		t.Fatalf("Mount: %v", err)
	}
	// Inject remove error for first attempt.
	vm.removeErr = errors.New("transient remove failure")
	err = c.Unmount(context.Background(), id)
	if err == nil {
		t.Fatal("expected detach error")
	}
	// Fix the error and retry.
	vm.removeErr = nil
	err = c.Unmount(context.Background(), id)
	if err != nil {
		t.Fatalf("retry Unmount: %v", err)
	}
	// Slot should be freed.
	_, err = c.Reserve(context.Background(), dc, mc)
	if err != nil {
		t.Fatalf("re-reserve after retry: %v", err)
	}
}
