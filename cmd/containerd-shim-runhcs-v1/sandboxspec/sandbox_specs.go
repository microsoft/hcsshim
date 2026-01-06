package sandboxspec

import (
	"context"
	"fmt"
	"strings"

	runhcsoptions "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	iannotations "github.com/Microsoft/hcsshim/internal/annotations"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/oci"
	shimannotations "github.com/Microsoft/hcsshim/pkg/annotations"

	"github.com/opencontainers/runtime-spec/specs-go"
)

// Generate generates Specs based on provided options and annotations.
// These specifications define the configuration for the sandbox environment.
func Generate(
	opts *runhcsoptions.Options,
	annotations map[string]string,
	devices []specs.WindowsDevice,
) (*Specs, error) {
	if opts == nil {
		return nil, fmt.Errorf("no runhcs options provided")
	}

	// Decide Isolation based on options: PROCESS vs HYPERVISOR
	switch opts.SandboxIsolation {
	case runhcsoptions.Options_PROCESS:
		// Windows process isolation -> no UVM
		return &Specs{
			IsolationLevel: &Specs_Process{
				Process: &ProcessIsolated{},
			},
		}, nil

	case runhcsoptions.Options_HYPERVISOR:
		ctx := context.Background()
		// UVM-backed isolation
		osName, arch, err := splitPlatform(opts.SandboxPlatform)
		if err != nil {
			return nil, fmt.Errorf("failed to parse platform: %w", err)
		}

		// Process annotations prior to parsing them into Specs.
		err = processAnnotations(ctx, opts, annotations)
		if err != nil {
			return nil, fmt.Errorf("failed to process annotations: %w", err)
		}

		// Create HypervisorIsolated spec
		hyper := &HypervisorIsolated{}

		// CPU Configuration
		cpuConfig, err := parseCPUParameters(ctx, opts, annotations, arch)
		if err != nil {
			return nil, fmt.Errorf("failed to parse CPU parameters: %w", err)
		}

		hyper.CpuConfig = cpuConfig

		// Memory Configuration
		memoryConfig, err := parseMemoryParameters(ctx, opts, annotations)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Memory parameters: %w", err)
		}

		hyper.MemoryConfig = memoryConfig

		// Storage Configuration
		storageConfig, err := parseStorageParameters(ctx, annotations)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Storage parameters: %w", err)
		}

		hyper.StorageConfig = storageConfig

		// NUMA Configuration
		numaConfig, err := parseNUMAParameters(ctx, annotations)
		if err != nil {
			return nil, fmt.Errorf("failed to parse NUMA parameters: %w", err)
		}

		hyper.NumaConfig = numaConfig

		// Additional Configurations
		additionalConfig, err := parseAdditionalConfigurations(ctx, opts, annotations)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Additional parameters: %w", err)
		}

		hyper.AdditionalConfig = additionalConfig

		// CPU group configuration
		cpuGroupID := oci.ParseAnnotationsString(annotations, shimannotations.CPUGroupID, "")
		if cpuGroupID != "" {
			hyper.CpuGroupID = &cpuGroupID
		}

		// Resource Partition ID
		resourcePartitionID := oci.ParseAnnotationsString(annotations, shimannotations.ResourcePartitionID, "")
		if resourcePartitionID != "" {
			hyper.ResourcePartitionID = &resourcePartitionID
		}

		switch osName {
		case "linux":
			// LCOW platform-specific options
			lcow := &LinuxHyperVOptions{}

			// Linux Boot Options
			bootOptions, err := parseLinuxBootOptions(ctx, opts, annotations)
			if err != nil {
				return nil, fmt.Errorf("failed to parse Linux boot options: %w", err)
			}

			lcow.LinuxBootOptions = bootOptions

			guestOptions, err := parseLinuxGuestOptions(ctx, annotations)
			if err != nil {
				return nil, fmt.Errorf("failed to parse Linux guest options: %w", err)
			}

			lcow.LinuxGuestOptions = guestOptions

			deviceOptions, err := parseLinuxDeviceOptions(ctx, annotations, devices)
			if err != nil {
				return nil, fmt.Errorf("failed to parse Linux device options: %w", err)
			}

			lcow.LinuxDeviceOptions = deviceOptions

			confidentialOptions, err := parseLinuxConfidentialOptions(ctx, annotations)
			if err != nil {
				return nil, fmt.Errorf("failed to parse Linux confidential options: %w", err)
			}

			lcow.ConfidentialOptions = confidentialOptions

			hyper.Platform = &HypervisorIsolated_Lcow{lcow}

		case "windows":
			// WCOW platform-specific options
			wcow := &WindowsHyperVOptions{}

			bootOptions, err := parseWindowsBootOptions(ctx, annotations)
			if err != nil {
				return nil, fmt.Errorf("failed to parse Windows boot options: %w", err)
			}

			wcow.WindowsBootOptions = bootOptions

			guestOptions, err := parseWindowsGuestOptions(ctx, annotations)
			if err != nil {
				return nil, fmt.Errorf("failed to parse Windows guest options: %w", err)
			}

			wcow.WindowsGuestOptions = guestOptions

			confidentialOptions, err := parseWindowsConfidentialOptions(ctx, annotations)
			if err != nil {
				return nil, fmt.Errorf("failed to parse Windows confidential options: %w", err)
			}

			wcow.ConfidentialOptions = confidentialOptions

			hyper.Platform = &HypervisorIsolated_Wcow{wcow}

		default:
			return nil, fmt.Errorf("unsupported sandbox platform os: %s", osName)
		}

		return &Specs{
			IsolationLevel: &Specs_Hypervisor{
				Hypervisor: hyper,
			},
		}, nil

	default:
		return nil, fmt.Errorf("unsupported sandbox_isolation: %v", opts.SandboxIsolation)
	}
}

