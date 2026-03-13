//go:build windows && functional

package parity

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opencontainers/runtime-spec/specs-go"

	runhcsopts "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	shimannotations "github.com/Microsoft/hcsshim/pkg/annotations"
	vm "github.com/Microsoft/hcsshim/sandbox-spec/vm/v2"
)

// TestLCOWDocumentParity feeds identical inputs to both the legacy and v2
// pipelines and verifies the resulting HCS documents match.
func TestLCOWDocumentParity(t *testing.T) {
	t.Parallel()
	bootDir := setupBootFiles(t)

	tests := []struct {
		name        string
		annotations map[string]string          // applied to both pipelines
		devices     []specs.WindowsDevice      // vPCI devices for both pipelines
		shimOpts    func() *runhcsopts.Options // nil = use default
	}{
		{
			name: "default config",
		},
		{
			name: "custom CPU and memory",
			annotations: map[string]string{
				shimannotations.ProcessorCount: "2",
				shimannotations.MemorySizeInMB: "2048",
			},
		},
		{
			name: "storage QoS",
			annotations: map[string]string{
				shimannotations.StorageQoSIopsMaximum:      "5000",
				shimannotations.StorageQoSBandwidthMaximum: "1000000",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			// Shim options — identical for both paths.
			shimOpts := &runhcsopts.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: bootDir,
			}
			if tc.shimOpts != nil {
				shimOpts = tc.shimOpts()
			}

			// --- Legacy path ---
			// Build OCI spec with annotations and devices, then run the full
			// old shim pipeline: UpdateSpecFromOptions → ProcessAnnotations →
			// SpecToUVMCreateOpts → MakeLCOWDocument.
			legacySpec := specs.Spec{
				Annotations: make(map[string]string),
				Linux:       &specs.Linux{},
				Windows: &specs.Windows{
					HyperV:  &specs.WindowsHyperV{},
					Devices: tc.devices,
				},
			}
			for k, v := range tc.annotations {
				legacySpec.Annotations[k] = v
			}
			legacyDoc, legacyOpts, err := buildLegacyDocument(ctx, legacySpec, shimOpts, bootDir)
			if err != nil {
				t.Fatalf("legacy: %v", err)
			}

			// --- V2 path ---
			// Build vm.Spec with the same annotations and devices, then call
			// the v2 builder: BuildSandboxConfig.
			v2Spec := &vm.Spec{
				Annotations: make(map[string]string),
				Devices:     tc.devices,
			}
			for k, v := range tc.annotations {
				v2Spec.Annotations[k] = v
			}
			v2Doc, sandboxOpts, err := buildV2Document(ctx, shimOpts, v2Spec, bootDir)
			if err != nil {
				t.Fatalf("v2: %v", err)
			}

			// Log both config structs side by side.
			t.Logf("Legacy opts: AllowOvercommit=%v PhysicallyBacked=%v ScratchEncrypt=%v "+
				"NoWriteShares=%v PolicyRouting=%v VPMemNoMultiMap=%v",
				legacyOpts.AllowOvercommit, legacyOpts.FullyPhysicallyBacked,
				legacyOpts.EnableScratchEncryption, legacyOpts.NoWritableFileShares,
				legacyOpts.PolicyBasedRouting, legacyOpts.VPMemNoMultiMapping)
			t.Logf("V2 sandbox:  PhysicallyBacked=%v ScratchEncrypt=%v "+
				"NoWriteShares=%v PolicyRouting=%v VPMEMMultiMap=%v Arch=%s",
				sandboxOpts.FullyPhysicallyBacked, sandboxOpts.EnableScratchEncryption,
				sandboxOpts.NoWritableFileShares, sandboxOpts.PolicyBasedRouting,
				sandboxOpts.VPMEMMultiMapping, sandboxOpts.Architecture)

			// Normalize and compare.
			normalizeDoc(legacyDoc)
			normalizeDoc(v2Doc)

			if diff := cmp.Diff(legacyDoc, v2Doc); diff != "" {
				t.Logf("Legacy doc:\n%s", mustJSON(legacyDoc))
				t.Logf("V2 doc:\n%s", mustJSON(v2Doc))
				t.Errorf("Document mismatch (-legacy +v2):\n%s", diff)
			}
		})
	}
}

// TestSandboxOptionsFieldParity verifies that config fields match between
// legacy OptionsLCOW and v2 SandboxOptions for default inputs.
func TestSandboxOptionsFieldParity(t *testing.T) {
	t.Parallel()
	bootDir := setupBootFiles(t)
	ctx := context.Background()

	shimOpts := &runhcsopts.Options{
		SandboxPlatform:   "linux/amd64",
		BootFilesRootPath: bootDir,
	}

	// Legacy: build OCI spec inline, run pipeline.
	legacySpec := specs.Spec{
		Annotations: map[string]string{},
		Linux:       &specs.Linux{},
		Windows:     &specs.Windows{HyperV: &specs.WindowsHyperV{}},
	}
	_, legacyOpts, err := buildLegacyDocument(ctx, legacySpec, shimOpts, bootDir)
	if err != nil {
		t.Fatalf("legacy: %v", err)
	}

	// V2: build vm.Spec inline, run builder.
	v2Spec := &vm.Spec{Annotations: map[string]string{}}
	_, sandboxOpts, err := buildV2Document(ctx, shimOpts, v2Spec, bootDir)
	if err != nil {
		t.Fatalf("v2: %v", err)
	}

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
				t.Errorf("%s mismatch: legacy=%v v2=%v", c.name, c.legacy, c.v2)
			}
		})
	}
}
