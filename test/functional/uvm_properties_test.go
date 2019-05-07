// +build functional uvmproperties

package functional

import (
	"os"
	"testing"

	"github.com/Microsoft/hcsshim/osversion"
	testutilities "github.com/Microsoft/hcsshim/test/functional/utilities"
)

func TestPropertiesGuestConnection_LCOW(t *testing.T) {
	testutilities.RequiresBuild(t, osversion.RS5)

	uvm := testutilities.CreateLCOWUVM(t, t.Name())
	defer uvm.Close()

	p, gc := uvm.Capabilities()
	if gc.NamespaceAddRequestSupported ||
		!gc.SignalProcessSupported ||
		p < 4 {
		t.Fatalf("unexpected values: %d %+v", p, gc)
	}
}

func TestPropertiesGuestConnection_WCOW(t *testing.T) {
	testutilities.RequiresBuild(t, osversion.RS5)
	uvm, _, uvmScratchDir := testutilities.CreateWCOWUVM(t, t.Name(), "microsoft/nanoserver")
	defer os.RemoveAll(uvmScratchDir)
	defer uvm.Close()

	p, gc := uvm.Capabilities()
	if !gc.NamespaceAddRequestSupported ||
		!gc.SignalProcessSupported ||
		p < 4 {
		t.Fatalf("unexpected values: %d %+v", p, gc)
	}
}