// processAnnotations applies default annotations and processes them.
func processAnnotations(ctx context.Context, opts *runhcsoptions.Options, annotations map[string]string) error {
	// Apply default annotations.
	for key, value := range opts.DefaultContainerAnnotations {
		// Only set default if not already set in annotations
		if _, exists := annotations[key]; !exists {
			annotations[key] = value
		}
	}

	err := oci.ProcessAnnotations(ctx, annotations)
	if err != nil {
		return err
	}

	return nil
}

// parseCPUParameters parses CPU related parameters from annotations and options.
func parseCPUParameters(ctx context.Context, opts *runhcsoptions.Options, annotations map[string]string, arch string) (*CPUConfig, error) {
	cpu := &CPUConfig{}

	if _, ok := annotations[shimannotations.ProcessorCount]; ok {
		cpuCount := oci.ParseAnnotationsInt32(ctx, annotations, shimannotations.ProcessorCount, 0)
		if cpuCount != 0 {
			cpu.ProcessorCount = &cpuCount
		}
	} else if opts.VmProcessorCount != 0 {
		cpu.ProcessorCount = &opts.VmProcessorCount
	}

	cpuLimit := oci.ParseAnnotationsInt32(ctx, annotations, shimannotations.ProcessorLimit, 0)
	if cpuLimit != 0 {
		cpu.ProcessorLimit = &cpuLimit
	}

	cpuWeight := oci.ParseAnnotationsInt32(ctx, annotations, shimannotations.ProcessorWeight, 0)
	if cpuWeight != 0 {
		cpu.ProcessorWeight = &cpuWeight
	}

	if arch != "" {
		cpu.Architecture = &arch
	}

	return cpu, nil
}

// parseMemoryParameters parses memory related parameters from annotations and options.
func parseMemoryParameters(ctx context.Context, opts *runhcsoptions.Options, annotations map[string]string) (*MemoryConfig, error) {
	mem := &MemoryConfig{}

	if _, ok := annotations[shimannotations.MemorySizeInMB]; ok {
		memorySizeMB := oci.ParseAnnotationsUint64(ctx, annotations, shimannotations.MemorySizeInMB, 0)
		if memorySizeMB != 0 {
			mem.MemorySizeInMb = &memorySizeMB
		}
	} else if opts.VmMemorySizeInMb != 0 {
		memorySizeMB := uint64(opts.VmMemorySizeInMb)
		mem.MemorySizeInMb = &memorySizeMB
	}

	lowMMIOGapInMB := oci.ParseAnnotationsUint64(ctx, annotations, shimannotations.MemoryLowMMIOGapInMB, 0)
	if lowMMIOGapInMB != 0 {
		mem.LowMmioGapInMb = &lowMMIOGapInMB
	}

	highMMIOBaseInMB := oci.ParseAnnotationsUint64(ctx, annotations, shimannotations.MemoryHighMMIOBaseInMB, 0)
	if highMMIOBaseInMB != 0 {
		mem.HighMmioBaseInMb = &highMMIOBaseInMB
	}

	highMMIOGapInMB := oci.ParseAnnotationsUint64(ctx, annotations, shimannotations.MemoryHighMMIOGapInMB, 0)
	if highMMIOGapInMB != 0 {
		mem.HighMmioGapInMb = &highMMIOGapInMB
	}

	allowOvercommit := oci.ParseAnnotationsNullableBool(ctx, annotations, shimannotations.AllowOvercommit)
	mem.AllowOvercommit = allowOvercommit

	enableDeferredCommit := oci.ParseAnnotationsNullableBool(ctx, annotations, shimannotations.EnableDeferredCommit)
	mem.EnableDeferredCommit = enableDeferredCommit

	fullyPhysicallyBacked := oci.ParseAnnotationsNullableBool(ctx, annotations, shimannotations.FullyPhysicallyBacked)
	mem.FullyPhysicallyBacked = fullyPhysicallyBacked

	return mem, nil
}

