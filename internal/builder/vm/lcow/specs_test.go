//go:build windows

package lcow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	runhcsoptions "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	iannotations "github.com/Microsoft/hcsshim/internal/annotations"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/processorinfo"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"
	"github.com/Microsoft/hcsshim/osversion"
	shimannotations "github.com/Microsoft/hcsshim/pkg/annotations"
	vm "github.com/Microsoft/hcsshim/sandbox-spec/vm/v2"

	"github.com/opencontainers/runtime-spec/specs-go"
)

type specTestCase struct {
	name        string
	opts        *runhcsoptions.Options
	spec        *vm.Spec
	wantErr     bool
	errContains string
	validate    func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions)
}

func runTestCases(t *testing.T, ctx context.Context, defaultOpts *runhcsoptions.Options, cases []specTestCase) {
	t.Helper()

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			opts := tt.opts
			if opts == nil {
				opts = defaultOpts
			}

			spec := tt.spec
			if spec == nil {
				spec = &vm.Spec{}
			}

			// Use a temp dir as bundlePath for confidential VM tests
			bundlePath := t.TempDir()

			doc, sandboxOpts, err := BuildSandboxConfig(ctx, "test-owner", bundlePath, opts, spec)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContains)
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.validate != nil {
				tt.validate(t, doc, sandboxOpts)
			}
		})
	}
}

// getKernelArgs extracts the kernel command line from the HCS ComputeSystem chipset config.
func getKernelArgs(doc *hcsschema.ComputeSystem) string {
	if doc == nil || doc.VirtualMachine == nil || doc.VirtualMachine.Chipset == nil {
		return ""
	}
	chipset := doc.VirtualMachine.Chipset
	if chipset.LinuxKernelDirect != nil {
		return chipset.LinuxKernelDirect.KernelCmdLine
	}
	if chipset.Uefi != nil && chipset.Uefi.BootThis != nil {
		return chipset.Uefi.BootThis.OptionalData
	}
	return ""
}

// hostProcessorCount queries the host's logical processor count via HCS.
func hostProcessorCount(t *testing.T) int32 {
	t.Helper()
	ctx := context.Background()
	processorTopology, err := processorinfo.HostProcessorInfo(ctx)
	if err != nil {
		t.Fatalf("failed to get host processor information: %v", err)
	}
	return int32(processorTopology.LogicalProcessorCount)
}

