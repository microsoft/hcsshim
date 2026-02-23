//go:build windows

package vmutils

import (
	"context"
	"testing"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

func TestNormalizeMemorySize(t *testing.T) {
	tests := []struct {
		name      string
		requested uint64
		expected  uint64
	}{
		{
			name:      "zero memory",
			requested: 0,
			expected:  0,
		},
		{
			name:      "already aligned to 2",
			requested: 2,
			expected:  2,
		},
		{
			name:      "odd number rounds up",
			requested: 1,
			expected:  2,
		},
		{
			name:      "even number stays same",
			requested: 4,
			expected:  4,
		},
		{
			name:      "large odd number rounds up",
			requested: 1023,
			expected:  1024,
		},
		{
			name:      "large even number stays same",
			requested: 1024,
			expected:  1024,
		},
		{
			name:      "very large odd number",
			requested: 65535,
			expected:  65536,
		},
		{
			name:      "very large even number",
			requested: 65536,
			expected:  65536,
		},
		{
			name:      "max uint64 minus 1 (even)",
			requested: 0xFFFFFFFFFFFFFFFE,
			expected:  0xFFFFFFFFFFFFFFFE,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			actual := NormalizeMemorySize(ctx, "test-uvm-id", tc.requested)
			if actual != tc.expected {
				t.Errorf("NormalizeMemorySize(%d) = %d, expected %d", tc.requested, actual, tc.expected)
			}
		})
	}
}

func TestNormalizeProcessorCount(t *testing.T) {
	tests := []struct {
		name                 string
		requested            int32
		hostProcessorLPCount uint32
		expected             int32
	}{
		{
			name:                 "zero requested",
			requested:            0,
			hostProcessorLPCount: 8,
			expected:             0,
		},
		{
			name:                 "requested less than host count",
			requested:            4,
			hostProcessorLPCount: 8,
			expected:             4,
		},
		{
			name:                 "requested equals host count",
			requested:            8,
			hostProcessorLPCount: 8,
			expected:             8,
		},
		{
			name:                 "requested exceeds host count",
			requested:            16,
			hostProcessorLPCount: 8,
			expected:             8,
		},
		{
			name:                 "requested is 1 with single processor host",
			requested:            1,
			hostProcessorLPCount: 1,
			expected:             1,
		},
		{
			name:                 "requested is 2 with single processor host",
			requested:            2,
			hostProcessorLPCount: 1,
			expected:             1,
		},
		{
			name:                 "large requested exceeds large host count",
			requested:            256,
			hostProcessorLPCount: 128,
			expected:             128,
		},
		{
			name:                 "negative requested returns negative (edge case)",
			requested:            -1,
			hostProcessorLPCount: 8,
			expected:             -1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			topology := &hcsschema.ProcessorTopology{
				LogicalProcessorCount: tc.hostProcessorLPCount,
			}
			actual := NormalizeProcessorCount(ctx, "test-uvm-id", tc.requested, topology)
			if actual != tc.expected {
				t.Errorf("NormalizeProcessorCount(%d, %d) = %d, expected %d",
					tc.requested, tc.hostProcessorLPCount, actual, tc.expected)
			}
		})
	}
}