// parseStorageParameters parses storage related parameters from annotations and options.
func parseStorageParameters(ctx context.Context, annotations map[string]string) (*StorageConfig, error) {
	storage := &StorageConfig{}

	noWritableFileShares := oci.ParseAnnotationsNullableBool(ctx, annotations, shimannotations.DisableWritableFileShares)
	storage.NoWritableFileShares = noWritableFileShares

	storageQosBandwidthMaximum := oci.ParseAnnotationsInt32(ctx, annotations, shimannotations.StorageQoSBandwidthMaximum, 0)
	if storageQosBandwidthMaximum != 0 {
		storage.StorageQosBandwidthMaximum = &storageQosBandwidthMaximum
	}

	storageQosIopsMaximum := oci.ParseAnnotationsInt32(ctx, annotations, shimannotations.StorageQoSIopsMaximum, 0)
	if storageQosIopsMaximum != 0 {
		storage.StorageQosIopsMaximum = &storageQosIopsMaximum
	}

	return storage, nil
}

// parseNUMAParameters parses NUMA related parameters from annotations.
func parseNUMAParameters(ctx context.Context, annotations map[string]string) (*NUMAConfig, error) {
	numa := &NUMAConfig{}

	maxProcessorsPerNumaNode := oci.ParseAnnotationsUint32(ctx, annotations, shimannotations.NumaMaximumProcessorsPerNode, 0)
	if maxProcessorsPerNumaNode != 0 {
		numa.MaxProcessorsPerNumaNode = &maxProcessorsPerNumaNode
	}

	maxMemorySizePerNumaNode := oci.ParseAnnotationsUint64(ctx, annotations, shimannotations.NumaMaximumMemorySizePerNode, 0)
	if maxMemorySizePerNumaNode != 0 {
		numa.MaxMemorySizePerNumaNode = &maxMemorySizePerNumaNode
	}

	preferredPhysicalNumaNodes := oci.ParseAnnotationCommaSeparatedUint32(ctx, annotations, shimannotations.NumaPreferredPhysicalNodes, []uint32{})
	numa.PreferredPhysicalNumaNodes = preferredPhysicalNumaNodes

	numaMappedPhysicalNodes := oci.ParseAnnotationCommaSeparatedUint32(ctx, annotations, shimannotations.NumaMappedPhysicalNodes, []uint32{})
	numa.NumaMappedPhysicalNodes = numaMappedPhysicalNodes

	numaProcessorCounts := oci.ParseAnnotationCommaSeparatedUint32(ctx, annotations, shimannotations.NumaCountOfProcessors, []uint32{})
	numa.NumaProcessorCounts = numaProcessorCounts

	numaMemoryBlocksCount := oci.ParseAnnotationCommaSeparatedUint64(ctx, annotations, shimannotations.NumaCountOfMemoryBlocks, []uint64{})
	numa.NumaMemoryBlocksCounts = numaMemoryBlocksCount

	return numa, nil
}