func TestBuildSandboxConfig(t *testing.T) {
	ctx := context.Background()

	validBootFilesPath := newBootFilesPath(t)
	hostCount := hostProcessorCount(t)

	tests := []specTestCase{
		{
			name:        "nil options should return error",
			opts:        nil,
			wantErr:     true,
			errContains: "no options provided",
		},
		{
			name: "unsupported platform should return error",
			opts: &runhcsoptions.Options{
				SandboxPlatform: "windows/amd64",
			},
			wantErr:     true,
			errContains: "unsupported sandbox platform",
		},
		{
			name: "minimal valid config for linux/amd64",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if doc == nil {
					t.Fatal("doc should not be nil")
				}
				if doc.VirtualMachine == nil || doc.VirtualMachine.ComputeTopology == nil {
					t.Fatal("compute topology should not be nil")
				}
				if doc.VirtualMachine.ComputeTopology.Processor == nil {
					t.Fatal("processor config should not be nil")
				}
				if sandboxOpts.Architecture != "amd64" {
					t.Errorf("expected architecture amd64, got %v", sandboxOpts.Architecture)
				}
				if doc.VirtualMachine.ComputeTopology.Memory == nil {
					t.Fatal("memory config should not be nil")
				}
				if doc.VirtualMachine.ComputeTopology.Memory.SizeInMB != 1024 {
					t.Errorf("expected default memory 1024MB, got %v", doc.VirtualMachine.ComputeTopology.Memory.SizeInMB)
				}
			},
		},
		{
			name: "minimal valid config for linux/arm64",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/arm64",
				BootFilesRootPath: validBootFilesPath,
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if sandboxOpts.Architecture != "arm64" {
					t.Errorf("expected architecture arm64, got %v", sandboxOpts.Architecture)
				}
			},
		},
		{
			name: "platform case insensitive",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "Linux/AMD64",
				BootFilesRootPath: validBootFilesPath,
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if sandboxOpts.Architecture != "amd64" {
					t.Errorf("expected architecture amd64, got %v", sandboxOpts.Architecture)
				}
			},
		},
		{
			name: "CPU configuration from options",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				VmProcessorCount:  4,
				BootFilesRootPath: validBootFilesPath,
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if doc.VirtualMachine.ComputeTopology.Processor.Count != 4 {
					t.Errorf("expected processor count 4, got %v", doc.VirtualMachine.ComputeTopology.Processor.Count)
				}
			},
		},
		{
			name: "CPU configuration from annotations",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.ProcessorCount:  fmt.Sprintf("%d", hostCount),
					shimannotations.ProcessorLimit:  "50000",
					shimannotations.ProcessorWeight: "500",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				proc := doc.VirtualMachine.ComputeTopology.Processor
				if proc.Count != uint32(hostCount) {
					t.Errorf("expected processor count %d, got %v", hostCount, proc.Count)
				}
				if proc.Limit != 50000 {
					t.Errorf("expected processor limit 50000, got %v", proc.Limit)
				}
				if proc.Weight != 500 {
					t.Errorf("expected processor weight 500, got %v", proc.Weight)
				}
			},
		},
		{
			name: "memory configuration from options",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				VmMemorySizeInMb:  2048,
				BootFilesRootPath: validBootFilesPath,
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if doc.VirtualMachine.ComputeTopology.Memory.SizeInMB != 2048 {
					t.Errorf("expected memory size 2048MB, got %v", doc.VirtualMachine.ComputeTopology.Memory.SizeInMB)
				}
			},
		},
		{
			name: "memory configuration from annotations",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.MemorySizeInMB:        "4096",
					shimannotations.AllowOvercommit:       "false",
					shimannotations.EnableDeferredCommit:  "false",
					shimannotations.FullyPhysicallyBacked: "true",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				mem := doc.VirtualMachine.ComputeTopology.Memory
				if mem.SizeInMB != 4096 {
					t.Errorf("expected memory size 4096MB, got %v", mem.SizeInMB)
				}
				if mem.AllowOvercommit != false {
					t.Errorf("expected allow overcommit false, got %v", mem.AllowOvercommit)
				}
				if sandboxOpts.FullyPhysicallyBacked != true {
					t.Errorf("expected fully physically backed true, got %v", sandboxOpts.FullyPhysicallyBacked)
				}
			},
		},
		{
			name: "memory MMIO configuration",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.MemoryLowMMIOGapInMB:   "256",
					shimannotations.MemoryHighMMIOBaseInMB: "1024",
					shimannotations.MemoryHighMMIOGapInMB:  "512",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				mem := doc.VirtualMachine.ComputeTopology.Memory
				if mem.LowMMIOGapInMB != 256 {
					t.Errorf("expected low MMIO gap 256MB, got %v", mem.LowMMIOGapInMB)
				}
				if mem.HighMMIOBaseInMB != 1024 {
					t.Errorf("expected high MMIO base 1024MB, got %v", mem.HighMMIOBaseInMB)
				}
				if mem.HighMMIOGapInMB != 512 {
					t.Errorf("expected high MMIO gap 512MB, got %v", mem.HighMMIOGapInMB)
				}
			},
		},
		{
			name: "storage QoS configuration",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.StorageQoSBandwidthMaximum: "1000000",
					shimannotations.StorageQoSIopsMaximum:      "5000",
					shimannotations.DisableWritableFileShares:  "true",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				qos := doc.VirtualMachine.StorageQoS
				if qos == nil {
					t.Fatal("expected StorageQoS to be configured")
				}
				if qos.BandwidthMaximum != 1000000 {
					t.Errorf("expected storage bandwidth 1000000, got %v", qos.BandwidthMaximum)
				}
				if qos.IopsMaximum != 5000 {
					t.Errorf("expected storage IOPS 5000, got %v", qos.IopsMaximum)
				}
				if sandboxOpts.NoWritableFileShares != true {
					t.Errorf("expected no writable file shares true, got %v", sandboxOpts.NoWritableFileShares)
				}
			},
		},
		{
			name: "boot options with kernel direct",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.KernelDirectBoot:      "true",
					shimannotations.KernelBootOptions:     "console=ttyS0",
					shimannotations.PreferredRootFSType:   "vhd",
					shimannotations.EnableColdDiscardHint: "true",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if doc.VirtualMachine.Chipset.LinuxKernelDirect == nil {
					t.Error("expected kernel direct boot (LinuxKernelDirect to be set)")
				}
				if !strings.Contains(getKernelArgs(doc), "console=ttyS0") {
					t.Errorf("expected kernel cmd line to contain 'console=ttyS0', got %v", getKernelArgs(doc))
				}
				if doc.VirtualMachine.ComputeTopology.Memory.EnableColdDiscardHint != true {
					t.Errorf("expected cold discard hint true, got %v", doc.VirtualMachine.ComputeTopology.Memory.EnableColdDiscardHint)
				}
			},
		},
		{
			name: "boot options with initrd preferred",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.PreferredRootFSType: "initrd",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				chipset := doc.VirtualMachine.Chipset
				// When initrd is preferred, LinuxKernelDirect should have InitRdPath set
				if chipset.LinuxKernelDirect != nil && chipset.LinuxKernelDirect.InitRdPath == "" {
					t.Error("expected InitRdPath to be set for initrd boot")
				}
			},
		},
		{
			name: "invalid preferred rootfs type",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.PreferredRootFSType: "invalid",
				},
			},
			wantErr:     true,
			errContains: "invalid PreferredRootFSType",
		},
		{
			name: "boot files path not found",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: "/nonexistent/path",
			},
			wantErr:     true,
			errContains: "boot_files_root_path",
		},
		{
			name: "guest options",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					iannotations.NetworkingPolicyBasedRouting: "true",
					iannotations.ExtraVSockPorts:              "8000,8001,8002",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if sandboxOpts.PolicyBasedRouting != true {
					t.Errorf("expected policy based routing true, got %v", sandboxOpts.PolicyBasedRouting)
				}
				if doc.VirtualMachine.Devices == nil {
					t.Error("expected Devices to be configured")
				}
			},
		},
		{
			name: "device options with VPMem",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.VPMemCount:          "32",
					shimannotations.VPMemSize:           "8589934592",
					shimannotations.VPMemNoMultiMapping: "true",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				vpmem := doc.VirtualMachine.Devices.VirtualPMem
				if vpmem == nil {
					t.Errorf("expected VirtualPMem to be configured")
					return
				}
				if vpmem.MaximumCount != 32 {
					t.Errorf("expected VPMem count 32, got %v", vpmem.MaximumCount)
				}
				if vpmem.MaximumSizeBytes != 8589934592 {
					t.Errorf("expected VPMem size 8589934592, got %v", vpmem.MaximumSizeBytes)
				}
				// VPMemNoMultiMapping=true means MultiMapping is disabled
				if sandboxOpts.VPMEMMultiMapping != false {
					t.Errorf("expected VPMem multi mapping false (no multi mapping true), got %v", sandboxOpts.VPMEMMultiMapping)
				}
			},
		},
		{
			name: "VPMem count exceeds maximum",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.VPMemCount: "200",
				},
			},
			wantErr:     true,
			errContains: "vp_mem_device_count cannot be greater than",
		},
		{
			name: "VPMem size not aligned to 4096",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.VPMemSize: "12345",
				},
			},
			wantErr:     true,
			errContains: "vp_mem_size_bytes must be a multiple of 4096",
		},
		{
			name: "fully physically backed disables VPMem",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.FullyPhysicallyBacked: "true",
					shimannotations.VPMemCount:            "64",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				vpmem := doc.VirtualMachine.Devices.VirtualPMem
				if vpmem != nil && vpmem.MaximumCount != 0 {
					t.Errorf("expected VPMem count 0 when fully physically backed, got %v", vpmem.MaximumCount)
				}
			},
		},
		{
			name: "assigned devices - VPCI",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Devices: []specs.WindowsDevice{
					{
						ID:     `PCIP\VEN_1234&DEV_5678`,
						IDType: "vpci-instance-id",
					},
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				vpci := doc.VirtualMachine.Devices.VirtualPci
				if len(vpci) != 1 {
					t.Errorf("expected 1 assigned device, got %d", len(vpci))
					return
				}
				for _, dev := range vpci {
					if len(dev.Functions) == 0 {
						t.Error("expected at least one function in VirtualPciDevice")
						return
					}
					if dev.Functions[0].DeviceInstancePath != `PCIP\VEN_1234&DEV_5678` {
						t.Errorf("unexpected device instance path: %s", dev.Functions[0].DeviceInstancePath)
					}
				}
			},
		},
		{
			name: "assigned devices - GPU",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Devices: []specs.WindowsDevice{
					{
						ID:     "GPU-12345678-1234-5678-1234-567812345678",
						IDType: "gpu",
					},
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if len(doc.VirtualMachine.Devices.VirtualPci) != 1 {
					t.Errorf("expected 1 assigned device, got %d", len(doc.VirtualMachine.Devices.VirtualPci))
				}
			},
		},
		{
			name: "assigned devices with virtual function index",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Devices: []specs.WindowsDevice{
					{
						ID:     `PCIP\VEN_1234&DEV_5678/2`,
						IDType: "vpci-instance-id",
					},
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				vpci := doc.VirtualMachine.Devices.VirtualPci
				if len(vpci) != 1 {
					t.Errorf("expected 1 assigned device, got %d", len(vpci))
					return
				}
				for _, dev := range vpci {
					if len(dev.Functions) == 0 {
						t.Error("expected at least one function")
						return
					}
					if dev.Functions[0].VirtualFunction != 2 {
						t.Errorf("expected virtual function index 2, got %d", dev.Functions[0].VirtualFunction)
					}
				}
			},
		},
		{
			name: "confidential options with security policy (no hardware bypass)",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.LCOWSecurityPolicy:         "eyJ0ZXN0IjoidGVzdCJ9", // valid base64: {"test":"test"}
					shimannotations.LCOWSecurityPolicyEnforcer: "rego",
					shimannotations.LCOWEncryptedScratchDisk:   "true",
					// Note: NoSecurityHardware NOT set, so it defaults to false, meaning SNP mode
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if sandboxOpts.ConfidentialConfig == nil {
					t.Fatal("expected ConfidentialConfig to be set")
				}
				if sandboxOpts.ConfidentialConfig.SecurityPolicy != "eyJ0ZXN0IjoidGVzdCJ9" {
					t.Errorf("expected security policy, got %v", sandboxOpts.ConfidentialConfig.SecurityPolicy)
				}
				if sandboxOpts.ConfidentialConfig.SecurityPolicyEnforcer != "rego" {
					t.Errorf("expected security policy enforcer 'rego', got %v", sandboxOpts.ConfidentialConfig.SecurityPolicyEnforcer)
				}
				if sandboxOpts.EnableScratchEncryption != true {
					t.Errorf("expected scratch encryption true, got %v", sandboxOpts.EnableScratchEncryption)
				}
				// GuestState should be set in confidential mode
				if doc.VirtualMachine.GuestState == nil || doc.VirtualMachine.GuestState.GuestStateFilePath == "" {
					t.Error("expected GuestState file path to be set in confidential mode")
				}
				// DM-Verity rootfs should be attached via SCSI
				if len(doc.VirtualMachine.Devices.Scsi) == 0 {
					t.Error("expected SCSI controllers to be configured in confidential mode")
				}
			},
		},
		{
			name: "confidential SNP mode configuration",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.LCOWSecurityPolicy: "eyJzbmAiOiJ0ZXN0In0=", // valid base64: {"snp":"test"}
					shimannotations.NoSecurityHardware: "false",
					shimannotations.VPMemCount:         "64",
					shimannotations.DmVerityCreateArgs: "test-verity-args", // Required for SNP mode VHD boot
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				// In SNP mode, VPMem should be disabled
				vpmem := doc.VirtualMachine.Devices.VirtualPMem
				if vpmem != nil && vpmem.MaximumCount != 0 {
					t.Errorf("expected VPMem count 0 in SNP mode, got %v", vpmem.MaximumCount)
				}
				// Memory should not allow overcommit
				if doc.VirtualMachine.ComputeTopology.Memory.AllowOvercommit != false {
					t.Errorf("expected allow overcommit false in SNP mode, got %v", doc.VirtualMachine.ComputeTopology.Memory.AllowOvercommit)
				}
				// GuestState file path should be set
				if doc.VirtualMachine.GuestState == nil || doc.VirtualMachine.GuestState.GuestStateFilePath == "" {
					t.Error("expected GuestState file path to be set in SNP mode")
				}
				// DM-Verity rootfs VHD should be attached to SCSI
				if len(doc.VirtualMachine.Devices.Scsi) == 0 {
					t.Error("expected SCSI controllers to be configured in SNP mode")
				}
			},
		},
		{
			name: "confidential options with custom files",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.LCOWSecurityPolicy:    "eyJ0ZXN0IjoidGVzdCJ9", // Must have policy to enable confidential mode
					shimannotations.LCOWGuestStateFile:    "custom.vmgs",
					shimannotations.DmVerityRootFsVhd:     "custom-rootfs.vhd",
					shimannotations.LCOWReferenceInfoFile: "custom-ref.cose",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if sandboxOpts.ConfidentialConfig == nil {
					t.Fatal("expected ConfidentialConfig to be set")
				}
				if doc.VirtualMachine.GuestState == nil {
					t.Fatal("expected GuestState to be set")
				}
				if !strings.Contains(doc.VirtualMachine.GuestState.GuestStateFilePath, "custom.vmgs") {
					t.Errorf("expected GuestState path to contain 'custom.vmgs', got %q", doc.VirtualMachine.GuestState.GuestStateFilePath)
				}
				if sandboxOpts.ConfidentialConfig.UvmReferenceInfoFile != "custom-ref.cose" {
					t.Errorf("expected custom reference info file, got %v", sandboxOpts.ConfidentialConfig.UvmReferenceInfoFile)
				}
				// DM-Verity custom VHD is copied to bundlePath/DefaultDmVerityRootfsVhd ("rootfs.vhd")
				// and that copy is attached via SCSI. Verify a SCSI attachment exists in bundlePath.
				found := false
				for _, ctrl := range doc.VirtualMachine.Devices.Scsi {
					for _, att := range ctrl.Attachments {
						if strings.Contains(att.Path, vmutils.DefaultDmVerityRootfsVhd) {
							found = true
						}
					}
				}
				if !found {
					t.Errorf("expected dm-verity rootfs VHD (%s) to be attached via SCSI", vmutils.DefaultDmVerityRootfsVhd)
				}
			},
		},
		{
			name: "additional config - console pipe",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					iannotations.UVMConsolePipe: `\\.\pipe\console`,
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				comPorts := doc.VirtualMachine.Devices.ComPorts
				if comPorts == nil {
					t.Fatal("expected ComPorts to be configured")
				}
				com1, ok := comPorts["0"]
				if !ok {
					t.Fatal("expected COM1 (port 0) to be configured")
				}
				if com1.NamedPipe != `\\.\pipe\console` {
					t.Errorf("expected console pipe, got %v", com1.NamedPipe)
				}
			},
		},
		{
			name: "CPU group ID",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.CPUGroupID: "12345678-1234-5678-1234-567812345678",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				proc := doc.VirtualMachine.ComputeTopology.Processor
				if proc.CpuGroup == nil || proc.CpuGroup.Id != "12345678-1234-5678-1234-567812345678" {
					t.Errorf("expected CPU group ID, got %v", proc.CpuGroup)
				}
			},
		},
		{
			name: "valid resource partition ID does not cause error",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.ResourcePartitionID: "87654321-4321-8765-4321-876543218765",
				},
			},
			// A valid GUID should be accepted without error.
		},
		{
			name: "CPU group and resource partition conflict",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.CPUGroupID:          "12345678-1234-5678-1234-567812345678",
					shimannotations.ResourcePartitionID: "87654321-4321-8765-4321-876543218765",
				},
			},
			wantErr:     true,
			errContains: "cpu_group_id and resource_partition_id cannot be set at the same time",
		},
		{
			name: "invalid resource partition GUID",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.ResourcePartitionID: "not-a-guid",
				},
			},
			wantErr:     true,
			errContains: "failed to parse resource_partition_id",
		},
		{
			name: "default container annotations applied",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
				DefaultContainerAnnotations: map[string]string{
					shimannotations.ProcessorCount: "4",
					shimannotations.MemorySizeInMB: "2048",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if doc.VirtualMachine.ComputeTopology.Processor.Count != 4 {
					t.Errorf("expected processor count 4 from defaults, got %v", doc.VirtualMachine.ComputeTopology.Processor.Count)
				}
				if doc.VirtualMachine.ComputeTopology.Memory.SizeInMB != 2048 {
					t.Errorf("expected memory 2048MB from defaults, got %v", doc.VirtualMachine.ComputeTopology.Memory.SizeInMB)
				}
			},
		},
		{
			name: "annotations override default container annotations",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
				DefaultContainerAnnotations: map[string]string{
					shimannotations.ProcessorCount: "2",
				},
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.ProcessorCount: fmt.Sprintf("%d", hostCount),
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if doc.VirtualMachine.ComputeTopology.Processor.Count != uint32(hostCount) {
					t.Errorf("expected processor count %d (annotation overrides default), got %v", hostCount, doc.VirtualMachine.ComputeTopology.Processor.Count)
				}
			},
		},
		{
			name: "comprehensive configuration",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				VmProcessorCount:  4,
				VmMemorySizeInMb:  4096,
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.ProcessorLimit:               "80000",
					shimannotations.ProcessorWeight:              "500",
					shimannotations.AllowOvercommit:              "false",
					shimannotations.FullyPhysicallyBacked:        "true",
					shimannotations.StorageQoSBandwidthMaximum:   "500000",
					shimannotations.StorageQoSIopsMaximum:        "3000",
					shimannotations.DisableWritableFileShares:    "true",
					shimannotations.KernelDirectBoot:             "true",
					shimannotations.KernelBootOptions:            "console=ttyS0 loglevel=7",
					shimannotations.PreferredRootFSType:          "vhd",
					shimannotations.DisableLCOWTimeSyncService:   "false",
					iannotations.ExtraVSockPorts:                 "9000,9001",
					shimannotations.VPCIEnabled:                  "true",
					shimannotations.ContainerProcessDumpLocation: "/var/dumps",
					shimannotations.DumpDirectoryPath:            "C:\\UVMDumps",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if doc.VirtualMachine.ComputeTopology.Processor.Count != 4 {
					t.Errorf("expected processor count 4, got %v", doc.VirtualMachine.ComputeTopology.Processor.Count)
				}
				if doc.VirtualMachine.ComputeTopology.Memory.SizeInMB != 4096 {
					t.Errorf("expected memory 4096MB, got %v", doc.VirtualMachine.ComputeTopology.Memory.SizeInMB)
				}
				if sandboxOpts.NoWritableFileShares != true {
					t.Error("expected no writable file shares")
				}
				if doc.VirtualMachine.Chipset.LinuxKernelDirect == nil {
					t.Error("expected kernel direct boot (LinuxKernelDirect to be set)")
				}
				// VPMem should be 0 due to fully physically backed
				vpmem := doc.VirtualMachine.Devices.VirtualPMem
				if vpmem != nil && vpmem.MaximumCount != 0 {
					t.Errorf("expected VPMem count 0 (fully physically backed), got %v", vpmem.MaximumCount)
				}
			},
		},
	}

	runTestCases(t, ctx, nil, tests)
}

