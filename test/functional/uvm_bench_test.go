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

func BenchmarkUVM(b *testing.B) {
	requireFeatures(b, featureUVM)
	requireAnyFeature(b, featureLCOW, featureWCOW)
	require.Build(b, osversion.RS5)

	pCtx := util.Context(context.Background(), b)

	for _, tt := range []struct {
		feature    string
		createOpts func(context.Context, testing.TB) any
	}{
		{
			feature: featureLCOW,
			//nolint: thelper
			createOpts: func(ctx context.Context, tb testing.TB) any { return defaultLCOWOptions(ctx, tb) },
		},
		{
			feature: featureWCOW,
			//nolint: thelper
			createOpts: func(ctx context.Context, tb testing.TB) any { return defaultWCOWOptions(ctx, tb) },
		},
	} {
		b.Run(tt.feature, func(b *testing.B) {
			requireFeatures(b, tt.feature)

			b.Run("Create", func(b *testing.B) {
				b.StopTimer()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					ctx, cancel := context.WithTimeout(pCtx, benchmarkIterationTimeout)

					opts := tt.createOpts(ctx, b)

					b.StartTimer()
					_, cleanup := testuvm.Create(ctx, b, opts)
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

					opts := tt.createOpts(ctx, b)
					vm, cleanup := testuvm.Create(ctx, b, opts)

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

					opts := tt.createOpts(ctx, b)
					vm, cleanup := testuvm.Create(ctx, b, opts)
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

					opts := tt.createOpts(ctx, b)
					vm, cleanup := testuvm.Create(ctx, b, opts)
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
		})
	}
}
