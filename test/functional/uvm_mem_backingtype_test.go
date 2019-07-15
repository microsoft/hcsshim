// +build functional uvmmem

package functional

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"
	testutilities "github.com/Microsoft/hcsshim/test/functional/utilities"
	"github.com/sirupsen/logrus"
)

func runMemStartLCOWTest(ctx context.Context, t *testing.T, opts *uvm.OptionsLCOW) {
	u := testutilities.CreateLCOWUVMFromOpts(ctx, t, opts)
	u.Close(ctx)
}

func runMemStartWCOWTest(ctx context.Context, t *testing.T, opts *uvm.OptionsWCOW) {
	u, _, scratchDir := testutilities.CreateWCOWUVMFromOptsWithImage(ctx, t, opts, "microsoft/nanoserver")
	defer os.RemoveAll(scratchDir)
	u.Close(ctx)
}

func runMemTests(t *testing.T, os string) {
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
			runMemStartWCOWTest(context.Background(), t, wopts)
		} else {
			lopts := uvm.NewDefaultOptionsLCOW(t.Name(), "")
			lopts.MemorySizeInMB = 512
			lopts.AllowOvercommit = bt.allowOvercommit
			lopts.EnableDeferredCommit = bt.enableDeferredCommit
			runMemStartLCOWTest(context.Background(), t, lopts)
		}
	}
}

func TestMemBackingTypeWCOW(t *testing.T) {
	testutilities.RequiresBuild(t, osversion.RS5)
	runMemTests(t, "windows")
}

func TestMemBackingTypeLCOW(t *testing.T) {
	testutilities.RequiresBuild(t, osversion.RS5)
	runMemTests(t, "linux")
}

func runBenchMemStartTest(ctx context.Context, b *testing.B, opts *uvm.OptionsLCOW) {
	// Cant use testutilities here because its `testing.B` not `testing.T`
	u, err := uvm.CreateLCOW(ctx, opts)
	if err != nil {
		b.Fatal(err)
	}
	defer u.Close(ctx)
	if err := u.Start(ctx); err != nil {
		b.Fatal(err)
	}
}

func runBenchMemStartLcowTest(b *testing.B, allowOvercommit bool, enableDeferredCommit bool) {
	for i := 0; i < b.N; i++ {
		opts := uvm.NewDefaultOptionsLCOW(b.Name(), "")
		opts.MemorySizeInMB = 512
		opts.AllowOvercommit = allowOvercommit
		opts.EnableDeferredCommit = enableDeferredCommit
		runBenchMemStartTest(context.Background(), b, opts)
	}
}

func BenchmarkMemBackingTypeVirtualLCOW(b *testing.B) {
	//testutilities.RequiresBuild(t, osversion.RS5)
	logrus.SetOutput(ioutil.Discard)

	runBenchMemStartLcowTest(b, true, false)
}

func BenchmarkMemBackingTypeVirtualDeferredLCOW(b *testing.B) {
	//testutilities.RequiresBuild(t, osversion.RS5)
	logrus.SetOutput(ioutil.Discard)

	runBenchMemStartLcowTest(b, true, true)
}

func BenchmarkMemBackingTypePhyscialLCOW(b *testing.B) {
	//testutilities.RequiresBuild(t, osversion.RS5)
	logrus.SetOutput(ioutil.Discard)

	runBenchMemStartLcowTest(b, false, false)
}
