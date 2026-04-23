//go:build windows

package vmparity

import (
	"context"
	"maps"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opencontainers/runtime-spec/specs-go"

	runhcsopts "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	lcowbuilder "github.com/Microsoft/hcsshim/internal/builder/vm/lcow"
	"github.com/Microsoft/hcsshim/internal/uvm"
	shimannotations "github.com/Microsoft/hcsshim/pkg/annotations"
	vm "github.com/Microsoft/hcsshim/sandbox-spec/vm/v2"
)

// TestLCOWDocumentParity feeds identical annotations, devices, and shim options
// to both the legacy and v2 LCOW pipelines and verifies the resulting HCS
// ComputeSystem documents are structurally identical.
func TestLCOWDocumentParity(t *testing.T) {
	bootDir := setupBootFiles(t)

	tests := []struct {
		name        string
		annotations map[string]string
		devices     []specs.WindowsDevice
		shimOpts    func() *runhcsopts.Options
	}{
		{
			name: "CPU + memory + QoS + MMIO + CPUGroup",
			annotations: map[string]string{
				shimannotations.ProcessorCount:             "2",
				shimannotations.ProcessorLimit:             "50000",
				shimannotations.ProcessorWeight:            "500",
				shimannotations.CPUGroupID:                 "test-cpu-group-id-123",
				shimannotations.MemorySizeInMB:             "2048",
				shimannotations.AllowOvercommit:            "true",
				shimannotations.EnableColdDiscardHint:      "true",
				shimannotations.MemoryLowMMIOGapInMB:       "256",
				shimannotations.MemoryHighMMIOBaseInMB:     "1024",
				shimannotations.MemoryHighMMIOGapInMB:      "512",
				shimannotations.StorageQoSIopsMaximum:      "5000",
				shimannotations.StorageQoSBandwidthMaximum: "1000000",
			},
		},
		{
			name: "memory + MMIO + QoS (overcommit off)",
			annotations: map[string]string{
				shimannotations.MemorySizeInMB:             "4096",
				shimannotations.AllowOvercommit:            "false",
				shimannotations.MemoryLowMMIOGapInMB:       "256",
				shimannotations.MemoryHighMMIOBaseInMB:     "1024",
				shimannotations.MemoryHighMMIOGapInMB:      "512",
				shimannotations.CPUGroupID:                 "test-cpu-group-id-456",
				shimannotations.StorageQoSIopsMaximum:      "3000",
				shimannotations.StorageQoSBandwidthMaximum: "500000",
			},
		},
		{
			name: "shim options CPU/memory + annotation QoS",
			shimOpts: func() *runhcsopts.Options {
				return &runhcsopts.Options{
					SandboxPlatform:   "linux/amd64",
					BootFilesRootPath: bootDir,
					VmProcessorCount:  2,
					VmMemorySizeInMb:  2048,
				}
			},
			annotations: map[string]string{
				shimannotations.CPUGroupID:                 "test-cpu-group-id-789",
				shimannotations.StorageQoSIopsMaximum:      "5000",
				shimannotations.StorageQoSBandwidthMaximum: "1000000",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			shimOpts := &runhcsopts.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: bootDir,
			}
			if tc.shimOpts != nil {
				shimOpts = tc.shimOpts()
			}

			// Legacy path: OCI spec with annotations and devices.
			legacySpec := specs.Spec{
				Annotations: maps.Clone(tc.annotations),
				Linux:       &specs.Linux{},
				Windows: &specs.Windows{
					HyperV:  &specs.WindowsHyperV{},
					Devices: tc.devices,
				},
			}
			if legacySpec.Annotations == nil {
				legacySpec.Annotations = map[string]string{}
			}
			// The v2 builder does not support vPMem devices and always routes the
			// rootfs through SCSI. Disable vPMem on the legacy side so the resulting
			// HCS documents are directly comparable.
			legacySpec.Annotations[shimannotations.VPMemCount] = "0"
			legacyDoc, legacyOpts, err := buildLegacyLCOWDocument(ctx, legacySpec, shimOpts, bootDir)
			if err != nil {
				t.Fatalf("failed to build legacy LCOW document: %v", err)
			}

			// V2 path: vm.Spec with the same annotations and devices.
			v2Spec := &vm.Spec{
				Annotations: maps.Clone(tc.annotations),
				Devices:     tc.devices,
			}
			if v2Spec.Annotations == nil {
				v2Spec.Annotations = map[string]string{}
			}
			v2Doc, sandboxOpts, err := buildV2LCOWDocument(ctx, shimOpts, v2Spec, bootDir)
			if err != nil {
				t.Fatalf("failed to build v2 LCOW document: %v", err)
			}

			if testing.Verbose() {
				t.Logf("Legacy options: %+v", legacyOpts)
				t.Logf("V2 sandbox options: %+v", sandboxOpts)
			}

			if diff := cmp.Diff(legacyDoc, v2Doc); diff != "" {
				t.Logf("Legacy document:\n%s", jsonToString(legacyDoc))
				t.Logf("V2 document:\n%s", jsonToString(v2Doc))
				t.Errorf("LCOW HCS document mismatch (-legacy +v2):\n%s", diff)
			}
		})
	}
}

