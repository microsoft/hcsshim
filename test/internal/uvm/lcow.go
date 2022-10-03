//go:build windows

package uvm

import (
	"context"
	"testing"

	"github.com/Microsoft/hcsshim/internal/uvm"
)

// CreateAndStartLCOW with all default options.
func CreateAndStartLCOW(ctx context.Context, tb testing.TB, id string) *uvm.UtilityVM {
	tb.Helper()
	return CreateAndStartLCOWFromOpts(ctx, tb, uvm.NewDefaultOptionsLCOW(id, ""))
}

// CreateAndStartLCOWFromOpts creates an LCOW utility VM with the specified options.
func CreateAndStartLCOWFromOpts(ctx context.Context, tb testing.TB, opts *uvm.OptionsLCOW) *uvm.UtilityVM {
	tb.Helper()

	if opts == nil {
		tb.Fatal("opts must be set")
	}

	vm := CreateLCOW(ctx, tb, opts)
	cleanup := Start(ctx, tb, vm)
	tb.Cleanup(cleanup)

	return vm
}

func CreateLCOW(ctx context.Context, tb testing.TB, opts *uvm.OptionsLCOW) *uvm.UtilityVM {
	tb.Helper()
	vm, err := uvm.CreateLCOW(ctx, opts)
	if err != nil {
		tb.Fatalf("could not create LCOW UVM: %v", err)
	}

	return vm
}

func SetSecurityPolicy(ctx context.Context, tb testing.TB, vm *uvm.UtilityVM, policy string) {
	tb.Helper()
	if err := vm.SetConfidentialUVMOptions(ctx, uvm.WithSecurityPolicy(policy)); err != nil {
		tb.Fatalf("could not set vm security policy to %q: %v", policy, err)
	}
}
