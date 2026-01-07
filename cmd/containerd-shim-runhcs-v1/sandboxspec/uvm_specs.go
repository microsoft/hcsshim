//go:build windows

package sandboxspec

import (
	"context"
	"fmt"
	"maps"
	"runtime"
	"strings"

	"github.com/Microsoft/go-winio/pkg/guid"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/containerd/platforms"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

var (
	// linuxPlatformFormat represents the format for linux platform.
	// Example: linux/amd64
	linuxPlatformFormat = "linux/%s"
	// windowsPlatformFormat represents the format for windows platform.
	// Example: windows/amd64
	windowsPlatformFormat = "windows/%s"
)

// BuildUVMOptions creates either LCOW or WCOW options from Specs.
// Defaults are set by NewDefaultOptionsLCOW/NewDefaultOptionsWCOW and
// then overridden by any fields present in the proto.
func BuildUVMOptions(ctx context.Context, spec *Specs, id, owner string) (*uvm.OptionsLCOW, *uvm.OptionsWCOW, *platforms.Platform, error) {
	if spec == nil {
		return nil, nil, nil, fmt.Errorf("nil sandbox specs")
	}

	switch isolation := spec.IsolationLevel.(type) {
	case *Specs_Process:
		// Process isolation: no UVM to create.
		return nil, nil, nil, fmt.Errorf("uvm options cannot be created for process isolation")

	case *Specs_Hypervisor:
		hypervisor := isolation.Hypervisor
		if hypervisor == nil {
			return nil, nil, nil, fmt.Errorf("hypervisor section is nil for isolation_level=hypervisor")
		}

		switch platform := hypervisor.Platform.(type) {
		case *HypervisorIsolated_Lcow:
			if platform.Lcow == nil {
				return nil, nil, nil, fmt.Errorf("lcow params are nil for isolation_level=hypervisor")
			}

			var plat *platforms.Platform
			var err error

			optionsLCOW := uvm.NewDefaultOptionsLCOW(id, owner)

			// Platform-specific overlays
			if lb := platform.Lcow.GetLinuxBootOptions(); lb != nil {
				applyLinuxBootOptions(ctx, optionsLCOW, lb)
			}
			if lg := platform.Lcow.GetLinuxGuestOptions(); lg != nil {
				applyLinuxGuestOptions(optionsLCOW, lg)
			}
			if ld := platform.Lcow.GetLinuxDeviceOptions(); ld != nil {
				if err = applyLinuxDeviceOptions(ctx, optionsLCOW, ld); err != nil {
					return nil, nil, nil, err
				}
			}

			// Common overlays
			if cpu := hypervisor.GetCpuConfig(); cpu != nil {
				applyCPUConfig(optionsLCOW.Options, cpu)
				plat, err = parsePlatform(cpu, true)
				if err != nil {
					return nil, nil, nil, err
				}
			}
			if mem := hypervisor.GetMemoryConfig(); mem != nil {
				applyMemoryConfig(optionsLCOW.Options, mem)

				if hypervisor.MemoryConfig.FullyPhysicallyBacked != nil && *hypervisor.MemoryConfig.FullyPhysicallyBacked {
					optionsLCOW.AllowOvercommit = false
					optionsLCOW.VPMemDeviceCount = 0
				}
			}
			if sto := hypervisor.GetStorageConfig(); sto != nil {
				applyStorageConfig(optionsLCOW.Options, sto)
			}
			if numa := hypervisor.GetNumaConfig(); numa != nil {
				applyNUMAConfig(optionsLCOW.Options, numa)
			}

			if add := hypervisor.GetAdditionalConfig(); add != nil {
				applyAdditionalConfig(ctx, optionsLCOW.Options, add)
			}

			if err = applyHypervisorConfig(optionsLCOW.Options, hypervisor); err != nil {
				return nil, nil, nil, err
			}
			// LCOW Confidential options
			if lc := platform.Lcow.GetConfidentialOptions(); lc != nil {
				applyLCOWConfidentialOptions(optionsLCOW, lc)
			}

			return optionsLCOW, nil, plat, nil

		case *HypervisorIsolated_Wcow:
			if platform.Wcow == nil {
				return nil, nil, nil, fmt.Errorf("wcow params are nil for isolation_level=hypervisor")
			}

			var plat *platforms.Platform
			var err error

			optionsWCOW := uvm.NewDefaultOptionsWCOW(id, owner)

			// Platform-specific overlays
			wb := platform.Wcow.GetWindowsBootOptions()
			if wb != nil {
				applyWindowsBootOptions(optionsWCOW, wb)
			}

			wg := platform.Wcow.GetWindowsGuestOptions()
			if wg != nil {
				if err = applyWindowsGuestOptions(ctx, optionsWCOW, wg); err != nil {
					return nil, nil, nil, err
				}
			}

			// Common overlays
			if cpu := hypervisor.GetCpuConfig(); cpu != nil {
				applyCPUConfig(optionsWCOW.Options, cpu)
				plat, err = parsePlatform(cpu, false)
				if err != nil {
					return nil, nil, nil, err
				}
			}
			if mem := hypervisor.GetMemoryConfig(); mem != nil {
				applyMemoryConfig(optionsWCOW.Options, mem)

				if hypervisor.MemoryConfig.FullyPhysicallyBacked != nil && *hypervisor.MemoryConfig.FullyPhysicallyBacked {
					optionsWCOW.AllowOvercommit = false
				}
			}
			if sto := hypervisor.GetStorageConfig(); sto != nil {
				applyStorageConfig(optionsWCOW.Options, sto)
			}
			if numa := hypervisor.GetNumaConfig(); numa != nil {
				applyNUMAConfig(optionsWCOW.Options, numa)
			}
			if add := hypervisor.GetAdditionalConfig(); add != nil {
				applyAdditionalConfig(ctx, optionsWCOW.Options, add)
			}

			if err = applyHypervisorConfig(optionsWCOW.Options, hypervisor); err != nil {
				return nil, nil, nil, err
			}

			// WCOW Confidential options
			if wc := platform.Wcow.GetConfidentialOptions(); wc != nil {
				err = applyWCOWConfidentialOptions(optionsWCOW, wc, wg, hypervisor.GetMemoryConfig())
				if err != nil {
					return nil, nil, nil, err
				}
			}

			return nil, optionsWCOW, plat, nil

		default:
			return nil, nil, nil, fmt.Errorf("hypervisor.platform must be LCOW or WCOW")
		}

	default:
		return nil, nil, nil, fmt.Errorf("unknown isolation_level")
	}
}

// -----------------------------------------------------------------------------
// Common overlays
// -----------------------------------------------------------------------------

func applyCPUConfig(common *uvm.Options, cpu *CPUConfig) {
	// processor_count: unset => keep defaults; set==0 => error
	if cpu.ProcessorCount != nil && *cpu.ProcessorCount > 0 {
		common.ProcessorCount = *cpu.ProcessorCount
	}

	// processor_limit: unset => keep defaults; set==0 => error
	if cpu.ProcessorLimit != nil && *cpu.ProcessorLimit > 0 {
		common.ProcessorLimit = *cpu.ProcessorLimit
	}

	// processor_weight: unset => keep defaults; set==0 => error
	if cpu.ProcessorWeight != nil && *cpu.ProcessorWeight > 0 {
		common.ProcessorWeight = *cpu.ProcessorWeight
	}
}

func parsePlatform(cpu *CPUConfig, isLinux bool) (*platforms.Platform, error) {
	arch := runtime.GOARCH
	if cpu.Architecture != nil {
		arch = *cpu.Architecture
	}

	var plat platforms.Platform
	var err error
	if isLinux {
		plat, err = platforms.Parse(fmt.Sprintf(linuxPlatformFormat, arch))
	} else {
		plat, err = platforms.Parse(fmt.Sprintf(windowsPlatformFormat, arch))
	}

	if err != nil {
		return nil, fmt.Errorf("failed to parse platform: %w", err)
	}

	return &plat, nil
}

func applyMemoryConfig(common *uvm.Options, mem *MemoryConfig) {
	// Additional check to ensure that Memory Size is non-zero.
	if mem.MemorySizeInMb != nil && *mem.MemorySizeInMb > 0 {
		common.MemorySizeInMB = *mem.MemorySizeInMb
	}

	// MMIO: only overlay when non-zero in proto (your defaults are zero unless tuned)
	setU64(&common.LowMMIOGapInMB, mem.LowMmioGapInMb)
	setU64(&common.HighMMIOBaseInMB, mem.HighMmioBaseInMb)
	setU64(&common.HighMMIOGapInMB, mem.HighMmioGapInMb)

	setBool(&common.AllowOvercommit, mem.AllowOvercommit)
	setBool(&common.FullyPhysicallyBacked, mem.FullyPhysicallyBacked)
	setBool(&common.EnableDeferredCommit, mem.EnableDeferredCommit)
}

func applyStorageConfig(common *uvm.Options, sto *StorageConfig) {
	if sto.StorageQosIopsMaximum != nil && *sto.StorageQosIopsMaximum > 0 {
		common.StorageQoSIopsMaximum = *sto.StorageQosIopsMaximum
	}

	if sto.StorageQosBandwidthMaximum != nil && *sto.StorageQosBandwidthMaximum > 0 {
		common.StorageQoSBandwidthMaximum = *sto.StorageQosBandwidthMaximum
	}

	setBool(&common.NoWritableFileShares, sto.NoWritableFileShares)
}

func applyNUMAConfig(common *uvm.Options, n *NUMAConfig) {
	setU32(&common.MaxProcessorsPerNumaNode, n.MaxProcessorsPerNumaNode)
	setU64(&common.MaxMemorySizePerNumaNode, n.MaxMemorySizePerNumaNode)

	if len(n.PreferredPhysicalNumaNodes) > 0 {
		common.PreferredPhysicalNumaNodes = copyU32(n.PreferredPhysicalNumaNodes)
	}
	if len(n.NumaMappedPhysicalNodes) > 0 {
		common.NumaMappedPhysicalNodes = copyU32(n.NumaMappedPhysicalNodes)
	}
	if len(n.NumaProcessorCounts) > 0 {
		common.NumaProcessorCounts = copyU32(n.NumaProcessorCounts)
	}
	if len(n.NumaMemoryBlocksCounts) > 0 {
		common.NumaMemoryBlocksCounts = copyU64(n.NumaMemoryBlocksCounts)
	}
}

func applyAdditionalConfig(ctx context.Context, common *uvm.Options, a *AdditionalConfig) {
	setStr(&common.NetworkConfigProxy, a.NetworkConfigProxy)
	setStr(&common.ProcessDumpLocation, a.ProcessDumpLocation)
	setStr(&common.DumpDirectoryPath, a.DumpDirectoryPath)
	setStr(&common.ConsolePipe, a.ConsolePipe)

	if len(a.AdditionalHypervConfig) > 0 {
		maps.Copy(common.AdditionalHyperVConfig, parseHVSocketServiceTable(ctx, a.AdditionalHypervConfig))
	}
}

func parseHVSocketServiceTable(ctx context.Context, hyperVConfig map[string]*HvSocketServiceConfig) map[string]hcsschema.HvSocketServiceConfig {
	parsedServiceTable := make(map[string]hcsschema.HvSocketServiceConfig)

	for key, val := range hyperVConfig {
		parsedGUID, err := guid.FromString(key)
		if err != nil {
			log.G(ctx).WithError(err).Warn("invalid GUID string for Hyper-V socket service configuration annotation")
			continue
		}
		guidStr := parsedGUID.String() // overwrite the GUID string to standardize format (capitalization)

		cfg := hcsschema.HvSocketServiceConfig{}
		setStr(&cfg.BindSecurityDescriptor, val.BindSecurityDescriptor)
		setStr(&cfg.ConnectSecurityDescriptor, val.ConnectSecurityDescriptor)
		setBool(&cfg.AllowWildcardBinds, val.AllowWildcardBinds)
		setBool(&cfg.Disabled, val.Disabled)

		if _, found := parsedServiceTable[guidStr]; found {
			log.G(ctx).WithFields(logrus.Fields{
				"guid": guidStr,
			}).Warn("overwritting existing Hyper-V socket service configuration")
		}

		if log.G(ctx).Logger.IsLevelEnabled(logrus.TraceLevel) {
			log.G(ctx).WithField("configuration", log.Format(ctx, cfg)).Trace("found Hyper-V socket service configuration annotation")
		}

		parsedServiceTable[guidStr] = cfg
	}

	return parsedServiceTable
}

func applyHypervisorConfig(common *uvm.Options, hypervisor *HypervisorIsolated) error {
	setStr(&common.CPUGroupID, hypervisor.CpuGroupID)

	if hypervisor.ResourcePartitionID != nil {
		resourcePartitionID := *hypervisor.ResourcePartitionID
		resourcePartitionIDGUID, err := guid.FromString(resourcePartitionID)
		if err != nil {
			return fmt.Errorf("failed to parse resource partition id %q to GUID: %w", resourcePartitionID, err)
		}
		common.ResourcePartitionID = &resourcePartitionIDGUID
	}

	return nil
}

// -----------------------------------------------------------------------------
// LCOW overlays
// -----------------------------------------------------------------------------

func applyLinuxBootOptions(ctx context.Context, opts *uvm.OptionsLCOW, lb *LinuxBootOptions) {
	setBool(&opts.EnableColdDiscardHint, lb.EnableColdDiscardHint)

	if lb.BootFilesPath != nil {
		// Prefer the helper to update associated fields automatically.
		opts.UpdateBootFilesPath(ctx, *lb.BootFilesPath)
	}

	setStr(&opts.KernelBootOptions, lb.KernelBootOptions)

	setBool(&opts.KernelDirect, lb.KernelDirect)
	if !opts.KernelDirect {
		opts.KernelFile = uvm.KernelFile
	}

	if lb.PreferredRootFsType != nil {
		switch *lb.PreferredRootFsType {
		case PreferredRootFSType_PREFERRED_ROOT_FS_TYPE_INITRD:
			opts.PreferredRootFSType = uvm.PreferredRootFSTypeInitRd
		case PreferredRootFSType_PREFERRED_ROOT_FS_TYPE_VHD:
			opts.PreferredRootFSType = uvm.PreferredRootFSTypeVHD
		default:
			log.G(ctx).Warn("PreferredRootFsType must be 'initrd' or 'vhd'")
		}
	}

	switch opts.PreferredRootFSType {
	case uvm.PreferredRootFSTypeInitRd:
		opts.RootFSFile = uvm.InitrdFile
	case uvm.PreferredRootFSTypeVHD:
		opts.RootFSFile = uvm.VhdFile
	}

	// HCL is presence-aware
	if lb.HclEnabled != nil {
		val := *lb.HclEnabled
		opts.HclEnabled = &val
	} else {
		opts.HclEnabled = nil
	}
}

func applyLinuxGuestOptions(opts *uvm.OptionsLCOW, lg *LinuxGuestOptions) {
	setBool(&opts.DisableTimeSyncService, lg.DisableTimeSyncService)

	if len(lg.ExtraVsockPorts) > 0 {
		opts.ExtraVSockPorts = copyU32(lg.ExtraVsockPorts)
	}
	setBool(&opts.PolicyBasedRouting, lg.PolicyBasedRouting)
	setBool(&opts.WritableOverlayDirs, lg.WritableOverlayDirs)
}

func applyLinuxDeviceOptions(ctx context.Context, opts *uvm.OptionsLCOW, ld *LinuxDeviceOptions) error {
	setU32(&opts.VPMemDeviceCount, ld.VpMemDeviceCount)
	setU64(&opts.VPMemSizeBytes, ld.VpMemSizeBytes)
	setBool(&opts.VPMemNoMultiMapping, ld.VpMemNoMultiMapping)
	setBool(&opts.VPCIEnabled, ld.VpciEnabled)

	windowsDevices := make([]specs.WindowsDevice, 0, len(ld.AssignedDevices))
	for _, device := range ld.AssignedDevices {
		windowsDevices = append(windowsDevices, specs.WindowsDevice{
			ID:     device.ID,
			IDType: device.IdType,
		})
	}
	opts.AssignedDevices = oci.ParseDevices(ctx, windowsDevices)

	return nil
}

func applyLCOWConfidentialOptions(opts *uvm.OptionsLCOW, lc *LCOWConfidentialOptions) {
	if lc.Options != nil {
		applyCommonConfidentialLCOW(opts.ConfidentialLCOWOptions, lc.Options)
	}

	setStr(&opts.GuestStateFile, lc.Options.GuestStateFile)
	setStr(&opts.DmVerityRootFsVhd, lc.DmVerityRootFsVhd)
	setBool(&opts.DmVerityMode, lc.DmVerityMode)
	setStr(&opts.DmVerityCreateArgs, lc.DmVerityCreateArgs)

	if lc.Options.NoSecurityHardware != nil {
		oci.HandleLCOWSecurityPolicyWithNoSecurityHardware(*lc.Options.NoSecurityHardware, opts)
	}

	if len(opts.SecurityPolicy) > 0 {
		opts.EnableScratchEncryption = true
		setBool(&opts.EnableScratchEncryption, lc.EnableScratchEncryption)
	}
}

// -----------------------------------------------------------------------------
// WCOW overlays
// -----------------------------------------------------------------------------

func applyWindowsBootOptions(opts *uvm.OptionsWCOW, wb *WindowsBootOptions) {
	setBool(&opts.DisableCompartmentNamespace, wb.DisableCompartmentNamespace)
	setBool(&opts.NoDirectMap, wb.NoDirectMap)
}

func applyWindowsGuestOptions(ctx context.Context, opts *uvm.OptionsWCOW, wg *WindowsGuestOptions) error {
	setBool(&opts.NoInheritHostTimezone, wg.NoInheritHostTimezone)

	if len(wg.AdditionalRegistryKeys) != 0 {
		opts.AdditionalRegistryKeys = append(opts.AdditionalRegistryKeys,
			oci.ValidateAndFilterRegistryValues(ctx, registryValuesFromProto(wg))...)
	}

	setBool(&opts.ForwardLogs, wg.ForwardLogs)
	setStr(&opts.LogSources, wg.LogSources)

	return nil
}

func applyWCOWConfidentialOptions(
	opts *uvm.OptionsWCOW,
	wc *WCOWConfidentialOptions,
	guestConfig *WindowsGuestOptions,
	memConfig *MemoryConfig) error {
	setStr(&opts.SecurityPolicy, wc.Options.SecurityPolicy)

	if len(opts.SecurityPolicy) > 0 {
		if wc.Options != nil {
			applyCommonConfidentialWCOW(opts.ConfidentialWCOWOptions, wc.Options)
		}

		opts.SecurityPolicyEnabled = true
		setBool(&opts.DisableSecureBoot, wc.DisableSecureBoot)

		// overcommit isn't allowed when running in confidential mode and minimum of 2GB memory is required.
		// We can change default values here, but if user provided specific values in annotations we should error out.
		if memConfig == nil || memConfig.GetMemorySizeInMb() == 0 {
			// No memory config provided, set to minimum required.
			opts.MemorySizeInMB = 2048
		}
		if opts.MemorySizeInMB < 2048 {
			return fmt.Errorf("minimum 2048MB of memory is required for confidential pods, got: %d", opts.MemorySizeInMB)
		}

		opts.IsolationType = "SecureNestedPaging"
		if wc.Options.NoSecurityHardware != nil && *wc.Options.NoSecurityHardware {
			opts.IsolationType = "GuestStateOnly"
		}
		setStr(&opts.IsolationType, wc.IsolationType)
		err := oci.HandleWCOWIsolationType(opts.IsolationType, opts)
		if err != nil {
			return err
		}

		if guestConfig == nil || guestConfig.ForwardLogs == nil {
			// Disable log forwarding by default for confidential containers.
			opts.ForwardLogs = false
		}
	}

	setBool(&opts.WritableEFI, wc.WritableEfi)

	return nil
}

// -----------------------------------------------------------------------------
// Confidential (common)
// -----------------------------------------------------------------------------

func applyCommonConfidentialLCOW(opts *uvm.ConfidentialLCOWOptions, c *ConfidentialOptions) {
	setStr(&opts.SecurityPolicy, c.SecurityPolicy)
	setStr(&opts.SecurityPolicyEnforcer, c.SecurityPolicyEnforcer)
	setStr(&opts.UVMReferenceInfoFile, c.UvmReferenceInfoFile)
}

func applyCommonConfidentialWCOW(opts *uvm.ConfidentialWCOWOptions, c *ConfidentialOptions) {
	setStr(&opts.GuestStateFilePath, c.GuestStateFile)
	setStr(&opts.SecurityPolicyEnforcer, c.SecurityPolicyEnforcer)
	setStr(&opts.UVMReferenceInfoFile, c.UvmReferenceInfoFile)
}

// Proto adapter: map proto values to hcsschema.RegistryValue.
func registryValuesFromProto(wgo *WindowsGuestOptions) []hcsschema.RegistryValue {
	if wgo == nil || len(wgo.AdditionalRegistryKeys) == 0 {
		return []hcsschema.RegistryValue{}
	}

	out := make([]hcsschema.RegistryValue, 0, len(wgo.AdditionalRegistryKeys))
	for _, pv := range wgo.AdditionalRegistryKeys {
		if pv == nil {
			continue
		}

		var key *hcsschema.RegistryKey
		if pv.Key != nil {
			key = &hcsschema.RegistryKey{
				Hive:     mapProtoHive(pv.Key.Hive),
				Name:     strings.TrimSpace(pv.Key.Name),
				Volatile: pv.Key.Volatile,
			}
		}

		out = append(out, hcsschema.RegistryValue{
			Key:         key,
			Name:        strings.TrimSpace(pv.Name),
			Type_:       mapProtoRegValueType(pv.Type),
			StringValue: pv.StringValue,
			BinaryValue: pv.BinaryValue,
			DWordValue:  pv.DwordValue,
			QWordValue:  pv.QwordValue,
			CustomType:  pv.CustomType,
		})
	}
	return out
}

// -----------------------------------------------------------------------------
// Small utilities to reduce nil-check boilerplate
// -----------------------------------------------------------------------------

// Map proto RegistryHive -> hcsschema.RegistryHive.
func mapProtoHive(h RegistryHive) hcsschema.RegistryHive {
	switch h {
	case RegistryHive_REGISTRY_HIVE_SYSTEM:
		return hcsschema.RegistryHive_SYSTEM
	case RegistryHive_REGISTRY_HIVE_SOFTWARE:
		return hcsschema.RegistryHive_SOFTWARE
	case RegistryHive_REGISTRY_HIVE_SECURITY:
		return hcsschema.RegistryHive_SECURITY
	case RegistryHive_REGISTRY_HIVE_SAM:
		return hcsschema.RegistryHive_SAM
	default:
		return hcsschema.RegistryHive_SYSTEM
	}
}

// Map proto RegistryValueType -> hcsschema.RegistryValueType.
func mapProtoRegValueType(t RegistryValueType) hcsschema.RegistryValueType {
	switch t {
	case RegistryValueType_REGISTRY_VALUE_TYPE_NONE:
		return hcsschema.RegistryValueType_NONE
	case RegistryValueType_REGISTRY_VALUE_TYPE_STRING:
		return hcsschema.RegistryValueType_STRING
	case RegistryValueType_REGISTRY_VALUE_TYPE_EXPANDED_STRING:
		return hcsschema.RegistryValueType_EXPANDED_STRING
	case RegistryValueType_REGISTRY_VALUE_TYPE_MULTI_STRING:
		return hcsschema.RegistryValueType_MULTI_STRING
	case RegistryValueType_REGISTRY_VALUE_TYPE_BINARY:
		return hcsschema.RegistryValueType_BINARY
	case RegistryValueType_REGISTRY_VALUE_TYPE_D_WORD:
		return hcsschema.RegistryValueType_D_WORD
	case RegistryValueType_REGISTRY_VALUE_TYPE_Q_WORD:
		return hcsschema.RegistryValueType_Q_WORD
	case RegistryValueType_REGISTRY_VALUE_TYPE_CUSTOM_TYPE:
		return hcsschema.RegistryValueType_CUSTOM_TYPE
	default:
		return hcsschema.RegistryValueType_NONE
	}
}

// setStr sets dst to the value of src if src is non-nil.
func setStr(dst *string, src *string) {
	if src != nil {
		*dst = *src
	}
}

// setBool sets dst to the value of src if src is non-nil.
func setBool(dst *bool, src *bool) {
	if src != nil {
		*dst = *src
	}
}

// setU32 sets dst to the value of src if src is non-nil.
func setU32(dst *uint32, src *uint32) {
	if src != nil {
		*dst = *src
	}
}

// setU64 sets dst to the value of src if src is non-nil.
func setU64(dst *uint64, src *uint64) {
	if src != nil {
		*dst = *src
	}
}

// copyU32 returns a copy of in.
func copyU32(in []uint32) []uint32 {
	out := make([]uint32, len(in))
	copy(out, in)
	return out
}

// copyU64 returns a copy of in.
func copyU64(in []uint64) []uint64 {
	out := make([]uint64, len(in))
	copy(out, in)
	return out
}