// TestBuildSandboxConfig_EdgeCases tests edge cases and boundary conditions
func TestBuildSandboxConfig_EdgeCases(t *testing.T) {
	ctx := context.Background()

	validBootFilesPath := newBootFilesPath(t)

	tests := []specTestCase{
		{
			name: "zero processor count falls back to default",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				VmProcessorCount:  0,
				BootFilesRootPath: validBootFilesPath,
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				count := doc.VirtualMachine.ComputeTopology.Processor.Count
				if count <= 0 {
					t.Errorf("expected positive default processor count, got %v", count)
				}
			},
		},
		{
			name: "negative processor count annotation falls back to default",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.ProcessorCount: "-1",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				count := doc.VirtualMachine.ComputeTopology.Processor.Count
				if count <= 0 {
					t.Errorf("expected positive default processor count, got %v", count)
				}
			},
		},
		{
			name: "zero memory size falls back to default 1024",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				VmMemorySizeInMb:  0,
				BootFilesRootPath: validBootFilesPath,
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if doc.VirtualMachine.ComputeTopology.Memory.SizeInMB != 1024 {
					t.Errorf("expected default memory 1024MB, got %v", doc.VirtualMachine.ComputeTopology.Memory.SizeInMB)
				}
			},
		},
		{
			name: "VPMem size exactly 4096",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.VPMemSize: "4096",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				vpmem := doc.VirtualMachine.Devices.VirtualPMem
				if vpmem == nil {
					t.Error("expected VirtualPMem to be configured")
					return
				}
				if vpmem.MaximumSizeBytes != 4096 {
					t.Errorf("expected VPMem size 4096, got %v", vpmem.MaximumSizeBytes)
				}
			},
		},
		{
			name: "VPMem count at maximum boundary",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.VPMemCount: "128",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				vpmem := doc.VirtualMachine.Devices.VirtualPMem
				if vpmem == nil {
					t.Error("expected VirtualPMem to be configured")
					return
				}
				if vpmem.MaximumCount != 128 {
					t.Errorf("expected VPMem count 128, got %v", vpmem.MaximumCount)
				}
			},
		},
		{
			name: "processor limit at maximum",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.ProcessorLimit: "100000",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if doc.VirtualMachine.ComputeTopology.Processor.Limit != 100000 {
					t.Errorf("expected processor limit 100000, got %v", doc.VirtualMachine.ComputeTopology.Processor.Limit)
				}
			},
		},
		{
			name: "processor weight at maximum",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.ProcessorWeight: "10000",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if doc.VirtualMachine.ComputeTopology.Processor.Weight != 10000 {
					t.Errorf("expected processor weight 10000, got %v", doc.VirtualMachine.ComputeTopology.Processor.Weight)
				}
			},
		},
		{
			name: "boot files path annotation overrides options",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: "/nonexistent/path",
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.BootFilesRootPath: validBootFilesPath,
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if doc.VirtualMachine.Chipset == nil {
					t.Error("expected boot options (Chipset) to be set")
				}
			},
		},
		{
			name: "empty annotations map with defaults",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
				DefaultContainerAnnotations: map[string]string{
					shimannotations.ProcessorCount: "4",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if doc.VirtualMachine.ComputeTopology.Processor.Count != 4 {
					t.Errorf("expected processor count 4 from defaults, got %v", doc.VirtualMachine.ComputeTopology.Processor.Count)
				}
			},
		},
	}

	runTestCases(t, ctx, nil, tests)
}

