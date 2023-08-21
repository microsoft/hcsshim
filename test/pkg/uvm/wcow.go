//go:build windows

package uvm

import (
	"context"
	"testing"

	"github.com/Microsoft/hcsshim/internal/uvm"

	"github.com/Microsoft/hcsshim/test/internal/layers"
	"github.com/Microsoft/hcsshim/test/pkg/images"
)

// TODO: add cleanup return to CreateWCOWUVM*(), and have it remove layer and scratch dirs

// CreateWCOWUVM creates a WCOW utility VM with all default options. Returns the
// UtilityVM object; folder used as its scratch.
func CreateWCOWUVM(ctx context.Context, tb testing.TB, id, image string) (*uvm.UtilityVM, []string, string) {
	tb.Helper()
	return CreateWCOWUVMFromOptsWithImage(ctx, tb, uvm.NewDefaultOptionsWCOW(id, ""), image)
}

// CreateWCOWUVMFromOpts creates a WCOW utility VM with the passed opts.
func CreateWCOWUVMFromOpts(ctx context.Context, tb testing.TB, opts *uvm.OptionsWCOW) *uvm.UtilityVM {
	tb.Helper()

	if opts == nil || len(opts.LayerFolders) < 2 {
		tb.Fatalf("opts must bet set with LayerFolders")
	}

	vm, err := uvm.CreateWCOW(ctx, opts)
	if err != nil {
		tb.Fatal(err)
	}
	if err := vm.Start(ctx); err != nil {
		_ = vm.CloseCtx(ctx)
		tb.Fatal(err)
	}

	return vm
}

// CreateWCOWUVMFromOptsWithImage creates a WCOW utility VM with the passed opts
// builds the LayerFolders based on `image`. Returns the UtilityVM object;
// folder used as its scratch.
//
//nolint:staticcheck // SA5011: staticcheck thinks `opts` may be nil, even though we fail if it is
func CreateWCOWUVMFromOptsWithImage(
	ctx context.Context,
	tb testing.TB,
	opts *uvm.OptionsWCOW,
	image string,
) (*uvm.UtilityVM, []string, string) {
	tb.Helper()
	if opts == nil {
		tb.Fatal("opts must be set")
	}

	img := layers.LazyImageLayers{Image: image, Platform: images.PlatformWindows}
	tb.Cleanup(func() {
		if err := img.Close(ctx); err != nil {
			tb.Errorf("could not close image %s: %v", image, err)
		}
	})

	uvmLayers := img.Layers(ctx, tb)
	scratchDir := tb.TempDir()
	opts.LayerFolders = append(opts.LayerFolders, uvmLayers...)
	opts.LayerFolders = append(opts.LayerFolders, scratchDir)

	return CreateWCOWUVMFromOpts(ctx, tb, opts), uvmLayers, scratchDir
}
