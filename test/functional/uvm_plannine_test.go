//go:build functional || uvmp9
// +build functional uvmp9

// This file isn't called uvm_plan9_test.go as go test skips when a number is in it... go figure (pun intended)

package functional

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"
	testutilities "github.com/Microsoft/hcsshim/test/functional/utilities"
)

// TestPlan9 tests adding/removing Plan9 shares to/from a v2 Linux utility VM
// TODO: This is very basic. Need multiple shares and so-on. Can be iterated on later.
func TestPlan9(t *testing.T) {
	testutilities.RequiresBuild(t, osversion.RS5)

	vm := testutilities.CreateLCOWUVM(context.Background(), t, t.Name())
	defer vm.Close()

	dir := testutilities.CreateTempDir(t)
	defer os.RemoveAll(dir)
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
	testutilities.RequiresBuild(t, osversion.RS5)

	opts := uvm.NewDefaultOptionsLCOW(t.Name(), "")
	opts.NoWritableFileShares = true
	vm := testutilities.CreateLCOWUVMFromOpts(context.Background(), t, opts)
	defer vm.Close()

	dir := testutilities.CreateTempDir(t)
	defer os.RemoveAll(dir)

	// mount as writable should fail
	share, err := vm.AddPlan9(context.Background(), dir, fmt.Sprintf("/tmp/%s", filepath.Base(dir)), false, false, nil)
	defer func() {
		if share == nil {
			return
		}
		if err := vm.RemovePlan9(context.Background(), share); err != nil {
			t.Fatalf("RemovePlan9 failed: %s", err)
		}
	}()
	if !errors.Is(err, hcs.ErrOperationDenied) {
		t.Fatalf("AddPlan9 should have failed with %v instead of: %v", hcs.ErrOperationDenied, err)
	}

	// mount as read-only should succeed
	share, err = vm.AddPlan9(context.Background(), dir, fmt.Sprintf("/tmp/%s", filepath.Base(dir)), true, false, nil)
	if err != nil {
		t.Fatalf("AddPlan9 failed: %v", err)
	}
}
