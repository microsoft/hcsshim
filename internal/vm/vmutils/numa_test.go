//go:build windows

package vmutils

import (
	"context"
	"testing"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/osversion"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		numa    *hcsschema.Numa
		wantErr bool
		errMsg  string
	}{
		{
			name: "empty settings is valid",
			numa: &hcsschema.Numa{
				Settings: []hcsschema.NumaSetting{},
			},
			wantErr: false,
		},
		{
			name: "nil settings is valid",
			numa: &hcsschema.Numa{
				Settings: nil,
			},
			wantErr: false,
		},
		{
			name: "valid single node topology",
			numa: &hcsschema.Numa{
				VirtualNodeCount: 1,
				Settings: []hcsschema.NumaSetting{
					{
						VirtualNodeNumber:   0,
						PhysicalNodeNumber:  0,
						VirtualSocketNumber: 0,
						CountOfProcessors:   4,
						CountOfMemoryBlocks: 1024,
						MemoryBackingType:   hcsschema.MemoryBackingType_PHYSICAL,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid multi-node topology",
			numa: &hcsschema.Numa{
				VirtualNodeCount: 2,
				Settings: []hcsschema.NumaSetting{
					{
						VirtualNodeNumber:   0,
						PhysicalNodeNumber:  0,
						VirtualSocketNumber: 0,
						CountOfProcessors:   4,
						CountOfMemoryBlocks: 1024,
						MemoryBackingType:   hcsschema.MemoryBackingType_PHYSICAL,
					},
					{
						VirtualNodeNumber:   1,
						PhysicalNodeNumber:  1,
						VirtualSocketNumber: 1,
						CountOfProcessors:   4,
						CountOfMemoryBlocks: 1024,
						MemoryBackingType:   hcsschema.MemoryBackingType_PHYSICAL,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid wildcard physical nodes",
			numa: &hcsschema.Numa{
				VirtualNodeCount: 2,
				Settings: []hcsschema.NumaSetting{
					{
						VirtualNodeNumber:   0,
						PhysicalNodeNumber:  0xFF, // wildcard
						VirtualSocketNumber: 0,
						CountOfProcessors:   4,
						CountOfMemoryBlocks: 1024,
						MemoryBackingType:   hcsschema.MemoryBackingType_PHYSICAL,
					},
					{
						VirtualNodeNumber:   1,
						PhysicalNodeNumber:  0xFF, // wildcard
						VirtualSocketNumber: 1,
						CountOfProcessors:   4,
						CountOfMemoryBlocks: 1024,
						MemoryBackingType:   hcsschema.MemoryBackingType_PHYSICAL,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "virtual node number exceeds max",
			numa: &hcsschema.Numa{
				VirtualNodeCount: 1,
				Settings: []hcsschema.NumaSetting{
					{
						VirtualNodeNumber:   65, // exceeds numaChildNodeCountMax (64)
						PhysicalNodeNumber:  0,
						VirtualSocketNumber: 0,
						CountOfProcessors:   4,
						CountOfMemoryBlocks: 1024,
						MemoryBackingType:   hcsschema.MemoryBackingType_PHYSICAL,
					},
				},
			},
			wantErr: true,
			errMsg:  "vNUMA virtual node number 65 exceeds maximum allowed value 64",
		},
		{
			name: "physical node number exceeds max (non-wildcard)",
			numa: &hcsschema.Numa{
				VirtualNodeCount: 1,
				Settings: []hcsschema.NumaSetting{
					{
						VirtualNodeNumber:   0,
						PhysicalNodeNumber:  64, // exceeds numaTopologyNodeCountMax (64)
						VirtualSocketNumber: 0,
						CountOfProcessors:   4,
						CountOfMemoryBlocks: 1024,
						MemoryBackingType:   hcsschema.MemoryBackingType_PHYSICAL,
					},
				},
			},
			wantErr: true,
			errMsg:  "vNUMA physical node number 64 exceeds maximum allowed value 64",
		},
		{
			name: "mixed wildcard and non-wildcard physical nodes",
			numa: &hcsschema.Numa{
				VirtualNodeCount: 2,
				Settings: []hcsschema.NumaSetting{
					{
						VirtualNodeNumber:   0,
						PhysicalNodeNumber:  0xFF, // wildcard
						VirtualSocketNumber: 0,
						CountOfProcessors:   4,
						CountOfMemoryBlocks: 1024,
						MemoryBackingType:   hcsschema.MemoryBackingType_PHYSICAL,
					},
					{
						VirtualNodeNumber:   1,
						PhysicalNodeNumber:  1, // non-wildcard
						VirtualSocketNumber: 1,
						CountOfProcessors:   4,
						CountOfMemoryBlocks: 1024,
						MemoryBackingType:   hcsschema.MemoryBackingType_PHYSICAL,
					},
				},
			},
			wantErr: true,
			errMsg:  "vNUMA has a mix of wildcard",
		},
		{
			name: "node with zero memory blocks",
			numa: &hcsschema.Numa{
				VirtualNodeCount: 1,
				Settings: []hcsschema.NumaSetting{
					{
						VirtualNodeNumber:   0,
						PhysicalNodeNumber:  0,
						VirtualSocketNumber: 0,
						CountOfProcessors:   4,
						CountOfMemoryBlocks: 0, // not allowed
						MemoryBackingType:   hcsschema.MemoryBackingType_PHYSICAL,
					},
				},
			},
			wantErr: true,
			errMsg:  "vNUMA nodes with no memory are not allowed",
		},
		{
			name: "duplicate virtual node numbers",
			numa: &hcsschema.Numa{
				VirtualNodeCount: 2,
				Settings: []hcsschema.NumaSetting{
					{
						VirtualNodeNumber:   0,
						PhysicalNodeNumber:  0,
						VirtualSocketNumber: 0,
						CountOfProcessors:   4,
						CountOfMemoryBlocks: 1024,
						MemoryBackingType:   hcsschema.MemoryBackingType_PHYSICAL,
					},
					{
						VirtualNodeNumber:   0, // duplicate
						PhysicalNodeNumber:  1,
						VirtualSocketNumber: 1,
						CountOfProcessors:   4,
						CountOfMemoryBlocks: 1024,
						MemoryBackingType:   hcsschema.MemoryBackingType_PHYSICAL,
					},
				},
			},
			wantErr: true,
			errMsg:  "vNUMA virtual node number 0 is duplicated",
		},
		{
			name: "partial resource allocation - memory only",
			numa: &hcsschema.Numa{
				VirtualNodeCount: 1,
				Settings: []hcsschema.NumaSetting{
					{
						VirtualNodeNumber:   0,
						PhysicalNodeNumber:  0,
						VirtualSocketNumber: 0,
						CountOfProcessors:   0,
						CountOfMemoryBlocks: 1024,
						MemoryBackingType:   hcsschema.MemoryBackingType_PHYSICAL,
					},
				},
			},
			wantErr: true,
			errMsg:  "partial resource allocation is not allowed",
		},
		{
			name: "completely empty topology with wildcard",
			numa: &hcsschema.Numa{
				VirtualNodeCount: 2,
				Settings: []hcsschema.NumaSetting{
					{
						VirtualNodeNumber:   0,
						PhysicalNodeNumber:  0xFF,
						VirtualSocketNumber: 0,
						CountOfProcessors:   0,
						CountOfMemoryBlocks: 0,
						MemoryBackingType:   hcsschema.MemoryBackingType_PHYSICAL,
					},
					{
						VirtualNodeNumber:   1,
						PhysicalNodeNumber:  0xFF,
						VirtualSocketNumber: 1,
						CountOfProcessors:   0,
						CountOfMemoryBlocks: 0,
						MemoryBackingType:   hcsschema.MemoryBackingType_PHYSICAL,
					},
				},
			},
			wantErr: true,
			errMsg:  "vNUMA nodes with no memory are not allowed",
		},
		{
			name: "valid - shared virtual socket number",
			numa: &hcsschema.Numa{
				VirtualNodeCount: 2,
				Settings: []hcsschema.NumaSetting{
					{
						VirtualNodeNumber:   0,
						PhysicalNodeNumber:  0,
						VirtualSocketNumber: 0, // shared socket
						CountOfProcessors:   4,
						CountOfMemoryBlocks: 1024,
						MemoryBackingType:   hcsschema.MemoryBackingType_PHYSICAL,
					},
					{
						VirtualNodeNumber:   1,
						PhysicalNodeNumber:  1,
						VirtualSocketNumber: 0, // shared socket
						CountOfProcessors:   4,
						CountOfMemoryBlocks: 1024,
						MemoryBackingType:   hcsschema.MemoryBackingType_PHYSICAL,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "physical node at boundary (63)",
			numa: &hcsschema.Numa{
				VirtualNodeCount: 1,
				Settings: []hcsschema.NumaSetting{
					{
						VirtualNodeNumber:   0,
						PhysicalNodeNumber:  63, // numaTopologyNodeCountMax - 1
						VirtualSocketNumber: 0,
						CountOfProcessors:   4,
						CountOfMemoryBlocks: 1024,
						MemoryBackingType:   hcsschema.MemoryBackingType_PHYSICAL,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "virtual node at boundary (64)",
			numa: &hcsschema.Numa{
				VirtualNodeCount: 1,
				Settings: []hcsschema.NumaSetting{
					{
						VirtualNodeNumber:   64, // numaChildNodeCountMax
						PhysicalNodeNumber:  0,
						VirtualSocketNumber: 0,
						CountOfProcessors:   4,
						CountOfMemoryBlocks: 1024,
						MemoryBackingType:   hcsschema.MemoryBackingType_PHYSICAL,
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validate(tc.numa)
			if tc.wantErr {
				if err == nil {
					t.Errorf("validate() expected error containing %q, got nil", tc.errMsg)
				} else if tc.errMsg != "" && !containsSubstring(err.Error(), tc.errMsg) {
					t.Errorf("validate() error = %q, expected to contain %q", err.Error(), tc.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validate() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateNumaForVM(t *testing.T) {
	tests := []struct {
		name      string
		numa      *hcsschema.Numa
		procCount uint32
		memInMb   uint64
		wantErr   bool
		errMsg    string
	}{
		{
			name: "empty settings always valid",
			numa: &hcsschema.Numa{
				Settings: []hcsschema.NumaSetting{},
			},
			procCount: 8,
			memInMb:   2048,
			wantErr:   false,
		},
		{
			name: "nil settings always valid",
			numa: &hcsschema.Numa{
				Settings: nil,
			},
			procCount: 8,
			memInMb:   2048,
			wantErr:   false,
		},
		{
			name: "matching processor and memory",
			numa: &hcsschema.Numa{
				VirtualNodeCount: 2,
				Settings: []hcsschema.NumaSetting{
					{
						VirtualNodeNumber:   0,
						PhysicalNodeNumber:  0,
						CountOfProcessors:   4,
						CountOfMemoryBlocks: 1024,
					},
					{
						VirtualNodeNumber:   1,
						PhysicalNodeNumber:  1,
						CountOfProcessors:   4,
						CountOfMemoryBlocks: 1024,
					},
				},
			},
			procCount: 8,
			memInMb:   2048,
			wantErr:   false,
		},
		{
			name: "mismatched processor count",
			numa: &hcsschema.Numa{
				VirtualNodeCount: 2,
				Settings: []hcsschema.NumaSetting{
					{
						VirtualNodeNumber:   0,
						PhysicalNodeNumber:  0,
						CountOfProcessors:   4,
						CountOfMemoryBlocks: 1024,
					},
					{
						VirtualNodeNumber:   1,
						PhysicalNodeNumber:  1,
						CountOfProcessors:   4,
						CountOfMemoryBlocks: 1024,
					},
				},
			},
			procCount: 16, // doesn't match total of 8
			memInMb:   2048,
			wantErr:   true,
			errMsg:    "vNUMA total processor count 8 does not match UVM processor count 16",
		},
		{
			name: "mismatched memory size",
			numa: &hcsschema.Numa{
				VirtualNodeCount: 2,
				Settings: []hcsschema.NumaSetting{
					{
						VirtualNodeNumber:   0,
						PhysicalNodeNumber:  0,
						CountOfProcessors:   4,
						CountOfMemoryBlocks: 1024,
					},
					{
						VirtualNodeNumber:   1,
						PhysicalNodeNumber:  1,
						CountOfProcessors:   4,
						CountOfMemoryBlocks: 1024,
					},
				},
			},
			procCount: 8,
			memInMb:   4096, // doesn't match total of 2048
			wantErr:   true,
			errMsg:    "vNUMA total memory 2048 does not match UVM memory 4096",
		},
		{
			name: "single node matching",
			numa: &hcsschema.Numa{
				VirtualNodeCount: 1,
				Settings: []hcsschema.NumaSetting{
					{
						VirtualNodeNumber:   0,
						PhysicalNodeNumber:  0,
						CountOfProcessors:   16,
						CountOfMemoryBlocks: 8192,
					},
				},
			},
			procCount: 16,
			memInMb:   8192,
			wantErr:   false,
		},
		{
			name: "zero processor count in NUMA (allowed)",
			numa: &hcsschema.Numa{
				VirtualNodeCount: 1,
				Settings: []hcsschema.NumaSetting{
					{
						VirtualNodeNumber:   0,
						PhysicalNodeNumber:  0,
						CountOfProcessors:   0,
						CountOfMemoryBlocks: 1024,
					},
				},
			},
			procCount: 8,
			memInMb:   1024,
			wantErr:   false, // zero processor count bypasses the check
		},
		{
			name: "zero memory in NUMA (allowed)",
			numa: &hcsschema.Numa{
				VirtualNodeCount: 1,
				Settings: []hcsschema.NumaSetting{
					{
						VirtualNodeNumber:   0,
						PhysicalNodeNumber:  0,
						CountOfProcessors:   4,
						CountOfMemoryBlocks: 0,
					},
				},
			},
			procCount: 4,
			memInMb:   2048,
			wantErr:   false, // zero memory bypasses the check
		},
		{
			name: "uneven distribution across nodes",
			numa: &hcsschema.Numa{
				VirtualNodeCount: 3,
				Settings: []hcsschema.NumaSetting{
					{
						VirtualNodeNumber:   0,
						PhysicalNodeNumber:  0,
						CountOfProcessors:   2,
						CountOfMemoryBlocks: 512,
					},
					{
						VirtualNodeNumber:   1,
						PhysicalNodeNumber:  1,
						CountOfProcessors:   4,
						CountOfMemoryBlocks: 1024,
					},
					{
						VirtualNodeNumber:   2,
						PhysicalNodeNumber:  2,
						CountOfProcessors:   2,
						CountOfMemoryBlocks: 512,
					},
				},
			},
			procCount: 8,
			memInMb:   2048,
			wantErr:   false,
		},
		{
			name: "large values matching",
			numa: &hcsschema.Numa{
				VirtualNodeCount: 4,
				Settings: []hcsschema.NumaSetting{
					{
						VirtualNodeNumber:   0,
						CountOfProcessors:   64,
						CountOfMemoryBlocks: 65536,
					},
					{
						VirtualNodeNumber:   1,
						CountOfProcessors:   64,
						CountOfMemoryBlocks: 65536,
					},
					{
						VirtualNodeNumber:   2,
						CountOfProcessors:   64,
						CountOfMemoryBlocks: 65536,
					},
					{
						VirtualNodeNumber:   3,
						CountOfProcessors:   64,
						CountOfMemoryBlocks: 65536,
					},
				},
			},
			procCount: 256,
			memInMb:   262144,
			wantErr:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateNumaForVM(tc.numa, tc.procCount, tc.memInMb)
			if tc.wantErr {
				if err == nil {
					t.Errorf("ValidateNumaForVM() expected error containing %q, got nil", tc.errMsg)
				} else if tc.errMsg != "" && !containsSubstring(err.Error(), tc.errMsg) {
					t.Errorf("ValidateNumaForVM() error = %q, expected to contain %q", err.Error(), tc.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateNumaForVM() unexpected error: %v", err)
				}
			}
		})
	}
}

// containsSubstring checks if s contains substr
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestPrepareVNumaTopology(t *testing.T) {
	// Skip tests on older Windows versions that don't support vNUMA
	if osversion.Build() < osversion.V25H1Server {
		t.Skipf("Skipping vNUMA tests on Windows build %d (requires %d or later)", osversion.Build(), osversion.V25H1Server)
	}

	tests := []struct {
		name               string
		opts               *NumaConfig
		wantNuma           bool
		wantNumaProcessors bool
		wantErr            bool
		errMsg             string
		validateNuma       func(t *testing.T, numa *hcsschema.Numa)
		validateProc       func(t *testing.T, proc *hcsschema.NumaProcessors)
	}{
		{
			name: "nil MaxProcessorsPerNumaNode and empty NumaMappedPhysicalNodes returns nil",
			opts: &NumaConfig{
				MaxProcessorsPerNumaNode:   0,
				MaxMemorySizePerNumaNode:   0,
				PreferredPhysicalNumaNodes: nil,
				NumaMappedPhysicalNodes:    nil,
				NumaProcessorCounts:        nil,
				NumaMemoryBlocksCounts:     nil,
			},
			wantNuma:           false,
			wantNumaProcessors: false,
			wantErr:            false,
		},
		{
			name: "implicit topology with MaxProcessorsPerNumaNode set",
			opts: &NumaConfig{
				MaxProcessorsPerNumaNode:   4,
				MaxMemorySizePerNumaNode:   1024,
				PreferredPhysicalNumaNodes: []uint32{0, 1},
			},
			wantNuma:           true,
			wantNumaProcessors: true,
			wantErr:            false,
			validateNuma: func(t *testing.T, numa *hcsschema.Numa) {
				t.Helper()
				if numa.MaxSizePerNode != 1024 {
					t.Errorf("expected MaxSizePerNode = 1024, got %d", numa.MaxSizePerNode)
				}
				if len(numa.PreferredPhysicalNodes) != 2 {
					t.Errorf("expected 2 PreferredPhysicalNodes, got %d", len(numa.PreferredPhysicalNodes))
				}
				if numa.PreferredPhysicalNodes[0] != 0 || numa.PreferredPhysicalNodes[1] != 1 {
					t.Errorf("unexpected PreferredPhysicalNodes values: %v", numa.PreferredPhysicalNodes)
				}
			},
			validateProc: func(t *testing.T, proc *hcsschema.NumaProcessors) {
				t.Helper()
				if proc.CountPerNode.Max != 4 {
					t.Errorf("expected CountPerNode.Max = 4, got %d", proc.CountPerNode.Max)
				}
			},
		},
		{
			name: "implicit topology without max memory size per node errors",
			opts: &NumaConfig{
				MaxProcessorsPerNumaNode:   4,
				MaxMemorySizePerNumaNode:   0, // missing
				PreferredPhysicalNumaNodes: nil,
			},
			wantErr: true,
			errMsg:  "max size per node must be set",
		},
		{
			name: "explicit topology with matching slices",
			opts: &NumaConfig{
				NumaMappedPhysicalNodes: []uint32{0, 1},
				NumaProcessorCounts:     []uint32{4, 4},
				NumaMemoryBlocksCounts:  []uint64{1024, 1024},
			},
			wantNuma:           true,
			wantNumaProcessors: false,
			wantErr:            false,
			validateNuma: func(t *testing.T, numa *hcsschema.Numa) {
				t.Helper()
				if numa.VirtualNodeCount != 2 {
					t.Errorf("expected VirtualNodeCount = 2, got %d", numa.VirtualNodeCount)
				}
				if len(numa.Settings) != 2 {
					t.Errorf("expected 2 settings, got %d", len(numa.Settings))
				}
				// Verify first setting
				if numa.Settings[0].VirtualNodeNumber != 0 {
					t.Errorf("expected VirtualNodeNumber = 0, got %d", numa.Settings[0].VirtualNodeNumber)
				}
				if numa.Settings[0].PhysicalNodeNumber != 0 {
					t.Errorf("expected PhysicalNodeNumber = 0, got %d", numa.Settings[0].PhysicalNodeNumber)
				}
				if numa.Settings[0].CountOfProcessors != 4 {
					t.Errorf("expected CountOfProcessors = 4, got %d", numa.Settings[0].CountOfProcessors)
				}
				if numa.Settings[0].CountOfMemoryBlocks != 1024 {
					t.Errorf("expected CountOfMemoryBlocks = 1024, got %d", numa.Settings[0].CountOfMemoryBlocks)
				}
				if numa.Settings[0].MemoryBackingType != hcsschema.MemoryBackingType_PHYSICAL {
					t.Errorf("expected MemoryBackingType = PHYSICAL, got %v", numa.Settings[0].MemoryBackingType)
				}
			},
		},
		{
			name: "explicit topology with mismatched slice lengths",
			opts: &NumaConfig{
				NumaMappedPhysicalNodes: []uint32{0, 1, 2},
				NumaProcessorCounts:     []uint32{4, 4},       // mismatched
				NumaMemoryBlocksCounts:  []uint64{1024, 1024}, // mismatched
			},
			wantErr: true,
			errMsg:  "mismatch in number of physical numa nodes",
		},
		{
			name: "explicit topology with invalid settings (zero memory)",
			opts: &NumaConfig{
				NumaMappedPhysicalNodes: []uint32{0},
				NumaProcessorCounts:     []uint32{4},
				NumaMemoryBlocksCounts:  []uint64{0}, // zero memory
			},
			wantErr: true,
			errMsg:  "vNUMA nodes with no memory are not allowed",
		},
		{
			name: "explicit topology with wildcard physical nodes",
			opts: &NumaConfig{
				NumaMappedPhysicalNodes: []uint32{0xFF, 0xFF},
				NumaProcessorCounts:     []uint32{4, 4},
				NumaMemoryBlocksCounts:  []uint64{1024, 1024},
			},
			wantNuma:           true,
			wantNumaProcessors: false,
			wantErr:            false,
			validateNuma: func(t *testing.T, numa *hcsschema.Numa) {
				t.Helper()
				for i, s := range numa.Settings {
					if s.PhysicalNodeNumber != 0xFF {
						t.Errorf("expected PhysicalNodeNumber = 0xFF for node %d, got %d", i, s.PhysicalNodeNumber)
					}
				}
			},
		},
		{
			name: "partial implicit vNUMA config warns but returns nil",
			opts: &NumaConfig{
				MaxProcessorsPerNumaNode:   0, // not set
				MaxMemorySizePerNumaNode:   1024,
				PreferredPhysicalNumaNodes: []uint32{0},
			},
			wantNuma:           false,
			wantNumaProcessors: false,
			wantErr:            false,
		},
		{
			name: "partial explicit vNUMA config warns but returns nil",
			opts: &NumaConfig{
				NumaMappedPhysicalNodes: nil, // not set
				NumaProcessorCounts:     []uint32{4},
				NumaMemoryBlocksCounts:  []uint64{1024},
			},
			wantNuma:           false,
			wantNumaProcessors: false,
			wantErr:            false,
		},
		{
			name: "explicit topology with preferred physical nodes",
			opts: &NumaConfig{
				NumaMappedPhysicalNodes:    []uint32{0, 1},
				NumaProcessorCounts:        []uint32{4, 4},
				NumaMemoryBlocksCounts:     []uint64{1024, 1024},
				PreferredPhysicalNumaNodes: []uint32{2, 3},
			},
			wantNuma:           true,
			wantNumaProcessors: false,
			wantErr:            false,
			validateNuma: func(t *testing.T, numa *hcsschema.Numa) {
				t.Helper()
				if len(numa.PreferredPhysicalNodes) != 2 {
					t.Errorf("expected 2 PreferredPhysicalNodes, got %d", len(numa.PreferredPhysicalNodes))
				}
				if numa.PreferredPhysicalNodes[0] != 2 || numa.PreferredPhysicalNodes[1] != 3 {
					t.Errorf("unexpected PreferredPhysicalNodes: %v", numa.PreferredPhysicalNodes)
				}
			},
		},
		{
			name: "implicit topology without preferred nodes",
			opts: &NumaConfig{
				MaxProcessorsPerNumaNode:   8,
				MaxMemorySizePerNumaNode:   2048,
				PreferredPhysicalNumaNodes: nil,
			},
			wantNuma:           true,
			wantNumaProcessors: true,
			wantErr:            false,
			validateNuma: func(t *testing.T, numa *hcsschema.Numa) {
				t.Helper()
				if numa.PreferredPhysicalNodes != nil {
					t.Errorf("expected nil PreferredPhysicalNodes, got %v", numa.PreferredPhysicalNodes)
				}
			},
		},
		{
			name: "single node explicit topology",
			opts: &NumaConfig{
				NumaMappedPhysicalNodes: []uint32{0},
				NumaProcessorCounts:     []uint32{16},
				NumaMemoryBlocksCounts:  []uint64{8192},
			},
			wantNuma:           true,
			wantNumaProcessors: false,
			wantErr:            false,
			validateNuma: func(t *testing.T, numa *hcsschema.Numa) {
				t.Helper()
				if numa.VirtualNodeCount != 1 {
					t.Errorf("expected VirtualNodeCount = 1, got %d", numa.VirtualNodeCount)
				}
			},
		},
		{
			name: "explicit topology virtual socket numbers are sequential",
			opts: &NumaConfig{
				NumaMappedPhysicalNodes: []uint32{0, 1, 2},
				NumaProcessorCounts:     []uint32{2, 2, 2},
				NumaMemoryBlocksCounts:  []uint64{512, 512, 512},
			},
			wantNuma:           true,
			wantNumaProcessors: false,
			wantErr:            false,
			validateNuma: func(t *testing.T, numa *hcsschema.Numa) {
				t.Helper()
				for i, s := range numa.Settings {
					if s.VirtualSocketNumber != uint32(i) {
						t.Errorf("expected VirtualSocketNumber = %d, got %d", i, s.VirtualSocketNumber)
					}
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			numa, numaProc, err := PrepareVNumaTopology(ctx, tc.opts)

			if tc.wantErr {
				if err == nil {
					t.Errorf("PrepareVNumaTopology() expected error containing %q, got nil", tc.errMsg)
				} else if tc.errMsg != "" && !containsSubstring(err.Error(), tc.errMsg) {
					t.Errorf("PrepareVNumaTopology() error = %q, expected to contain %q", err.Error(), tc.errMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("PrepareVNumaTopology() unexpected error: %v", err)
			}

			if tc.wantNuma && numa == nil {
				t.Error("PrepareVNumaTopology() expected Numa result, got nil")
			}
			if !tc.wantNuma && numa != nil {
				t.Errorf("PrepareVNumaTopology() expected nil Numa, got %v", numa)
			}

			if tc.wantNumaProcessors && numaProc == nil {
				t.Error("PrepareVNumaTopology() expected NumaProcessors result, got nil")
			}
			if !tc.wantNumaProcessors && numaProc != nil {
				t.Errorf("PrepareVNumaTopology() expected nil NumaProcessors, got %v", numaProc)
			}

			if tc.validateNuma != nil && numa != nil {
				tc.validateNuma(t, numa)
			}
			if tc.validateProc != nil && numaProc != nil {
				tc.validateProc(t, numaProc)
			}
		})
	}
}

func TestPrepareVNumaTopology_OldWindowsVersion(t *testing.T) {
	// This test only runs on older Windows versions
	if osversion.Build() >= osversion.V25H1Server {
		t.Skip("Skipping test for old Windows version check - running on new Windows")
	}

	ctx := context.Background()
	opts := &NumaConfig{
		MaxProcessorsPerNumaNode: 4,
		MaxMemorySizePerNumaNode: 1024,
	}

	_, _, err := PrepareVNumaTopology(ctx, opts)
	if err == nil {
		t.Fatal("PrepareVNumaTopology() expected error on old Windows version, got nil")
	}
	if !containsSubstring(err.Error(), "vNUMA topology is not supported") {
		t.Errorf("PrepareVNumaTopology() error = %q, expected to contain 'vNUMA topology is not supported'", err.Error())
	}
}