// TestBuildSandboxConfig_SecurityPolicyInteractions tests complex interactions with security policies
func TestBuildSandboxConfig_SecurityPolicyInteractions(t *testing.T) {
	ctx := context.Background()

	validBootFilesPath := newBootFilesPath(t)
	defaultOpts := defaultSandboxOpts(validBootFilesPath)

	tests := []specTestCase{
		{
			name: "security policy without hardware forces SNP mode",
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.LCOWSecurityPolicy: "eyJ0ZXN0IjoidGVzdCJ9",
					shimannotations.NoSecurityHardware: "false",
					shimannotations.VPMemCount:         "64",
					shimannotations.AllowOvercommit:    "true",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				// VPMem disabled in SNP
				vpmem := doc.VirtualMachine.Devices.VirtualPMem
				if vpmem != nil && vpmem.MaximumCount != 0 {
					t.Error("expected VPMem disabled in SNP mode")
				}
				// Overcommit disabled in SNP
				if doc.VirtualMachine.ComputeTopology.Memory.AllowOvercommit != false {
					t.Error("expected overcommit disabled in SNP mode")
				}
				// GuestState path should be set (contains the VMGS file)
				if doc.VirtualMachine.GuestState == nil || doc.VirtualMachine.GuestState.GuestStateFilePath == "" {
					t.Error("expected GuestState file path to be set in SNP mode")
				}
				// SCSI should be configured for dm-verity rootfs
				if len(doc.VirtualMachine.Devices.Scsi) == 0 {
					t.Error("expected SCSI controllers to be set in SNP mode")
				}
			},
		},
		{
			name: "security policy with hardware bypass",
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.LCOWSecurityPolicy: "eyJ0ZXN0IjoidGVzdCJ9",
					shimannotations.NoSecurityHardware: "true",
					shimannotations.VPMemCount:         "64",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				// VPMem NOT disabled when no security hardware
				vpmem := doc.VirtualMachine.Devices.VirtualPMem
				if vpmem == nil || vpmem.MaximumCount == 0 {
					t.Error("expected VPMem NOT disabled when no security hardware")
				}
			},
		},
		{
			name: "scratch encryption defaults to false when security hardware is bypassed",
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.LCOWSecurityPolicy: "eyJ0ZXN0IjoidGVzdCJ9",
					shimannotations.NoSecurityHardware: "true",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				// When NoSecurityHardware is true, isConfidential is false,
				// so EnableScratchEncryption defaults to false
				if sandboxOpts.EnableScratchEncryption != false {
					t.Error("expected scratch encryption disabled by default when security hardware is bypassed")
				}
			},
		},
		{
			name: "scratch encryption can be disabled explicitly",
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.LCOWSecurityPolicy:       "policy",
					shimannotations.LCOWEncryptedScratchDisk: "false",
					shimannotations.NoSecurityHardware:       "true", // Bypass SNP mode for this test
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if sandboxOpts.EnableScratchEncryption != false {
					t.Error("expected scratch encryption disabled when explicitly set")
				}
			},
		},
		{
			name: "scratch encryption defaults to false without security policy",
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if sandboxOpts.EnableScratchEncryption != false {
					t.Error("expected scratch encryption disabled without security policy")
				}
			},
		},
	}

	runTestCases(t, ctx, defaultOpts, tests)
}

