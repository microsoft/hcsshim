//go:build windows && functional
// +build windows,functional

package functional

import (
	"context"
	"testing"

	"github.com/Microsoft/hcsshim/osversion"

	"github.com/Microsoft/hcsshim/test/internal/util"
	"github.com/Microsoft/hcsshim/test/pkg/require"
	testuvm "github.com/Microsoft/hcsshim/test/pkg/uvm"
)

func BenchmarkLCOW_UVM(b *testing.B) {
	requireFeatures(b, featureLCOW, featureUVM)
	require.Build(b, osversion.RS5)

	pCtx := util.Context(context.Background(), b)

	b.Run("Create", func(b *testing.B) {
		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ctx, cancel := context.WithTimeout(pCtx, benchmarkIterationTimeout)

			opts := defaultLCOWOptions(ctx, b)
			opts.ID += util.RandNameSuffix(i)

			b.StartTimer()
			_, cleanup := testuvm.CreateLCOW(ctx, b, opts)
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

			opts := defaultLCOWOptions(ctx, b)
			opts.ID += util.RandNameSuffix(i)
			vm, cleanup := testuvm.CreateLCOW(ctx, b, opts)

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

			opts := defaultLCOWOptions(ctx, b)
			opts.ID += util.RandNameSuffix(i)
			vm, cleanup := testuvm.CreateLCOW(ctx, b, opts)
			testuvm.Start(ctx, b, vm)

			b.StartTimer()
			testuvm.Kill(ctx, b, vm)
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

			opts := defaultLCOWOptions(ctx, b)
			opts.ID += util.RandNameSuffix(i)
			vm, cleanup := testuvm.CreateLCOW(ctx, b, opts)
			testuvm.Start(ctx, b, vm)

			b.StartTimer()
			if err := vm.CloseCtx(ctx); err != nil {
				b.Fatalf("could not kill uvm %q: %v", vm.ID(), err)
			}
			b.StopTimer()

			cleanup(ctx)
			cancel()
		}
	})
}
