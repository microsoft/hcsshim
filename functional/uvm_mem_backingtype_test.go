// +build functional uvmmem

package functional

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/Microsoft/hcsshim/functional/utilities"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/sirupsen/logrus"
)

func runMemStartTest(t *testing.T, opts *uvm.UVMOptions) {
	u, err := uvm.Create(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer u.Terminate()
	if err := u.Start(); err != nil {
		t.Fatal(err)
	}
}

func runMemStartWCOWTest(t *testing.T, opts *uvm.UVMOptions) {
	imageName := "microsoft/nanoserver"
	layers := testutilities.LayerFolders(t, imageName)
	scratchDir := testutilities.CreateTempDir(t)
	defer os.RemoveAll(scratchDir)

	opts.LayerFolders = append(layers, scratchDir)
	runMemStartTest(t, opts)
}

func runMemTests(t *testing.T, os string) {

	type testCase struct {
		allowOvercommit      *bool
		enableDeferredCommit *bool
	}

	yes := true
	no := false

	testCases := []testCase{
		{nil, nil}, // Implicit default - Virtual
		{allowOvercommit: &yes, enableDeferredCommit: &no},  // Explicit default - Virtual
		{allowOvercommit: &yes, enableDeferredCommit: &yes}, // Virtual Deferred
		{allowOvercommit: &no, enableDeferredCommit: &no},   // Physical
	}

	for _, bt := range testCases {
		opts := &uvm.UVMOptions{
			OperatingSystem:      os,
			AllowOvercommit:      bt.allowOvercommit,
			EnableDeferredCommit: bt.enableDeferredCommit,
		}

		if os == "windows" {
			runMemStartWCOWTest(t, opts)
		} else {
			runMemStartTest(t, opts)
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

func runBenchMemStartTest(b *testing.B, opts *uvm.UVMOptions) {
	u, err := uvm.Create(opts)
	if err != nil {
		b.Fatal(err)
	}
	defer u.Terminate()
	if err := u.Start(); err != nil {
		b.Fatal(err)
	}
}

func runBenchMemStartLcowTest(b *testing.B, allowOverCommit bool, enableDeferredCommit bool) {
	for i := 0; i < b.N; i++ {
		opts := &uvm.UVMOptions{
			OperatingSystem:      "linux",
			AllowOvercommit:      &allowOverCommit,
			EnableDeferredCommit: &enableDeferredCommit,
		}
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