func newBootFilesPath(t *testing.T) string {
	t.Helper()

	tempBootDir := t.TempDir()
	validBootFilesPath := filepath.Join(tempBootDir, "bootfiles")
	if err := os.MkdirAll(validBootFilesPath, 0755); err != nil {
		t.Fatalf("failed to create temp boot files dir: %v", err)
	}

	// Create the required boot files (kernel and rootfs)
	// Also create vmgs and dm-verity files for confidential tests
	for _, filename := range []string{
		vmutils.KernelFile,
		vmutils.UncompressedKernelFile,
		vmutils.InitrdFile,
		vmutils.VhdFile,
		vmutils.DefaultGuestStateFile,    // for confidential tests
		vmutils.DefaultDmVerityRootfsVhd, // for confidential tests
		"custom.vmgs",                    // for custom files test
		"custom-rootfs.vhd",              // for custom files test
	} {
		filePath := filepath.Join(validBootFilesPath, filename)
		if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create test boot file %s: %v", filename, err)
		}
	}

	return validBootFilesPath
}

func defaultSandboxOpts(bootFilesPath string) *runhcsoptions.Options {
	return &runhcsoptions.Options{
		SandboxPlatform:   "linux/amd64",
		BootFilesRootPath: bootFilesPath,
	}
}

// TestBuildSandboxConfig_ErrorPaths tests error handling paths
func TestBuildSandboxConfig_ErrorPaths(t *testing.T) {
	ctx := context.Background()

	validBootFilesPath := newBootFilesPath(t)
	defaultOpts := defaultSandboxOpts(validBootFilesPath)

	// Create a boot files path with missing kernel file for error testing
	missingKernelPath := filepath.Join(t.TempDir(), "missing_kernel")
	if err := os.MkdirAll(missingKernelPath, 0755); err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	// Only create initrd, no kernel
	if err := os.WriteFile(filepath.Join(missingKernelPath, vmutils.InitrdFile), []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create initrd: %v", err)
	}

	// Create a boot files path with missing initrd for error testing
	missingInitrdPath := filepath.Join(t.TempDir(), "missing_initrd")
	if err := os.MkdirAll(missingInitrdPath, 0755); err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	// Only create kernel, no initrd
	if err := os.WriteFile(filepath.Join(missingInitrdPath, vmutils.KernelFile), []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create kernel: %v", err)
	}

	// Create a boot files path for confidential VM error testing (missing VMGS)
	missingVMGSPath := filepath.Join(t.TempDir(), "missing_vmgs")
	if err := os.MkdirAll(missingVMGSPath, 0755); err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Create a boot files path for confidential VM error testing (missing dm-verity VHD)
	missingDmVerityPath := filepath.Join(t.TempDir(), "missing_dmverity")
	if err := os.MkdirAll(missingDmVerityPath, 0755); err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	// Create VMGS but not dm-verity VHD
	if err := os.WriteFile(filepath.Join(missingDmVerityPath, vmutils.DefaultGuestStateFile), []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create vmgs: %v", err)
	}

	tests := []specTestCase{
		{
			name: "processAnnotations error - unsupported NetworkConfigProxy annotation",
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.NetworkConfigProxy: "some-proxy",
				},
			},
			wantErr:     true,
			errContains: "annotation is not supported",
		},
		{
			name: "kernel file not found in boot files path",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: missingKernelPath,
			},
			wantErr:     true,
			errContains: "kernel file",
		},
		{
			name: "initrd file not found when preferred",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: missingInitrdPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.PreferredRootFSType: "initrd",
				},
			},
			wantErr:     true,
			errContains: "not found in boot files path",
		},
		{
			name: "deferred commit not supported with physical backing",
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.EnableDeferredCommit:  "true",
					shimannotations.FullyPhysicallyBacked: "true",
				},
			},
			wantErr:     true,
			errContains: "enable_deferred_commit is not supported on physically backed vms",
		},
		{
			name: "confidential VM missing guest state file",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: missingVMGSPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.LCOWSecurityPolicy: "eyJ0ZXN0IjoidGVzdCJ9",
				},
			},
			wantErr:     true,
			errContains: "GuestState vmgs file",
		},
		{
			name: "confidential VM missing dm-verity VHD",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: missingDmVerityPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.LCOWSecurityPolicy: "eyJ0ZXN0IjoidGVzdCJ9",
				},
			},
			wantErr:     true,
			errContains: "DM Verity VHD file",
		},
		{
			name: "duplicate assigned devices",
			spec: &vm.Spec{
				Devices: []specs.WindowsDevice{
					{
						ID:     `PCIP\VEN_1234&DEV_5678`,
						IDType: "vpci-instance-id",
					},
					{
						ID:     `PCIP\VEN_1234&DEV_5678`,
						IDType: "vpci-instance-id",
					},
				},
			},
			wantErr:     true,
			errContains: "specified multiple times",
		},
		{
			name: "duplicate assigned devices with same function index",
			spec: &vm.Spec{
				Devices: []specs.WindowsDevice{
					{
						ID:     `PCIP\VEN_1234&DEV_5678/1`,
						IDType: "vpci-instance-id",
					},
					{
						ID:     `PCIP\VEN_1234&DEV_5678/1`,
						IDType: "vpci-instance-id",
					},
				},
			},
			wantErr:     true,
			errContains: "specified multiple times",
		},
		{
			name: "invalid console pipe path",
			spec: &vm.Spec{
				Annotations: map[string]string{
					iannotations.UVMConsolePipe: "/invalid/path",
				},
			},
			wantErr:     true,
			errContains: "listener for serial console is not a named pipe",
		},
	}

	runTestCases(t, ctx, defaultOpts, tests)
}