// parseAdditionalConfigurations parses additional configurations from annotations and options.
func parseAdditionalConfigurations(ctx context.Context, opts *runhcsoptions.Options, annotations map[string]string) (*AdditionalConfig, error) {
	additionalConfig := &AdditionalConfig{}

	networkConfigProxy := oci.ParseAnnotationsString(annotations, shimannotations.NetworkConfigProxy, "")
	if networkConfigProxy != "" {
		additionalConfig.NetworkConfigProxy = &networkConfigProxy
	} else if opts.NCProxyAddr != "" {
		additionalConfig.NetworkConfigProxy = &opts.NCProxyAddr
	}

	processDumpLocation := oci.ParseAnnotationsString(annotations, shimannotations.ContainerProcessDumpLocation, "")
	if processDumpLocation != "" {
		additionalConfig.ProcessDumpLocation = &processDumpLocation
	}

	dumpDirectoryPath := oci.ParseAnnotationsString(annotations, shimannotations.DumpDirectoryPath, "")
	if dumpDirectoryPath != "" {
		additionalConfig.DumpDirectoryPath = &dumpDirectoryPath
	}

	consolePipe := oci.ParseAnnotationsString(annotations, iannotations.UVMConsolePipe, "")
	if consolePipe != "" {
		additionalConfig.ConsolePipe = &consolePipe
	}

	hvSocketServiceTable, err := parseHVSocketServiceTableFromAnnotations(ctx, annotations)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HVSocket service table: %w", err)
	}
	additionalConfig.AdditionalHypervConfig = hvSocketServiceTable

	return additionalConfig, nil
}

// parseHVSocketServiceTableFromAnnotations parses HVSocket service table from annotations.
func parseHVSocketServiceTableFromAnnotations(ctx context.Context, annotations map[string]string) (map[string]*HvSocketServiceConfig, error) {
	hcsHvSocketServiceTable := oci.ParseHVSocketServiceTable(ctx, annotations)
	if len(hcsHvSocketServiceTable) == 0 {
		return make(map[string]*HvSocketServiceConfig), nil
	}

	sc := make(map[string]*HvSocketServiceConfig, len(hcsHvSocketServiceTable))
	for name, entry := range hcsHvSocketServiceTable {
		conf := &HvSocketServiceConfig{}
		conf.BindSecurityDescriptor = &entry.BindSecurityDescriptor
		conf.ConnectSecurityDescriptor = &entry.ConnectSecurityDescriptor
		conf.AllowWildcardBinds = &entry.AllowWildcardBinds
		conf.Disabled = &entry.Disabled
		sc[name] = conf
	}

	return sc, nil
}

// parseLinuxBootOptions parses Linux boot options from annotations and options.
func parseLinuxBootOptions(ctx context.Context, opts *runhcsoptions.Options, annotations map[string]string) (*LinuxBootOptions, error) {
	bootOptions := &LinuxBootOptions{}

	if _, ok := annotations[shimannotations.BootFilesRootPath]; ok {
		bootFilesRootPath := oci.ParseAnnotationsString(annotations, shimannotations.BootFilesRootPath, "")
		if bootFilesRootPath != "" {
			bootOptions.BootFilesPath = &bootFilesRootPath
		}
	} else if opts.BootFilesRootPath != "" {
		bootOptions.BootFilesPath = &opts.BootFilesRootPath
	}

	kernelDirect := oci.ParseAnnotationsNullableBool(ctx, annotations, shimannotations.KernelDirectBoot)
	bootOptions.KernelDirect = kernelDirect

	kernelBootOptions := oci.ParseAnnotationsString(annotations, shimannotations.KernelBootOptions, "")
	if kernelBootOptions != "" {
		bootOptions.KernelBootOptions = &kernelBootOptions
	}

	enableColdDiscardHint := oci.ParseAnnotationsNullableBool(ctx, annotations, shimannotations.EnableColdDiscardHint)
	bootOptions.EnableColdDiscardHint = enableColdDiscardHint

	hclEnabled := oci.ParseAnnotationsNullableBool(ctx, annotations, shimannotations.LCOWHclEnabled)
	bootOptions.HclEnabled = hclEnabled

	preferredRootfsType := oci.ParseAnnotationsString(annotations, shimannotations.PreferredRootFSType, "")
	if preferredRootfsType != "" {
		var t PreferredRootFSType
		switch preferredRootfsType {
		case "initrd":
			t = PreferredRootFSType_PREFERRED_ROOT_FS_TYPE_INITRD
		case "vhd":
			t = PreferredRootFSType_PREFERRED_ROOT_FS_TYPE_VHD
		default:
			return nil, fmt.Errorf("invalid PreferredRootFSType: %s", preferredRootfsType)
		}
		bootOptions.PreferredRootFsType = &t
	}

	return bootOptions, nil
}

