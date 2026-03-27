//go:build windows

package device

import (
	"context"
	"errors"
	"testing"

	"github.com/Microsoft/hcsshim/internal/controller/device/vpmem/mount"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// --- Mock types ---

type mockVMVPMemAdder struct {
	err error
}

func (m *mockVMVPMemAdder) AddVPMemDevice(_ context.Context, _ hcsschema.VirtualPMemDevice, _ uint32) error {
	return m.err
}

type mockVMVPMemRemover struct {
	err error
}

func (m *mockVMVPMemRemover) RemoveVPMemDevice(_ context.Context, _ uint32) error {
	return m.err
}

type mockLinuxGuestVPMemMounter struct {
	err error
}

func (m *mockLinuxGuestVPMemMounter) AddLCOWMappedVPMemDevice(_ context.Context, _ guestresource.LCOWMappedVPMemDevice) error {
	return m.err
}

type mockLinuxGuestVPMemUnmounter struct {
	err error
}

func (m *mockLinuxGuestVPMemUnmounter) RemoveLCOWMappedVPMemDevice(_ context.Context, _ guestresource.LCOWMappedVPMemDevice) error {
	return m.err
}

// --- Helpers ---

func defaultConfig() DeviceConfig {
	return DeviceConfig{
		HostPath:    `C:\test\layer.vhd`,
		ReadOnly:    true,
		ImageFormat: "Vhd1",
	}
}

func attachedDevice(t *testing.T) *Device {
	t.Helper()
	d := NewReserved(0, defaultConfig())
	if err := d.AttachToVM(context.Background(), &mockVMVPMemAdder{}); err != nil {
		t.Fatalf("setup AttachToVM: %v", err)
	}
	return d
}

// --- Tests ---

func TestNewReserved(t *testing.T) {
	cfg := DeviceConfig{
		HostPath:    `C:\test\layer.vhd`,
		ReadOnly:    true,
		ImageFormat: "Vhd1",
	}
	d := NewReserved(3, cfg)

	if d.State() != DeviceStateReserved {
		t.Errorf("expected state %d, got %d", DeviceStateReserved, d.State())
	}
	if d.Config() != cfg {
		t.Errorf("expected config %+v, got %+v", cfg, d.Config())
	}
	if d.HostPath() != cfg.HostPath {
		t.Errorf("expected host path %q, got %q", cfg.HostPath, d.HostPath())
	}
}

func TestDeviceConfigEquals(t *testing.T) {
	base := DeviceConfig{
		HostPath:    `C:\a.vhd`,
		ReadOnly:    true,
		ImageFormat: "Vhd1",
	}
	same := base
	if !base.Equals(same) {
		t.Error("expected equal configs to be equal")
	}

	diffPath := base
	diffPath.HostPath = `C:\b.vhd`
	if base.Equals(diffPath) {
		t.Error("expected different HostPath to be not equal")
	}

	diffReadOnly := base
	diffReadOnly.ReadOnly = false
	if base.Equals(diffReadOnly) {
		t.Error("expected different ReadOnly to be not equal")
	}

	diffFormat := base
	diffFormat.ImageFormat = "Vhdx"
	if base.Equals(diffFormat) {
		t.Error("expected different ImageFormat to be not equal")
	}
}

func TestAttachToVM_FromReserved_Success(t *testing.T) {
	d := NewReserved(0, defaultConfig())
	err := d.AttachToVM(context.Background(), &mockVMVPMemAdder{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.State() != DeviceStateAttached {
		t.Errorf("expected state %d, got %d", DeviceStateAttached, d.State())
	}
}

func TestAttachToVM_FromReserved_Error(t *testing.T) {
	addErr := errors.New("add failed")
	d := NewReserved(0, defaultConfig())
	err := d.AttachToVM(context.Background(), &mockVMVPMemAdder{err: addErr})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, addErr) {
		t.Errorf("expected wrapped error %v, got %v", addErr, err)
	}
	if d.State() != DeviceStateDetached {
		t.Errorf("expected state %d after failure, got %d", DeviceStateDetached, d.State())
	}
}

func TestAttachToVM_Idempotent_WhenAttached(t *testing.T) {
	d := attachedDevice(t)
	if err := d.AttachToVM(context.Background(), &mockVMVPMemAdder{}); err != nil {
		t.Fatalf("unexpected error on idempotent attach: %v", err)
	}
	if d.State() != DeviceStateAttached {
		t.Errorf("expected state %d, got %d", DeviceStateAttached, d.State())
	}
}

func TestAttachToVM_ErrorWhenDetached(t *testing.T) {
	d := NewReserved(0, defaultConfig())
	// Fail attachment to move to detached.
	_ = d.AttachToVM(context.Background(), &mockVMVPMemAdder{err: errors.New("fail")})
	if d.State() != DeviceStateDetached {
		t.Fatalf("expected detached state, got %d", d.State())
	}
	err := d.AttachToVM(context.Background(), &mockVMVPMemAdder{})
	if err == nil {
		t.Fatal("expected error when attaching detached device")
	}
}

func TestDetachFromVM_FromAttached_Success(t *testing.T) {
	d := attachedDevice(t)
	err := d.DetachFromVM(context.Background(), &mockVMVPMemRemover{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.State() != DeviceStateDetached {
		t.Errorf("expected state %d, got %d", DeviceStateDetached, d.State())
	}
}

func TestDetachFromVM_RemoveError(t *testing.T) {
	d := attachedDevice(t)
	removeErr := errors.New("remove failed")
	err := d.DetachFromVM(context.Background(), &mockVMVPMemRemover{err: removeErr})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, removeErr) {
		t.Errorf("expected wrapped error %v, got %v", removeErr, err)
	}
	// State should remain attached since removal failed.
	if d.State() != DeviceStateAttached {
		t.Errorf("expected state %d, got %d", DeviceStateAttached, d.State())
	}
}

func TestDetachFromVM_SkipsWhenMountExists(t *testing.T) {
	d := attachedDevice(t)
	_, err := d.ReserveMount(context.Background(), mount.MountConfig{})
	if err != nil {
		t.Fatalf("ReserveMount: %v", err)
	}
	err = d.DetachFromVM(context.Background(), &mockVMVPMemRemover{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should remain attached because there is an outstanding mount.
	if d.State() != DeviceStateAttached {
		t.Errorf("expected state %d, got %d", DeviceStateAttached, d.State())
	}
}

func TestDetachFromVM_Idempotent_WhenReserved(t *testing.T) {
	d := NewReserved(0, defaultConfig())
	err := d.DetachFromVM(context.Background(), &mockVMVPMemRemover{})
	if err != nil {
		t.Fatalf("unexpected error detaching reserved device: %v", err)
	}
}

func TestDetachFromVM_Idempotent_WhenDetached(t *testing.T) {
	d := attachedDevice(t)
	_ = d.DetachFromVM(context.Background(), &mockVMVPMemRemover{})
	// Second detach should be idempotent.
	err := d.DetachFromVM(context.Background(), &mockVMVPMemRemover{})
	if err != nil {
		t.Fatalf("unexpected error on idempotent detach: %v", err)
	}
}

func TestReserveMount_Success(t *testing.T) {
	d := attachedDevice(t)
	cfg := mount.MountConfig{}
	m, err := d.ReserveMount(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil mount")
	}
	if m.State() != mount.MountStateReserved {
		t.Errorf("expected mount state %d, got %d", mount.MountStateReserved, m.State())
	}
}

func TestReserveMount_SuccessFromReservedDevice(t *testing.T) {
	d := NewReserved(0, defaultConfig())
	cfg := mount.MountConfig{}
	m, err := d.ReserveMount(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil mount")
	}
}

func TestReserveMount_DuplicateReturnsSameMount(t *testing.T) {
	d := attachedDevice(t)
	cfg := mount.MountConfig{}
	m1, err := d.ReserveMount(context.Background(), cfg)
	if err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	m2, err := d.ReserveMount(context.Background(), cfg)
	if err != nil {
		t.Fatalf("second reserve: %v", err)
	}
	if m1 != m2 {
		t.Error("expected same mount object on duplicate reservation")
	}
}

func TestReserveMount_ErrorWhenDetached(t *testing.T) {
	d := NewReserved(0, defaultConfig())
	_ = d.AttachToVM(context.Background(), &mockVMVPMemAdder{err: errors.New("fail")})
	_, err := d.ReserveMount(context.Background(), mount.MountConfig{})
	if err == nil {
		t.Fatal("expected error when device is detached")
	}
}

func TestMountToGuest_Success(t *testing.T) {
	d := attachedDevice(t)
	cfg := mount.MountConfig{}
	_, err := d.ReserveMount(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ReserveMount: %v", err)
	}
	guestPath, err := d.MountToGuest(context.Background(), &mockLinuxGuestVPMemMounter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if guestPath == "" {
		t.Error("expected non-empty guestPath")
	}
}

func TestMountToGuest_ErrorWhenNotAttached(t *testing.T) {
	d := NewReserved(0, defaultConfig())
	_, err := d.MountToGuest(context.Background(), &mockLinuxGuestVPMemMounter{})
	if err == nil {
		t.Fatal("expected error when device is not attached")
	}
}

func TestMountToGuest_NoMount(t *testing.T) {
	d := attachedDevice(t)
	_, err := d.MountToGuest(context.Background(), &mockLinuxGuestVPMemMounter{})
	if err == nil {
		t.Fatal("expected error for unreserved mount")
	}
}

func TestMountToGuest_MountError(t *testing.T) {
	d := attachedDevice(t)
	cfg := mount.MountConfig{}
	_, err := d.ReserveMount(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ReserveMount: %v", err)
	}
	_, err = d.MountToGuest(context.Background(), &mockLinuxGuestVPMemMounter{err: errors.New("mount fail")})
	if err != nil {
		// This is expected - the mount error propagates.
		return
	}
}

func TestUnmountFromGuest_Success(t *testing.T) {
	d := attachedDevice(t)
	cfg := mount.MountConfig{}
	_, err := d.ReserveMount(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ReserveMount: %v", err)
	}
	_, err = d.MountToGuest(context.Background(), &mockLinuxGuestVPMemMounter{})
	if err != nil {
		t.Fatalf("MountToGuest: %v", err)
	}
	err = d.UnmountFromGuest(context.Background(), &mockLinuxGuestVPMemUnmounter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnmountFromGuest_NoMount_IsNoOp(t *testing.T) {
	d := attachedDevice(t)
	err := d.UnmountFromGuest(context.Background(), &mockLinuxGuestVPMemUnmounter{})
	if err != nil {
		t.Fatalf("expected nil error for missing mount, got: %v", err)
	}
}

func TestUnmountFromGuest_SucceedsWhenNotAttached(t *testing.T) {
	d := NewReserved(0, defaultConfig())
	err := d.UnmountFromGuest(context.Background(), &mockLinuxGuestVPMemUnmounter{})
	if err != nil {
		t.Fatalf("expected nil error for missing mount on non-attached device, got: %v", err)
	}
}

func TestUnmountFromGuest_UnmountError(t *testing.T) {
	d := attachedDevice(t)
	cfg := mount.MountConfig{}
	_, err := d.ReserveMount(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ReserveMount: %v", err)
	}
	_, err = d.MountToGuest(context.Background(), &mockLinuxGuestVPMemMounter{})
	if err != nil {
		t.Fatalf("MountToGuest: %v", err)
	}
	unmountErr := errors.New("unmount fail")
	err = d.UnmountFromGuest(context.Background(), &mockLinuxGuestVPMemUnmounter{err: unmountErr})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, unmountErr) {
		t.Errorf("expected wrapped error %v, got %v", unmountErr, err)
	}
}

func TestUnmountFromGuest_RemovesMountWhenFullyUnmounted(t *testing.T) {
	d := attachedDevice(t)
	cfg := mount.MountConfig{}
	_, err := d.ReserveMount(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ReserveMount: %v", err)
	}
	_, err = d.MountToGuest(context.Background(), &mockLinuxGuestVPMemMounter{})
	if err != nil {
		t.Fatalf("MountToGuest: %v", err)
	}
	err = d.UnmountFromGuest(context.Background(), &mockLinuxGuestVPMemUnmounter{})
	if err != nil {
		t.Fatalf("UnmountFromGuest: %v", err)
	}
	// After full unmount the mount should be removed, so detach should succeed.
	err = d.DetachFromVM(context.Background(), &mockVMVPMemRemover{})
	if err != nil {
		t.Fatalf("DetachFromVM after unmount: %v", err)
	}
	if d.State() != DeviceStateDetached {
		t.Errorf("expected state %d, got %d", DeviceStateDetached, d.State())
	}
}

func TestUnmountFromGuest_RetryAfterDetachFailure(t *testing.T) {
	d := attachedDevice(t)
	cfg := mount.MountConfig{}
	_, err := d.ReserveMount(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ReserveMount: %v", err)
	}
	_, err = d.MountToGuest(context.Background(), &mockLinuxGuestVPMemMounter{})
	if err != nil {
		t.Fatalf("MountToGuest: %v", err)
	}
	// Unmount succeeds and removes the mount.
	err = d.UnmountFromGuest(context.Background(), &mockLinuxGuestVPMemUnmounter{})
	if err != nil {
		t.Fatalf("UnmountFromGuest: %v", err)
	}
	// Detach fails (e.g. transient error).
	err = d.DetachFromVM(context.Background(), &mockVMVPMemRemover{err: errors.New("transient")})
	if err == nil {
		t.Fatal("expected detach error")
	}
	// Retry: unmount is a no-op since mount is gone.
	err = d.UnmountFromGuest(context.Background(), &mockLinuxGuestVPMemUnmounter{})
	if err != nil {
		t.Fatalf("retry UnmountFromGuest should be no-op, got: %v", err)
	}
	// Retry detach now succeeds.
	err = d.DetachFromVM(context.Background(), &mockVMVPMemRemover{})
	if err != nil {
		t.Fatalf("retry DetachFromVM: %v", err)
	}
	if d.State() != DeviceStateDetached {
		t.Errorf("expected state %d, got %d", DeviceStateDetached, d.State())
	}
}
