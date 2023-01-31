//go:build windows && (functional || uvmvsmb)
// +build windows
// +build functional uvmvsmb

package functional

import (
	"context"
	"errors"
	"testing"

	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"

	"github.com/Microsoft/hcsshim/test/internal/require"
	tuvm "github.com/Microsoft/hcsshim/test/internal/uvm"
)

// TestVSMB tests adding/removing VSMB layers from a v2 Windows utility VM
func TestVSMB(t *testing.T) {
	t.Skip("not yet updated")

	require.Build(t, osversion.RS5)
	requireFeatures(t, featureWCOW, featureVSMB)

	uvm, _, _ := tuvm.CreateWCOWUVM(context.Background(), t, t.Name(), "microsoft/nanoserver")
	defer uvm.Close()

	dir := t.TempDir()
	var iterations uint32 = 64
	options := uvm.DefaultVSMBOptions(true)
	options.TakeBackupPrivilege = true
	for i := 0; i < int(iterations); i++ {
		if _, err := uvm.AddVSMB(context.Background(), dir, options); err != nil {
			t.Fatalf("AddVSMB failed: %s", err)
		}
	}

	// Remove them all
	for i := 0; i < int(iterations); i++ {
		if err := uvm.RemoveVSMB(context.Background(), dir, true); err != nil {
			t.Fatalf("RemoveVSMB failed: %s", err)
		}
	}
}

// TODO: VSMB for mapped directories

func TestVSMB_Writable(t *testing.T) {
	t.Skip("not yet updated")

	require.Build(t, osversion.RS5)
	requireFeatures(t, featureWCOW, featureVSMB)

	opts := uvm.NewDefaultOptionsWCOW(t.Name(), "")
	opts.NoWritableFileShares = true
	vm, _, _ := tuvm.CreateWCOWUVMFromOptsWithImage(context.Background(), t, opts, "microsoft/nanoserver")
	defer vm.Close()

	dir := t.TempDir()
	options := vm.DefaultVSMBOptions(true)
	options.TakeBackupPrivilege = true
	options.ReadOnly = false
	_, err := vm.AddVSMB(context.Background(), dir, options)
	defer func() {
		if err == nil {
			return
		}
		if err = vm.RemoveVSMB(context.Background(), dir, true); err != nil {
			t.Fatalf("RemoveVSMB failed: %s", err)
		}
	}()

	if !errors.Is(err, hcs.ErrOperationDenied) {
		t.Fatalf("AddVSMB should have failed with %v instead of: %v", hcs.ErrOperationDenied, err)
	}

	options.ReadOnly = true
	_, err = vm.AddVSMB(context.Background(), dir, options)
	if err != nil {
		t.Fatalf("AddVSMB failed: %s", err)
	}
}
