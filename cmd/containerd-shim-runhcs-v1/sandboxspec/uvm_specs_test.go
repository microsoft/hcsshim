//go:build windows

package sandboxspec

import (
	"context"
	"reflect"
	"testing"

	"github.com/Microsoft/go-winio/pkg/guid"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/uvm"

	"github.com/containerd/platforms"
	"github.com/google/go-cmp/cmp"
)

func TestBuildUVMOptions(t *testing.T) {
	// Common test constants
	const (
		id    = "test-id"
		owner = "test-owner"
	)
	validGUID := "00000000-0000-0000-0000-000000000001"

	// Helper to create a base LCOW option set for expectations
	baseLCOW := func() *uvm.OptionsLCOW {
		return uvm.NewDefaultOptionsLCOW(id, owner)
	}

	// Helper to create a base WCOW option set for expectations
	baseWCOW := func() *uvm.OptionsWCOW {
		return uvm.NewDefaultOptionsWCOW(id, owner)
	}

	type testCase struct {
		name         string
		spec         *Specs
		expectedLCOW *uvm.OptionsLCOW
		expectedWCOW *uvm.OptionsWCOW
		expectedPlat *platforms.Platform
		expectError  bool
	}

	tests := []testCase{
		// ====================================================================
		// LCOW Tests
		// ====================================================================
		{
			name: "LCOW_Success_FullMapping",
			spec: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig: &CPUConfig{
							Architecture:   ptr("amd64"),
							ProcessorCount: ptr(int32(4)),
						},
						MemoryConfig: &MemoryConfig{
							MemorySizeInMb: ptr(uint64(2048)),
						},
						StorageConfig: &StorageConfig{
							StorageQosIopsMaximum: ptr(int32(1000)),
						},
						ResourcePartitionID: ptr(validGUID),
						Platform: &HypervisorIsolated_Lcow{
							Lcow: &LinuxHyperVOptions{
								LinuxBootOptions: &LinuxBootOptions{
									KernelDirect:      ptr(true),
									KernelBootOptions: ptr("debug"),
								},
								LinuxGuestOptions: &LinuxGuestOptions{
									DisableTimeSyncService: ptr(true),
								},
							},
						},
					},
				},
			},
			expectedLCOW: func() *uvm.OptionsLCOW {
				o := baseLCOW()
				o.ProcessorCount = 4
				o.MemorySizeInMB = 2048
				o.StorageQoSIopsMaximum = 1000
				g, _ := guid.FromString(validGUID)
				o.ResourcePartitionID = &g

				// LCOW Specifics
				o.KernelDirect = true
				o.KernelBootOptions = "debug"
				o.DisableTimeSyncService = true
				o.EnableScratchEncryption = false // default
				return o
			}(),
			expectedPlat: &platforms.Platform{OS: "linux", Architecture: "amd64"},
		},
		// LCOW Confidential Overrides
		// Testing that Confidential options are applied correctly.
		{
			name: "LCOW_Confidential_Overrides",
			spec: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig: &CPUConfig{Architecture: ptr("amd64")},
						Platform: &HypervisorIsolated_Lcow{
							Lcow: &LinuxHyperVOptions{
								ConfidentialOptions: &LCOWConfidentialOptions{
									Options: &ConfidentialOptions{
										SecurityPolicy: ptr("policy-string"),
									},
								},
							},
						},
					},
				},
			},
			expectedLCOW: func() *uvm.OptionsLCOW {
				o := baseLCOW()
				o.SecurityPolicy = "policy-string"
				o.EnableScratchEncryption = true // implicitly set
				return o
			}(),
			expectedPlat: &platforms.Platform{OS: "linux", Architecture: "amd64"},
		},

		// LCOW "Kitchen Sink"
		// Testing the interaction of Custom Boot, complex NUMA, Device assignment,
		// and Confidential settings simultaneously.
		{
			name: "LCOW_KitchenSink",
			spec: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig: &CPUConfig{
							Architecture:    ptr("amd64"),
							ProcessorCount:  ptr(int32(8)),
							ProcessorLimit:  ptr(int32(90000)), // 90%
							ProcessorWeight: ptr(int32(500)),
						},
						MemoryConfig: &MemoryConfig{
							MemorySizeInMb:   ptr(uint64(4096)),
							LowMmioGapInMb:   ptr(uint64(128)),
							HighMmioBaseInMb: ptr(uint64(1024)),
							AllowOvercommit:  ptr(false),
						},
						NumaConfig: &NUMAConfig{
							MaxProcessorsPerNumaNode:   ptr(uint32(4)),
							PreferredPhysicalNumaNodes: []uint32{0, 1},
						},
						Platform: &HypervisorIsolated_Lcow{
							Lcow: &LinuxHyperVOptions{
								LinuxBootOptions: &LinuxBootOptions{
									// Setting a custom path should trigger UpdateBootFilesPath logic
									BootFilesPath:       ptr(`C:\Custom\Boot`),
									PreferredRootFsType: ptr(PreferredRootFSType_PREFERRED_ROOT_FS_TYPE_VHD),
									HclEnabled:          ptr(true),
								},
								LinuxGuestOptions: &LinuxGuestOptions{
									ExtraVsockPorts:     []uint32{8080, 9090},
									PolicyBasedRouting:  ptr(true),
									WritableOverlayDirs: ptr(true),
								},
								ConfidentialOptions: &LCOWConfidentialOptions{
									Options: &ConfidentialOptions{
										SecurityPolicy: ptr("base64-policy-data"),
									},
									DmVerityMode: ptr(true),
								},
								// Device options with mocked values
								LinuxDeviceOptions: &LinuxDeviceOptions{
									VpMemDeviceCount: ptr(uint32(16)),
									VpciEnabled:      ptr(true),
									// Note: Actual device ID parsing depends on 'oci.ParseDevices' logic.
									// We assume simple passthrough for the unit test context here.
									AssignedDevices: []*Device{
										{ID: "class/8888", IdType: "vpci"},
									},
								},
							},
						},
					},
				},
			},
			expectedLCOW: func() *uvm.OptionsLCOW {
				o := baseLCOW()
				// CPU & Memory
				o.ProcessorCount = 8
				o.ProcessorLimit = 90000
				o.ProcessorWeight = 500
				o.MemorySizeInMB = 4096
				o.LowMMIOGapInMB = 128
				o.HighMMIOBaseInMB = 1024
				o.AllowOvercommit = false

				// NUMA
				o.MaxProcessorsPerNumaNode = 4
				o.PreferredPhysicalNumaNodes = []uint32{0, 1}

				// Boot Options (UpdateBootFilesPath logic simulation)
				o.BootFilesPath = `C:\Custom\Boot`
				// Note: UpdateBootFilesPath in the real code checks for file existence (os.Stat).
				// In a unit test environment without those files, it updates the path string
				// but might not auto-update RootFSFile unless mocked.
				// Based on your provided code, it sets the path string at minimum.
				o.PreferredRootFSType = uvm.PreferredRootFSTypeVHD
				o.RootFSFile = uvm.VhdFile // VHD type implies this file name
				hcl := true
				o.HclEnabled = &hcl

				// Guest Options
				o.ExtraVSockPorts = []uint32{8080, 9090}
				o.PolicyBasedRouting = true
				o.WritableOverlayDirs = true

				// Confidential
				o.SecurityPolicy = "base64-policy-data"
				o.DmVerityMode = true
				o.EnableScratchEncryption = true // Side effect of having SecurityPolicy

				// Devices
				o.VPMemDeviceCount = 16
				o.VPCIEnabled = true
				devs := make([]uvm.VPCIDeviceID, 0)
				devs = append(devs, uvm.NewVPCIDeviceID("class", 8888))
				o.AssignedDevices = devs

				return o
			}(),
			expectedPlat: &platforms.Platform{OS: "linux", Architecture: "amd64"},
		},

		// Logic Test: Logic_LCOW_Overrides Overrides for LCOW
		// - FullyPhysicallyBacked == true, then AllowOvercommit=false and VPMemDeviceCount=0 (for LCOW).
		// - Check for KernelFile when KernelDirect is false.
		// - Validate the PreferredRootFsType mapping.
		{
			name: "Logic_LCOW_Overrides",
			spec: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig: &CPUConfig{Architecture: ptr("amd64")},
						MemoryConfig: &MemoryConfig{
							FullyPhysicallyBacked: ptr(true),
							// Even if we explicitly ask for overcommit, PhysicallyBacked should win
							AllowOvercommit: ptr(true),
						},
						Platform: &HypervisorIsolated_Lcow{
							Lcow: &LinuxHyperVOptions{
								LinuxBootOptions: &LinuxBootOptions{
									KernelDirect:        ptr(false),
									PreferredRootFsType: ptr(PreferredRootFSType_PREFERRED_ROOT_FS_TYPE_INITRD),
								},
								LinuxDeviceOptions: &LinuxDeviceOptions{
									// Even if we ask for devices, PhysicallyBacked should force 0
									VpMemDeviceCount: ptr(uint32(50)),
								},
							},
						},
					},
				},
			},
			expectedLCOW: func() *uvm.OptionsLCOW {
				o := baseLCOW()
				o.FullyPhysicallyBacked = true
				o.AllowOvercommit = false // Overridden
				o.VPMemDeviceCount = 0    // Overridden
				o.KernelDirect = false
				o.KernelFile = uvm.KernelFile
				o.PreferredRootFSType = uvm.PreferredRootFSTypeInitRd
				o.RootFSFile = uvm.InitrdFile
				return o
			}(),
			expectedPlat: &platforms.Platform{OS: "linux", Architecture: "amd64"},
		},

		// Logic Test: LCOW SNP Enforcement (NoSecurityHardware = false)
		// This verifies that valid SNP hardware settings FORCE specific overrides,
		// ignoring conflicting user inputs (like VpMemDeviceCount).
		{
			name: "Logic_LCOW_SNP_Hardware_Enforcement",
			spec: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig: &CPUConfig{Architecture: ptr("amd64")},
						// User tries to enable Overcommit, but SNP should force it to false
						MemoryConfig: &MemoryConfig{AllowOvercommit: ptr(true)},
						Platform: &HypervisorIsolated_Lcow{
							Lcow: &LinuxHyperVOptions{
								LinuxDeviceOptions: &LinuxDeviceOptions{
									// User tries to add VPMem, but SNP (without VHD boot) forces 0
									VpMemDeviceCount: ptr(uint32(10)),
								},
								ConfidentialOptions: &LCOWConfidentialOptions{
									Options: &ConfidentialOptions{
										GuestStateFile:     ptr("abcd"),
										SecurityPolicy:     ptr("policy-string"),
										NoSecurityHardware: ptr(false), // Enforce Hardware rules
									},
									DmVerityRootFsVhd: ptr("hello"),
								},
							},
						},
					},
				},
			},
			expectedLCOW: func() *uvm.OptionsLCOW {
				o := baseLCOW()
				o.SecurityPolicy = "policy-string"
				o.EnableScratchEncryption = true

				// --- Forced Overrides by HandleLCOWSecurityPolicyWithNoSecurityHardware ---
				o.VPMemDeviceCount = 0
				o.AllowOvercommit = false
				o.SecurityPolicyEnabled = true
				o.GuestStateFile = uvm.GuestStateFile // uvm.GuestStateFile
				o.KernelBootOptions = ""
				o.PreferredRootFSType = uvm.PreferredRootFSTypeNA // uvm.PreferredRootFSTypeNA
				o.RootFSFile = ""
				o.DmVerityRootFsVhd = uvm.DefaultDmVerityRootfsVhd // uvm.DefaultDmVerityRootfsVhd
				o.DmVerityMode = true

				return o
			}(),
			expectedPlat: &platforms.Platform{OS: "linux", Architecture: "amd64"},
		},

		// Logic Test: LCOW SNP Dev Mode (NoSecurityHardware = true)
		// This verifies that when NoSecurityHardware is set, we SKIP the enforcement
		// logic, allowing "unsafe" configurations like VPMem and Overcommit to persist.
		{
			name: "Logic_LCOW_SNP_NoSecurityHardware_Skip",
			spec: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig: &CPUConfig{Architecture: ptr("amd64")},
						// These settings should be preserved because we are skipping enforcement
						MemoryConfig: &MemoryConfig{AllowOvercommit: ptr(true)},
						Platform: &HypervisorIsolated_Lcow{
							Lcow: &LinuxHyperVOptions{
								LinuxDeviceOptions: &LinuxDeviceOptions{
									VpMemDeviceCount: ptr(uint32(10)),
								},
								ConfidentialOptions: &LCOWConfidentialOptions{
									Options: &ConfidentialOptions{
										SecurityPolicy:     ptr("policy-string"),
										NoSecurityHardware: ptr(true), // Skip Enforcement
									},
								},
							},
						},
					},
				},
			},
			expectedLCOW: func() *uvm.OptionsLCOW {
				o := baseLCOW()
				o.SecurityPolicy = "policy-string"

				// --- Values should REMAIN as set by user ---
				o.VPMemDeviceCount = 10
				o.AllowOvercommit = true

				// Side effect still happens because policy string exists
				o.EnableScratchEncryption = true

				return o
			}(),
			expectedPlat: &platforms.Platform{OS: "linux", Architecture: "amd64"},
		},

		// ====================================================================
		// WCOW Tests
		// ====================================================================
		{
			name: "WCOW_Success_FullMapping",
			spec: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig: &CPUConfig{
							Architecture:   ptr("amd64"),
							ProcessorCount: ptr(int32(2)),
						},
						AdditionalConfig: &AdditionalConfig{
							ConsolePipe: ptr(`\\.\pipe\test`),
						},
						Platform: &HypervisorIsolated_Wcow{
							Wcow: &WindowsHyperVOptions{
								WindowsBootOptions: &WindowsBootOptions{
									NoDirectMap: ptr(true),
								},
								WindowsGuestOptions: &WindowsGuestOptions{
									NoInheritHostTimezone: ptr(true),
									LogSources:            ptr("Microsoft-Windows-HyperV-Guest-Logs"),
								},
							},
						},
					},
				},
			},
			expectedWCOW: func() *uvm.OptionsWCOW {
				o := baseWCOW()
				o.ProcessorCount = 2
				o.ConsolePipe = `\\.\pipe\test`
				o.NoDirectMap = true
				o.NoInheritHostTimezone = true
				o.ForwardLogs = true // By default for non-confidential
				o.LogSources = "Microsoft-Windows-HyperV-Guest-Logs"
				return o
			}(),
			expectedPlat: &platforms.Platform{OS: "windows", Architecture: "amd64"},
		},
		{
			name: "WCOW_Confidential_Overrides",
			spec: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig:    &CPUConfig{Architecture: ptr("amd64")},
						MemoryConfig: &MemoryConfig{},
						Platform: &HypervisorIsolated_Wcow{
							Wcow: &WindowsHyperVOptions{
								ConfidentialOptions: &WCOWConfidentialOptions{
									Options: &ConfidentialOptions{
										SecurityPolicy: ptr("policy"),
									},
									IsolationType: ptr("SNP"), // Short for SecureNestedPaging
								},
							},
						},
					},
				},
			},
			expectedWCOW: func() *uvm.OptionsWCOW {
				o := baseWCOW()
				o.SecurityPolicy = "policy"
				o.SecurityPolicyEnabled = true
				o.MemorySizeInMB = 2048 // Default minimum forced by confidential
				o.IsolationType = "SecureNestedPaging"
				o.AllowOvercommit = false // Forced by SNP
				o.ForwardLogs = false     // Default for confidential
				return o
			}(),
			expectedPlat: &platforms.Platform{OS: "windows", Architecture: "amd64"},
		},
		// WCOW "Kitchen Sink" with Registry Complexity
		// Testing Registry mapping, complex types, and filtering logic interactions.
		{
			name: "WCOW_KitchenSink",
			spec: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig: &CPUConfig{Architecture: ptr("amd64")},
						MemoryConfig: &MemoryConfig{
							FullyPhysicallyBacked: ptr(true),
							// Even if we explicitly ask for overcommit, PhysicallyBacked should win
							AllowOvercommit: ptr(true),
						},
						Platform: &HypervisorIsolated_Wcow{
							Wcow: &WindowsHyperVOptions{
								WindowsBootOptions: &WindowsBootOptions{
									DisableCompartmentNamespace: ptr(true),
								},
								WindowsGuestOptions: &WindowsGuestOptions{
									// We must use registry keys that pass the `ValidateAndFilterRegistryValues` allow-list.
									// Allowed: HKLM\Software\...
									AdditionalRegistryKeys: []*RegistryValue{
										{
											Key: &RegistryKey{
												Hive: RegistryHive_REGISTRY_HIVE_SOFTWARE,
												Name: "Software\\ContainerPlat\\Test",
											},
											Name:        "StringVal",
											Type:        RegistryValueType_REGISTRY_VALUE_TYPE_STRING,
											StringValue: "MyString",
										},
										{
											Key: &RegistryKey{
												Hive: RegistryHive_REGISTRY_HIVE_SOFTWARE,
												Name: "Software\\ContainerPlat\\Test",
											},
											Name:       "DwordVal",
											Type:       RegistryValueType_REGISTRY_VALUE_TYPE_D_WORD,
											DwordValue: 12345,
										},
									},
								},
							},
						},
					},
				},
			},
			expectedWCOW: func() *uvm.OptionsWCOW {
				o := baseWCOW()
				o.DisableCompartmentNamespace = true
				o.AdditionalRegistryKeys = []hcsschema.RegistryValue{
					{
						Key:         &hcsschema.RegistryKey{Hive: hcsschema.RegistryHive_SOFTWARE, Name: "Software\\ContainerPlat\\Test"},
						Name:        "StringVal",
						Type_:       hcsschema.RegistryValueType_STRING,
						StringValue: "MyString",
					},
					{
						Key:        &hcsschema.RegistryKey{Hive: hcsschema.RegistryHive_SOFTWARE, Name: "Software\\ContainerPlat\\Test"},
						Name:       "DwordVal",
						Type_:      hcsschema.RegistryValueType_D_WORD,
						DWordValue: 12345,
					},
				}
				o.FullyPhysicallyBacked = true
				o.AllowOvercommit = false // Overridden
				return o
			}(),
			expectedPlat: &platforms.Platform{OS: "windows", Architecture: "amd64"},
		},
		// Logic Test: WCOW Confidential Memory Constraint
		// Confidential WCOW requires at least 2048MB.
		{
			name: "Logic_WCOW_Confidential_LowMemory_Error",
			spec: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig: &CPUConfig{Architecture: ptr("amd64")},
						MemoryConfig: &MemoryConfig{
							MemorySizeInMb: ptr(uint64(1024)), // Too low
						},
						Platform: &HypervisorIsolated_Wcow{
							Wcow: &WindowsHyperVOptions{
								ConfidentialOptions: &WCOWConfidentialOptions{
									Options: &ConfidentialOptions{
										SecurityPolicy: ptr("policy"),
									},
								},
							},
						},
					},
				},
			},
			expectError: true, // Should fail validation
		},
		// Logic Test: WCOW Isolation Type Shorthands
		// Verifying that "VBS" maps to "VirtualizationBasedSecurity"
		{
			name: "Logic_WCOW_IsolationType",
			spec: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig: &CPUConfig{Architecture: ptr("amd64")},
						MemoryConfig: &MemoryConfig{
							AllowOvercommit: ptr(true), // Should be overridden
						},
						Platform: &HypervisorIsolated_Wcow{
							Wcow: &WindowsHyperVOptions{
								ConfidentialOptions: &WCOWConfidentialOptions{
									Options: &ConfidentialOptions{
										SecurityPolicy: ptr("policy"),
									},
									IsolationType: ptr("VBS"),
								},
							},
						},
					},
				},
			},
			expectedWCOW: func() *uvm.OptionsWCOW {
				o := baseWCOW()
				o.SecurityPolicy = "policy"
				o.SecurityPolicyEnabled = true
				o.MemorySizeInMB = 2048
				o.IsolationType = "VirtualizationBasedSecurity" // Expanded
				o.AllowOvercommit = false
				o.ForwardLogs = false
				return o
			}(),
			expectedPlat: &platforms.Platform{OS: "windows", Architecture: "amd64"},
		},
		{
			name: "WCOW_Non_Confidential_DoesNotOverride",
			spec: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig:    &CPUConfig{Architecture: ptr("amd64")},
						MemoryConfig: &MemoryConfig{},
						Platform: &HypervisorIsolated_Wcow{
							Wcow: &WindowsHyperVOptions{
								ConfidentialOptions: &WCOWConfidentialOptions{
									Options: &ConfidentialOptions{
										GuestStateFile: ptr("abcd"),
									},
									DisableSecureBoot: ptr(true),
									IsolationType:     ptr("SNP"), // Short for SecureNestedPaging
								},
							},
						},
					},
				},
			},
			expectedWCOW: func() *uvm.OptionsWCOW {
				o := baseWCOW()
				return o
			}(),
			expectedPlat: &platforms.Platform{OS: "windows", Architecture: "amd64"},
		},

		// ====================================================================
		// Common Config Tests (Registry, HvSocket)
		// ====================================================================
		{
			name: "Common_HvSocket_ServiceTable",
			spec: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig: &CPUConfig{Architecture: ptr("amd64")},
						AdditionalConfig: &AdditionalConfig{
							AdditionalHypervConfig: map[string]*HvSocketServiceConfig{
								validGUID: {
									AllowWildcardBinds: ptr(true),
									Disabled:           ptr(false),
								},
							},
						},
						Platform: &HypervisorIsolated_Lcow{
							Lcow: &LinuxHyperVOptions{},
						},
					},
				},
			},
			expectedLCOW: func() *uvm.OptionsLCOW {
				o := baseLCOW()
				o.AdditionalHyperVConfig[validGUID] = hcsschema.HvSocketServiceConfig{
					AllowWildcardBinds: true,
					Disabled:           false,
				}
				return o
			}(),
			expectedPlat: &platforms.Platform{OS: "linux", Architecture: "amd64"},
		},

		// ====================================================================
		// Error Cases
		// ====================================================================
		{
			name:        "Error_NilSpec",
			spec:        nil,
			expectError: true,
		},
		{
			name: "Error_ProcessIsolation",
			spec: &Specs{
				IsolationLevel: &Specs_Process{},
			},
			expectError: true,
		},
		{
			name: "Error_MissingHypervisor",
			spec: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: nil,
				},
			},
			expectError: true,
		},
		{
			name: "Error_MissingPlatform_LCOW",
			spec: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						Platform: &HypervisorIsolated_Lcow{
							Lcow: nil,
						},
					},
				},
			},
			expectError: true,
		},
		{
			name: "Error_MissingPlatform_WCOW",
			spec: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						Platform: &HypervisorIsolated_Wcow{
							Wcow: nil,
						},
					},
				},
			},
			expectError: true,
		},
		{
			name: "Error_Invalid_ResourcePartition_GUID",
			spec: &Specs{
				IsolationLevel: &Specs_Hypervisor{
					Hypervisor: &HypervisorIsolated{
						CpuConfig:           &CPUConfig{Architecture: ptr("amd64")},
						ResourcePartitionID: ptr("not-a-guid"),
						Platform: &HypervisorIsolated_Lcow{
							Lcow: &LinuxHyperVOptions{},
						},
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLCOW, gotWCOW, gotPlat, err := BuildUVMOptions(context.Background(), tt.spec, id, owner)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Validate Platform
			if !reflect.DeepEqual(gotPlat, tt.expectedPlat) {
				t.Errorf("Platform mismatch.\nGot:  %+v\nWant: %+v", gotPlat, tt.expectedPlat)
			}

			opts := cmp.Options{
				cmp.AllowUnexported(uvm.VPCIDeviceID{}),
			}

			if tt.expectedLCOW != nil {
				if gotLCOW == nil {
					t.Fatal("Expected LCOW options, got nil")
				}

				gotLCOW.OutputHandlerCreator = nil
				tt.expectedLCOW.OutputHandlerCreator = nil

				if diff := cmp.Diff(tt.expectedLCOW, gotLCOW, opts); diff != "" {
					t.Errorf("LCOW Options mismatch (-want +got):\n%s", diff)
				}
			} else if tt.expectedWCOW != nil {
				if gotWCOW == nil {
					t.Fatal("Expected WCOW options, got nil")
				}

				gotWCOW.OutputHandlerCreator = nil
				tt.expectedWCOW.OutputHandlerCreator = nil

				if diff := cmp.Diff(tt.expectedWCOW, gotWCOW, opts); diff != "" {
					t.Errorf("WCOW Options mismatch (-want +got):\n%s", diff)
				}
			} else {
				t.Fatal("Test case did not specify expected LCOW or WCOW output")
			}
		})
	}
}