// parseLinuxGuestOptions parses Linux guest options from annotations.
func parseLinuxGuestOptions(ctx context.Context, annotations map[string]string) (*LinuxGuestOptions, error) {
	guestOptions := &LinuxGuestOptions{}

	disableTimeSyncService := oci.ParseAnnotationsNullableBool(ctx, annotations, shimannotations.DisableLCOWTimeSyncService)
	guestOptions.DisableTimeSyncService = disableTimeSyncService

	networkingPolicyBasedRouting := oci.ParseAnnotationsNullableBool(ctx, annotations, iannotations.NetworkingPolicyBasedRouting)
	guestOptions.PolicyBasedRouting = networkingPolicyBasedRouting

	writableOverlayDirs := oci.ParseAnnotationsNullableBool(ctx, annotations, iannotations.WritableOverlayDirs)
	guestOptions.WritableOverlayDirs = writableOverlayDirs

	extraVsockPorts := oci.ParseAnnotationCommaSeparatedUint32(ctx, annotations, iannotations.ExtraVSockPorts, []uint32{})
	guestOptions.ExtraVsockPorts = extraVsockPorts

	return guestOptions, nil
}

// parseLinuxDeviceOptions parses Linux device options from annotations.
func parseLinuxDeviceOptions(ctx context.Context, annotations map[string]string, devices []specs.WindowsDevice) (*LinuxDeviceOptions, error) {
	deviceOptions := &LinuxDeviceOptions{}

	vpMemDeviceCount := oci.ParseAnnotationsUint32(ctx, annotations, shimannotations.VPMemCount, 0)
	if vpMemDeviceCount != 0 {
		deviceOptions.VpMemDeviceCount = &vpMemDeviceCount
	}

	vpMemSizeBytes := oci.ParseAnnotationsUint64(ctx, annotations, shimannotations.VPMemSize, 0)
	if vpMemSizeBytes != 0 {
		deviceOptions.VpMemSizeBytes = &vpMemSizeBytes
	}

	vpMemNoMultiMapping := oci.ParseAnnotationsNullableBool(ctx, annotations, shimannotations.VPMemNoMultiMapping)
	deviceOptions.VpMemNoMultiMapping = vpMemNoMultiMapping

	vpciEnabled := oci.ParseAnnotationsNullableBool(ctx, annotations, shimannotations.VPCIEnabled)
	deviceOptions.VpciEnabled = vpciEnabled

	devicesToAdd := make([]*Device, 0, len(devices))
	for _, dev := range devices {
		devAdd := &Device{
			ID:     dev.ID,
			IdType: dev.IDType,
		}
		devicesToAdd = append(devicesToAdd, devAdd)
	}

	deviceOptions.AssignedDevices = devicesToAdd

	return deviceOptions, nil
}

// parseLinuxConfidentialOptions parses Linux confidential options from annotations.
func parseLinuxConfidentialOptions(ctx context.Context, annotations map[string]string) (*LCOWConfidentialOptions, error) {
	lcowConfidentialOptions := &LCOWConfidentialOptions{}

	confidentialOptions := &ConfidentialOptions{}

	guestStateFile := oci.ParseAnnotationsString(annotations, shimannotations.LCOWGuestStateFile, "")
	if guestStateFile != "" {
		confidentialOptions.GuestStateFile = &guestStateFile
	}

	securityPolicy := oci.ParseAnnotationsString(annotations, shimannotations.LCOWSecurityPolicy, "")
	if securityPolicy != "" {
		confidentialOptions.SecurityPolicy = &securityPolicy
	}

	securityPolicyEnforcer := oci.ParseAnnotationsString(annotations, shimannotations.LCOWSecurityPolicyEnforcer, "")
	if securityPolicyEnforcer != "" {
		confidentialOptions.SecurityPolicyEnforcer = &securityPolicyEnforcer
	}

	uvmReferenceInfoFile := oci.ParseAnnotationsString(annotations, shimannotations.LCOWReferenceInfoFile, "")
	if uvmReferenceInfoFile != "" {
		confidentialOptions.UvmReferenceInfoFile = &uvmReferenceInfoFile
	}

	noSecurityHardware := oci.ParseAnnotationsBool(ctx, annotations, shimannotations.NoSecurityHardware, false)
	confidentialOptions.NoSecurityHardware = &noSecurityHardware

	lcowConfidentialOptions.Options = confidentialOptions

	enableScratchEncryption := oci.ParseAnnotationsNullableBool(ctx, annotations, shimannotations.LCOWEncryptedScratchDisk)
	lcowConfidentialOptions.EnableScratchEncryption = enableScratchEncryption

	dmVerityMode := oci.ParseAnnotationsNullableBool(ctx, annotations, shimannotations.DmVerityMode)
	lcowConfidentialOptions.DmVerityMode = dmVerityMode

	dmVerityRootFsVhd := oci.ParseAnnotationsString(annotations, shimannotations.DmVerityRootFsVhd, "")
	if dmVerityRootFsVhd != "" {
		lcowConfidentialOptions.DmVerityRootFsVhd = &dmVerityRootFsVhd
	}

	dmVerityCreateArgs := oci.ParseAnnotationsString(annotations, shimannotations.DmVerityCreateArgs, "")
	if dmVerityCreateArgs != "" {
		lcowConfidentialOptions.DmVerityCreateArgs = &dmVerityCreateArgs
	}

	return lcowConfidentialOptions, nil
}

