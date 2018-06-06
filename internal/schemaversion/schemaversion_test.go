package schemaversion

import (
	"testing"

	_ "github.com/Microsoft/hcsshim/functional/manifest"
	"github.com/Microsoft/hcsshim/internal/osversion"
)

func TestDetermineSchemaVersion(t *testing.T) {
	osv := osversion.Get()

	if osv.Build >= osversion.RS5 {
		if sv := DetermineSchemaVersion(nil); !sv.IsV10() { // TODO: Toggle this at some point so default is 2.0
			t.Fatalf("expected v1")
		}
		if sv := DetermineSchemaVersion(SchemaV20()); !sv.IsV20() {
			t.Fatalf("expected requested v2")
		}
		if sv := DetermineSchemaVersion(SchemaV10()); !sv.IsV10() {
			t.Fatalf("expected requested v1")
		}
		if sv := DetermineSchemaVersion(&SchemaVersion{}); !sv.IsV10() { // Logs a warning that 0.0 is ignored // TODO: Toggle this too
			t.Fatalf("expected requested v1")
		}

		if err := SchemaV20().IsSupported(); err != nil {
			t.Fatalf("v2 expected to be supported")
		}
		if err := SchemaV10().IsSupported(); err != nil {
			t.Fatalf("v1 expected to be supported")
		}

	} else {
		if sv := DetermineSchemaVersion(nil); !sv.IsV10() {
			t.Fatalf("expected v1")
		}
		// Pre RS5 will downgrade to v1 even if request v2
		if sv := DetermineSchemaVersion(SchemaV20()); !sv.IsV10() { // Logs a warning that 2.0 is ignored.
			t.Fatalf("expected requested v1")
		}
		if sv := DetermineSchemaVersion(SchemaV10()); !sv.IsV10() {
			t.Fatalf("expected requested v1")
		}
		if sv := DetermineSchemaVersion(&SchemaVersion{}); !sv.IsV10() { // Log a warning that 0.0 is ignored
			t.Fatalf("expected requested v1")
		}

		if err := SchemaV20().IsSupported(); err == nil {
			t.Fatalf("didn't expect v2 to be supported")
		}
		if err := SchemaV10().IsSupported(); err != nil {
			t.Fatalf("v1 expected to be supported")
		}
	}
}
