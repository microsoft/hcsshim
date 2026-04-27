//go:build windows && lcow

package vmparity

import (
	"context"
	"maps"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opencontainers/runtime-spec/specs-go"

	runhcsopts "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	iannotations "github.com/Microsoft/hcsshim/internal/annotations"
	shimannotations "github.com/Microsoft/hcsshim/pkg/annotations"
	vm "github.com/Microsoft/hcsshim/sandbox-spec/vm/v2"
)

// TestLCOWDocumentParityPermutations exercises annotation and option combinations
// that drive different branches in the legacy and v2 LCOW pipelines. Each case
// populates every field it depends on so the comparison checks real values
// rather than defaults. The v2 builder rejects vPMem entirely, so all cases
// force vPMem off on the legacy side to keep the documents directly comparable.
func TestLCOWDocumentParityPermutations(t *testing.T) {
	bootDir := setupBootFiles(t)

	tests := []struct {
		name        string
		annotations map[string]string
		devices     []specs.WindowsDevice
		shimOpts    func() *runhcsopts.Options
	}{
		// ─── CPU partial combinations ───

		{
			name: "CPU: count only",
			annotations: map[string]string{
				shimannotations.ProcessorCount:             "2",
				shimannotations.CPUGroupID:                 "cpu-only-group",
				shimannotations.StorageQoSIopsMaximum:      "1000",
				shimannotations.StorageQoSBandwidthMaximum: "100000",
			},
		},
		{
			name: "CPU: limit only",
			annotations: map[string]string{
				shimannotations.ProcessorLimit:             "50000",
				shimannotations.CPUGroupID:                 "limit-only-group",
				shimannotations.StorageQoSIopsMaximum:      "1000",
				shimannotations.StorageQoSBandwidthMaximum: "100000",
			},
		},
		{
			name: "CPU: weight only",
			annotations: map[string]string{
				shimannotations.ProcessorWeight:            "500",
				shimannotations.CPUGroupID:                 "weight-only-group",
				shimannotations.StorageQoSIopsMaximum:      "1000",
				shimannotations.StorageQoSBandwidthMaximum: "100000",
			},
		},

		// ─── Memory partial combinations ───

		{
			name: "memory: overcommit disabled",
			annotations: map[string]string{
				shimannotations.MemorySizeInMB:             "2048",
				shimannotations.AllowOvercommit:            "false",
				shimannotations.CPUGroupID:                 "mem-nocommit-group",
				shimannotations.StorageQoSIopsMaximum:      "1000",
				shimannotations.StorageQoSBandwidthMaximum: "100000",
			},
		},
		{
			name: "memory: cold discard hint",
			annotations: map[string]string{
				shimannotations.MemorySizeInMB:             "1024",
				shimannotations.EnableColdDiscardHint:      "true",
				shimannotations.CPUGroupID:                 "cold-discard-group",
				shimannotations.StorageQoSIopsMaximum:      "1000",
				shimannotations.StorageQoSBandwidthMaximum: "100000",
			},
		},
		{
			name: "memory: deferred commit enabled (with overcommit)",
			annotations: map[string]string{
				shimannotations.AllowOvercommit:            "true",
				shimannotations.EnableDeferredCommit:       "true",
				shimannotations.MemorySizeInMB:             "2048",
				shimannotations.CPUGroupID:                 "deferred-commit-group",
				shimannotations.StorageQoSIopsMaximum:      "1000",
				shimannotations.StorageQoSBandwidthMaximum: "100000",
			},
		},
		{
			name: "memory: non-256-aligned size triggers normalization",
			annotations: map[string]string{
				shimannotations.MemorySizeInMB:             "1000",
				shimannotations.CPUGroupID:                 "mem-normalize-group",
				shimannotations.StorageQoSIopsMaximum:      "1000",
				shimannotations.StorageQoSBandwidthMaximum: "100000",
			},
		},

		// ─── Boot mode interactions ───

		{
			name: "boot: kernel direct + VHD rootfs",
			annotations: map[string]string{
				shimannotations.KernelDirectBoot:           "true",
				shimannotations.PreferredRootFSType:        "vhd",
				shimannotations.CPUGroupID:                 "vhd-boot-group",
				shimannotations.StorageQoSIopsMaximum:      "1000",
				shimannotations.StorageQoSBandwidthMaximum: "100000",
			},
		},
		{
			name: "boot: UEFI (kernel direct disabled)",
			annotations: map[string]string{
				shimannotations.KernelDirectBoot:           "false",
				shimannotations.CPUGroupID:                 "uefi-boot-group",
				shimannotations.StorageQoSIopsMaximum:      "1000",
				shimannotations.StorageQoSBandwidthMaximum: "100000",
			},
		},
		{
			name: "boot: UEFI + VHD rootfs",
			annotations: map[string]string{
				shimannotations.KernelDirectBoot:           "false",
				shimannotations.PreferredRootFSType:        "vhd",
				shimannotations.CPUGroupID:                 "uefi-vhd-group",
				shimannotations.StorageQoSIopsMaximum:      "1000",
				shimannotations.StorageQoSBandwidthMaximum: "100000",
			},
		},

		// ─── Feature flag combinations ───

		{
			name: "feature: scratch encryption + disable writable shares",
			annotations: map[string]string{
				shimannotations.LCOWEncryptedScratchDisk:   "true",
				shimannotations.DisableWritableFileShares:  "true",
				shimannotations.CPUGroupID:                 "scratch-enc-group",
				shimannotations.StorageQoSIopsMaximum:      "1000",
				shimannotations.StorageQoSBandwidthMaximum: "100000",
			},
		},
		{
			name: "feature: writable overlay dirs (VHD rootfs)",
			annotations: map[string]string{
				shimannotations.PreferredRootFSType:        "vhd",
				shimannotations.KernelDirectBoot:           "true",
				iannotations.WritableOverlayDirs:           "true",
				shimannotations.CPUGroupID:                 "overlay-group",
				shimannotations.StorageQoSIopsMaximum:      "1000",
				shimannotations.StorageQoSBandwidthMaximum: "100000",
			},
		},
		{
			name: "feature: policy-based routing enabled",
			annotations: map[string]string{
				iannotations.NetworkingPolicyBasedRouting:  "true",
				shimannotations.CPUGroupID:                 "pbr-group",
				shimannotations.StorageQoSIopsMaximum:      "1000",
				shimannotations.StorageQoSBandwidthMaximum: "100000",
			},
		},

		// ─── HvSocket / extra VSock ports ───

		{
			name: "HvSocket: extra VSock ports",
			annotations: map[string]string{
				iannotations.ExtraVSockPorts:               "1234,5678",
				shimannotations.CPUGroupID:                 "vsock-ports-group",
				shimannotations.StorageQoSIopsMaximum:      "1000",
				shimannotations.StorageQoSBandwidthMaximum: "100000",
			},
		},

		// ─── Kernel args combinations ───

		{
			name: "kernel args: VPCIEnabled + custom boot options",
			annotations: map[string]string{
				shimannotations.VPCIEnabled:                "true",
				shimannotations.KernelBootOptions:          "loglevel=7 debug",
				shimannotations.CPUGroupID:                 "vpci-boot-group",
				shimannotations.StorageQoSIopsMaximum:      "1000",
				shimannotations.StorageQoSBandwidthMaximum: "100000",
			},
		},
		{
			name: "kernel args: disable time sync + process dump + writable overlay (VHD)",
			annotations: map[string]string{
				shimannotations.KernelDirectBoot:             "true",
				shimannotations.PreferredRootFSType:          "vhd",
				shimannotations.DisableLCOWTimeSyncService:   "true",
				shimannotations.ContainerProcessDumpLocation: "/tmp/dumps",
				iannotations.WritableOverlayDirs:             "true",
				shimannotations.CPUGroupID:                   "kargs-combo-group",
				shimannotations.StorageQoSIopsMaximum:        "1000",
				shimannotations.StorageQoSBandwidthMaximum:   "100000",
			},
		},
		{
			name: "kernel args: dump directory path",
			annotations: map[string]string{
				shimannotations.DumpDirectoryPath:          `C:\dumps`,
				shimannotations.CPUGroupID:                 "dump-dir-group",
				shimannotations.StorageQoSIopsMaximum:      "1000",
				shimannotations.StorageQoSBandwidthMaximum: "100000",
			},
		},

		// ─── Cross-group interactions ───

		{
			name: "cross: physically backed + scratch encryption",
			annotations: map[string]string{
				shimannotations.FullyPhysicallyBacked:      "true",
				shimannotations.LCOWEncryptedScratchDisk:   "true",
				shimannotations.MemorySizeInMB:             "4096",
				shimannotations.CPUGroupID:                 "phys-backed-group",
				shimannotations.StorageQoSIopsMaximum:      "5000",
				shimannotations.StorageQoSBandwidthMaximum: "1000000",
			},
		},
		{
			name: "cross: phys backed forces overcommit off",
			annotations: map[string]string{
				shimannotations.FullyPhysicallyBacked:      "true",
				shimannotations.AllowOvercommit:            "true", // builders should override to false
				shimannotations.MemorySizeInMB:             "2048",
				shimannotations.CPUGroupID:                 "phys-backed-override-group",
				shimannotations.StorageQoSIopsMaximum:      "1000",
				shimannotations.StorageQoSBandwidthMaximum: "100000",
			},
		},
		{
			name: "cross: CPU + memory + MMIO + QoS + cold discard + VHD boot",
			annotations: map[string]string{
				shimannotations.ProcessorCount:             "2",
				shimannotations.ProcessorLimit:             "80000",
				shimannotations.ProcessorWeight:            "300",
				shimannotations.CPUGroupID:                 "full-combo-group",
				shimannotations.MemorySizeInMB:             "4096",
				shimannotations.AllowOvercommit:            "true",
				shimannotations.EnableColdDiscardHint:      "true",
				shimannotations.MemoryLowMMIOGapInMB:       "512",
				shimannotations.MemoryHighMMIOBaseInMB:     "2048",
				shimannotations.MemoryHighMMIOGapInMB:      "1024",
				shimannotations.StorageQoSIopsMaximum:      "10000",
				shimannotations.StorageQoSBandwidthMaximum: "2000000",
				shimannotations.KernelDirectBoot:           "true",
				shimannotations.PreferredRootFSType:        "vhd",
			},
		},

		// ─── Shim option overrides vs annotation priority ───

		{
			name: "override: annotation CPU overrides shim option CPU",
			shimOpts: func() *runhcsopts.Options {
				return &runhcsopts.Options{
					SandboxPlatform:   "linux/amd64",
					BootFilesRootPath: bootDir,
					VmProcessorCount:  1,
				}
			},
			annotations: map[string]string{
				shimannotations.ProcessorCount:             "2",
				shimannotations.CPUGroupID:                 "override-cpu-group",
				shimannotations.StorageQoSIopsMaximum:      "1000",
				shimannotations.StorageQoSBandwidthMaximum: "100000",
			},
		},
		{
			name: "override: annotation memory overrides shim option memory",
			shimOpts: func() *runhcsopts.Options {
				return &runhcsopts.Options{
					SandboxPlatform:   "linux/amd64",
					BootFilesRootPath: bootDir,
					VmMemorySizeInMb:  1024,
				}
			},
			annotations: map[string]string{
				shimannotations.MemorySizeInMB:             "4096",
				shimannotations.CPUGroupID:                 "override-mem-group",
				shimannotations.StorageQoSIopsMaximum:      "1000",
				shimannotations.StorageQoSBandwidthMaximum: "100000",
			},
		},
		{
			name: "shim: only shim defaults (no annotations)",
			shimOpts: func() *runhcsopts.Options {
				return &runhcsopts.Options{
					SandboxPlatform:   "linux/amd64",
					BootFilesRootPath: bootDir,
					VmProcessorCount:  4,
					VmMemorySizeInMb:  4096,
				}
			},
			annotations: map[string]string{
				shimannotations.CPUGroupID:                 "shim-defaults-group",
				shimannotations.StorageQoSIopsMaximum:      "1000",
				shimannotations.StorageQoSBandwidthMaximum: "100000",
			},
		},
		{
			name: "shim: default container annotations merged",
			shimOpts: func() *runhcsopts.Options {
				return &runhcsopts.Options{
					SandboxPlatform:   "linux/amd64",
					BootFilesRootPath: bootDir,
					DefaultContainerAnnotations: map[string]string{
						shimannotations.ProcessorCount: "2",
						shimannotations.MemorySizeInMB: "2048",
					},
				}
			},
			annotations: map[string]string{
				shimannotations.CPUGroupID:                 "default-annot-group",
				shimannotations.StorageQoSIopsMaximum:      "1000",
				shimannotations.StorageQoSBandwidthMaximum: "100000",
			},
		},
		{
			name: "regression: no CPUGroupID",
			annotations: map[string]string{
				shimannotations.ProcessorCount:             "2",
				shimannotations.MemorySizeInMB:             "2048",
				shimannotations.StorageQoSIopsMaximum:      "1000",
				shimannotations.StorageQoSBandwidthMaximum: "100000",
			},
		},
		{
			name: "regression: no StorageQoS",
			annotations: map[string]string{
				shimannotations.ProcessorCount: "2",
				shimannotations.MemorySizeInMB: "2048",
				shimannotations.CPUGroupID:     "no-qos-group",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runDocParityCase(t, bootDir, tc.annotations, tc.devices, tc.shimOpts)
		})
	}
}

