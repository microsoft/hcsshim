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

func runMemStartTest(t *testing.T, u *uvm.UtilityVM) {
	defer u.Terminate()
	if err := u.Start(); err != nil {
		t.Fatal(err)
	}
}

func runMemStartLCOWTest(t *testing.T, opts *uvm.Options) {
	u, err := uvm.CreateLCOW(&uvm.OptionsLCOW{Options: opts})
	if err != nil {
		t.Fatal(err)
	}
	runMemStartTest(t, u)
}

func runMemStartWCOWTest(t *testing.T, opts *uvm.Options) {
	imageName := "microsoft/nanoserver"
	layers := testutilities.LayerFolders(t, imageName)
	scratchDir := testutilities.CreateTempDir(t)
	defer os.RemoveAll(scratchDir)

	wopts := &uvm.OptionsWCOW{
		Options:      opts,
		LayerFolders: append(layers, scratchDir),
	}
	u, err := uvm.CreateWCOW(wopts)
	if err != nil {
		t.Fatal(err)
	}
	runMemStartTest(t, u)
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
		opts := &uvm.Options{
			AllowOvercommit:      bt.allowOvercommit,
			EnableDeferredCommit: bt.enableDeferredCommit,
		}

		if os == "windows" {
			runMemStartWCOWTest(t, opts)
		} else {
			runMemStartLCOWTest(t, opts)
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

func runBenchMemStartTest(b *testing.B, opts *uvm.Options) {
	u, err := uvm.CreateLCOW(&uvm.OptionsLCOW{Options: opts})
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
		opts := &uvm.Options{
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
