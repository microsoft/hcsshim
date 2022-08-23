//go:build windows && (functional || uvmproperties)
// +build windows
// +build functional uvmproperties

package functional

import (
	"context"
	"testing"

	"github.com/Microsoft/hcsshim/osversion"
	"github.com/Microsoft/hcsshim/test/internal/require"
	tuvm "github.com/Microsoft/hcsshim/test/internal/uvm"
)

func TestPropertiesGuestConnection_LCOW(t *testing.T) {
	t.Skip("not yet updated")

	require.Build(t, osversion.RS5)
	requireFeatures(t, featureLCOW)

	uvm := tuvm.CreateAndStartLCOWFromOpts(context.Background(), t, defaultLCOWOptions(t))
	defer uvm.Close()

	p, gc := uvm.Capabilities()
	if gc.NamespaceAddRequestSupported ||
		!gc.SignalProcessSupported ||
		p < 4 {
		t.Fatalf("unexpected values: %d %+v", p, gc)
	}
}

func TestPropertiesGuestConnection_WCOW(t *testing.T) {
	t.Skip("not yet updated")

	require.Build(t, osversion.RS5)
	requireFeatures(t, featureWCOW)

	uvm, _, _ := tuvm.CreateWCOWUVM(context.Background(), t, t.Name(), "microsoft/nanoserver")
	defer uvm.Close()

	p, gc := uvm.Capabilities()
	if !gc.NamespaceAddRequestSupported ||
		!gc.SignalProcessSupported ||
		p < 4 {
		t.Fatalf("unexpected values: %d %+v", p, gc)
	}
}
