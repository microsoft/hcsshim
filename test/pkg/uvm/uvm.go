//go:build windows

package uvm

import (
	"context"
	"testing"

	"github.com/Microsoft/hcsshim/internal/uvm"
)

type CleanupFn = func(context.Context)

func newCleanupFn(_ context.Context, tb testing.TB, vm *uvm.UtilityVM) CleanupFn {
	tb.Helper()

	return func(ctx context.Context) {
		if vm == nil {
			return
		}

		if err := vm.CloseCtx(ctx); err != nil {
			tb.Errorf("could not close vm %q: %v", vm.ID(), err)
		}
	}
}

// TODO: create interface in "internal/uvm" that both [OptionsLCOW] and [OptionsWCOW] implement
//
// can't use generic interface { OptionsLCOW | OptionsWCOW } since that is a type constraint and requires
// making all calls generic as well.

// Create creates a utility VM with the passed opts.
func Create(ctx context.Context, tb testing.TB, opts any) (*uvm.UtilityVM, CleanupFn) {
	tb.Helper()

	switch opts := opts.(type) {
	case *uvm.OptionsLCOW:
		return CreateLCOW(ctx, tb, opts)
	case *uvm.OptionsWCOW:
		return CreateWCOW(ctx, tb, opts)
	}
	tb.Fatalf("unknown uVM creation options: %T", opts)
	return nil, nil
}

// CreateAndStartWCOWFromOpts creates a utility VM with the specified options.
//
// The cleanup function will be added to `tb.Cleanup`.
func CreateAndStart(ctx context.Context, tb testing.TB, opts any) *uvm.UtilityVM {
	tb.Helper()

	vm, cleanup := Create(ctx, tb, opts)
	Start(ctx, tb, vm)
	tb.Cleanup(func() { cleanup(ctx) })

	return vm
}

func Start(ctx context.Context, tb testing.TB, vm *uvm.UtilityVM) {
	tb.Helper()
	err := vm.Start(ctx)

	if err != nil {
		tb.Fatalf("could not start UVM: %v", err)
	}
}

func Wait(ctx context.Context, tb testing.TB, vm *uvm.UtilityVM) {
	tb.Helper()
	if err := vm.WaitCtx(ctx); err != nil {
		tb.Fatalf("could not wait for uvm %q: %v", vm.ID(), err)
	}
}

func Kill(ctx context.Context, tb testing.TB, vm *uvm.UtilityVM) {
	tb.Helper()
	if err := vm.Terminate(ctx); err != nil {
		tb.Fatalf("could not kill uvm %q: %v", vm.ID(), err)
	}
}

func Close(ctx context.Context, tb testing.TB, vm *uvm.UtilityVM) {
	tb.Helper()
	if err := vm.CloseCtx(ctx); err != nil {
		tb.Fatalf("could not close uvm %q: %v", vm.ID(), err)
	}
}

func Share(ctx context.Context, tb testing.TB, vm *uvm.UtilityVM, hostPath, guestPath string, readOnly bool) {
	tb.Helper()
	tb.Logf("sharing %q to %q inside uvm %s", hostPath, guestPath, vm.ID())

	if err := vm.Share(ctx, hostPath, guestPath, readOnly); err != nil {
		tb.Fatalf("could not share %q into uvm %s as %q: %v", hostPath, vm.ID(), guestPath, err)
	}
}
