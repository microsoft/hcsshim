//go:build windows && (lcow || wcow)

package vpci

import (
	"errors"
	"testing"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/containerd/errdefs"
)

func TestSave_EmptyOK(t *testing.T) {
	c := &Controller{
		devices:      map[guid.GUID]*deviceInfo{},
		deviceToGUID: map[Device]guid.GUID{},
	}

	if err := c.Save(); err != nil {
		t.Fatalf("Save on empty controller: %v", err)
	}
}

func TestSave_NonEmptyErrors(t *testing.T) {
	g := guid.GUID{}
	dev := Device{DeviceInstanceID: "PCI\\VEN_X"}

	c := &Controller{
		devices:      map[guid.GUID]*deviceInfo{g: {device: dev, vmBusGUID: g, state: StateReady, refCount: 1}},
		deviceToGUID: map[Device]guid.GUID{dev: g},
	}

	// Save is unsupported here, so it must report a failed precondition
	// that callers can detect with errors.Is.
	err := c.Save()
	if err == nil {
		t.Fatal("expected Save to error when devices are present")
	}
	if !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("expected ErrFailedPrecondition, got %v", err)
	}
}
