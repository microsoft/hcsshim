//go:build windows

package uvm

import (
	"context"
	"testing"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/uvm"

	"github.com/Microsoft/hcsshim/test/internal/layers"
	"github.com/Microsoft/hcsshim/test/pkg/images"
)

// DefaultWCOWOptions returns default options for a bootable WCOW uVM.
//
// See [uvm.NewDefaultOptionsWCOW] for more information.
func DefaultWCOWOptions(_ context.Context, tb testing.TB, id, owner string) *uvm.OptionsWCOW {
	// mostly here to match [DefaultLCOWOptions]
	tb.Helper()
	opts := uvm.NewDefaultOptionsWCOW(id, owner)
	return opts
}

// CreateWCOWUVM creates a WCOW utility VM with all default options. Returns the
// UtilityVM object; folder used as its scratch.
//
// Deprecated: use [CreateWCOW] and [layers.WCOWScratchDir].
func CreateWCOWUVM(ctx context.Context, tb testing.TB, id, image string) (*uvm.UtilityVM, []string, string) {
	tb.Helper()
	return CreateWCOWUVMFromOptsWithImage(ctx, tb, uvm.NewDefaultOptionsWCOW(id, ""), image)
}

// CreateWCOW creates a WCOW utility VM with the passed opts.
func CreateWCOW(ctx context.Context, tb testing.TB, opts *uvm.OptionsWCOW) (*uvm.UtilityVM, CleanupFn) {
	tb.Helper()

	if opts == nil || len(opts.LayerFolders) < 2 {
		tb.Fatalf("opts must bet set with LayerFolders")
	}

	vm, err := uvm.CreateWCOW(ctx, opts)
	if err != nil {
		tb.Fatalf("could not create WCOW UVM: %v", err)
	}

	return vm, newCleanupFn(ctx, tb, vm)
}

// CreateWCOWUVMFromOptsWithImage creates a WCOW utility VM with the passed opts
// builds the LayerFolders based on `image`. Returns the UtilityVM object;
// folder used as its scratch.
//
// Deprecated: use [CreateWCOW] and [layers.WCOWScratchDir].
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

	vm, cleanup := CreateWCOW(ctx, tb, opts)
	tb.Cleanup(func() { cleanup(ctx) })

	return vm, uvmLayers, scratchDir
}

func AddVSMB(ctx context.Context, tb testing.TB, vm *uvm.UtilityVM, path string, options *hcsschema.VirtualSmbShareOptions) *uvm.VSMBShare {
	tb.Helper()

	s, err := vm.AddVSMB(ctx, path, options)
	if err != nil {
		tb.Fatalf("failed to add vSMB share: %v", err)
	}

	tb.Cleanup(func() {
		if err := s.Release(ctx); err != nil {
			tb.Fatalf("failed to remove vSMB share: %v", err)
		}
	})

	return s
}
