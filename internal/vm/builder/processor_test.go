//go:build windows

package builder

import (
	"testing"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

func TestSetProcessor(t *testing.T) {
	tests := []struct {
		name string
		// configs is a sequence of SetProcessor calls to apply.
		configs []*hcsschema.VirtualMachineProcessor
		// wantNoop when true means the final Processor should be the default (non-nil, zero-value).
		wantNoop bool
		// want is the expected final Processor state (ignored when wantNoop is true).
		want *hcsschema.VirtualMachineProcessor
	}{
		{
			name: "all fields",
			configs: []*hcsschema.VirtualMachineProcessor{
				{Count: 4, Limit: 2500, Weight: 200, Reservation: 1000},
			},
			want: &hcsschema.VirtualMachineProcessor{Count: 4, Limit: 2500, Weight: 200, Reservation: 1000},
		},
		{
			name:     "nil is no-op",
			configs:  []*hcsschema.VirtualMachineProcessor{nil},
			wantNoop: true,
		},
		{
			name: "overwrite replaces completely",
			configs: []*hcsschema.VirtualMachineProcessor{
				{Count: 2, Weight: 100},
				{Count: 8, Limit: 5000},
			},
			want: &hcsschema.VirtualMachineProcessor{Count: 8, Limit: 5000},
		},
		{
			name: "nil after set is no-op",
			configs: []*hcsschema.VirtualMachineProcessor{
				{Count: 4, Weight: 300},
				nil,
			},
			want: &hcsschema.VirtualMachineProcessor{Count: 4, Weight: 300},
		},
		{
			name: "count only",
			configs: []*hcsschema.VirtualMachineProcessor{
				{Count: 16},
			},
			want: &hcsschema.VirtualMachineProcessor{Count: 16},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, cs := newBuilder(t)
			var processor ProcessorOptions = b

			for _, cfg := range tt.configs {
				processor.SetProcessor(cfg)
			}

			proc := cs.VirtualMachine.ComputeTopology.Processor
			if proc == nil {
				t.Fatal("Processor should never be nil")
			}

			if tt.wantNoop {
				// Default processor from New() is a zero-value struct.
				if proc.Count != 0 || proc.Limit != 0 || proc.Weight != 0 || proc.Reservation != 0 {
					t.Fatalf("expected default processor, got %+v", proc)
				}
				return
			}

			if proc.Count != tt.want.Count {
				t.Fatalf("Processor.Count = %d, want %d", proc.Count, tt.want.Count)
			}
			if proc.Limit != tt.want.Limit {
				t.Fatalf("Processor.Limit = %d, want %d", proc.Limit, tt.want.Limit)
			}
			if proc.Weight != tt.want.Weight {
				t.Fatalf("Processor.Weight = %d, want %d", proc.Weight, tt.want.Weight)
			}
			if proc.Reservation != tt.want.Reservation {
				t.Fatalf("Processor.Reservation = %d, want %d", proc.Reservation, tt.want.Reservation)
			}
		})
	}
}

func TestSetCPUGroup(t *testing.T) {
	tests := []struct {
		name string
		// processor is an optional SetProcessor call before SetCPUGroup.
		// nil means use the default processor from New().
		processor *hcsschema.VirtualMachineProcessor
		// groups is a sequence of SetCPUGroup calls to apply.
		groups []*hcsschema.CpuGroup
		// wantGroupID is the expected CpuGroup.Id after all calls. Empty means CpuGroup should be nil.
		wantGroupID string
		// wantCount is the expected Processor.Count after all calls (to verify SetCPUGroup doesn't clobber processor config).
		wantCount uint32
	}{
		{
			name:        "set group on default processor",
			groups:      []*hcsschema.CpuGroup{{Id: "group-1"}},
			wantGroupID: "group-1",
		},
		{
			name:        "nil clears group",
			groups:      []*hcsschema.CpuGroup{{Id: "group-1"}, nil},
			wantGroupID: "",
		},
		{
			name:        "overwrite group",
			groups:      []*hcsschema.CpuGroup{{Id: "first"}, {Id: "second"}},
			wantGroupID: "second",
		},
		{
			name:        "after SetProcessor",
			processor:   &hcsschema.VirtualMachineProcessor{Count: 4, Limit: 2500},
			groups:      []*hcsschema.CpuGroup{{Id: "cg-after-set"}},
			wantGroupID: "cg-after-set",
			wantCount:   4,
		},
		{
			name:        "after nil SetProcessor (no-op)",
			groups:      []*hcsschema.CpuGroup{{Id: "safe-group"}},
			wantGroupID: "safe-group",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, cs := newBuilder(t)
			var p ProcessorOptions = b

			if tt.processor != nil {
				p.SetProcessor(tt.processor)
			}

			for _, g := range tt.groups {
				p.SetCPUGroup(g)
			}

			proc := cs.VirtualMachine.ComputeTopology.Processor
			if tt.wantGroupID == "" {
				if proc.CpuGroup != nil {
					t.Fatalf("CpuGroup = %+v, want nil", proc.CpuGroup)
				}
			} else {
				if proc.CpuGroup == nil {
					t.Fatal("CpuGroup should not be nil")
				}
				if proc.CpuGroup.Id != tt.wantGroupID {
					t.Fatalf("CpuGroup.Id = %q, want %q", proc.CpuGroup.Id, tt.wantGroupID)
				}
			}

			if tt.wantCount != 0 && proc.Count != tt.wantCount {
				t.Fatalf("Processor.Count = %d, want %d", proc.Count, tt.wantCount)
			}
		})
	}
}
