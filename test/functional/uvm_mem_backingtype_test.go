//go:build windows && functional
// +build windows,functional

package functional

import (
	"context"
	"testing"

	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"

	"github.com/Microsoft/hcsshim/test/pkg/require"
	testuvm "github.com/Microsoft/hcsshim/test/pkg/uvm"
)

func runMemStartLCOWTest(t *testing.T, opts *uvm.OptionsLCOW) {
	t.Helper()
	u := testuvm.CreateAndStartLCOWFromOpts(context.Background(), t, opts)
	u.Close()
}

func runMemStartWCOWTest(t *testing.T, opts *uvm.OptionsWCOW) {
	t.Helper()

	//nolint:staticcheck // SA1019: deprecated; will be replaced when test is updated
	u, _, _ := testuvm.CreateWCOWUVMFromOptsWithImage(context.Background(), t, opts, "microsoft/nanoserver")
	u.Close()
}

func runMemTests(t *testing.T, os string) {
	t.Helper()
	type testCase struct {
		allowOvercommit      bool
		enableDeferredCommit bool
	}

	testCases := []testCase{
		{allowOvercommit: true, enableDeferredCommit: false},  // Explicit default - Virtual
		{allowOvercommit: true, enableDeferredCommit: true},   // Virtual Deferred
		{allowOvercommit: false, enableDeferredCommit: false}, // Physical
	}

	for _, bt := range testCases {
		if os == "windows" {
			wopts := defaultWCOWOptions(context.Background(), t)
			wopts.MemorySizeInMB = 512
			wopts.AllowOvercommit = bt.allowOvercommit
			wopts.EnableDeferredCommit = bt.enableDeferredCommit
			runMemStartWCOWTest(t, wopts)
		} else {
			lopts := defaultLCOWOptions(context.Background(), t)
			lopts.MemorySizeInMB = 512
			lopts.AllowOvercommit = bt.allowOvercommit
			lopts.EnableDeferredCommit = bt.enableDeferredCommit
			runMemStartLCOWTest(t, lopts)
		}
	}
}

func TestMemBackingTypeWCOW(t *testing.T) {
	t.Skip("not yet updated")

	require.Build(t, osversion.RS5)
	requireFeatures(t, featureWCOW, featureUVM)
	runMemTests(t, "windows")
}

func TestMemBackingTypeLCOW(t *testing.T) {
	t.Skip("not yet updated")

	require.Build(t, osversion.RS5)
	requireFeatures(t, featureLCOW, featureUVM)
	runMemTests(t, "linux")
}

func runBenchMemStartTest(b *testing.B, opts *uvm.OptionsLCOW) {
	b.Helper()
	// Cant use testutilities here because its `testing.B` not `testing.T`
	u, err := uvm.CreateLCOW(context.Background(), opts)
	if err != nil {
		b.Fatal(err)
	}
	defer u.Close()
	if err := u.Start(context.Background()); err != nil {
		b.Fatal(err)
	}
}

func runBenchMemStartLCOWTest(b *testing.B, allowOvercommit bool, enableDeferredCommit bool) {
	b.Helper()
	for i := 0; i < b.N; i++ {
		opts := uvm.NewDefaultOptionsLCOW(b.Name(), "")
		opts.MemorySizeInMB = 512
		opts.AllowOvercommit = allowOvercommit
		opts.EnableDeferredCommit = enableDeferredCommit
		runBenchMemStartTest(b, opts)
	}
}

func BenchmarkMemBackingTypeVirtualLCOW(b *testing.B) {
	b.Skip("not yet updated")

	require.Build(b, osversion.RS5)
	requireFeatures(b, featureLCOW, featureUVM)

	runBenchMemStartLCOWTest(b, true, false)
}

func BenchmarkMemBackingTypeVirtualDeferredLCOW(b *testing.B) {
	b.Skip("not yet updated")

	require.Build(b, osversion.RS5)
	requireFeatures(b, featureLCOW, featureUVM)

	runBenchMemStartLCOWTest(b, true, true)
}

func BenchmarkMemBackingTypePhyscialLCOW(b *testing.B) {
	b.Skip("not yet updated")

	require.Build(b, osversion.RS5)
	requireFeatures(b, featureLCOW, featureUVM)

	runBenchMemStartLCOWTest(b, false, false)
}