// parseWindowsBootOptions parses Windows boot options from annotations.
func parseWindowsBootOptions(ctx context.Context, annotations map[string]string) (*WindowsBootOptions, error) {
	bootOptions := &WindowsBootOptions{}

	disableCompartmentNamespace := oci.ParseAnnotationsNullableBool(ctx, annotations, shimannotations.DisableCompartmentNamespace)
	bootOptions.DisableCompartmentNamespace = disableCompartmentNamespace

	noDirectMap := oci.ParseAnnotationsNullableBool(ctx, annotations, shimannotations.VSMBNoDirectMap)
	bootOptions.NoDirectMap = noDirectMap

	return bootOptions, nil
}

// parseWindowsGuestOptions parses Windows guest options from annotations.
func parseWindowsGuestOptions(ctx context.Context, annotations map[string]string) (*WindowsGuestOptions, error) {
	guestOptions := &WindowsGuestOptions{}

	noInheritTimezone := oci.ParseAnnotationsNullableBool(ctx, annotations, shimannotations.NoInheritHostTimezone)
	guestOptions.NoInheritHostTimezone = noInheritTimezone

	hcsRegistryKeys := oci.ParseAdditionalRegistryValues(ctx, annotations)
	guestOptions.AdditionalRegistryKeys = registryValuesToProto(hcsRegistryKeys)

	forwardLogs := oci.ParseAnnotationsNullableBool(ctx, annotations, shimannotations.ForwardLogs)
	guestOptions.ForwardLogs = forwardLogs

	logSources := oci.ParseAnnotationsString(annotations, shimannotations.LogSources, "")
	if logSources != "" {
		guestOptions.LogSources = &logSources
	}

	return guestOptions, nil
}

// parseWindowsConfidentialOptions parses Windows confidential options from annotations.
func parseWindowsConfidentialOptions(ctx context.Context, annotations map[string]string) (*WCOWConfidentialOptions, error) {
	wcowConfidentialOptions := &WCOWConfidentialOptions{}

	confidentialOptions := &ConfidentialOptions{}

	securityPolicy := oci.ParseAnnotationsString(annotations, shimannotations.WCOWSecurityPolicy, "")
	if securityPolicy != "" {
		confidentialOptions.SecurityPolicy = &securityPolicy
	}

	guestStateFile := oci.ParseAnnotationsString(annotations, shimannotations.WCOWGuestStateFile, "")
	if guestStateFile != "" {
		confidentialOptions.GuestStateFile = &guestStateFile
	}

	securityPolicyEnforcer := oci.ParseAnnotationsString(annotations, shimannotations.WCOWSecurityPolicyEnforcer, "")
	if securityPolicyEnforcer != "" {
		confidentialOptions.SecurityPolicyEnforcer = &securityPolicyEnforcer
	}

	uvmReferenceInfoFile := oci.ParseAnnotationsString(annotations, shimannotations.WCOWReferenceInfoFile, "")
	if uvmReferenceInfoFile != "" {
		confidentialOptions.UvmReferenceInfoFile = &uvmReferenceInfoFile
	}

	noSecurityHardware := oci.ParseAnnotationsBool(ctx, annotations, shimannotations.NoSecurityHardware, false)
	confidentialOptions.NoSecurityHardware = &noSecurityHardware

	wcowConfidentialOptions.Options = confidentialOptions

	writableEFI := oci.ParseAnnotationsNullableBool(ctx, annotations, shimannotations.WCOWWritableEFI)
	wcowConfidentialOptions.WritableEfi = writableEFI

	disableSecureBoot := oci.ParseAnnotationsBool(ctx, annotations, shimannotations.WCOWDisableSecureBoot, false)
	wcowConfidentialOptions.DisableSecureBoot = &disableSecureBoot

	isolationType := oci.ParseAnnotationsString(annotations, shimannotations.WCOWIsolationType, "")
	if isolationType != "" {
		wcowConfidentialOptions.IsolationType = &isolationType
	}

	return wcowConfidentialOptions, nil
}

