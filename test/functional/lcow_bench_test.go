//go:build windows && functional
// +build windows,functional

package functional

import (
	"context"
	"testing"

	"github.com/Microsoft/hcsshim/osversion"

	"github.com/Microsoft/hcsshim/test/internal/util"
	"github.com/Microsoft/hcsshim/test/pkg/require"
	"github.com/Microsoft/hcsshim/test/pkg/uvm"
)

func BenchmarkLCOW_UVM(b *testing.B) {
	requireFeatures(b, featureLCOW)
	require.Build(b, osversion.RS5)

	pCtx := context.Background()

	b.Run("Create", func(b *testing.B) {
		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ctx, cancel := context.WithTimeout(pCtx, benchmarkIterationTimeout)

			opts := defaultLCOWOptions(b)
			opts.ID += util.RandNameSuffix(i)

			b.StartTimer()
			_, cleanup := uvm.CreateLCOW(ctx, b, opts)
			b.StopTimer()

			cleanup(ctx)
			cancel()
		}
	})

	b.Run("Start", func(b *testing.B) {
		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ctx, cancel := context.WithTimeout(pCtx, benchmarkIterationTimeout)

			opts := defaultLCOWOptions(b)
			opts.ID += util.RandNameSuffix(i)
			vm, cleanup := uvm.CreateLCOW(ctx, b, opts)

			b.StartTimer()
			if err := vm.Start(ctx); err != nil {
				b.Fatalf("could not start UVM: %v", err)
			}
			b.StopTimer()

			cleanup(ctx)
			cancel()
		}
	})

	b.Run("Kill", func(b *testing.B) {
		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ctx, cancel := context.WithTimeout(pCtx, benchmarkIterationTimeout)

			opts := defaultLCOWOptions(b)
			opts.ID += util.RandNameSuffix(i)
			vm, cleanup := uvm.CreateLCOW(ctx, b, opts)
			uvm.Start(ctx, b, vm)

			b.StartTimer()
			uvm.Kill(ctx, b, vm)
			if err := vm.WaitCtx(ctx); err != nil {
				b.Fatalf("could not kill uvm %q: %v", vm.ID(), err)
			}
			b.StopTimer()

			cleanup(ctx)
			cancel()
		}
	})

	b.Run("Close", func(b *testing.B) {
		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ctx, cancel := context.WithTimeout(pCtx, benchmarkIterationTimeout)

			opts := defaultLCOWOptions(b)
			opts.ID += util.RandNameSuffix(i)
			vm, cleanup := uvm.CreateLCOW(ctx, b, opts)
			uvm.Start(ctx, b, vm)

			b.StartTimer()
			if err := vm.Close(); err != nil {
				b.Fatalf("could not kill uvm %q: %v", vm.ID(), err)
			}
			b.StopTimer()

			cleanup(ctx)
			cancel()
		}
	})
}
