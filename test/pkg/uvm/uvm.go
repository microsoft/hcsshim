//go:build windows

package uvm

import (
	"context"
	"testing"

	"github.com/Microsoft/hcsshim/internal/uvm"
)

type CleanupFn = func(context.Context)

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
	// Terminate will error on context cancellation, but close does not accept contexts
	if err := vm.CloseCtx(ctx); err != nil {
		tb.Fatalf("could not close uvm %q: %s", vm.ID(), err)
	}
}