// -----------------------------------------------------------------------------
// Small utilities to reduce nil-check boilerplate
// -----------------------------------------------------------------------------

// splitPlatform expects "linux/amd64" or "windows/amd64"
// and splits it into osName and arch.
func splitPlatform(p string) (osName, arch string, err error) {
	if p == "" {
		return "", "", fmt.Errorf("sandbox_platform empty; expected \"linux/amd64\" or \"windows/amd64\"")
	}
	parts := strings.Split(p, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid sandbox_platform %q; expected \"linux/amd64\" or \"windows/amd64\"", p)
	}
	return parts[0], parts[1], nil
}

// Convert a slice of hcsschema.RegistryValue -> slice of *proto RegistryValue.
func registryValuesToProto(in []hcsschema.RegistryValue) []*RegistryValue {
	out := make([]*RegistryValue, 0, len(in))
	for _, reg := range in {
		var key *RegistryKey
		if reg.Key != nil {
			key = &RegistryKey{
				Hive:     mapHcsHiveToProto(reg.Key.Hive),
				Name:     strings.TrimSpace(reg.Key.Name),
				Volatile: reg.Key.Volatile,
			}
		}

		rv := &RegistryValue{
			Key:         key,
			Name:        strings.TrimSpace(reg.Name),
			Type:        mapHcsRegValueTypeToProto(reg.Type_),
			StringValue: reg.StringValue,
			BinaryValue: reg.BinaryValue,
			DwordValue:  reg.DWordValue,
			QwordValue:  reg.QWordValue,
			CustomType:  reg.CustomType,
		}

		out = append(out, rv)
	}
	return out
}

// Map hcsschema.RegistryHive -> proto RegistryHive.
func mapHcsHiveToProto(h hcsschema.RegistryHive) RegistryHive {
	switch h {
	case hcsschema.RegistryHive_SYSTEM:
		return RegistryHive_REGISTRY_HIVE_SYSTEM
	case hcsschema.RegistryHive_SOFTWARE:
		return RegistryHive_REGISTRY_HIVE_SOFTWARE
	case hcsschema.RegistryHive_SECURITY:
		return RegistryHive_REGISTRY_HIVE_SECURITY
	case hcsschema.RegistryHive_SAM:
		return RegistryHive_REGISTRY_HIVE_SAM
	default:
		// Choose a sensible default. SYSTEM matches your proto->HCS default.
		return RegistryHive_REGISTRY_HIVE_SYSTEM
	}
}

// Map hcsschema.RegistryValueType -> proto RegistryValueType.
func mapHcsRegValueTypeToProto(t hcsschema.RegistryValueType) RegistryValueType {
	switch t {
	case hcsschema.RegistryValueType_NONE:
		return RegistryValueType_REGISTRY_VALUE_TYPE_NONE
	case hcsschema.RegistryValueType_STRING:
		return RegistryValueType_REGISTRY_VALUE_TYPE_STRING
	case hcsschema.RegistryValueType_EXPANDED_STRING:
		return RegistryValueType_REGISTRY_VALUE_TYPE_EXPANDED_STRING
	case hcsschema.RegistryValueType_MULTI_STRING:
		return RegistryValueType_REGISTRY_VALUE_TYPE_MULTI_STRING
	case hcsschema.RegistryValueType_BINARY:
		return RegistryValueType_REGISTRY_VALUE_TYPE_BINARY
	case hcsschema.RegistryValueType_D_WORD:
		return RegistryValueType_REGISTRY_VALUE_TYPE_D_WORD
	case hcsschema.RegistryValueType_Q_WORD:
		return RegistryValueType_REGISTRY_VALUE_TYPE_Q_WORD
	case hcsschema.RegistryValueType_CUSTOM_TYPE:
		return RegistryValueType_REGISTRY_VALUE_TYPE_CUSTOM_TYPE
	default:
		return RegistryValueType_REGISTRY_VALUE_TYPE_NONE
	}
}
