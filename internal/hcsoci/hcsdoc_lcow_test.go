//go:build windows

package hcsoci

import (
	"context"
	"reflect"
	"testing"

	"github.com/Microsoft/hcsshim/internal/schemaversion"

	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// baseSpec returns a minimal valid spec to reduce boilerplate in tests.
func baseSpec() *specs.Spec {
	return &specs.Spec{
		Linux: &specs.Linux{
			Resources: &specs.LinuxResources{
				CPU: &specs.LinuxCPU{},
			},
		},
		Windows:     &specs.Windows{},
		Annotations: map[string]string{"key": "original"},
	}
}

// TestCreateLCOWSpec_CPUDefaults tests the logic for applying default CPU Period and Quota values.
func TestCreateLCOWSpec_CPUDefaults(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		setup      func() *specs.Spec
		wantPeriod *uint64 // Using pointer to differentiate between 0 and nil
		wantQuota  *int64
	}{
		{
			name: "Defaults applied when explicit 0",
			setup: func() *specs.Spec {
				s := baseSpec()
				s.Linux.Resources.CPU.Period = ptrUint64(0)
				s.Linux.Resources.CPU.Quota = ptrInt64(0)
				return s
			},
			wantPeriod: ptrUint64(100000),
			wantQuota:  ptrInt64(-1),
		},
		{
			name: "Defaults ignored when non-zero",
			setup: func() *specs.Spec {
				s := baseSpec()
				s.Linux.Resources.CPU.Period = ptrUint64(50000)
				s.Linux.Resources.CPU.Quota = ptrInt64(20000)
				return s
			},
			wantPeriod: ptrUint64(50000),
			wantQuota:  ptrInt64(20000),
		},
		{
			name: "Defaults ignored when omitted (nil)",
			// The code checks `if ptr != nil && *ptr == 0`, so nil inputs remain nil.
			setup: func() *specs.Spec {
				s := baseSpec()
				s.Linux.Resources.CPU.Period = nil
				s.Linux.Resources.CPU.Quota = nil
				return s
			},
			wantPeriod: nil,
			wantQuota:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputSpec := tt.setup()
			coi := &createOptionsInternal{CreateOptions: &CreateOptions{Spec: inputSpec}}

			result, err := createLCOWSpec(ctx, coi)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Validate Period
			gotPeriod := result.Linux.Resources.CPU.Period
			if tt.wantPeriod == nil {
				if gotPeriod != nil {
					t.Errorf("Period: want nil, got %d", *gotPeriod)
				}
			} else {
				if gotPeriod == nil || *gotPeriod != *tt.wantPeriod {
					t.Errorf("Period: want %d, got %v", *tt.wantPeriod, gotPeriod)
				}
			}

			// Validate Quota
			gotQuota := result.Linux.Resources.CPU.Quota
			if tt.wantQuota == nil {
				if gotQuota != nil {
					t.Errorf("Quota: want nil, got %d", *gotQuota)
				}
			} else {
				if gotQuota == nil || *gotQuota != *tt.wantQuota {
					t.Errorf("Quota: want %d, got %v", *tt.wantQuota, gotQuota)
				}
			}
		})
	}
}

// TestCreateLCOWSpec_ResourcesDeepCopy tests that all fields in Resources are correctly deep-copied.
func TestCreateLCOWSpec_ResourcesDeepCopy(t *testing.T) {
	// Goal: Verify that other CPU fields and Memory fields are correctly copied
	// and are not lost or corrupted during the process.
	ctx := context.Background()
	s := baseSpec()

	// Populate extensive fields
	s.Linux.Resources.CPU = &specs.LinuxCPU{
		Shares:          ptrUint64(1024),
		RealtimePeriod:  ptrUint64(1000),
		RealtimeRuntime: ptrInt64(500),
		Cpus:            "0-1",
		Mems:            "0",
	}
	s.Linux.Resources.Memory = &specs.LinuxMemory{
		Limit:       ptrInt64(2048),
		Reservation: ptrInt64(1024),
		Swap:        ptrInt64(4096),
		Swappiness:  ptrUint64(60),
	}

	coi := &createOptionsInternal{CreateOptions: &CreateOptions{Spec: s}}
	result, err := createLCOWSpec(ctx, coi)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Validate CPU fields
	resCPU := result.Linux.Resources.CPU
	if *resCPU.Shares != 1024 {
		t.Errorf("CPU Shares mismatch: got %v", *resCPU.Shares)
	}
	if *resCPU.RealtimePeriod != 1000 {
		t.Errorf("CPU RealtimePeriod mismatch: got %v", *resCPU.RealtimePeriod)
	}
	if *resCPU.RealtimeRuntime != 500 {
		t.Errorf("CPU RealtimeRuntime mismatch: got %v", *resCPU.RealtimeRuntime)
	}
	if resCPU.Cpus != "0-1" {
		t.Errorf("CPU Cpus mismatch: got %v", resCPU.Cpus)
	}
	if resCPU.Mems != "0" {
		t.Errorf("CPU Mems mismatch: got %v", resCPU.Mems)
	}

	// Validate Memory fields
	resMem := result.Linux.Resources.Memory
	if *resMem.Limit != 2048 {
		t.Errorf("Memory Limit mismatch: got %v", *resMem.Limit)
	}
	if *resMem.Reservation != 1024 {
		t.Errorf("Memory Reservation mismatch: got %v", *resMem.Reservation)
	}
	if *resMem.Swap != 4096 {
		t.Errorf("Memory Swap mismatch: got %v", *resMem.Swap)
	}
	if *resMem.Swappiness != 60 {
		t.Errorf("Memory Swappiness mismatch: got %v", *resMem.Swappiness)
	}
}