// TestLCOWSandboxOptionsFieldParity verifies that configuration fields carried
// by the legacy OptionsLCOW have matching values in the v2 SandboxOptions when
// both pipelines receive the same default inputs.
func TestLCOWSandboxOptionsFieldParity(t *testing.T) {
	bootDir := setupBootFiles(t)

	tests := []struct {
		name        string
		annotations map[string]string
	}{
		{
			name: "default config",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			shimOpts := &runhcsopts.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: bootDir,
			}

			legacySpec := specs.Spec{
				Annotations: maps.Clone(tc.annotations),
				Linux:       &specs.Linux{},
				Windows:     &specs.Windows{HyperV: &specs.WindowsHyperV{}},
			}
			if legacySpec.Annotations == nil {
				legacySpec.Annotations = map[string]string{}
			}
			_, legacyOpts, err := buildLegacyLCOWDocument(ctx, legacySpec, shimOpts, bootDir)
			if err != nil {
				t.Fatalf("failed to build legacy LCOW document: %v", err)
			}

			v2Spec := &vm.Spec{Annotations: maps.Clone(tc.annotations)}
			if v2Spec.Annotations == nil {
				v2Spec.Annotations = map[string]string{}
			}
			_, sandboxOpts, err := buildV2LCOWDocument(ctx, shimOpts, v2Spec, bootDir)
			if err != nil {
				t.Fatalf("failed to build v2 LCOW document: %v", err)
			}

			checkSandboxOptionsParity(t, legacyOpts, sandboxOpts)
		})
	}
}

// checkSandboxOptionsParity verifies that each configuration field in the legacy
// OptionsLCOW matches its v2 SandboxOptions counterpart. Extracted as a helper
// for extensibility across test cases.
func checkSandboxOptionsParity(t *testing.T, legacyOpts *uvm.OptionsLCOW, sandboxOpts *lcowbuilder.SandboxOptions) {
	t.Helper()

	checks := []struct {
		name   string
		legacy interface{}
		v2     interface{}
	}{
		{"NoWritableFileShares", legacyOpts.NoWritableFileShares, sandboxOpts.NoWritableFileShares},
		{"EnableScratchEncryption", legacyOpts.EnableScratchEncryption, sandboxOpts.EnableScratchEncryption},
		{"PolicyBasedRouting", legacyOpts.PolicyBasedRouting, sandboxOpts.PolicyBasedRouting},
		{"FullyPhysicallyBacked", legacyOpts.FullyPhysicallyBacked, sandboxOpts.FullyPhysicallyBacked},
		{"VPMEMMultiMapping", !legacyOpts.VPMemNoMultiMapping, sandboxOpts.VPMEMMultiMapping},
	}

	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if c.legacy != c.v2 {
				t.Errorf("sandbox option %s mismatch: legacy=%v, v2=%v", c.name, c.legacy, c.v2)
			}
		})
	}
}
