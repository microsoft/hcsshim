package uvm

import (
	"context"
	"testing"

	"github.com/Microsoft/go-winio/pkg/guid"
)

// Unit tests for cpugroup creation, modification, and deletion
func DisabledTestCPUGroupCreateAndDelete(t *testing.T) {
	lps := []uint32{0, 1}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	id, err := createNewCPUGroup(ctx, lps)
	if err != nil {
		t.Fatalf("failed to create cpugroup %s with: %v", id, err)
	}

	defer func() {
		if err := deleteCPUGroup(ctx, id); err != nil {
			t.Fatalf("failed to delete cpugroup %s with: %v", id, err)
		}
	}()

	exists, err := cpuGroupExists(ctx, id)
	if err != nil {
		t.Fatalf("failed to determine if cpugroup exists with: %v", err)
	}
	if !exists {
		t.Fatalf("expected to find cpugroup %s on machine but didn't", id)
	}
}

func DisabledTestCPUGroupCreateWithIDAndDelete(t *testing.T) {
	lps := []uint32{0, 1}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	id, err := guid.NewV4()
	if err != nil {
		t.Fatalf("failed to create cpugroup guid with: %v", err)
	}
	err = createNewCPUGroupWithID(ctx, id.String(), lps)
	if err != nil {
		t.Fatalf("failed to create cpugroup %s with: %v", id.String(), err)
	}
	defer func() {
		if err := deleteCPUGroup(ctx, id.String()); err != nil {
			t.Fatalf("failed to delete cpugroup %s with: %v", id.String(), err)
		}
	}()

	exists, err := cpuGroupExists(ctx, id.String())
	if err != nil {
		t.Fatalf("failed to determine if cpugroup exists with: %v", err)
	}
	if !exists {
		t.Fatalf("expected to find cpugroup %s on machine but didn't", id.String())
	}
}

func DisabledTestCPUGroupSetCap(t *testing.T) {
	lps := []uint32{0, 1}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	id, err := createNewCPUGroup(ctx, lps)
	if err != nil {
		t.Fatalf("failed to create cpugroup %s with: %v", id, err)
	}

	defer func() {
		if err := deleteCPUGroup(ctx, id); err != nil {
			t.Fatalf("failed to delete cpugroup %s with: %v", id, err)
		}
	}()
	cap := uint32(32768)
	if err := setCPUGroupCap(ctx, id, cap); err != nil {
		t.Fatalf("failed to set cpugroup %s cap to %d with: %v", id, cap, err)
	}

	actualCap, err := getCPUGroupCap(ctx, id)
	if err != nil {
		t.Fatalf("failed to get group cap with: %v", err)
	}
	if actualCap != cap {
		t.Fatalf("expected to get a cpugroup cap of %d, instead got %d for group %s", cap, actualCap, id)
	}
}

func DisabledTestCPUGroupSetPriority(t *testing.T) {
	lps := []uint32{0, 1}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	id, err := createNewCPUGroup(ctx, lps)
	if err != nil {
		t.Fatalf("failed to create cpugroup %s with: %v", id, err)
	}

	defer func() {
		if err := deleteCPUGroup(ctx, id); err != nil {
			t.Fatalf("failed to delete cpugroup %s with: %v", id, err)
		}
	}()
	priority := uint32(1)
	if err := setCPUGroupSchedulingPriority(ctx, id, priority); err != nil {
		t.Fatalf("failed to set cpugroup %s priority to %d with: %v", id, priority, err)
	}

	actualPriority, err := getCPUGroupPriority(ctx, id)
	if err != nil {
		t.Fatalf("failed to get group priority value with: %v", err)
	}
	if actualPriority != priority {
		t.Fatalf("expected to get cpugroup priority of %d, instead got %d for cpugroup %s", priority, actualPriority, id)
	}
}
