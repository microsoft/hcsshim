//go:build windows && lcow

package plan9

import (
	"errors"
	"testing"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/containerd/errdefs"

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

	// Save is unsupported here, so it must report a failed precondition
	// that callers can detect with errors.Is.
	err := c.Save()
	if err == nil {
		t.Fatal("expected Save to error when reservations are present")
	}
	if !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("expected ErrFailedPrecondition, got %v", err)
	}
}
