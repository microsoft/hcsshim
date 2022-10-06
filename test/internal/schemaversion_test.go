//go:build windows

package internal

import (
	"io"
	"testing"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/sirupsen/logrus"

	_ "github.com/Microsoft/hcsshim/test/internal/manifest"
)

func init() {
	logrus.SetOutput(io.Discard)
}

func TestDetermineSchemaVersion(t *testing.T) {
	t.Helper()
	osv := osversion.Get()

	if osv.Build >= osversion.RS5 {
		if sv := schemaversion.DetermineSchemaVersion(nil); !schemaversion.IsV21(sv) {
			t.Fatalf("expected v2")
		}
		if sv := schemaversion.DetermineSchemaVersion(schemaversion.SchemaV21()); !schemaversion.IsV21(sv) {
			t.Fatalf("expected requested v2")
		}
		if sv := schemaversion.DetermineSchemaVersion(schemaversion.SchemaV10()); !schemaversion.IsV10(sv) {
			t.Fatalf("expected requested v1")
		}
		if sv := schemaversion.DetermineSchemaVersion(&hcsschema.Version{}); !schemaversion.IsV21(sv) {
			t.Fatalf("expected requested v2")
		}

		if err := schemaversion.IsSupported(schemaversion.SchemaV21()); err != nil {
			t.Fatalf("v2 expected to be supported")
		}
		if err := schemaversion.IsSupported(schemaversion.SchemaV10()); err != nil {
			t.Fatalf("v1 expected to be supported")
		}
	} else {
		if sv := schemaversion.DetermineSchemaVersion(nil); !schemaversion.IsV10(sv) {
			t.Fatalf("expected v1")
		}
		// Pre RS5 will downgrade to v1 even if request v2
		if sv := schemaversion.DetermineSchemaVersion(schemaversion.SchemaV21()); !schemaversion.IsV10(sv) {
			t.Fatalf("expected requested v1")
		}
		if sv := schemaversion.DetermineSchemaVersion(schemaversion.SchemaV10()); !schemaversion.IsV10(sv) {
			t.Fatalf("expected requested v1")
		}
		if sv := schemaversion.DetermineSchemaVersion(&hcsschema.Version{}); !schemaversion.IsV10(sv) {
			t.Fatalf("expected requested v1")
		}

		if err := schemaversion.IsSupported(schemaversion.SchemaV21()); err == nil {
			t.Fatalf("didn't expect v2 to be supported")
		}
		if err := schemaversion.IsSupported(schemaversion.SchemaV10()); err != nil {
			t.Fatalf("v1 expected to be supported")
		}
	}
}
