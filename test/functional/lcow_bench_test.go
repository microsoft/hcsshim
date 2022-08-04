//go:build windows && functional
// +build windows,functional

package functional

import (
	"context"
	"testing"

	"github.com/Microsoft/hcsshim/osversion"

	"github.com/Microsoft/hcsshim/test/internal/require"
	"github.com/Microsoft/hcsshim/test/internal/uvm"
)

func BenchmarkLCOW_UVM_Create(b *testing.B) {
	requireFeatures(b, featureLCOW)
	require.Build(b, osversion.RS5)

	ctx := context.Background()

	b.StopTimer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		opts := defaultLCOWOptions(b)

		b.StartTimer()
		vm := uvm.CreateLCOW(ctx, b, opts)
		b.StopTimer()

		// vm.Close() hangs unless the vm was started
		cleanup := uvm.Start(ctx, b, vm)
		cleanup()
	}
}

func BenchmarkLCOW_UVM_Start(b *testing.B) {
	requireFeatures(b, featureLCOW)
	require.Build(b, osversion.RS5)

	ctx := context.Background()

	b.StopTimer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vm := uvm.CreateLCOW(ctx, b, defaultLCOWOptions(b))

		b.StartTimer()
		if err := vm.Start(ctx); err != nil {
			b.Fatalf("could not start UVM: %v", err)
		}
		b.StopTimer()

		vm.Close()
	}
}

func BenchmarkLCOW_UVM_Kill(b *testing.B) {
	requireFeatures(b, featureLCOW)
	require.Build(b, osversion.RS5)

	ctx := context.Background()

	b.StopTimer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vm := uvm.CreateLCOW(ctx, b, defaultLCOWOptions(b))
		cleanup := uvm.Start(ctx, b, vm)

		b.StartTimer()
		uvm.Kill(ctx, b, vm)
		if err := vm.Wait(); err != nil {
			b.Fatalf("could not kill uvm %q: %v", vm.ID(), err)
		}
		b.StopTimer()

		cleanup()
	}
}

func BenchmarkLCOW_UVM_Close(b *testing.B) {
	requireFeatures(b, featureLCOW)
	require.Build(b, osversion.RS5)

	ctx := context.Background()

	b.StopTimer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vm := uvm.CreateLCOW(ctx, b, defaultLCOWOptions(b))
		cleanup := uvm.Start(ctx, b, vm)

		b.StartTimer()
		if err := vm.Close(); err != nil {
			b.Fatalf("could not kill uvm %q: %v", vm.ID(), err)
		}
		b.StopTimer()

		cleanup()
	}
}
