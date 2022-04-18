//go:build windows

package uvm

import (
	"context"
	"testing"

	"github.com/Microsoft/hcsshim/internal/memory"
)

func setupNewVPMemScenario(ctx context.Context, t *testing.T, size uint64, hostPath, uvmPath string) (*vPMemInfoMulti, *mappedDeviceInfo) {
	pmem := newPackedVPMemDevice()
	memReg, err := pmem.Allocate(size)
	if err != nil {
		t.Fatalf("failed to setup multi-mapping VPMem Scenario: %s", err)
	}
	mappedDev := newVPMemMappedDevice(hostPath, uvmPath, size, memReg)

	if err := pmem.mapVHDLayer(ctx, mappedDev); err != nil {
		t.Errorf("unexpected error: %s", err)
	}

	// do some basic checks
	md, ok := pmem.mappings[hostPath]
	if !ok {
		t.Fatalf("mapping '%s' not added", hostPath)
	}
	if md.hostPath != hostPath {
		t.Fatalf("expected hostPath=%s, got hostPath=%s", hostPath, md.hostPath)
	}
	if md.uvmPath != uvmPath {
		t.Fatalf("expected uvmPath=%s, got uvmPath=%s", uvmPath, md.uvmPath)
	}
	if md.refCount != 1 {
		t.Fatalf("expected refCount=1, got refCount=%d", md.refCount)
	}

	return pmem, md
}

func Test_VPMem_MapDevice_New(t *testing.T) {
	// basic scenario already validated in the helper function
	setupNewVPMemScenario(context.TODO(), t, memory.MiB, "foo", "bar")
}

func Test_VPMem_UnmapDevice_With_Removal(t *testing.T) {
	pmem, _ := setupNewVPMemScenario(context.TODO(), t, memory.MiB, "foo", "bar")

	err := pmem.unmapVHDLayer(context.TODO(), "foo")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if m, ok := pmem.mappings["foo"]; ok {
		t.Fatalf("mapping should've been removed: %v", m)
	}
}

func Test_VPMem_UnmapDevice_Without_Removal(t *testing.T) {
	pmem, mappedDevice := setupNewVPMemScenario(context.TODO(), t, memory.MiB, "foo", "bar")
	err := pmem.mapVHDLayer(context.TODO(), mappedDevice)
	if err != nil {
		t.Fatalf("unexpected error when mapping device: %s", err)
	}
	m, ok := pmem.mappings["foo"]
	if !ok {
		t.Fatalf("mapping not found")
	}
	if m.refCount != 2 {
		t.Fatalf("expected refCount=2, got refCount=%d", m.refCount)
	}

	err = pmem.unmapVHDLayer(context.TODO(), "foo")
	if err != nil {
		t.Fatalf("error unmapping device: %s", err)
	}
	m, ok = pmem.mappings["foo"]
	if !ok {
		t.Fatalf("mapping should still be present")
	}
	if m.refCount != 1 {
		t.Fatalf("expected refCount=1, got refCount=%d", m.refCount)
	}
}
