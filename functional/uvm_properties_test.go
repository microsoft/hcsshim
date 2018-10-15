// +build functional uvmproperties

package functional

import (
	"os"
	"testing"

	"github.com/Microsoft/hcsshim/functional/utilities"
	"github.com/Microsoft/hcsshim/internal/schema1"
	"github.com/Microsoft/hcsshim/osversion"
)

func TestPropertiesGuestConnection_LCOW(t *testing.T) {
	testutilities.RequiresBuild(t, osversion.RS5)
	tempDir := testutilities.CreateTempDir(t)
	defer os.RemoveAll(tempDir)

	uvm := testutilities.CreateLCOWUVM(t, "TestCreateLCOWScratch")
	defer uvm.Terminate()

	p, err := uvm.ComputeSystem().Properties(schema1.PropertyTypeGuestConnection)
	if err != nil {
		t.Fatalf("Failed to query properties: %s", err)
	}

	if p.GuestConnectionInfo.GuestDefinedCapabilities.NamespaceAddRequestSupported ||
		!p.GuestConnectionInfo.GuestDefinedCapabilities.SignalProcessSupported ||
		p.GuestConnectionInfo.ProtocolVersion < 4 {
		t.Fatalf("unexpected values: %+v", p.GuestConnectionInfo)
	}
}

func TestPropertiesGuestConnection_WCOW(t *testing.T) {
	testutilities.RequiresBuild(t, osversion.RS5)
	imageName := "microsoft/nanoserver"
	layers := testutilities.LayerFolders(t, imageName)
	uvm, uvmScratchDir := testutilities.CreateWCOWUVM(t, layers, "", nil)
	defer os.RemoveAll(uvmScratchDir)
	defer uvm.Terminate()

	p, err := uvm.ComputeSystem().Properties(schema1.PropertyTypeGuestConnection)
	if err != nil {
		t.Fatalf("Failed to query properties: %s", err)
	}

	if !p.GuestConnectionInfo.GuestDefinedCapabilities.NamespaceAddRequestSupported ||
		!p.GuestConnectionInfo.GuestDefinedCapabilities.SignalProcessSupported ||
		p.GuestConnectionInfo.ProtocolVersion < 4 {
		t.Fatalf("unexpected values: %+v", p.GuestConnectionInfo)
	}
}