// runDocParityCase builds the legacy and v2 LCOW HCS documents for a single
// permutation case using the same annotations, devices, and shim options, then
// fails the test if the resulting documents differ. It mirrors the comparison
// performed by [TestLCOWDocumentParity] so all permutation cases share the same
// failure semantics.
func runDocParityCase(
	t *testing.T,
	bootDir string,
	annotations map[string]string,
	devices []specs.WindowsDevice,
	shimOptsFn func() *runhcsopts.Options,
) {
	t.Helper()
	ctx := context.Background()

	shimOpts := &runhcsopts.Options{
		SandboxPlatform:   "linux/amd64",
		BootFilesRootPath: bootDir,
	}
	if shimOptsFn != nil {
		shimOpts = shimOptsFn()
	}

	// Legacy path: OCI spec with annotations and devices.
	legacySpec := specs.Spec{
		Annotations: maps.Clone(annotations),
		Linux:       &specs.Linux{},
		Windows: &specs.Windows{
			HyperV:  &specs.WindowsHyperV{},
			Devices: devices,
		},
	}
	if legacySpec.Annotations == nil {
		legacySpec.Annotations = map[string]string{}
	}
	// The v2 builder rejects any non-zero vPMem count and never emits a
	// VirtualPMemController. Force vPMem off on the legacy side so the
	// documents are directly comparable.
	legacySpec.Annotations[shimannotations.VPMemCount] = "0"

	legacyDoc, legacyOpts, err := buildLegacyLCOWDocument(ctx, legacySpec, shimOpts, bootDir)
	if err != nil {
		t.Fatalf("failed to build legacy LCOW document: %v", err)
	}

	// V2 path: vm.Spec with the same annotations and devices.
	v2Spec := &vm.Spec{
		Annotations: maps.Clone(annotations),
		Devices:     devices,
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
}

// TestLCOWErrorPathParity verifies that both pipelines reject invalid inputs
// with errors rather than producing divergent documents. Each case asserts that
// either both pipelines fail or both succeed; silent divergence on bad input
// is treated as a parity violation.
func TestLCOWErrorPathParity(t *testing.T) {
	bootDir := setupBootFiles(t)

	tests := []struct {
		name        string
		annotations map[string]string
		shimOpts    func() *runhcsopts.Options
	}{
		{
			name: "invalid boot files path",
			shimOpts: func() *runhcsopts.Options {
				return &runhcsopts.Options{
					SandboxPlatform:   "linux/amd64",
					BootFilesRootPath: `C:\nonexistent\boot\path\that\does\not\exist`,
				}
			},
		},
		{
			// Both pipelines reject deferred commit on physically backed memory:
			// legacy returns "EnableDeferredCommit is not supported on physically
			// backed VMs", v2 returns "enable_deferred_commit is not supported on
			// physically backed vms". The error texts differ but the parity
			// requirement (both fail) holds.
			name: "deferred commit on physically backed memory",
			annotations: map[string]string{
				shimannotations.AllowOvercommit:      "false",
				shimannotations.EnableDeferredCommit: "true",
				shimannotations.MemorySizeInMB:       "2048",
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

			legacySpec := specs.Spec{
				Annotations: maps.Clone(tc.annotations),
				Linux:       &specs.Linux{},
				Windows:     &specs.Windows{HyperV: &specs.WindowsHyperV{}},
			}
			if legacySpec.Annotations == nil {
				legacySpec.Annotations = map[string]string{}
			}
			legacySpec.Annotations[shimannotations.VPMemCount] = "0"
			_, _, legacyErr := buildLegacyLCOWDocument(ctx, legacySpec, shimOpts, bootDir)

			v2Spec := &vm.Spec{Annotations: maps.Clone(tc.annotations)}
			if v2Spec.Annotations == nil {
				v2Spec.Annotations = map[string]string{}
			}
			_, _, v2Err := buildV2LCOWDocument(ctx, shimOpts, v2Spec, bootDir)

			if testing.Verbose() {
				t.Logf("legacy error: %v", legacyErr)
				t.Logf("v2 error: %v", v2Err)
			}

			if (legacyErr == nil) != (v2Err == nil) {
				t.Errorf("error parity mismatch for %q: legacy err=%v, v2 err=%v", tc.name, legacyErr, v2Err)
			}
			if legacyErr == nil && v2Err == nil {
				t.Errorf("expected both LCOW pipelines to reject %q, but both succeeded", tc.name)
			}
		})
	}
}

// TestLCOWSandboxOptionsFieldParityNonDefault verifies that the SandboxOptions
// fields surfaced by the v2 builder match the corresponding OptionsLCOW fields
// from the legacy pipeline when callers opt into non-default values. This
// complements [TestLCOWSandboxOptionsFieldParity] which only covers defaults.
func TestLCOWSandboxOptionsFieldParityNonDefault(t *testing.T) {
	bootDir := setupBootFiles(t)

	tests := []struct {
		name        string
		annotations map[string]string
	}{
		{
			name: "scratch encryption enabled",
			annotations: map[string]string{
				shimannotations.LCOWEncryptedScratchDisk: "true",
			},
		},
		{
			name: "policy-based routing enabled",
			annotations: map[string]string{
				iannotations.NetworkingPolicyBasedRouting: "true",
			},
		},
		{
			name: "fully physically backed",
			annotations: map[string]string{
				shimannotations.FullyPhysicallyBacked: "true",
				shimannotations.MemorySizeInMB:        "2048",
			},
		},
		{
			name: "disable writable file shares",
			annotations: map[string]string{
				shimannotations.DisableWritableFileShares: "true",
			},
		},
		{
			name: "VPMem no multi-mapping",
			annotations: map[string]string{
				shimannotations.VPMemNoMultiMapping: "true",
			},
		},
		{
			name: "all sandbox options non-default",
			annotations: map[string]string{
				shimannotations.LCOWEncryptedScratchDisk:  "true",
				iannotations.NetworkingPolicyBasedRouting: "true",
				shimannotations.FullyPhysicallyBacked:     "true",
				shimannotations.DisableWritableFileShares: "true",
				shimannotations.VPMemNoMultiMapping:       "true",
				shimannotations.MemorySizeInMB:            "2048",
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
