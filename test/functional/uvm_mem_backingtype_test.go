//go:build functional || uvmmem
// +build functional uvmmem

package functional

import (
	"context"
	"io/ioutil"
	"testing"

	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"
	testutilities "github.com/Microsoft/hcsshim/test/functional/utilities"
	"github.com/sirupsen/logrus"
)

func runMemStartLCOWTest(t *testing.T, opts *uvm.OptionsLCOW) {
	client, ctx := newCtrdClient(context.Background(), t)
	u := testutilities.CreateLCOWUVMFromOpts(ctx, t, client, opts)
	u.Close()
}

func runMemStartWCOWTest(t *testing.T, opts *uvm.OptionsWCOW) {
	client, ctx := newCtrdClient(context.Background(), t)

	testutilities.CreateWCOWUVMFromOptsWithImage(ctx, t, client, opts, testutilities.ImageWindowsNanoserver1809)
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
			wopts := getDefaultWCOWUvmOptions(t, t.Name())
			wopts.MemorySizeInMB = 512
			wopts.AllowOvercommit = bt.allowOvercommit
			wopts.EnableDeferredCommit = bt.enableDeferredCommit
			runMemStartWCOWTest(t, wopts)
		} else {
			lopts := getDefaultLCOWUvmOptions(t, t.Name())
			lopts.MemorySizeInMB = 512
			lopts.AllowOvercommit = bt.allowOvercommit
			lopts.EnableDeferredCommit = bt.enableDeferredCommit
			runMemStartLCOWTest(t, lopts)
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

func runBenchMemStartTest(b *testing.B, opts *uvm.OptionsLCOW) {
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
	for i := 0; i < b.N; i++ {
		opts := uvm.NewDefaultOptionsLCOW(b.Name(), "")
		opts.MemorySizeInMB = 512
		opts.AllowOvercommit = allowOvercommit
		opts.EnableDeferredCommit = enableDeferredCommit
		runBenchMemStartTest(b, opts)
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
