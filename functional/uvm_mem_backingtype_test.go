// +build functional uvmmem

package functional

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/Microsoft/hcsshim/functional/utilities"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/sirupsen/logrus"
)

func memoryBackingTypeToString(bt uvm.MemoryBackingType) string {
	switch bt {
	case uvm.MemoryBackingTypeVirtual:
		return "Virtual"
	case uvm.MemoryBackingTypeVirtualDeferred:
		return "VirtualDeferred"
	case uvm.MemoryBackingTypePhysical:
		return "Physical"
	default:
		panic(fmt.Sprintf("unknown memory type: %v", bt))
	}
}

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

	opts.LayerFolders = layers
	runMemStartTest(t, opts)
}

func runMemTests(t *testing.T, os string) {
	types := [3]uvm.MemoryBackingType{
		uvm.MemoryBackingTypeVirtual,
		uvm.MemoryBackingTypeVirtualDeferred,
		uvm.MemoryBackingTypePhysical,
	}

	for _, bt := range types {
		opts := &uvm.UVMOptions{
			ID:                fmt.Sprintf("%s-%s", t.Name(), memoryBackingTypeToString(bt)),
			OperatingSystem:   os,
			MemoryBackingType: &bt,
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

func runBenchMemStartLcowTest(b *testing.B, bt uvm.MemoryBackingType) {
	for i := 0; i < b.N; i++ {
		opts := &uvm.UVMOptions{
			ID:                fmt.Sprintf("%s-%s-%d", b.Name(), memoryBackingTypeToString(bt), i),
			OperatingSystem:   "linux",
			MemoryBackingType: &bt,
		}
		runBenchMemStartTest(b, opts)
	}
}

func BenchmarkMemBackingTypeVirtualLCOW(b *testing.B) {
	//testutilities.RequiresBuild(t, osversion.RS5)
	logrus.SetOutput(ioutil.Discard)

	runBenchMemStartLcowTest(b, uvm.MemoryBackingTypeVirtual)
}

func BenchmarkMemBackingTypeVirtualDeferredLCOW(b *testing.B) {
	//testutilities.RequiresBuild(t, osversion.RS5)
	logrus.SetOutput(ioutil.Discard)

	runBenchMemStartLcowTest(b, uvm.MemoryBackingTypeVirtualDeferred)
}

func BenchmarkMemBackingTypePhyscialLCOW(b *testing.B) {
	//testutilities.RequiresBuild(t, osversion.RS5)
	logrus.SetOutput(ioutil.Discard)

	runBenchMemStartLcowTest(b, uvm.MemoryBackingTypePhysical)
}
