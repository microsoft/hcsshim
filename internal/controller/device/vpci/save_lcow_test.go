//go:build windows && (lcow || wcow)

package vpci

import (
	"testing"

	"github.com/Microsoft/go-winio/pkg/guid"
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

	if err := c.Save(); err == nil {
		t.Fatal("expected Save to error when devices are present")
	}
}
