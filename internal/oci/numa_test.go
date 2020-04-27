package oci

import (
	"context"
	"reflect"
	"testing"

	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
)

func Test_NUMA_Topology(t *testing.T) {
	a := map[string]string{
		annotationVirtualNodeCount:       "2",
		annotationPreferredPhysicalNodes: "0,1",
		"io.microsoft.virtualmachine.computetopology.numa.virtualnodes.0.virtualnode":    "0",
		"io.microsoft.virtualmachine.computetopology.numa.virtualnodes.0.physicalnode":   "0",
		"io.microsoft.virtualmachine.computetopology.numa.virtualnodes.0.virtualsocket":  "0",
		"io.microsoft.virtualmachine.computetopology.numa.virtualnodes.0.processorcount": "1",
		"io.microsoft.virtualmachine.computetopology.numa.virtualnodes.0.memoryamount":   "100",
		"io.microsoft.virtualmachine.computetopology.numa.virtualnodes.1.virtualnode":    "1",
		"io.microsoft.virtualmachine.computetopology.numa.virtualnodes.1.physicalnode":   "1",
		"io.microsoft.virtualmachine.computetopology.numa.virtualnodes.1.virtualsocket":  "1",
		"io.microsoft.virtualmachine.computetopology.numa.virtualnodes.1.processorcount": "1",
		"io.microsoft.virtualmachine.computetopology.numa.virtualnodes.1.memoryamount":   "100",
	}
	numa := parseAnnotationsNUMATopology(context.Background(), a)
	copy := &hcsschema.Numa{
		VirtualNodeCount:       2,
		PreferredPhysicalNodes: []uint8{0, 1},
		Settings: []hcsschema.NumaSetting{
			{
				VirtualNodeNumber:   0,
				PhysicalNodeNumber:  0,
				VirtualSocketNumber: 0,
				CountOfProcessors:   1,
				CountOfMemoryBlocks: 100,
			},
			{
				VirtualNodeNumber:   1,
				PhysicalNodeNumber:  1,
				VirtualSocketNumber: 1,
				CountOfProcessors:   1,
				CountOfMemoryBlocks: 100,
			},
		},
	}

	if !reflect.DeepEqual(numa, copy) {
		t.Fatalf("numa topologies are not equal")
	}
}
