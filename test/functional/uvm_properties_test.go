//go:build windows && functional
// +build windows,functional

package functional

import (
	"context"
	"testing"

	"github.com/Microsoft/hcsshim/osversion"

	"github.com/Microsoft/hcsshim/internal/gcs"
	"github.com/Microsoft/hcsshim/test/internal/util"
	"github.com/Microsoft/hcsshim/test/pkg/require"
	testuvm "github.com/Microsoft/hcsshim/test/pkg/uvm"
)

func TestPropertiesGuestConnection_LCOW(t *testing.T) {
	require.Build(t, osversion.RS5)
	requireFeatures(t, featureLCOW, featureUVM)

	ctx := util.Context(context.Background(), t)
	uvm := testuvm.CreateAndStart(ctx, t, defaultLCOWOptions(ctx, t))
	defer uvm.Close()

	p, gc := uvm.Capabilities()
	if !gc.IsNamespaceAddRequestSupported() ||
		!gc.IsSignalProcessSupported() ||
		p < 4 {
		t.Fatalf("unexpected values: %d %+v", p, gc)
	}

	// check the type of the capabilities
	gdc := gcs.GetLCOWCapabilities(gc)
	if gdc == nil {
		t.Fatal("capabilities are unexpected type")
	}
}

func TestPropertiesGuestConnection_WCOW(t *testing.T) {
	require.Build(t, osversion.RS5)
	requireFeatures(t, featureWCOW, featureUVM)

	ctx := util.Context(context.Background(), t)
	uvm := testuvm.CreateAndStart(ctx, t, defaultWCOWOptions(ctx, t))
	defer uvm.Close()

	p, gc := uvm.Capabilities()
	if !gc.IsNamespaceAddRequestSupported() ||
		!gc.IsSignalProcessSupported() ||
		p < 4 {
		t.Fatalf("unexpected values: %d %+v", p, gc)
	}

	// check the type of the capabilities
	gdc := gcs.GetWCOWCapabilities(gc)
	if gdc == nil {
		t.Fatal("capabilities are unexpected type")
	}
}
