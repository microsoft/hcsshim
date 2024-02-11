//go:build windows && functional
// +build windows,functional

package functional

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/osversion"

	"github.com/Microsoft/hcsshim/test/internal/util"
	"github.com/Microsoft/hcsshim/test/pkg/require"
	testuvm "github.com/Microsoft/hcsshim/test/pkg/uvm"
)

func TestUVM(t *testing.T) {
	requireFeatures(t, featureUVM)
	requireAnyFeature(t, featureLCOW, featureWCOW)
	require.Build(t, osversion.RS5)

	ctx := util.Context(context.Background(), t)

	for _, tt := range []struct {
		feature    string
		createOpts func(context.Context, testing.TB) any
	}{
		{
			feature: featureLCOW,
			//nolint: thelper
			createOpts: func(_ context.Context, tb testing.TB) any { return defaultLCOWOptions(ctx, tb) },
		},
		{
			feature: featureWCOW,
			//nolint: thelper
			createOpts: func(ctx context.Context, tb testing.TB) any { return defaultWCOWOptions(ctx, tb) },
		},
	} {
		t.Run(tt.feature, func(t *testing.T) {
			requireFeatures(t, tt.feature)

			// test if closing a created (but not started) uVM succeeds.
			t.Run("Close_Created", func(t *testing.T) {
				vm, cleanup := testuvm.Create(ctx, t, tt.createOpts(ctx, t))

				testuvm.Close(ctx, t, vm)

				// also run cleanup to make sure that works fine too
				cleanup(ctx)
			})

			// test if waiting after creating (but not starting) a uVM times out.
			t.Run("Wait_Created", func(t *testing.T) {
				vm, cleanup := testuvm.Create(ctx, t, tt.createOpts(ctx, t))
				t.Cleanup(func() { cleanup(ctx) })

				// arbitrary timeout
				timeoutCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				t.Cleanup(cancel)
				switch err := vm.WaitCtx(timeoutCtx); {
				case err == nil:
					t.Fatal("wait did not error")
				case !errors.Is(err, context.DeadlineExceeded):
					t.Fatalf("wait should have errored with '%v'; got '%v'", context.DeadlineExceeded, err)
				}
			})
		})
	}
}
