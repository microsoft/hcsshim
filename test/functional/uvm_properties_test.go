//go:build windows && functional
// +build windows,functional

package functional

import (
	"context"
	"testing"

	"github.com/Microsoft/hcsshim/osversion"

	"github.com/Microsoft/hcsshim/test/internal/util"
	"github.com/Microsoft/hcsshim/test/pkg/require"
	testuvm "github.com/Microsoft/hcsshim/test/pkg/uvm"
)

func TestPropertiesGuestConnection_LCOW(t *testing.T) {
	t.Skip("not yet updated")

	require.Build(t, osversion.RS5)
	requireFeatures(t, featureLCOW, featureUVM)

	ctx := util.Context(context.Background(), t)
	uvm := testuvm.CreateAndStartLCOWFromOpts(ctx, t, defaultLCOWOptions(ctx, t))
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
	requireFeatures(t, featureWCOW, featureUVM)

	ctx := util.Context(context.Background(), t)
	//nolint:staticcheck // SA1019: deprecated; will be replaced when test is updated
	uvm, _, _ := testuvm.CreateWCOWUVM(ctx, t, t.Name(), "microsoft/nanoserver")
	defer uvm.Close()

	p, gc := uvm.Capabilities()
	if !gc.NamespaceAddRequestSupported ||
		!gc.SignalProcessSupported ||
		p < 4 {
		t.Fatalf("unexpected values: %d %+v", p, gc)
	}
}
