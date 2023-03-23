//go:build windows && (functional || uvmmem)
// +build windows
// +build functional uvmmem

package functional

import (
	"context"
	"io"
	"testing"

	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/Microsoft/hcsshim/test/internal/require"
	"github.com/sirupsen/logrus"

	tuvm "github.com/Microsoft/hcsshim/test/internal/uvm"
)

func runMemStartLCOWTest(t *testing.T, opts *uvm.OptionsLCOW) {
	t.Helper()
	u := tuvm.CreateAndStartLCOWFromOpts(context.Background(), t, opts)
	u.Close()
}

func runMemStartWCOWTest(t *testing.T, opts *uvm.OptionsWCOW) {
	t.Helper()
	u, _, _ := tuvm.CreateWCOWUVMFromOptsWithImage(context.Background(), t, opts, "microsoft/nanoserver")
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
			wopts := uvm.NewDefaultOptionsWCOW(t.Name(), "")
			wopts.MemorySizeInMB = 512
			wopts.AllowOvercommit = bt.allowOvercommit
			wopts.EnableDeferredCommit = bt.enableDeferredCommit
			runMemStartWCOWTest(t, wopts)
		} else {
			lopts := defaultLCOWOptions(t)
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
	requireFeatures(t, featureWCOW)
	runMemTests(t, "windows")
}

func TestMemBackingTypeLCOW(t *testing.T) {
	t.Skip("not yet updated")

	require.Build(t, osversion.RS5)
	requireFeatures(t, featureLCOW)
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

func runBenchMemStartLcowTest(b *testing.B, allowOvercommit bool, enableDeferredCommit bool) {
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
	requireFeatures(b, featureLCOW)
	logrus.SetOutput(io.Discard)

	runBenchMemStartLcowTest(b, true, false)
}

func BenchmarkMemBackingTypeVirtualDeferredLCOW(b *testing.B) {
	b.Skip("not yet updated")

	require.Build(b, osversion.RS5)
	requireFeatures(b, featureLCOW)
	logrus.SetOutput(io.Discard)

	runBenchMemStartLcowTest(b, true, true)
}

func BenchmarkMemBackingTypePhyscialLCOW(b *testing.B) {
	b.Skip("not yet updated")

	require.Build(b, osversion.RS5)
	requireFeatures(b, featureLCOW)
	logrus.SetOutput(io.Discard)

	runBenchMemStartLcowTest(b, false, false)
}
