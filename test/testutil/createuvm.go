package testutil

import (
	"context"
	"testing"

	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/containerd/containerd"
)

// CreateWCOWUVM creates a WCOW utility VM with all default options. Returns the
// UtilityVM object; folder used as its scratch
func CreateWCOWUVM(ctx context.Context, t *testing.T, client *containerd.Client, id, image string) (*uvm.UtilityVM, []string, string) {
	return CreateWCOWUVMFromOptsWithImage(ctx, t, client, uvm.NewDefaultOptionsWCOW(id, ""), image)

}

// CreateWCOWUVMFromOptsWithImage creates a WCOW utility VM with the passed opts
// builds the LayerFolders based on `image`. Returns the UtilityVM object;
// folder used as its scratch
func CreateWCOWUVMFromOptsWithImage(ctx context.Context, t *testing.T, client *containerd.Client, opts *uvm.OptionsWCOW, image string) (*uvm.UtilityVM, []string, string) {
	if opts == nil {
		t.Fatal("opts must be set")
	}

	layers := LayerFolders(ctx, t, client, image)
	scratch := t.TempDir()
	opts.LayerFolders = append(opts.LayerFolders, layers...)
	opts.LayerFolders = append(opts.LayerFolders, scratch)

	return CreateWCOWUVMFromOpts(ctx, t, client, opts), layers, scratch
}

// CreateWCOWUVMFromOpts creates a WCOW utility VM with the passed opts.
func CreateWCOWUVMFromOpts(ctx context.Context, t *testing.T, _ *containerd.Client, opts *uvm.OptionsWCOW) *uvm.UtilityVM {
	if opts == nil || len(opts.LayerFolders) < 2 {
		t.Fatalf("opts must bet set with LayerFolders")
	}

	vm, err := uvm.CreateWCOW(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// should be idempotent
		if err := vm.Close(); err != nil {
			t.Fatalf("could not close uvm %q: %v", vm.ID(), err)
		}
	})

	if err := vm.Start(ctx); err != nil {
		vm.Close()
		t.Fatal(err)
	}

	return vm
}

// CreateLCOWUVM with all default options.
// client is only to maintain call line compatibility with WCOW versions
func CreateLCOWUVM(ctx context.Context, t *testing.T, client *containerd.Client, id string) *uvm.UtilityVM {
	return CreateLCOWUVMFromOpts(ctx, t, client, uvm.NewDefaultOptionsLCOW(id, ""))
}

// CreateLCOWUVMFromOpts creates an LCOW utility VM with the specified options.
func CreateLCOWUVMFromOpts(ctx context.Context, t *testing.T, _ *containerd.Client, opts *uvm.OptionsLCOW) *uvm.UtilityVM {
	if opts == nil {
		t.Fatal("opts must be set")
	}

	vm, err := uvm.CreateLCOW(ctx, opts)
	if err != nil {
		t.Fatalf("could not create LCOW UVM: %v", err)
	}
	t.Cleanup(func() {
		// should be idempotent
		if err := vm.Close(); err != nil {
			t.Fatalf("could not close uvm %q: %v", vm.ID(), err)
		}
	})

	if err := vm.Start(ctx); err != nil {
		t.Fatalf("could not start LCOW UVM: %v", err)
	}

	return vm
}
