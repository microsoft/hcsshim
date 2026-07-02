//go:build windows && (lcow || wcow)

package scsi

import (
	"context"
	"strings"
	"testing"

	"github.com/Microsoft/hcsshim/internal/controller/device/scsi/disk"
	scsisave "github.com/Microsoft/hcsshim/internal/controller/device/scsi/save"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"

	"github.com/Microsoft/go-winio/pkg/guid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// --- Helpers ---

func mustGUID(t *testing.T) guid.GUID {
	t.Helper()
	g, err := guid.NewV4()
	if err != nil {
		t.Fatalf("NewV4: %v", err)
	}
	return g
}

// wrapImport imports a hand-built payload wrapped with the SCSI save type url.
func wrapImport(t *testing.T, p *scsisave.Payload) *Controller {
	t.Helper()
	b, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	c, err := Import(t.Context(), &anypb.Any{TypeUrl: scsisave.TypeURL, Value: b})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	return c
}

// importedReserved snapshots a controller holding a single reservation and
// imports it, returning the migrating controller and that reservation's ID.
func importedReserved(t *testing.T) (*Controller, guid.GUID) {
	t.Helper()
	src, id := reservedController(t)
	env, err := src.Save(t.Context())
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	c, err := Import(t.Context(), env)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	return c, id
}

// --- Tests: Save + Import round trip ---

func TestSave_RoundTrip(t *testing.T) {
	src, id := mappedController(t)

	env, err := src.Save(t.Context())
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	// A caller sees a payload tagged with the SCSI save type.
	if env.GetTypeUrl() != scsisave.TypeURL {
		t.Fatalf("unexpected type url %q", env.GetTypeUrl())
	}

	dst, err := Import(t.Context(), env)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	// The attached disk and its host path survive the round trip.
	cfg := defaultDiskConfig()
	if disks := dst.Disks(); len(disks) != 1 || disks[0].HostPath != cfg.HostPath {
		t.Fatalf("unexpected disks after import: %+v", disks)
	}
	if !attachmentsContainPath(dst.HCSAttachments(), cfg.HostPath) {
		t.Errorf("expected %q in HCS attachments", cfg.HostPath)
	}

	// The reservation survives, so its disk path can be corrected before resume.
	const newPath = `C:\migrated\disk.vhdx`
	if err := dst.UpdateDiskHostPath(t.Context(), id, newPath); err != nil {
		t.Fatalf("UpdateDiskHostPath: %v", err)
	}
	if disks := dst.Disks(); len(disks) != 1 || disks[0].HostPath != newPath {
		t.Fatalf("expected host path %q after update, got %+v", newPath, disks)
	}
}

func TestSave_EmptyRoundTrip(t *testing.T) {
	env, err := New(2, &mockVMOps{}, newMockGuestOps()).Save(t.Context())
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	dst, err := Import(t.Context(), env)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if d := dst.Disks(); len(d) != 0 {
		t.Errorf("expected no disks, got %+v", d)
	}
	if a := dst.HCSAttachments(); len(a) != 0 {
		t.Errorf("expected no attachments, got %+v", a)
	}
}

// --- Tests: Import errors ---

func TestImport_Errors(t *testing.T) {
	wrap := func(p *scsisave.Payload) *anypb.Any {
		b, err := proto.Marshal(p)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return &anypb.Any{TypeUrl: scsisave.TypeURL, Value: b}
	}

	tests := []struct {
		name    string
		env     *anypb.Any
		wantErr string
	}{
		{"nil envelope", nil, "nil"},
		{"wrong type url", &anypb.Any{TypeUrl: "bogus"}, "unsupported scsi saved-state type"},
		{"corrupt payload", &anypb.Any{TypeUrl: scsisave.TypeURL, Value: []byte{0xff}}, "unmarshal"},
		{"schema mismatch", wrap(&scsisave.Payload{SchemaVersion: scsisave.SchemaVersion + 1}), "schema version"},
		{"slot out of range", wrap(&scsisave.Payload{
			SchemaVersion:  scsisave.SchemaVersion,
			NumControllers: 1,
			Disks:          map[uint32]*scsisave.DiskState{numLUNsPerController: {}},
		}), "invalid controller slot"},
		{"bad reservation id", wrap(&scsisave.Payload{
			SchemaVersion:  scsisave.SchemaVersion,
			NumControllers: 1,
			Reservations:   map[string]*scsisave.Reservation{"not-a-guid": {}},
		}), "invalid reservation id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Import(t.Context(), tt.env); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

// --- Tests: HCSAttachments schema mapping ---

func TestHCSAttachments(t *testing.T) {
	c := New(1, &mockVMOps{}, newMockGuestOps())
	cfg := disk.Config{HostPath: `C:\rootfs.vhdx`, ReadOnly: true, Type: disk.TypeVirtualDisk}
	if err := c.ReserveForRootfs(context.Background(), 0, 5, cfg); err != nil {
		t.Fatalf("ReserveForRootfs: %v", err)
	}

	// The disk surfaces under its controller GUID, keyed by LUN, with its config.
	s, ok := c.HCSAttachments()[guestrequest.ScsiControllerGuids[0]]
	if !ok {
		t.Fatalf("expected attachment under controller 0 guid, got %+v", c.HCSAttachments())
	}
	a, ok := s.Attachments["5"]
	if !ok {
		t.Fatalf("expected attachment at lun 5, got %+v", s.Attachments)
	}
	if a.Path != cfg.HostPath || a.Type_ != string(cfg.Type) || !a.ReadOnly {
		t.Errorf("unexpected attachment: %+v", a)
	}
}

// --- Tests: UpdateDiskHostPath ---

func TestUpdateDiskHostPath(t *testing.T) {
	resID := mustGUID(t)
	// Migrating controller whose lone reservation points at an empty slot.
	noDisk := wrapImport(t, &scsisave.Payload{
		SchemaVersion:  scsisave.SchemaVersion,
		NumControllers: 1,
		Reservations:   map[string]*scsisave.Reservation{resID.String(): {Slot: 5}},
	})

	t.Run("reservation not found", func(t *testing.T) {
		if err := noDisk.UpdateDiskHostPath(t.Context(), mustGUID(t), `C:\x.vhdx`); err == nil ||
			!strings.Contains(err.Error(), "not found") {
			t.Fatalf("expected not-found error, got %v", err)
		}
	})
	t.Run("disk not found", func(t *testing.T) {
		if err := noDisk.UpdateDiskHostPath(t.Context(), resID, `C:\x.vhdx`); err == nil ||
			!strings.Contains(err.Error(), "disk for reservation") {
			t.Fatalf("expected disk-not-found error, got %v", err)
		}
	})
	t.Run("same path is a no-op", func(t *testing.T) {
		c, id := importedReserved(t)
		if err := c.UpdateDiskHostPath(t.Context(), id, defaultDiskConfig().HostPath); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("rejected after resume", func(t *testing.T) {
		c, id := importedReserved(t)
		c.Resume(t.Context(), &mockVMOps{}, newMockGuestOps())
		if err := c.UpdateDiskHostPath(t.Context(), id, `C:\x.vhdx`); err == nil ||
			!strings.Contains(err.Error(), "migrating") {
			t.Fatalf("expected migrating error, got %v", err)
		}
	})
}
