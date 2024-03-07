//go:build windows && functional
// +build windows,functional

package functional

import (
	"context"
	"errors"
	"testing"

	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"

	"github.com/Microsoft/hcsshim/test/internal/util"
	"github.com/Microsoft/hcsshim/test/pkg/require"
	testuvm "github.com/Microsoft/hcsshim/test/pkg/uvm"
)

// TestVSMB tests adding/removing VSMB layers from a v2 Windows utility VM.
func TestVSMB(t *testing.T) {
	t.Skip("not yet updated")

	require.Build(t, osversion.RS5)
	requireFeatures(t, featureWCOW, featureUVM, featureVSMB)

	ctx := util.Context(context.Background(), t)
	//nolint:staticcheck // SA1019: deprecated; will be replaced when test is updated
	uvm, _, _ := testuvm.CreateWCOWUVM(ctx, t, t.Name(), "microsoft/nanoserver")
	defer uvm.Close()

	dir := t.TempDir()
	var iterations uint32 = 64
	options := uvm.DefaultVSMBOptions(true)
	options.TakeBackupPrivilege = true
	for i := 0; i < int(iterations); i++ {
		if _, err := uvm.AddVSMB(ctx, dir, options); err != nil {
			t.Fatalf("AddVSMB failed: %s", err)
		}
	}

	// Remove them all
	for i := 0; i < int(iterations); i++ {
		if err := uvm.RemoveVSMB(ctx, dir, true); err != nil {
			t.Fatalf("RemoveVSMB failed: %s", err)
		}
	}
}

// TODO: VSMB for mapped directories

func TestVSMB_Writable(t *testing.T) {
	t.Skip("not yet updated")

	require.Build(t, osversion.RS5)
	requireFeatures(t, featureWCOW, featureUVM, featureVSMB)
	ctx := util.Context(context.Background(), t)

	opts := uvm.NewDefaultOptionsWCOW(t.Name(), "")
	opts.NoWritableFileShares = true
	//nolint:staticcheck // SA1019: deprecated; will be replaced when test is updated
	vm, _, _ := testuvm.CreateWCOWUVMFromOptsWithImage(ctx, t, opts, "microsoft/nanoserver")
	defer vm.Close()

	dir := t.TempDir()
	options := vm.DefaultVSMBOptions(true)
	options.TakeBackupPrivilege = true
	options.ReadOnly = false
	_, err := vm.AddVSMB(ctx, dir, options)
	defer func() {
		if err == nil {
			return
		}
		if err = vm.RemoveVSMB(ctx, dir, true); err != nil {
			t.Fatalf("RemoveVSMB failed: %s", err)
		}
	}()

	if !errors.Is(err, hcs.ErrOperationDenied) {
		t.Fatalf("AddVSMB should have failed with %v instead of: %v", hcs.ErrOperationDenied, err)
	}

	options.ReadOnly = true
	_, err = vm.AddVSMB(ctx, dir, options)
	if err != nil {
		t.Fatalf("AddVSMB failed: %s", err)
	}
}
