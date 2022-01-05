//go:build functional || uvmproperties
// +build functional uvmproperties

package functional

import (
	"context"
	"testing"

	"github.com/Microsoft/hcsshim/osversion"
	testutilities "github.com/Microsoft/hcsshim/test/functional/utilities"
)

func TestPropertiesGuestConnection_LCOW(t *testing.T) {
	testutilities.RequiresBuild(t, osversion.RS5)

	uvm := testutilities.CreateLCOWUVMFromOpts(context.Background(), t, nil, getDefaultLCOWUvmOptions(t, t.Name()))

	p, gc := uvm.Capabilities()
	if gc.NamespaceAddRequestSupported ||
		!gc.SignalProcessSupported ||
		p < 4 {
		t.Fatalf("unexpected values: %d %+v", p, gc)
	}
}

func TestPropertiesGuestConnection_WCOW(t *testing.T) {
	testutilities.RequiresBuild(t, osversion.RS5)
	client, ctx := newCtrdClient(context.Background(), t)
	uvm, _, _ := testutilities.CreateWCOWUVM(ctx, t, client, t.Name(), testutilities.ImageWindowsNanoserver1809)

	p, gc := uvm.Capabilities()
	if !gc.NamespaceAddRequestSupported ||
		!gc.SignalProcessSupported ||
		p < 4 {
		t.Fatalf("unexpected values: %d %+v", p, gc)
	}
}