// TestBuildSandboxConfig_BootOptions tests various boot option scenarios
func TestBuildSandboxConfig_BootOptions(t *testing.T) {
	ctx := context.Background()

	tempDir := t.TempDir()

	// Configuration 1: Only VHD, no initrd
	vhdOnlyPath := filepath.Join(tempDir, "vhd_only")
	if err := os.MkdirAll(vhdOnlyPath, 0755); err != nil {
		t.Fatalf("failed to create vhd only dir: %v", err)
	}
	for _, f := range []string{vmutils.KernelFile, vmutils.UncompressedKernelFile, vmutils.VhdFile} {
		if err := os.WriteFile(filepath.Join(vhdOnlyPath, f), []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create file %s: %v", f, err)
		}
	}

	// Configuration 2: Only initrd, no VHD
	initrdOnlyPath := filepath.Join(tempDir, "initrd_only")
	if err := os.MkdirAll(initrdOnlyPath, 0755); err != nil {
		t.Fatalf("failed to create initrd only dir: %v", err)
	}
	for _, f := range []string{vmutils.KernelFile, vmutils.InitrdFile} {
		if err := os.WriteFile(filepath.Join(initrdOnlyPath, f), []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create file %s: %v", f, err)
		}
	}

	// Configuration 3: Only uncompressed kernel for kernel direct
	uncompressedOnlyPath := filepath.Join(tempDir, "uncompressed_only")
	if err := os.MkdirAll(uncompressedOnlyPath, 0755); err != nil {
		t.Fatalf("failed to create uncompressed only dir: %v", err)
	}
	for _, f := range []string{vmutils.UncompressedKernelFile, vmutils.InitrdFile, vmutils.VhdFile} {
		if err := os.WriteFile(filepath.Join(uncompressedOnlyPath, f), []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create file %s: %v", f, err)
		}
	}

	// Configuration 4: No kernel direct support (only kernel, no vmlinux)
	noKernelDirectPath := filepath.Join(tempDir, "no_kernel_direct")
	if err := os.MkdirAll(noKernelDirectPath, 0755); err != nil {
		t.Fatalf("failed to create no kernel direct dir: %v", err)
	}
	for _, f := range []string{vmutils.KernelFile, vmutils.InitrdFile} {
		if err := os.WriteFile(filepath.Join(noKernelDirectPath, f), []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create file %s: %v", f, err)
		}
	}

	tests := []specTestCase{
		{
			name: "boot with VHD only (no initrd)",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: vhdOnlyPath,
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				chipset := doc.VirtualMachine.Chipset
				if chipset.LinuxKernelDirect != nil && chipset.LinuxKernelDirect.InitRdPath != "" {
					t.Error("expected InitRdPath to be empty when VHD is default")
				}
			},
		},
		{
			name: "boot with initrd only (no VHD)",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: initrdOnlyPath,
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				chipset := doc.VirtualMachine.Chipset
				if chipset.LinuxKernelDirect != nil && chipset.LinuxKernelDirect.InitRdPath == "" {
					t.Error("expected InitRdPath to be set when only initrd is present")
				}
			},
		},
		{
			name: "kernel direct with only uncompressed kernel",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: uncompressedOnlyPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.KernelDirectBoot: "true",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				chipset := doc.VirtualMachine.Chipset
				if chipset.LinuxKernelDirect == nil {
					t.Fatal("expected LinuxKernelDirect to be set")
				}
				if !strings.Contains(chipset.LinuxKernelDirect.KernelFilePath, vmutils.UncompressedKernelFile) {
					t.Errorf("expected kernel path to contain %s, got %s", vmutils.UncompressedKernelFile, chipset.LinuxKernelDirect.KernelFilePath)
				}
			},
		},
		{
			name: "kernel direct with regular kernel (no uncompressed)",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: noKernelDirectPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.KernelDirectBoot: "true",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				chipset := doc.VirtualMachine.Chipset
				if chipset.LinuxKernelDirect == nil {
					t.Fatal("expected LinuxKernelDirect to be set")
				}
				if !strings.Contains(chipset.LinuxKernelDirect.KernelFilePath, vmutils.KernelFile) {
					t.Errorf("expected kernel path to contain %s", vmutils.KernelFile)
				}
			},
		},
		{
			name: "UEFI boot mode (kernel direct disabled)",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: initrdOnlyPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.KernelDirectBoot: "false",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				chipset := doc.VirtualMachine.Chipset
				if chipset.Uefi == nil {
					t.Error("expected UEFI boot mode")
				}
				if chipset.LinuxKernelDirect != nil {
					t.Error("expected LinuxKernelDirect to be nil for UEFI boot")
				}
			},
		},
		{
			name: "dm-verity mode with SCSI boot",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: vhdOnlyPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.DmVerityMode:       "true",
					shimannotations.DmVerityCreateArgs: "test-dm-verity-args",
					shimannotations.VPMemCount:         "0",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if !strings.Contains(getKernelArgs(doc), "dm-mod.create") {
					t.Error("expected dm-verity configuration in kernel args")
				}
			},
		},
		{
			name: "SCSI boot from VHD without dm-verity",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: vhdOnlyPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.VPMemCount: "0",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if len(doc.VirtualMachine.Devices.Scsi) == 0 {
					t.Error("expected SCSI controllers to be configured")
				}
			},
		},
		{
			name: "VPMem boot from VHD",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: vhdOnlyPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.VPMemCount: "32",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				vpmem := doc.VirtualMachine.Devices.VirtualPMem
				if vpmem == nil {
					t.Fatal("expected VirtualPMem to be configured")
				}
				if len(vpmem.Devices) == 0 {
					t.Error("expected VPMem device to be configured for rootfs")
				}
			},
		},
		{
			name: "writable overlay dirs with VHD",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: vhdOnlyPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					iannotations.WritableOverlayDirs: "true",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if !strings.Contains(getKernelArgs(doc), "-w") {
					t.Error("expected -w flag in kernel args for writable overlay dirs")
				}
			},
		},
		{
			name: "disable time sync service",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: vhdOnlyPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.DisableLCOWTimeSyncService: "true",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if !strings.Contains(getKernelArgs(doc), "-disable-time-sync") {
					t.Error("expected -disable-time-sync flag in kernel args")
				}
			},
		},
		{
			name: "process dump location",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: vhdOnlyPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.ContainerProcessDumpLocation: "/tmp/dumps",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				kernelArgs := getKernelArgs(doc)
				if !strings.Contains(kernelArgs, "-core-dump-location") || !strings.Contains(kernelArgs, "/tmp/dumps") {
					t.Error("expected -core-dump-location in kernel args")
				}
			},
		},
		{
			name: "VPCIEnabled annotation enables PCI",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: vhdOnlyPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.VPCIEnabled: "true",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if strings.Contains(getKernelArgs(doc), "pci=off") {
					t.Error("expected PCI to be enabled, but found pci=off in kernel args")
				}
			},
		},
		{
			name: "VPCI Enabled defaults to false (PCI disabled)",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: vhdOnlyPath,
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if !strings.Contains(getKernelArgs(doc), "pci=off") {
					t.Error("expected pci=off in kernel args when VPCIEnabled is false")
				}
			},
		},
		{
			name: "gcs log level from options",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: vhdOnlyPath,
				LogLevel:          "debug",
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if !strings.Contains(getKernelArgs(doc), "-loglevel debug") {
					t.Error("expected -loglevel debug in kernel args")
				}
			},
		},
		{
			name: "scrub logs option",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: vhdOnlyPath,
				ScrubLogs:         true,
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if !strings.Contains(getKernelArgs(doc), "-scrub-logs") {
					t.Error("expected -scrub-logs in kernel args")
				}
			},
		},
	}

	runTestCases(t, ctx, nil, tests)
}

