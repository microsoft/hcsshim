// +build functional uvmproperties

package functional

import (
	"context"
	"os"
	"testing"

	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/uvm"
	testutilities "github.com/Microsoft/hcsshim/test/functional/utilities"
)

func Test_NUMA_Node_Count_LCOW(t *testing.T) {
	t.Skip("NUMA is not supported on LCOW")
	testutilities.RequiresBuild(t, 18943)

	numa := &hcsschema.Numa{
		VirtualNodeCount: 2,
	}
	opts := uvm.NewDefaultOptionsLCOW(t.Name(), "")
	opts.NUMATopology = numa
	uvm := testutilities.CreateLCOWUVMFromOpts(context.Background(), t, opts)
	defer uvm.Close()

	stats, err := uvm.Stats(context.Background())
	if err != nil {
		t.Fatalf("failed to retrieve UVM memory stats: %s", err)
	}
	if stats.Memory.VirtualNodeCount != uint32(numa.VirtualNodeCount) {
		t.Fatalf("virtual node count incorrect. expected: %d but got %d", numa.VirtualNodeCount, stats.Memory.VirtualNodeCount)
	}
}

func Test_NUMA_Node_Count_WCOW_Hypervisor(t *testing.T) {
	testutilities.RequiresBuild(t, 18943)

	numa := &hcsschema.Numa{
		VirtualNodeCount: 2,
	}
	opts := uvm.NewDefaultOptionsWCOW(t.Name(), "")
	opts.NUMATopology = numa
	uvm, _, uvmScratchDir := testutilities.CreateWCOWUVMCustom(context.Background(), t, t.Name(), "microsoft/nanoserver", opts)
	defer os.RemoveAll(uvmScratchDir)
	defer uvm.Close()

	stats, err := uvm.Stats(context.Background())
	if err != nil {
		t.Fatalf("failed to retrieve UVM memory stats: %s", err)
	}
	if stats.Memory.VirtualNodeCount != uint32(numa.VirtualNodeCount) {
		t.Fatalf("virtual node count incorrect. expected: %d but got %d", numa.VirtualNodeCount, stats.Memory.VirtualNodeCount)
	}
}
