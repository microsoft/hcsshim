package sandboxspec

import (
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"

	runhcsoptions "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	iannotations "github.com/Microsoft/hcsshim/internal/annotations"
	shimannotations "github.com/Microsoft/hcsshim/pkg/annotations"

	"github.com/opencontainers/runtime-spec/specs-go"
)

// ptr is a helper to return a pointer to a value.
// Required for initializing optional pointer fields in structs.
func ptr[T any](v T) *T {
	return &v
}

func TestGenerateSpecss(t *testing.T) {
	type testCase struct {
		name        string
		opts        *runhcsoptions.Options
		annotations map[string]string
		devices     []specs.WindowsDevice
		expected    *Specs
		expectError bool
	}

	tests := []testCase{
		// ====================================================================
		// SECTION 1: LCOW (Linux) Specific Tests
		// ====================================================================
		{
			name: "LCOW_Boot_And_Guest_Options_Comprehensive",
			opts: &runhcsoptions.Options{
				SandboxIsolation: runhcsoptions.Options_HYPERVISOR,
				SandboxPlatform:  "linux/amd64",
			},
			annotations: map[string]string{
				// Boot Options
				shimannotations.KernelBootOptions:     "debug",
				shimannotations.KernelDirectBoot:      "true",
				shimannotations.LCOWHclEnabled:        "true",
				shimannotations.EnableColdDiscardHint: "true",
				shimannotations.PreferredRootFSType:   "initrd",
				shimannotations.BootFilesRootPath:     "/boot/files",
				// Guest Options
				shimannotations.DisableLCOWTimeSyncService: "true",
				iannotations.NetworkingPolicyBasedRouting:  "true",
				iannotations.WritableOverlayDirs:           "true",
				iannotations.ExtraVSockPorts:               "1024,2048",
			},
			expectError: false,
			expected: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig:        &CPUConfig{Architecture: ptr("amd64")},
						MemoryConfig:     &MemoryConfig{},
						StorageConfig:    &StorageConfig{},
						NumaConfig:       &NUMAConfig{},
						AdditionalConfig: &AdditionalConfig{},
						Platform: &HypervisorIsolated_Lcow{
							Lcow: &LinuxHyperVOptions{
								LinuxBootOptions: &LinuxBootOptions{
									KernelBootOptions:     ptr("debug"),
									KernelDirect:          ptr(true),
									HclEnabled:            ptr(true),
									EnableColdDiscardHint: ptr(true),
									PreferredRootFsType:   ptr(PreferredRootFSType_PREFERRED_ROOT_FS_TYPE_INITRD),
									BootFilesPath:         ptr("/boot/files"),
								},
								LinuxGuestOptions: &LinuxGuestOptions{
									DisableTimeSyncService: ptr(true),
									PolicyBasedRouting:     ptr(true),
									WritableOverlayDirs:    ptr(true),
									ExtraVsockPorts:        []uint32{1024, 2048},
								},
								LinuxDeviceOptions: &LinuxDeviceOptions{
									AssignedDevices: []*Device{},
								},
								ConfidentialOptions: &LCOWConfidentialOptions{
									Options: &ConfidentialOptions{NoSecurityHardware: ptr(false)},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "LCOW_DeviceOptions_Comprehensive",
			opts: &runhcsoptions.Options{
				SandboxIsolation: runhcsoptions.Options_HYPERVISOR,
				SandboxPlatform:  "linux/amd64",
			},
			annotations: map[string]string{
				shimannotations.VPMemCount:          "4",
				shimannotations.VPMemSize:           "1073741824", // 1GiB
				shimannotations.VPCIEnabled:         "true",
				shimannotations.VPMemNoMultiMapping: "true",
			},
			devices: []specs.WindowsDevice{
				{ID: "1111-1111", IDType: "class"},
				{ID: "2222-2222", IDType: "class"},
			},
			expectError: false,
			expected: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig:        &CPUConfig{Architecture: ptr("amd64")},
						MemoryConfig:     &MemoryConfig{},
						StorageConfig:    &StorageConfig{},
						NumaConfig:       &NUMAConfig{},
						AdditionalConfig: &AdditionalConfig{},
						Platform: &HypervisorIsolated_Lcow{
							Lcow: &LinuxHyperVOptions{
								LinuxBootOptions:  &LinuxBootOptions{},
								LinuxGuestOptions: &LinuxGuestOptions{ExtraVsockPorts: []uint32{}},
								LinuxDeviceOptions: &LinuxDeviceOptions{
									VpMemDeviceCount:    ptr(uint32(4)),
									VpMemSizeBytes:      ptr(uint64(1073741824)),
									VpciEnabled:         ptr(true),
									VpMemNoMultiMapping: ptr(true),
									AssignedDevices: []*Device{
										{ID: "1111-1111", IdType: "class"},
										{ID: "2222-2222", IdType: "class"},
									},
								},
								ConfidentialOptions: &LCOWConfidentialOptions{
									Options: &ConfidentialOptions{NoSecurityHardware: ptr(false)},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "LCOW_Confidential_Comprehensive",
			opts: &runhcsoptions.Options{
				SandboxIsolation: runhcsoptions.Options_HYPERVISOR,
				SandboxPlatform:  "linux/amd64",
			},
			annotations: map[string]string{
				shimannotations.LCOWGuestStateFile:         "guest_state.vmgs",
				shimannotations.LCOWSecurityPolicy:         "abc123",
				shimannotations.LCOWSecurityPolicyEnforcer: "XYZ",
				shimannotations.LCOWReferenceInfoFile:      "uvm_ref_info.json",
				shimannotations.NoSecurityHardware:         "false",
				shimannotations.DmVerityMode:               "true",
				shimannotations.DmVerityRootFsVhd:          "/host/paths/rootfs.vhd",
				shimannotations.DmVerityCreateArgs:         "1 /dev/vda1 /dev/vda2",
				shimannotations.LCOWEncryptedScratchDisk:   "true",
			},
			expectError: false,
			expected: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig:        &CPUConfig{Architecture: ptr("amd64")},
						MemoryConfig:     &MemoryConfig{},
						StorageConfig:    &StorageConfig{},
						NumaConfig:       &NUMAConfig{},
						AdditionalConfig: &AdditionalConfig{},
						Platform: &HypervisorIsolated_Lcow{
							Lcow: &LinuxHyperVOptions{
								LinuxBootOptions:   &LinuxBootOptions{},
								LinuxGuestOptions:  &LinuxGuestOptions{ExtraVsockPorts: []uint32{}},
								LinuxDeviceOptions: &LinuxDeviceOptions{AssignedDevices: []*Device{}},
								ConfidentialOptions: &LCOWConfidentialOptions{
									DmVerityMode:            ptr(true),
									DmVerityRootFsVhd:       ptr("/host/paths/rootfs.vhd"),
									DmVerityCreateArgs:      ptr("1 /dev/vda1 /dev/vda2"),
									EnableScratchEncryption: ptr(true),
									Options: &ConfidentialOptions{
										NoSecurityHardware:     ptr(false),
										GuestStateFile:         ptr("guest_state.vmgs"),
										SecurityPolicy:         ptr("abc123"),
										SecurityPolicyEnforcer: ptr("XYZ"),
										UvmReferenceInfoFile:   ptr("uvm_ref_info.json"),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "LCOW_RunHCSOptions_Override",
			opts: &runhcsoptions.Options{
				SandboxIsolation: runhcsoptions.Options_HYPERVISOR,
				SandboxPlatform:  "linux/amd64",
				// Use boot files from Options when annotation is missing.
				BootFilesRootPath: "/default/boot/files",
				VmProcessorCount:  4,
				VmMemorySizeInMb:  100,
				NCProxyAddr:       "//./pipe/proxy",
			},
			annotations: map[string]string{},
			expectError: false,
			expected: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig: &CPUConfig{
							ProcessorCount: ptr(int32(4)),
							Architecture:   ptr("amd64"),
						},
						MemoryConfig: &MemoryConfig{
							MemorySizeInMb: ptr(uint64(100)),
						},
						StorageConfig: &StorageConfig{},
						NumaConfig:    &NUMAConfig{},
						AdditionalConfig: &AdditionalConfig{
							NetworkConfigProxy: ptr("//./pipe/proxy"),
						},
						Platform: &HypervisorIsolated_Lcow{
							Lcow: &LinuxHyperVOptions{
								LinuxBootOptions: &LinuxBootOptions{
									BootFilesPath: ptr("/default/boot/files"),
								},
								LinuxGuestOptions: &LinuxGuestOptions{
									ExtraVsockPorts: []uint32{},
								},
								LinuxDeviceOptions: &LinuxDeviceOptions{
									AssignedDevices: []*Device{},
								},
								ConfidentialOptions: &LCOWConfidentialOptions{
									Options: &ConfidentialOptions{NoSecurityHardware: ptr(false)},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "LCOW_MissingValues",
			opts: &runhcsoptions.Options{
				SandboxIsolation: runhcsoptions.Options_HYPERVISOR,
				SandboxPlatform:  "linux/amd64",
			},
			annotations: map[string]string{},
			expectError: false,
			expected: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig:        &CPUConfig{Architecture: ptr("amd64")},
						MemoryConfig:     &MemoryConfig{},
						StorageConfig:    &StorageConfig{},
						NumaConfig:       &NUMAConfig{},
						AdditionalConfig: &AdditionalConfig{},
						Platform: &HypervisorIsolated_Lcow{
							Lcow: &LinuxHyperVOptions{
								LinuxBootOptions: &LinuxBootOptions{},
								LinuxGuestOptions: &LinuxGuestOptions{
									ExtraVsockPorts: []uint32{},
								},
								LinuxDeviceOptions: &LinuxDeviceOptions{
									AssignedDevices: []*Device{},
								},
								ConfidentialOptions: &LCOWConfidentialOptions{
									Options: &ConfidentialOptions{NoSecurityHardware: ptr(false)},
								},
							},
						},
					},
				},
			},
		},

		// ====================================================================
		// SECTION 2: WCOW (Windows) Specific Tests
		// ====================================================================
		{
			name: "WCOW_Boot_Guest_And_Registry",
			opts: &runhcsoptions.Options{
				SandboxIsolation: runhcsoptions.Options_HYPERVISOR,
				SandboxPlatform:  "windows/amd64",
			},
			annotations: map[string]string{
				// Boot Options
				shimannotations.DisableCompartmentNamespace: "true",
				shimannotations.VSMBNoDirectMap:             "true",
				// Guest Options
				shimannotations.NoInheritHostTimezone: "true",
				// Registry
				iannotations.AdditionalRegistryValues: `[
					{
						"Key": {"Hive": "Software", "Name": "Software\\TestKey"},
						"Name": "TestVal",
						"Type": "String",
						"StringValue": "MyValue"
					}
				]`,
				shimannotations.ForwardLogs: "true",
				shimannotations.LogSources:  "Microsoft-Windows-Containers-GuestLog",
			},
			expectError: false,
			expected: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig:        &CPUConfig{Architecture: ptr("amd64")},
						MemoryConfig:     &MemoryConfig{},
						StorageConfig:    &StorageConfig{},
						NumaConfig:       &NUMAConfig{},
						AdditionalConfig: &AdditionalConfig{},
						Platform: &HypervisorIsolated_Wcow{
							Wcow: &WindowsHyperVOptions{
								WindowsBootOptions: &WindowsBootOptions{
									DisableCompartmentNamespace: ptr(true),
									NoDirectMap:                 ptr(true),
								},
								WindowsGuestOptions: &WindowsGuestOptions{
									NoInheritHostTimezone: ptr(true),
									AdditionalRegistryKeys: []*RegistryValue{
										{
											Key: &RegistryKey{
												Hive: RegistryHive_REGISTRY_HIVE_SOFTWARE,
												Name: "Software\\TestKey",
											},
											Name:        "TestVal",
											Type:        RegistryValueType_REGISTRY_VALUE_TYPE_STRING,
											StringValue: "MyValue",
										},
									},
									ForwardLogs: ptr(true),
									LogSources:  ptr("Microsoft-Windows-Containers-GuestLog"),
								},
								ConfidentialOptions: &WCOWConfidentialOptions{
									Options:           &ConfidentialOptions{NoSecurityHardware: ptr(false)},
									DisableSecureBoot: ptr(false),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "WCOW_Confidential_Comprehensive",
			opts: &runhcsoptions.Options{
				SandboxIsolation: runhcsoptions.Options_HYPERVISOR,
				SandboxPlatform:  "windows/amd64",
			},
			annotations: map[string]string{
				shimannotations.WCOWDisableSecureBoot: "true",
				shimannotations.WCOWWritableEFI:       "true",
				shimannotations.WCOWIsolationType:     "NoIsolation",
			},
			expectError: false,
			expected: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig:        &CPUConfig{Architecture: ptr("amd64")},
						MemoryConfig:     &MemoryConfig{},
						StorageConfig:    &StorageConfig{},
						NumaConfig:       &NUMAConfig{},
						AdditionalConfig: &AdditionalConfig{},
						Platform: &HypervisorIsolated_Wcow{
							Wcow: &WindowsHyperVOptions{
								WindowsBootOptions:  &WindowsBootOptions{},
								WindowsGuestOptions: &WindowsGuestOptions{AdditionalRegistryKeys: []*RegistryValue{}},
								ConfidentialOptions: &WCOWConfidentialOptions{
									Options:           &ConfidentialOptions{NoSecurityHardware: ptr(false)},
									DisableSecureBoot: ptr(true),
									WritableEfi:       ptr(true),
									IsolationType:     ptr("NoIsolation"),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "WCOW_MissingValues",
			opts: &runhcsoptions.Options{
				SandboxIsolation: runhcsoptions.Options_HYPERVISOR,
				SandboxPlatform:  "windows/amd64",
			},
			annotations: map[string]string{},
			expectError: false,
			expected: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig:        &CPUConfig{Architecture: ptr("amd64")},
						MemoryConfig:     &MemoryConfig{},
						StorageConfig:    &StorageConfig{},
						NumaConfig:       &NUMAConfig{},
						AdditionalConfig: &AdditionalConfig{},
						Platform: &HypervisorIsolated_Wcow{
							Wcow: &WindowsHyperVOptions{
								WindowsBootOptions:  &WindowsBootOptions{},
								WindowsGuestOptions: &WindowsGuestOptions{AdditionalRegistryKeys: []*RegistryValue{}},
								ConfidentialOptions: &WCOWConfidentialOptions{
									Options:           &ConfidentialOptions{NoSecurityHardware: ptr(false)},
									DisableSecureBoot: ptr(false),
								},
							},
						},
					},
				},
			},
		},

		// ====================================================================
		// SECTION 3: Common Resources (Memory, Storage, Additional)
		// ====================================================================
		{
			name: "Memory_Storage_Comprehensive",
			opts: &runhcsoptions.Options{
				SandboxIsolation: runhcsoptions.Options_HYPERVISOR,
				SandboxPlatform:  "linux/amd64",
			},
			annotations: map[string]string{
				// Memory
				shimannotations.MemorySizeInMB:         "2048",
				shimannotations.MemoryLowMMIOGapInMB:   "128",
				shimannotations.MemoryHighMMIOBaseInMB: "512",
				shimannotations.MemoryHighMMIOGapInMB:  "256",
				shimannotations.AllowOvercommit:        "false",
				shimannotations.FullyPhysicallyBacked:  "true",
				shimannotations.EnableDeferredCommit:   "true",
				// Storage
				shimannotations.DisableWritableFileShares:  "true",
				shimannotations.StorageQoSBandwidthMaximum: "1048576", // 1MB/s
				shimannotations.StorageQoSIopsMaximum:      "500",
			},
			expectError: false,
			expected: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig: &CPUConfig{Architecture: ptr("amd64")},
						MemoryConfig: &MemoryConfig{
							MemorySizeInMb:        ptr(uint64(2048)),
							LowMmioGapInMb:        ptr(uint64(128)),
							HighMmioBaseInMb:      ptr(uint64(512)),
							HighMmioGapInMb:       ptr(uint64(256)),
							AllowOvercommit:       ptr(false),
							FullyPhysicallyBacked: ptr(true),
							EnableDeferredCommit:  ptr(true),
						},
						StorageConfig: &StorageConfig{
							NoWritableFileShares:       ptr(true),
							StorageQosBandwidthMaximum: ptr(int32(1048576)),
							StorageQosIopsMaximum:      ptr(int32(500)),
						},
						NumaConfig:       &NUMAConfig{},
						AdditionalConfig: &AdditionalConfig{},
						Platform: &HypervisorIsolated_Lcow{
							Lcow: &LinuxHyperVOptions{
								LinuxBootOptions:   &LinuxBootOptions{},
								LinuxGuestOptions:  &LinuxGuestOptions{ExtraVsockPorts: []uint32{}},
								LinuxDeviceOptions: &LinuxDeviceOptions{AssignedDevices: []*Device{}},
								ConfidentialOptions: &LCOWConfidentialOptions{
									Options: &ConfidentialOptions{NoSecurityHardware: ptr(false)},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "AdditionalConfig_Comprehensive",
			opts: &runhcsoptions.Options{
				SandboxIsolation: runhcsoptions.Options_HYPERVISOR,
				SandboxPlatform:  "windows/amd64",
			},
			annotations: map[string]string{
				shimannotations.NetworkConfigProxy:           "//./pipe/proxy",
				shimannotations.ContainerProcessDumpLocation: "C:\\dumps",
				shimannotations.DumpDirectoryPath:            "C:\\host\\dumps",
				iannotations.UVMConsolePipe:                  "\\\\.\\pipe\\vm-console",
				"io.microsoft.virtualmachine.hv-socket.service-table.00000000-0000-0000-0000-000000000001": `{"AllowWildcardBinds": true, "BindSecurityDescriptor": "D:P(A;;FA;;;WD)"}`,
			},
			expectError: false,
			expected: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig:     &CPUConfig{Architecture: ptr("amd64")},
						MemoryConfig:  &MemoryConfig{},
						StorageConfig: &StorageConfig{},
						NumaConfig:    &NUMAConfig{},
						AdditionalConfig: &AdditionalConfig{
							NetworkConfigProxy:  ptr("//./pipe/proxy"),
							ProcessDumpLocation: ptr("C:\\dumps"),
							DumpDirectoryPath:   ptr("C:\\host\\dumps"),
							ConsolePipe:         ptr("\\\\.\\pipe\\vm-console"),
							AdditionalHypervConfig: map[string]*HvSocketServiceConfig{
								"00000000-0000-0000-0000-000000000001": {
									AllowWildcardBinds:        ptr(true),
									BindSecurityDescriptor:    ptr("D:P(A;;FA;;;WD)"),
									ConnectSecurityDescriptor: ptr(""),
									Disabled:                  ptr(false),
								},
							},
						},
						Platform: &HypervisorIsolated_Wcow{
							Wcow: &WindowsHyperVOptions{
								WindowsBootOptions:  &WindowsBootOptions{},
								WindowsGuestOptions: &WindowsGuestOptions{AdditionalRegistryKeys: []*RegistryValue{}},
								ConfidentialOptions: &WCOWConfidentialOptions{
									Options:           &ConfidentialOptions{NoSecurityHardware: ptr(false)},
									DisableSecureBoot: ptr(false),
								},
							},
						},
					},
				},
			},
		},

		// ====================================================================
		// SECTION 4: Logic, Precedence and Error Handling
		// ====================================================================
		{
			name: "Mixed_Defaults_And_Overrides_Logic",
			opts: &runhcsoptions.Options{
				SandboxIsolation: runhcsoptions.Options_HYPERVISOR,
				SandboxPlatform:  "windows/amd64",
				// Default annotations passed via Options
				DefaultContainerAnnotations: map[string]string{
					shimannotations.ProcessorCount:        "2",
					shimannotations.StorageQoSIopsMaximum: "100",
				},
			},
			annotations: map[string]string{
				// This should OVERRIDE the DefaultContainerAnnotations above
				shimannotations.ProcessorCount: "8",
			},
			expectError: false,
			expected: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig: &CPUConfig{
							Architecture:   ptr("amd64"),
							ProcessorCount: ptr(int32(8)), // Overridden
						},
						MemoryConfig: &MemoryConfig{},
						StorageConfig: &StorageConfig{
							StorageQosIopsMaximum: ptr(int32(100)), // From default
						},
						NumaConfig:       &NUMAConfig{},
						AdditionalConfig: &AdditionalConfig{},
						Platform: &HypervisorIsolated_Wcow{
							Wcow: &WindowsHyperVOptions{
								WindowsBootOptions:  &WindowsBootOptions{},
								WindowsGuestOptions: &WindowsGuestOptions{AdditionalRegistryKeys: []*RegistryValue{}},
								ConfidentialOptions: &WCOWConfidentialOptions{
									Options:           &ConfidentialOptions{NoSecurityHardware: ptr(false)},
									DisableSecureBoot: ptr(false),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Explicit_NUMA_Topology",
			opts: &runhcsoptions.Options{
				SandboxIsolation: runhcsoptions.Options_HYPERVISOR,
				SandboxPlatform:  "linux/amd64",
			},
			annotations: map[string]string{
				shimannotations.NumaMappedPhysicalNodes: "0,1",
				shimannotations.NumaCountOfProcessors:   "4,4",
				shimannotations.NumaCountOfMemoryBlocks: "8192,8192",
			},
			expectError: false,
			expected: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig:     &CPUConfig{Architecture: ptr("amd64")},
						MemoryConfig:  &MemoryConfig{},
						StorageConfig: &StorageConfig{},
						NumaConfig: &NUMAConfig{
							NumaMappedPhysicalNodes: []uint32{0, 1},
							NumaProcessorCounts:     []uint32{4, 4},
							NumaMemoryBlocksCounts:  []uint64{8192, 8192},
						},
						AdditionalConfig: &AdditionalConfig{},
						Platform: &HypervisorIsolated_Lcow{
							Lcow: &LinuxHyperVOptions{
								LinuxBootOptions:   &LinuxBootOptions{},
								LinuxGuestOptions:  &LinuxGuestOptions{ExtraVsockPorts: []uint32{}},
								LinuxDeviceOptions: &LinuxDeviceOptions{AssignedDevices: []*Device{}},
								ConfidentialOptions: &LCOWConfidentialOptions{
									Options: &ConfidentialOptions{NoSecurityHardware: ptr(false)},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "CPUGroup_And_ResourcePartition",
			opts: &runhcsoptions.Options{
				SandboxIsolation: runhcsoptions.Options_HYPERVISOR,
				SandboxPlatform:  "windows/amd64",
			},
			annotations: map[string]string{
				shimannotations.CPUGroupID:          "cg-123",
				shimannotations.ResourcePartitionID: "rp-abc",
			},
			expectError: false,
			expected: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig:           &CPUConfig{Architecture: ptr("amd64")},
						MemoryConfig:        &MemoryConfig{},
						StorageConfig:       &StorageConfig{},
						NumaConfig:          &NUMAConfig{},
						AdditionalConfig:    &AdditionalConfig{},
						CpuGroupID:          ptr("cg-123"),
						ResourcePartitionID: ptr("rp-abc"),
						Platform: &HypervisorIsolated_Wcow{
							Wcow: &WindowsHyperVOptions{
								WindowsBootOptions:  &WindowsBootOptions{},
								WindowsGuestOptions: &WindowsGuestOptions{AdditionalRegistryKeys: []*RegistryValue{}},
								ConfidentialOptions: &WCOWConfidentialOptions{
									Options:           &ConfidentialOptions{NoSecurityHardware: ptr(false)},
									DisableSecureBoot: ptr(false),
								},
							},
						},
					},
				},
			},
		},

		// ====================================================================
		// SECTION 5: Edge Cases, Nil Checks, and Errors
		// ====================================================================
		{
			name: "ProcessIsolation_Returns_ProcessSpec",
			opts: &runhcsoptions.Options{
				SandboxIsolation: runhcsoptions.Options_PROCESS,
			},
			annotations: nil,
			expectError: false,
			expected: &Specs{
				IsolationLevel: &Specs_Process{
					Process: &ProcessIsolated{},
				},
			},
		},
		{
			name:        "NilOptions_ShouldError",
			opts:        nil,
			annotations: map[string]string{},
			expectError: true,
			expected:    nil,
		},
		{
			name: "Platform_Empty_ShouldError",
			opts: &runhcsoptions.Options{
				SandboxIsolation: runhcsoptions.Options_HYPERVISOR,
				SandboxPlatform:  "",
			},
			annotations: map[string]string{},
			expectError: true,
			expected:    nil,
		},
		{
			name: "Platform_UnsupportedOS_ShouldError",
			opts: &runhcsoptions.Options{
				SandboxIsolation: runhcsoptions.Options_HYPERVISOR,
				SandboxPlatform:  "darwin/arm64",
			},
			annotations: map[string]string{},
			expectError: true,
			expected:    nil,
		},
		{
			name: "LinuxBoot_PreferredRootFSType_Invalid_ShouldError",
			opts: &runhcsoptions.Options{
				SandboxIsolation: runhcsoptions.Options_HYPERVISOR,
				SandboxPlatform:  "linux/amd64",
			},
			annotations: map[string]string{
				shimannotations.PreferredRootFSType: "not-a-valid-type",
			},
			expectError: true,
			expected:    nil,
		},
		{
			name: "Safety_Garbage_Inputs_ShouldNotPanic",
			opts: &runhcsoptions.Options{
				SandboxIsolation: runhcsoptions.Options_HYPERVISOR,
				SandboxPlatform:  "linux/amd64",
			},
			annotations: map[string]string{
				shimannotations.ProcessorCount:             "NOT_A_NUMBER",
				shimannotations.MemorySizeInMB:             "-100",
				shimannotations.NumaPreferredPhysicalNodes: "garbage,data",
			},
			expectError: false,
			// Should return a valid default spec because parsing errors are logged
			// and ignored (returning default values like 0/nil).
			expected: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig:        &CPUConfig{Architecture: ptr("amd64")},
						MemoryConfig:     &MemoryConfig{},
						StorageConfig:    &StorageConfig{},
						NumaConfig:       &NUMAConfig{},
						AdditionalConfig: &AdditionalConfig{},
						Platform: &HypervisorIsolated_Lcow{
							Lcow: &LinuxHyperVOptions{
								LinuxBootOptions:   &LinuxBootOptions{},
								LinuxGuestOptions:  &LinuxGuestOptions{ExtraVsockPorts: []uint32{}},
								LinuxDeviceOptions: &LinuxDeviceOptions{AssignedDevices: []*Device{}},
								ConfidentialOptions: &LCOWConfidentialOptions{
									Options: &ConfidentialOptions{NoSecurityHardware: ptr(false)},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Safety_Nil_Maps_And_Slices",
			opts: &runhcsoptions.Options{
				SandboxIsolation: runhcsoptions.Options_HYPERVISOR,
				SandboxPlatform:  "windows/amd64",
			},
			annotations: nil, // Nil annotations map
			devices:     nil, // Nil devices slice
			expectError: false,
			expected: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig:        &CPUConfig{Architecture: ptr("amd64")},
						MemoryConfig:     &MemoryConfig{},
						StorageConfig:    &StorageConfig{},
						NumaConfig:       &NUMAConfig{},
						AdditionalConfig: &AdditionalConfig{},
						Platform: &HypervisorIsolated_Wcow{
							Wcow: &WindowsHyperVOptions{
								WindowsBootOptions:  &WindowsBootOptions{},
								WindowsGuestOptions: &WindowsGuestOptions{AdditionalRegistryKeys: []*RegistryValue{}},
								ConfidentialOptions: &WCOWConfidentialOptions{
									Options:           &ConfidentialOptions{NoSecurityHardware: ptr(false)},
									DisableSecureBoot: ptr(false),
								},
							},
						},
					},
				},
			},
		},

		// ====================================================================
		// SECTION 6: NEGATIVE TEST CASES (Expect Errors)
		// ====================================================================
		{
			name: "Negative_Conflicting_HostProcess_Annotations",
			opts: &runhcsoptions.Options{
				SandboxIsolation: runhcsoptions.Options_HYPERVISOR,
				SandboxPlatform:  "windows/amd64",
			},
			annotations: map[string]string{
				shimannotations.HostProcessContainer:        "true",
				shimannotations.DisableHostProcessContainer: "true",
			},
			expectError: true,
			expected:    nil,
		},
		{
			name: "Negative_Invalid_Platform_Format_NoSlash",
			opts: &runhcsoptions.Options{
				SandboxIsolation: runhcsoptions.Options_HYPERVISOR,
				SandboxPlatform:  "windows_amd64",
			},
			annotations: map[string]string{},
			expectError: true,
			expected:    nil,
		},
		{
			name: "Negative_Invalid_Platform_Format_MissingArch",
			opts: &runhcsoptions.Options{
				SandboxIsolation: runhcsoptions.Options_HYPERVISOR,
				SandboxPlatform:  "windows/",
			},
			annotations: map[string]string{},
			expectError: true,
			expected:    nil,
		},
	}

	for _, tt := range tests {
		safeTestRun(t, tt.name, func(t *testing.T) {
			t.Helper()
			got, err := Generate(tt.opts, tt.annotations, tt.devices)

			// 1. Error Validation
			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// 2. Golden Object Comparison using proto.Equal
			if !proto.Equal(got, tt.expected) {
				// Since we can't use cmp.Diff, we use JSON for a readable diff in the error logs
				gotJSON, _ := json.MarshalIndent(got, "", "  ")
				wantJSON, _ := json.MarshalIndent(tt.expected, "", "  ")
				t.Errorf("GenerateSpecss() mismatch.\nGot:\n%s\n\nWant:\n%s", gotJSON, wantJSON)
			}
		})
	}
}

// safeTestRun wraps the test execution to catch any panics.
func safeTestRun(t *testing.T, name string, testFunc func(t *testing.T)) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("PANIC DETECTED in test case '%s': %v", name, r)
			}
		}()
		testFunc(t)
	})
}