// TestBuildSandboxConfig_DeviceOptions tests various device option scenarios
func TestBuildSandboxConfig_DeviceOptions(t *testing.T) {
	ctx := context.Background()

	validBootFilesPath := newBootFilesPath(t)
	defaultOpts := defaultSandboxOpts(validBootFilesPath)

	tests := []specTestCase{
		{
			name: "assigned devices with unsupported type",
			spec: &vm.Spec{
				Devices: []specs.WindowsDevice{
					{
						ID:     "some-device-id",
						IDType: "unsupported-type",
					},
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if len(doc.VirtualMachine.Devices.VirtualPci) != 0 {
					t.Errorf("expected 0 vpci devices (unsupported type should be skipped), got %d", len(doc.VirtualMachine.Devices.VirtualPci))
				}
			},
		},
		{
			name: "assigned devices - legacy vpci type",
			spec: &vm.Spec{
				Devices: []specs.WindowsDevice{
					{
						ID:     `PCIP\VEN_ABCD&DEV_1234`,
						IDType: "vpci",
					},
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if len(doc.VirtualMachine.Devices.VirtualPci) != 1 {
					t.Errorf("expected 1 assigned device, got %d", len(doc.VirtualMachine.Devices.VirtualPci))
				}
			},
		},
		{
			name: "confidential VM disables vPCI devices",
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.LCOWSecurityPolicy: "eyJ0ZXN0IjoidGVzdCJ9",
				},
				Devices: []specs.WindowsDevice{
					{
						ID:     `PCIP\VEN_1234&DEV_5678`,
						IDType: "vpci-instance-id",
					},
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				if len(doc.VirtualMachine.Devices.VirtualPci) != 0 {
					t.Error("expected vPCI devices to be disabled in confidential VMs")
				}
			},
		},
	}

	runTestCases(t, ctx, defaultOpts, tests)
}

// TestBuildSandboxConfig_HvSocketServiceTable tests HvSocket service table parsing
func TestBuildSandboxConfig_HvSocketServiceTable(t *testing.T) {
	ctx := context.Background()

	validBootFilesPath := newBootFilesPath(t)
	defaultOpts := defaultSandboxOpts(validBootFilesPath)

	tests := []specTestCase{
		{
			name: "HvSocket service table parsing from annotations",
			spec: &vm.Spec{
				Annotations: map[string]string{
					iannotations.UVMHyperVSocketConfigPrefix + "12345678-1234-1234-1234-123456789abc": `{"BindSecurityDescriptor":"D:P(A;;FA;;;WD)","ConnectSecurityDescriptor":"D:P(A;;FA;;;WD)","AllowWildcardBinds":true}`,
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				hvSocket := doc.VirtualMachine.Devices.HvSocket
				if hvSocket == nil || hvSocket.HvSocketConfig == nil {
					t.Fatal("expected HvSocket config to be set")
				}
				serviceTable := hvSocket.HvSocketConfig.ServiceTable
				if len(serviceTable) == 0 {
					t.Fatal("expected HvSocket service table to be populated")
				}
				found := false
				for guid := range serviceTable {
					if strings.Contains(strings.ToLower(guid), "12345678-1234-1234-1234-123456789abc") {
						found = true
						break
					}
				}
				if !found {
					t.Error("expected HvSocket service GUID to be in service table")
				}
			},
		},
		{
			name: "confidential VM adds extra vsock ports",
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.LCOWSecurityPolicy: "eyJ0ZXN0IjoidGVzdCJ9",
					iannotations.ExtraVSockPorts:       "8000,8001,8002",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				hvSocket := doc.VirtualMachine.Devices.HvSocket
				if hvSocket == nil || hvSocket.HvSocketConfig == nil {
					t.Fatal("expected HvSocket config to be set")
				}
				serviceTable := hvSocket.HvSocketConfig.ServiceTable
				// Should contain at least the default ports plus extra ports
				minExpectedPorts := 4 + 3 // 4 default (entropy, log, gcs, bridge) + 3 extra
				if len(serviceTable) < minExpectedPorts {
					t.Errorf("expected at least %d vsock ports, got %d", minExpectedPorts, len(serviceTable))
				}
			},
		},
		{
			name: "confidential VM HvSocket default ports",
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.LCOWSecurityPolicy: "eyJ0ZXN0IjoidGVzdCJ9",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				hvSocket := doc.VirtualMachine.Devices.HvSocket
				if hvSocket == nil || hvSocket.HvSocketConfig == nil {
					t.Fatal("expected HvSocket config to be set")
				}
				serviceTable := hvSocket.HvSocketConfig.ServiceTable
				// Should contain default ports: entropy (1), log (109), gcs (0x40000000), bridge (0x40000001)
				if len(serviceTable) < 4 {
					t.Errorf("expected at least 4 default vsock ports, got %d", len(serviceTable))
				}
			},
		},
		{
			name: "confidential VM with HclEnabled annotation",
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.LCOWSecurityPolicy: "eyJ0ZXN0IjoidGVzdCJ9",
					shimannotations.LCOWHclEnabled:     "true",
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				secSettings := doc.VirtualMachine.SecuritySettings
				if secSettings == nil {
					t.Fatal("expected SecuritySettings to be set")
				}
				if secSettings.Isolation == nil {
					t.Fatal("expected Isolation settings to be set")
				}
				if secSettings.Isolation.HclEnabled == nil {
					t.Fatal("expected HclEnabled to be set")
				}
				if !*secSettings.Isolation.HclEnabled {
					t.Error("expected HclEnabled to be true")
				}
				// GuestState path should be set in confidential mode
				if doc.VirtualMachine.GuestState == nil || doc.VirtualMachine.GuestState.GuestStateFilePath == "" {
					t.Error("expected GuestState file path to be set in confidential mode")
				}
			},
		},
	}

	runTestCases(t, ctx, defaultOpts, tests)
}

