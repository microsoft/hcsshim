//go:build windows && functional
// +build windows,functional

// This file isn't called uvm_plan9_test.go as go assumes that it should only run on plan9 OS's... go figure (pun intended)

package functional

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"

	"github.com/Microsoft/hcsshim/test/internal/util"
	"github.com/Microsoft/hcsshim/test/pkg/require"
	testuvm "github.com/Microsoft/hcsshim/test/pkg/uvm"
)

// TestPlan9 tests adding/removing Plan9 shares to/from a v2 Linux utility VM
// TODO: This is very basic. Need multiple shares and so-on. Can be iterated on later.
func TestPlan9(t *testing.T) {
	t.Skip("not yet updated")

	require.Build(t, osversion.RS5)
	requireFeatures(t, featureLCOW, featureUVM, featurePlan9)
	ctx := util.Context(context.Background(), t)

	vm := testuvm.CreateAndStartLCOWFromOpts(ctx, t, defaultLCOWOptions(ctx, t))

	dir := t.TempDir()
	var iterations uint32 = 64
	var shares []*uvm.Plan9Share
	for i := 0; i < int(iterations); i++ {
		share, err := vm.AddPlan9(context.Background(), dir, fmt.Sprintf("/tmp/%s", filepath.Base(dir)), false, false, nil)
		if err != nil {
			t.Fatalf("AddPlan9 failed: %s", err)
		}
		shares = append(shares, share)
	}

	// Remove them all
	for _, share := range shares {
		if err := vm.RemovePlan9(context.Background(), share); err != nil {
			t.Fatalf("RemovePlan9 failed: %s", err)
		}
	}
}

func TestPlan9_Writable(t *testing.T) {
	require.Build(t, osversion.RS5)
	requireFeatures(t, featureLCOW, featureUVM, featurePlan9)
	ctx := util.Context(context.Background(), t)

	opts := defaultLCOWOptions(ctx, t)
	opts.NoWritableFileShares = true
	vm := testuvm.CreateAndStartLCOWFromOpts(ctx, t, opts)

	dir := t.TempDir()

	// mount as writable should fail
	share, err := vm.AddPlan9(ctx, dir, fmt.Sprintf("/tmp/%s", filepath.Base(dir)), false, false, nil)
	defer func() {
		if share == nil {
			return
		}
		if err := vm.RemovePlan9(ctx, share); err != nil {
			t.Fatalf("RemovePlan9 failed: %s", err)
		}
	}()
	if !errors.Is(err, hcs.ErrOperationDenied) {
		t.Fatalf("AddPlan9 should have failed with %v instead of: %v", hcs.ErrOperationDenied, err)
	}

	// mount as read-only should succeed
	share, err = vm.AddPlan9(ctx, dir, fmt.Sprintf("/tmp/%s", filepath.Base(dir)), true, false, nil)
	if err != nil {
		t.Fatalf("AddPlan9 failed: %v", err)
	}
}
