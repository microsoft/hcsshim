//go:build linux
// +build linux

package main

import (
	"reflect"
	"strings"
	"testing"

	oci "github.com/opencontainers/runtime-spec/specs-go"
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

func TestSetCgroupMemoryLimitsUpdatesParentBeforeChild(t *testing.T) {
	var updateOrder []string
	podsControl := &testCgroupUpdater{name: "pods", updateOrder: &updateOrder}

	limit, updated, err := setCgroupMemoryLimits(podsControl, 4096, 1024, 2048)
	if err != nil {
		t.Fatalf("setCgroupMemoryLimits returned error: %v", err)
	}
	if !updated {
		t.Fatal("setCgroupMemoryLimits did not report an update")
	}
	if limit != 3072 {
		t.Fatalf("setCgroupMemoryLimits returned limit %d, want 3072", limit)
	}
	if want := []string{"pods"}; !reflect.DeepEqual(updateOrder, want) {
		t.Fatalf("cgroups updated in order %v, want %v", updateOrder, want)
	}
	if podsControl.limit != 3072 {
		t.Fatalf("cgroup limits are pods=%d, want 3072", podsControl.limit)
	}
}

func TestSetCgroupMemoryLimitsSkipsUnchangedLimit(t *testing.T) {
	podsControl := &testCgroupUpdater{}

	limit, updated, err := setCgroupMemoryLimits(podsControl, 4096, 1024, 3072)
	if err != nil {
		t.Fatalf("setCgroupMemoryLimits returned error: %v", err)
	}
	if updated {
		t.Fatal("setCgroupMemoryLimits reported an update for an unchanged limit")
	}
	if limit != 3072 {
		t.Fatalf("setCgroupMemoryLimits returned limit %d, want 3072", limit)
	}
	if podsControl.updates != 0 {
		t.Fatalf("unchanged limit updated cgroups: pods=%d", podsControl.updates)
	}
}

type testCgroupUpdater struct {
	name        string
	limit       int64
	updates     int
	updateErr   error
	updateOrder *[]string
}

func (cgroup *testCgroupUpdater) Update(resources *oci.LinuxResources) error {
	cgroup.updates++
	if cgroup.updateErr != nil {
		return cgroup.updateErr
	}
	cgroup.limit = *resources.Memory.Limit
	if cgroup.updateOrder != nil {
		*cgroup.updateOrder = append(*cgroup.updateOrder, cgroup.name)
	}
	return nil
}
