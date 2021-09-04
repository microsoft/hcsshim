package cpugroup

import (
	"context"
	"testing"

	"github.com/Microsoft/go-winio/pkg/guid"
)

// Unit tests for creating and deleting a CPU group on the host
func TestCPUGroupCreateWithIDAndDelete(t *testing.T) {
	t.Skip("only works on classic/core scheduler, skipping as we can't check this dynamically right now")

	lps := []uint32{0, 1}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	id, err := guid.NewV4()
	if err != nil {
		t.Fatalf("failed to create cpugroup guid with: %v", err)
	}

	err = Create(ctx, id.String(), lps)
	if err != nil {
		t.Fatalf("failed to create cpugroup %s with: %v", id.String(), err)
	}

	defer func() {
		if err := Delete(ctx, id.String()); err != nil {
			t.Fatalf("failed to delete cpugroup %s with: %v", id.String(), err)
		}
	}()

	_, err = GetCPUGroupConfig(ctx, id.String())
	if err != nil {
		t.Fatalf("failed to find cpugroup on host with: %v", err)
	}
}
