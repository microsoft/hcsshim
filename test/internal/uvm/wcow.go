//go:build windows

package uvm

import (
	"context"
	"testing"

	"github.com/Microsoft/hcsshim/internal/uvm"

	"github.com/Microsoft/hcsshim/test/internal/layers"
)

// CreateWCOWUVM creates a WCOW utility VM with all default options. Returns the
// UtilityVM object; folder used as its scratch
func CreateWCOWUVM(ctx context.Context, t testing.TB, id, image string) (*uvm.UtilityVM, []string, string) {
	return CreateWCOWUVMFromOptsWithImage(ctx, t, uvm.NewDefaultOptionsWCOW(id, ""), image)
}

// CreateWCOWUVMFromOpts creates a WCOW utility VM with the passed opts.
func CreateWCOWUVMFromOpts(ctx context.Context, t testing.TB, opts *uvm.OptionsWCOW) *uvm.UtilityVM {
	t.Helper()

	if opts == nil || len(opts.LayerFolders) < 2 {
		t.Fatalf("opts must bet set with LayerFolders")
	}

	uvm, err := uvm.CreateWCOW(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := uvm.Start(ctx); err != nil {
		uvm.Close()
		t.Fatal(err)
	}

	return uvm
}

// CreateWCOWUVMFromOptsWithImage creates a WCOW utility VM with the passed opts
// builds the LayerFolders based on `image`. Returns the UtilityVM object;
// folder used as its scratch
func CreateWCOWUVMFromOptsWithImage(ctx context.Context, t testing.TB, opts *uvm.OptionsWCOW, image string) (*uvm.UtilityVM, []string, string) {
	t.Helper()

	if opts == nil {
		t.Fatal("opts must be set")
	}

	uvmLayers := layers.LayerFolders(t, image)
	scratchDir := t.TempDir()
	opts.LayerFolders = append(opts.LayerFolders, uvmLayers...)
	opts.LayerFolders = append(opts.LayerFolders, scratchDir)

	return CreateWCOWUVMFromOpts(ctx, t, opts), uvmLayers, scratchDir
}
