//go:build functional || uvmproperties
// +build functional uvmproperties

package functional

import (
	"context"
	"testing"

	"github.com/Microsoft/hcsshim/osversion"
	"github.com/Microsoft/hcsshim/test/testutil"
)

func TestPropertiesGuestConnection_LCOW(t *testing.T) {
	testutil.RequiresBuild(t, osversion.RS5)

	uvm := testutil.CreateLCOWUVMFromOpts(context.Background(), t, nil, getDefaultLCOWUvmOptions(t, t.Name()))

	p, gc := uvm.Capabilities()
	if gc.NamespaceAddRequestSupported ||
		!gc.SignalProcessSupported ||
		p < 4 {
		t.Fatalf("unexpected values: %d %+v", p, gc)
	}
}

func TestPropertiesGuestConnection_WCOW(t *testing.T) {
	testutil.RequiresBuild(t, osversion.RS5)
	client, ctx := newCtrdClient(context.Background(), t)
	uvm, _, _ := testutil.CreateWCOWUVM(ctx, t, client, t.Name(), testutil.ImageWindowsNanoserver1809)

	p, gc := uvm.Capabilities()
	if !gc.NamespaceAddRequestSupported ||
		!gc.SignalProcessSupported ||
		p < 4 {
		t.Fatalf("unexpected values: %d %+v", p, gc)
	}
}
