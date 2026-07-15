//go:build windows && lcow

package plan9

import (
	"testing"

	"github.com/Microsoft/go-winio/pkg/guid"

	"github.com/Microsoft/hcsshim/internal/controller/device/plan9/share"
)

func TestSave_EmptyOK(t *testing.T) {
	c := &Controller{
		reservations:     map[guid.GUID]*reservation{},
		sharesByHostPath: map[string]*share.Share{},
	}

	if err := c.Save(); err != nil {
		t.Fatalf("Save on empty controller: %v", err)
	}
}

func TestSave_NonEmptyErrors(t *testing.T) {
	c := &Controller{
		reservations:     map[guid.GUID]*reservation{{}: {hostPath: "/h"}},
		sharesByHostPath: map[string]*share.Share{},
	}

	if err := c.Save(); err == nil {
		t.Fatal("expected Save to error when reservations are present")
	}
}
