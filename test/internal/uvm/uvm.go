//go:build windows

package uvm

import (
	"context"
	"fmt"
	"testing"

	"github.com/Microsoft/hcsshim/internal/uvm"

	"github.com/Microsoft/hcsshim/test/internal/timeout"
)

func Start(ctx context.Context, tb testing.TB, vm *uvm.UtilityVM) func() {
	tb.Helper()
	err := vm.Start(ctx)
	f := func() {
		if err := vm.Close(); err != nil {
			tb.Logf("could not close vm %q: %v", vm.ID(), err)
		}
	}

	if err != nil {
		tb.Fatalf("could not start UVM: %v", err)
	}

	return f
}

func Wait(ctx context.Context, tb testing.TB, vm *uvm.UtilityVM) {
	tb.Helper()
	fe := func(err error) error {
		if err != nil {
			err = fmt.Errorf("could not wait for uvm %q: %w", vm.ID(), err)
		}

		return err
	}
	timeout.WaitForError(ctx, tb, vm.Wait, fe)
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
	fe := func(err error) error {
		if err != nil {
			err = fmt.Errorf("could not close uvm %q: %w", vm.ID(), err)
		}

		return err
	}
	timeout.WaitForError(ctx, tb, vm.Close, fe)
}