// TestBuildSandboxConfig_NUMA tests NUMA configuration scenarios.
// These tests require Windows build >= V25H1Server which supports vNUMA topology.
func TestBuildSandboxConfig_NUMA(t *testing.T) {
	if osversion.Build() < osversion.V25H1Server {
		t.Skipf("Skipping vNUMA tests on Windows build %d (requires %d or later)", osversion.Build(), osversion.V25H1Server)
	}

	ctx := context.Background()

	validBootFilesPath := newBootFilesPath(t)
	defaultOpts := defaultSandboxOpts(validBootFilesPath)

	tests := []specTestCase{
		{
			name: "NUMA configuration implicit",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.NumaMaximumProcessorsPerNode: "4",
					shimannotations.NumaMaximumMemorySizePerNode: "2048",
					shimannotations.NumaPreferredPhysicalNodes:   "0,1,2",
					shimannotations.FullyPhysicallyBacked:        "true", // Required for NUMA
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				proc := doc.VirtualMachine.ComputeTopology.Processor
				if proc.NumaProcessorsSettings == nil {
					t.Fatal("expected NumaProcessorsSettings to be set for implicit topology")
				}
				if proc.NumaProcessorsSettings.CountPerNode.Max != 4 {
					t.Errorf("expected max processors per NUMA node 4, got %v", proc.NumaProcessorsSettings.CountPerNode.Max)
				}
				numa := doc.VirtualMachine.ComputeTopology.Numa
				if numa == nil {
					t.Fatal("expected Numa to be set for implicit topology")
				}
				if numa.MaxSizePerNode != 2048 {
					t.Errorf("expected max memory per NUMA node 2048MB, got %v", numa.MaxSizePerNode)
				}
				if len(numa.PreferredPhysicalNodes) != 3 {
					t.Errorf("expected 3 preferred physical NUMA nodes, got %d", len(numa.PreferredPhysicalNodes))
				}
			},
		},
		{
			name: "NUMA configuration explicit",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
				VmProcessorCount:  4,           // Must match total in NumaCountOfProcessors (2+2)
				VmMemorySizeInMb:  int32(2048), // Must match total in NumaCountOfMemoryBlocks (1024+1024)
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.NumaMappedPhysicalNodes: "0,1",
					shimannotations.NumaCountOfProcessors:   "2,2",
					shimannotations.NumaCountOfMemoryBlocks: "1024,1024",
					shimannotations.FullyPhysicallyBacked:   "true", // NUMA requires physical memory backing
				},
			},
			validate: func(t *testing.T, doc *hcsschema.ComputeSystem, sandboxOpts *SandboxOptions) {
				t.Helper()
				numa := doc.VirtualMachine.ComputeTopology.Numa
				if numa == nil {
					t.Fatal("expected Numa to be set for explicit topology")
				}
				if numa.VirtualNodeCount != 2 {
					t.Errorf("expected 2 virtual nodes, got %d", numa.VirtualNodeCount)
				}
				if len(numa.Settings) != 2 {
					t.Errorf("expected 2 NUMA settings, got %d", len(numa.Settings))
				}
				if numa.Settings[0].PhysicalNodeNumber != 0 {
					t.Errorf("expected physical node 0, got %d", numa.Settings[0].PhysicalNodeNumber)
				}
				if numa.Settings[0].CountOfProcessors != 2 {
					t.Errorf("expected 2 processors, got %d", numa.Settings[0].CountOfProcessors)
				}
				if numa.Settings[0].CountOfMemoryBlocks != 1024 {
					t.Errorf("expected 1024 memory blocks, got %d", numa.Settings[0].CountOfMemoryBlocks)
				}
				if numa.Settings[0].MemoryBackingType != hcsschema.MemoryBackingType_PHYSICAL {
					t.Errorf("expected Physical backing type, got %s", numa.Settings[0].MemoryBackingType)
				}
			},
		},
		{
			name: "NUMA configuration explicit mismatch error",
			opts: &runhcsoptions.Options{
				SandboxPlatform:   "linux/amd64",
				BootFilesRootPath: validBootFilesPath,
			},
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.NumaMappedPhysicalNodes: "0,1",
					shimannotations.NumaCountOfProcessors:   "2", // Only 1 value instead of 2
					shimannotations.NumaCountOfMemoryBlocks: "1024,1024",
				},
			},
			wantErr:     true,
			errContains: "mismatch in number of physical numa nodes",
		},
		{
			name: "NUMA configuration requires physical backing",
			spec: &vm.Spec{
				Annotations: map[string]string{
					shimannotations.NumaMaximumProcessorsPerNode: "4",
					shimannotations.NumaMaximumMemorySizePerNode: "2048",
					shimannotations.AllowOvercommit:              "true",
				},
			},
			wantErr:     true,
			errContains: "vNUMA supports only Physical memory backing type",
		},
	}

	runTestCases(t, ctx, defaultOpts, tests)
}

// TestBuildSandboxConfig_NUMA_OldWindows validates that NUMA configuration
// returns an appropriate OS version error on older Windows builds that do not
// support vNUMA topology.
func TestBuildSandboxConfig_NUMA_OldWindows(t *testing.T) {
	if osversion.Build() >= osversion.V25H1Server {
		t.Skipf("Skipping old Windows vNUMA test on build %d (test targets builds older than %d)", osversion.Build(), osversion.V25H1Server)
	}

	ctx := context.Background()

	validBootFilesPath := newBootFilesPath(t)

	doc, _, err := BuildSandboxConfig(ctx, "test-owner", t.TempDir(), &runhcsoptions.Options{
		SandboxPlatform:   "linux/amd64",
		BootFilesRootPath: validBootFilesPath,
	}, &vm.Spec{
		Annotations: map[string]string{
			shimannotations.NumaMaximumProcessorsPerNode: "4",
			shimannotations.NumaMaximumMemorySizePerNode: "2048",
			shimannotations.FullyPhysicallyBacked:        "true",
		},
	})
	if err == nil {
		t.Fatalf("expected error on old Windows version, got nil (doc: %v)", doc)
	}
	if !strings.Contains(err.Error(), "vNUMA topology is not supported") {
		t.Errorf("expected error containing %q, got %q", "vNUMA topology is not supported", err.Error())
	}
}

// TestBuildSandboxConfig_CPUClamping validates that requesting more CPUs than
// the host has results in the count being clamped to the host's logical
// processor count, rather than returning an error.
func TestBuildSandboxConfig_CPUClamping(t *testing.T) {
	ctx := context.Background()

	validBootFilesPath := newBootFilesPath(t)
	hostCount := hostProcessorCount(t)
	requestedCount := hostCount * 2

	doc, _, err := BuildSandboxConfig(ctx, "test-owner", t.TempDir(), &runhcsoptions.Options{
		SandboxPlatform:   "linux/amd64",
		BootFilesRootPath: validBootFilesPath,
	}, &vm.Spec{
		Annotations: map[string]string{
			shimannotations.ProcessorCount: fmt.Sprintf("%d", requestedCount),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	actualCount := doc.VirtualMachine.ComputeTopology.Processor.Count
	if actualCount != uint32(hostCount) {
		t.Errorf("expected processor count to be clamped to host count %d, got %d", hostCount, actualCount)
	}
}