// TestCreateLCOWSpec_WindowsLogicMatrix tests various combinations of Windows struct fields.
func TestCreateLCOWSpec_WindowsLogicMatrix(t *testing.T) {
	// Goal: Test Partial Windows structs.
	ctx := context.Background()

	tests := []struct {
		name          string
		inputWindows  *specs.Windows
		expectWindows bool
		expectNet     string
		expectDevs    int
	}{
		{
			name:          "Nil Windows",
			inputWindows:  nil,
			expectWindows: false,
		},
		{
			name:          "Empty Windows",
			inputWindows:  &specs.Windows{},
			expectWindows: false,
		},
		{
			name: "Network Only",
			inputWindows: &specs.Windows{
				Network: &specs.WindowsNetwork{NetworkNamespace: "host-ns"},
			},
			expectWindows: true,
			expectNet:     "host-ns",
		},
		{
			name: "Devices Only",
			inputWindows: &specs.Windows{
				Devices: []specs.WindowsDevice{{ID: "gpu1"}},
			},
			expectWindows: true,
			expectDevs:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := baseSpec()
			s.Windows = tt.inputWindows
			coi := &createOptionsInternal{CreateOptions: &CreateOptions{Spec: s}}

			res, err := createLCOWSpec(ctx, coi)
			if err != nil {
				t.Fatalf("error: %v", err)
			}

			if !tt.expectWindows && res.Windows != nil {
				t.Errorf("Expected nil Windows, got: %+v", res.Windows)
			}
			if tt.expectWindows && res.Windows == nil {
				t.Fatal("Expected Windows section, got nil")
			}
			if tt.expectNet != "" && (res.Windows.Network == nil || res.Windows.Network.NetworkNamespace != tt.expectNet) {
				t.Errorf("Network Namespace mismatch")
			}
			if res.Windows != nil && len(res.Windows.Devices) != tt.expectDevs {
				t.Errorf("Device count mismatch")
			}
		})
	}
}

// TestCreateLCOWSpec_FieldSanitization tests that specific fields are cleared from the spec.
func TestCreateLCOWSpec_FieldSanitization(t *testing.T) {
	// Goal: Ensure specific fields are cleared but valid ones preserved.
	ctx := context.Background()
	s := baseSpec()

	// Fields to remove
	s.Hooks = &specs.Hooks{CreateContainer: []specs.Hook{{Path: "bad"}}}
	s.Linux.CgroupsPath = "/bad/path"
	s.Linux.Resources.BlockIO = &specs.LinuxBlockIO{Weight: ptrUint16(10)}

	coi := &createOptionsInternal{CreateOptions: &CreateOptions{Spec: s}}
	res, err := createLCOWSpec(ctx, coi)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if res.Hooks != nil {
		t.Error("Hooks not cleared")
	}
	if res.Linux.CgroupsPath != "" {
		t.Error("CgroupsPath not cleared")
	}
	if res.Linux.Resources.BlockIO != nil {
		t.Error("BlockIO not cleared")
	}
}

// TestCreateLinuxContainerDocument_PopulatesFields tests that createLinuxContainerDocument populates fields correctly.
func TestCreateLinuxContainerDocument_PopulatesFields(t *testing.T) {
	ctx := context.Background()

	inputSpec := baseSpec()
	// Trigger defaults
	inputSpec.Linux.Resources.CPU.Period = ptrUint64(0)
	inputSpec.Linux.Resources.CPU.Quota = ptrInt64(0)

	coi := &createOptionsInternal{CreateOptions: &CreateOptions{Spec: inputSpec}}
	guestRoot := "C:\\guest\\root"
	scratch := "C:\\scratch\\path"

	doc, err := createLinuxContainerDocument(ctx, coi, guestRoot, scratch)
	if err != nil {
		t.Fatalf("createLinuxContainerDocument error: %v", err)
	}

	// 1. Schema
	if !reflect.DeepEqual(doc.SchemaVersion, schemaversion.SchemaV21()) {
		t.Errorf("SchemaVersion mismatch")
	}
	// 2. Paths
	if doc.OciBundlePath != guestRoot || doc.ScratchDirPath != scratch {
		t.Errorf("Path mismatch")
	}
	// 3. Spec Defaults
	if *doc.OciSpecification.Linux.Resources.CPU.Period != 100000 {
		t.Errorf("Defaults not applied in wrapper")
	}
}

// --- Helper Functions ---
func ptrUint16(v uint16) *uint16 { return &v }
func ptrUint64(v uint64) *uint64 { return &v }
func ptrInt64(v int64) *int64    { return &v }
