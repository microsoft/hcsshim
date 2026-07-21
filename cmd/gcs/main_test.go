//go:build linux
// +build linux

package main

import (
	"strings"
	"testing"
)

func TestCalculateContainersMemoryLimit(t *testing.T) {
	tests := []struct {
		name        string
		total       uint64
		reserve     uint64
		want        int64
		wantErrText string
	}{
		{name: "subtracts reserve", total: 4096, reserve: 1024, want: 3072},
		{name: "rejects equal reserve", total: 1024, reserve: 1024, wantErrText: "must be greater"},
		{name: "rejects below reserve", total: 512, reserve: 1024, wantErrText: "must be greater"},
		{name: "rejects int64 overflow", total: uint64(1 << 63), wantErrText: "exceeds maximum"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := calculateContainersMemoryLimit(test.total, test.reserve)
			if test.wantErrText != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErrText) {
					t.Fatalf("expected error containing %q, got %v", test.wantErrText, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("calculateContainersMemoryLimit returned error: %v", err)
			}
			if got != test.want {
				t.Fatalf("calculateContainersMemoryLimit returned %d, want %d", got, test.want)
			}
		})
	}
}
