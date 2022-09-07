//go:build windows

package uvm

import (
	"context"
	"testing"

	"github.com/Microsoft/hcsshim/internal/uvm"
)

// CreateAndStartLCOW with all default options.
func CreateAndStartLCOW(ctx context.Context, t testing.TB, id string) *uvm.UtilityVM {
	return CreateAndStartLCOWFromOpts(ctx, t, uvm.NewDefaultOptionsLCOW(id, ""))
}

// CreateAndStartLCOWFromOpts creates an LCOW utility VM with the specified options.
func CreateAndStartLCOWFromOpts(ctx context.Context, t testing.TB, opts *uvm.OptionsLCOW) *uvm.UtilityVM {
	t.Helper()

	if opts == nil {
		t.Fatal("opts must be set")
	}

	vm := CreateLCOW(ctx, t, opts)
	cleanup := Start(ctx, t, vm)
	t.Cleanup(cleanup)

	return vm
}

func CreateLCOW(ctx context.Context, t testing.TB, opts *uvm.OptionsLCOW) *uvm.UtilityVM {
	vm, err := uvm.CreateLCOW(ctx, opts)
	if err != nil {
		t.Helper()
		t.Fatalf("could not create LCOW UVM: %v", err)
	}

	return vm
}

func SetSecurityPolicy(ctx context.Context, t testing.TB, vm *uvm.UtilityVM, policy string) {
	if err := vm.SetConfidentialUVMOptions(ctx, uvm.WithSecurityPolicyEnforcer("allow_all"), uvm.WithSecurityPolicy(policy)); err != nil {
		t.Helper()
		t.Fatalf("could not set vm security policy to %q: %v", policy, err)
	}
}
