//go:build windows && lcow

package mount

import (
	"context"
	"testing"

	scsisave "github.com/Microsoft/hcsshim/internal/controller/device/scsi/save"
)

func TestSave_ErrorWhenUnmounted(t *testing.T) {
	m := mountedMount(t)
	if err := m.UnmountFromGuest(context.Background(), newDefaultUnmounter()); err != nil {
		t.Fatalf("setup UnmountFromGuest: %v", err)
	}
	if _, err := m.Save(); err == nil {
		t.Fatal("expected error saving an unmounted mount")
	}
}

func TestSave_Mounted_RoundTripsConfig(t *testing.T) {
	m := NewReserved(2, 3, Config{
		Partition:  1,
		ReadOnly:   true,
		Encrypted:  true,
		Options:    []string{"noatime"},
		Filesystem: "ext4",
	})
	if _, err := m.MountToGuest(context.Background(), newDefaultMounter()); err != nil {
		t.Fatalf("setup MountToGuest: %v", err)
	}

	state, err := m.Save()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The snapshot exposes the guest path and config the caller will restore from.
	if state.GetGuestPath() != m.GuestPath() {
		t.Errorf("expected guest path %q, got %q", m.GuestPath(), state.GetGuestPath())
	}
	c := state.GetConfig()
	if !c.GetReadOnly() || !c.GetEncrypted() || c.GetFilesystem() != "ext4" {
		t.Errorf("unexpected config snapshot: %+v", c)
	}
}

func TestSave_Reserved(t *testing.T) {
	// A reserved (not-yet-mounted) mount can still be snapshotted.
	if _, err := NewReserved(0, 0, defaultConfig()).Save(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestImport_NilReturnsNil(t *testing.T) {
	if m := Import(nil, 0, 0, 0); m != nil {
		t.Errorf("expected nil mount, got %+v", m)
	}
}

func TestImport_NilConfig_UsesDefaults(t *testing.T) {
	// A snapshot without config still yields a live mount at the given partition.
	m := Import(&scsisave.MountState{GuestPath: "/run/mounts/scsi/0_0_7"}, 0, 0, 7)
	if m == nil {
		t.Fatal("expected non-nil mount")
	}
	if m.State() != StateMounted {
		t.Errorf("expected state %d, got %d", StateMounted, m.State())
	}
	if m.GuestPath() != "/run/mounts/scsi/0_0_7" {
		t.Errorf("unexpected guest path %q", m.GuestPath())
	}
}

func TestSaveImport_RoundTrip(t *testing.T) {
	state := &scsisave.MountState{
		Config: &scsisave.MountConfig{
			ReadOnly:   true,
			Options:    []string{"ro"},
			Filesystem: "ext4",
		},
		RefCount:  2,
		GuestPath: "/run/mounts/scsi/0_0_5",
	}

	m := Import(state, 0, 0, 5)
	if m == nil {
		t.Fatal("expected non-nil mount")
	}
	// An imported mount is live, exposing its guest path from the mounted state.
	if m.State() != StateMounted {
		t.Errorf("expected state %d, got %d", StateMounted, m.State())
	}
	if m.GuestPath() != state.GetGuestPath() {
		t.Errorf("expected guest path %q, got %q", state.GetGuestPath(), m.GuestPath())
	}

	out, err := m.Save()
	if err != nil {
		t.Fatalf("re-save: %v", err)
	}
	if out.GetRefCount() != state.GetRefCount() {
		t.Errorf("expected ref count %d, got %d", state.GetRefCount(), out.GetRefCount())
	}
}
