//go:build functional || uvmvsmb
// +build functional uvmvsmb

package functional

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/Microsoft/hcsshim/internal/errdefs"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"
	testutilities "github.com/Microsoft/hcsshim/test/functional/utilities"
)

// TestVSMB tests adding/removing VSMB layers from a v2 Windows utility VM
func TestVSMB(t *testing.T) {
	testutilities.RequiresBuild(t, osversion.RS5)
	uvm, _, uvmScratchDir := testutilities.CreateWCOWUVM(context.Background(), t, t.Name(), "microsoft/nanoserver")
	defer os.RemoveAll(uvmScratchDir)
	defer uvm.Close()

	dir := testutilities.CreateTempDir(t)
	defer os.RemoveAll(dir)
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
	testutilities.RequiresBuild(t, osversion.RS5)

	opts := uvm.NewDefaultOptionsWCOW(t.Name(), "")
	opts.NoWritableFileShares = true
	vm, _, uvmScratchDir := testutilities.CreateWCOWUVMFromOptsWithImage(context.Background(), t, opts, "microsoft/nanoserver")
	defer os.RemoveAll(uvmScratchDir)
	defer vm.Close()

	dir := testutilities.CreateTempDir(t)
	defer os.RemoveAll(dir)
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

	if !errors.Is(err, errdefs.ErrOperationDenied) {
		t.Fatalf("AddVSMB should have failed with %v instead of: %v", errdefs.ErrOperationDenied, err)
	}

	options.ReadOnly = true
	_, err = vm.AddVSMB(context.Background(), dir, options)
	if err != nil {
		t.Fatalf("AddVSMB failed: %s", err)
	}

}
