//go:build windows && (lcow || wcow)

package disk

import (
	"context"
	"testing"

	"github.com/Microsoft/hcsshim/internal/controller/device/scsi/mount"
	scsisave "github.com/Microsoft/hcsshim/internal/controller/device/scsi/save"
)

func TestSave_ErrorWhenDetached(t *testing.T) {
	d := attachedDisk(t)
	if err := d.DetachFromVM(context.Background(), &mockVMSCSIRemover{}, newDefaultEjector()); err != nil {
		t.Fatalf("setup DetachFromVM: %v", err)
	}
	if _, err := d.Save(); err == nil {
		t.Fatal("expected error saving a detached disk")
	}
}

func TestSave_ReservedDisk_RoundTripsConfig(t *testing.T) {
	cfg := Config{
		HostPath: `C:\test\disk.vhdx`,
		ReadOnly: true,
		Type:     TypePassThru,
		EVDType:  "evd",
	}
	state, err := NewReserved(0, 0, cfg).Save()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The snapshot exposes the host-side config the caller supplied.
	c := state.GetConfig()
	if c.GetHostPath() != cfg.HostPath || c.GetReadOnly() != cfg.ReadOnly ||
		c.GetType() != string(cfg.Type) || c.GetEvdType() != cfg.EVDType {
		t.Errorf("unexpected config snapshot: %+v", c)
	}
	if len(state.GetMounts()) != 0 {
		t.Errorf("expected no mounts, got %d", len(state.GetMounts()))
	}
}

func TestSave_IncludesReservedMounts(t *testing.T) {
	d := attachedDisk(t)
	if _, err := d.ReservePartition(context.Background(), mount.Config{Partition: 1}); err != nil {
		t.Fatalf("ReservePartition: %v", err)
	}
	state, err := d.Save()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := state.GetMounts()[1]; !ok {
		t.Errorf("expected snapshot to include partition 1, got %v", state.GetMounts())
	}
}

func TestImport_NilReturnsNil(t *testing.T) {
	if d := Import(nil, 0, 0); d != nil {
		t.Errorf("expected nil disk, got %+v", d)
	}
}

func TestImport_NilConfig_UsesDefaults(t *testing.T) {
	// A snapshot without config still yields a usable, live disk.
	d := Import(&scsisave.DiskState{}, 0, 0)
	if d == nil {
		t.Fatal("expected non-nil disk")
	}
	if d.State() != StateAttached {
		t.Errorf("expected state %d, got %d", StateAttached, d.State())
	}
	if d.HostPath() != "" {
		t.Errorf("expected empty host path, got %q", d.HostPath())
	}
}

func TestImport_SkipsNilMountEntry(t *testing.T) {
	// A malformed (nil) mount entry must not break import.
	d := Import(&scsisave.DiskState{Mounts: map[uint64]*scsisave.MountState{1: nil}}, 0, 0)
	if d == nil {
		t.Fatal("expected non-nil disk")
	}
	if d.State() != StateAttached {
		t.Errorf("expected state %d, got %d", StateAttached, d.State())
	}
}

func TestUpdateHostPath(t *testing.T) {
	d := NewReserved(0, 0, defaultConfig())
	const newPath = `C:\new\path.vhdx`
	d.UpdateHostPath(newPath)
	if d.HostPath() != newPath {
		t.Errorf("expected host path %q, got %q", newPath, d.HostPath())
	}
}

func TestSaveImport_RoundTrip(t *testing.T) {
	state := &scsisave.DiskState{
		Config: &scsisave.DiskConfig{
			HostPath: `C:\test\disk.vhdx`,
			ReadOnly: true,
			Type:     string(TypeVirtualDisk),
			EvdType:  "evd",
		},
		Mounts: map[uint64]*scsisave.MountState{2: {}},
	}

	d := Import(state, 3, 4)
	if d == nil {
		t.Fatal("expected non-nil disk")
	}
	// An imported disk is live, so its config is queryable and it can be re-saved.
	if d.State() != StateAttached {
		t.Errorf("expected state %d, got %d", StateAttached, d.State())
	}
	if d.HostPath() != state.GetConfig().GetHostPath() {
		t.Errorf("expected host path %q, got %q", state.GetConfig().GetHostPath(), d.HostPath())
	}

	out, err := d.Save()
	if err != nil {
		t.Fatalf("re-save: %v", err)
	}
	if _, ok := out.GetMounts()[2]; !ok {
		t.Errorf("expected re-saved snapshot to include partition 2, got %v", out.GetMounts())
	}
}
