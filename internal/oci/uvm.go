//go:build windows

package oci

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strconv"

	runhcsopts "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"

	iannotations "github.com/Microsoft/hcsshim/internal/annotations"
	"github.com/Microsoft/hcsshim/internal/devices"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/uvm"
)

// UVM specific annotation parsing

// ParseAnnotationsCPUCount searches `s.Annotations` for the CPU annotation. If
// not found searches `s` for the Windows CPU section. If neither are found
// returns `def`.
func ParseAnnotationsCPUCount(ctx context.Context, s *specs.Spec, annotation string, def int32) int32 {
	if m := ParseAnnotationsInt32(ctx, s.Annotations, annotation, 0); m != 0 {
		return m
	}
	if s.Windows != nil &&
		s.Windows.Resources != nil &&
		s.Windows.Resources.CPU != nil &&
		s.Windows.Resources.CPU.Count != nil &&
		*s.Windows.Resources.CPU.Count > 0 {
		return int32(*s.Windows.Resources.CPU.Count)
	}
	return def
}

// ParseAnnotationsCPULimit searches `s.Annotations` for the CPU annotation. If
// not found searches `s` for the Windows CPU section. If neither are found
// returns `def`.
func ParseAnnotationsCPULimit(ctx context.Context, s *specs.Spec, annotation string, def int32) int32 {
	if m := ParseAnnotationsInt32(ctx, s.Annotations, annotation, 0); m != 0 {
		return m
	}
	if s.Windows != nil &&
		s.Windows.Resources != nil &&
		s.Windows.Resources.CPU != nil &&
		s.Windows.Resources.CPU.Maximum != nil &&
		*s.Windows.Resources.CPU.Maximum > 0 {
		return int32(*s.Windows.Resources.CPU.Maximum)
	}
	return def
}

// ParseAnnotationsCPUWeight searches `s.Annotations` for the CPU annotation. If
// not found searches `s` for the Windows CPU section. If neither are found
// returns `def`.
func ParseAnnotationsCPUWeight(ctx context.Context, s *specs.Spec, annotation string, def int32) int32 {
	if m := ParseAnnotationsInt32(ctx, s.Annotations, annotation, 0); m != 0 {
		return m
	}
	if s.Windows != nil &&
		s.Windows.Resources != nil &&
		s.Windows.Resources.CPU != nil &&
		s.Windows.Resources.CPU.Shares != nil &&
		*s.Windows.Resources.CPU.Shares > 0 {
		return int32(*s.Windows.Resources.CPU.Shares)
	}
	return def
}

// ParseAnnotationsStorageIops searches `s.Annotations` for the `Iops`
// annotation. If not found searches `s` for the Windows Storage section. If
// neither are found returns `def`.
func ParseAnnotationsStorageIops(ctx context.Context, s *specs.Spec, annotation string, def int32) int32 {
	if m := ParseAnnotationsInt32(ctx, s.Annotations, annotation, 0); m != 0 {
		return m
	}
	if s.Windows != nil &&
		s.Windows.Resources != nil &&
		s.Windows.Resources.Storage != nil &&
		s.Windows.Resources.Storage.Iops != nil &&
		*s.Windows.Resources.Storage.Iops > 0 {
		return int32(*s.Windows.Resources.Storage.Iops)
	}
	return def
}

// ParseAnnotationsStorageBps searches `s.Annotations` for the `Bps` annotation.
// If not found searches `s` for the Windows Storage section. If neither are
// found returns `def`.
func ParseAnnotationsStorageBps(ctx context.Context, s *specs.Spec, annotation string, def int32) int32 {
	if m := ParseAnnotationsInt32(ctx, s.Annotations, annotation, 0); m != 0 {
		return m
	}
	if s.Windows != nil &&
		s.Windows.Resources != nil &&
		s.Windows.Resources.Storage != nil &&
		s.Windows.Resources.Storage.Bps != nil &&
		*s.Windows.Resources.Storage.Bps > 0 {
		return int32(*s.Windows.Resources.Storage.Bps)
	}
	return def
}

// ParseAnnotationsMemory searches `s.Annotations` for the memory annotation. If
// not found searches `s` for the Windows memory section. If neither are found
// returns `def`.
//
// Note: The returned value is in `MB`.
func ParseAnnotationsMemory(ctx context.Context, s *specs.Spec, annotation string, def uint64) uint64 {
	if m := ParseAnnotationsUint64(ctx, s.Annotations, annotation, 0); m != 0 {
		return m
	}
	if s.Windows != nil &&
		s.Windows.Resources != nil &&
		s.Windows.Resources.Memory != nil &&
		s.Windows.Resources.Memory.Limit != nil &&
		*s.Windows.Resources.Memory.Limit > 0 {
		return (*s.Windows.Resources.Memory.Limit / 1024 / 1024)
	}
	return def
}

// parseAnnotationsPreferredRootFSType searches `a` for `key` and verifies that the
// value is in the set of allowed values. If `key` is not found returns `def`.
func parseAnnotationsPreferredRootFSType(ctx context.Context, a map[string]string, key string, def uvm.PreferredRootFSType) uvm.PreferredRootFSType {
	if v, ok := a[key]; ok {
		switch v {
		case "initrd":
			return uvm.PreferredRootFSTypeInitRd
		case "vhd":
			return uvm.PreferredRootFSTypeVHD
		default:
			log.G(ctx).WithFields(logrus.Fields{
				"annotation": key,
				"value":      v,
			}).Warn("annotation value must be 'initrd' or 'vhd'")
		}
	}
	return def
}

// handleAnnotationBootFilesPath handles parsing annotations.BootFilesRootPath and setting
// implied options from the result.
func handleAnnotationBootFilesPath(ctx context.Context, a map[string]string, lopts *uvm.OptionsLCOW) {
	lopts.UpdateBootFilesPath(ctx, ParseAnnotationsString(a, annotations.BootFilesRootPath, lopts.BootFilesPath))
}

// handleAnnotationKernelDirectBoot handles parsing annotations.KernelDirectBoot and setting
// implied options from the result.
func handleAnnotationKernelDirectBoot(ctx context.Context, a map[string]string, lopts *uvm.OptionsLCOW) {
	lopts.KernelDirect = ParseAnnotationsBool(ctx, a, annotations.KernelDirectBoot, lopts.KernelDirect)
	if !lopts.KernelDirect {
		lopts.KernelFile = uvm.KernelFile
	}
}

// handleAnnotationPreferredRootFSType handles parsing annotations.PreferredRootFSType and setting
// implied options from the result.
func handleAnnotationPreferredRootFSType(ctx context.Context, a map[string]string, lopts *uvm.OptionsLCOW) {
	lopts.PreferredRootFSType = parseAnnotationsPreferredRootFSType(ctx, a, annotations.PreferredRootFSType, lopts.PreferredRootFSType)
	switch lopts.PreferredRootFSType {
	case uvm.PreferredRootFSTypeInitRd:
		lopts.RootFSFile = uvm.InitrdFile
	case uvm.PreferredRootFSTypeVHD:
		lopts.RootFSFile = uvm.VhdFile
	}
}

// handleAnnotationFullyPhysicallyBacked handles parsing annotations.FullyPhysicallyBacked and setting
// implied options from the result. For both LCOW and WCOW options.
func handleAnnotationFullyPhysicallyBacked(ctx context.Context, a map[string]string, opts interface{}) {
	switch options := opts.(type) {
	case *uvm.OptionsLCOW:
		options.FullyPhysicallyBacked = ParseAnnotationsBool(ctx, a, annotations.FullyPhysicallyBacked, options.FullyPhysicallyBacked)
		if options.FullyPhysicallyBacked {
			options.AllowOvercommit = false
			options.VPMemDeviceCount = 0
		}
	case *uvm.OptionsWCOW:
		options.FullyPhysicallyBacked = ParseAnnotationsBool(ctx, a, annotations.FullyPhysicallyBacked, options.FullyPhysicallyBacked)
		if options.FullyPhysicallyBacked {
			options.AllowOvercommit = false
		}
	}
}

// handleLCOWSecurityPolicy handles parsing SecurityPolicy and NoSecurityHardware and setting
// implied options from the results for LCOW.
func handleLCOWSecurityPolicy(ctx context.Context, a map[string]string, lopts *uvm.OptionsLCOW) {
	lopts.SecurityPolicy = ParseAnnotationsString(a, annotations.LCOWSecurityPolicy, lopts.SecurityPolicy)
	// allow actual isolated boot etc to be ignored if we have no hardware. Required for dev
	// this is not a security issue as the attestation will fail without a genuine report
	noSecurityHardware := ParseAnnotationsBool(ctx, a, annotations.NoSecurityHardware, false)

	// if there is a security policy (and SNP) we currently boot in a way that doesn't support any boot options
	// this might change if the building of the vmgs file were to be done on demand but that is likely
	// much slower and noy very useful. We do respect the filename of the vmgs file so if it is necessary to
	// have different options then multiple files could be used.
	if len(lopts.SecurityPolicy) > 0 && !noSecurityHardware {
		// VPMem not supported by the enlightened kernel for SNP so set count to zero.
		lopts.VPMemDeviceCount = 0
		// set the default GuestState filename.
		lopts.GuestStateFilePath = uvm.GuestStateFile
		lopts.KernelBootOptions = ""
		lopts.AllowOvercommit = false
		lopts.SecurityPolicyEnabled = true

		// There are two possible ways to boot SNP mode. Either kernelinitrd.vmgs which consists of kernel plus initrd.cpio.gz
		// Or a kernel.vmgs file (without an initrd) plus a separate vhd file which is dmverity protected via a hash tree
		// appended to rootfs ext4 filesystem.
		// We only currently support using the dmverity scheme. Note that the dmverity file name may be explicitly specified via
		// an annotation this is deliberately not the same annotation as the non-SNP rootfs vhd file.
		// The default behavior is to use kernel.vmgs and a rootfs-verity.vhd file with Merkle tree appended to ext4 filesystem.
		lopts.PreferredRootFSType = uvm.PreferredRootFSTypeNA
		lopts.RootFSFile = ""
		lopts.DmVerityRootFsVhd = uvm.DefaultDmVerityRootfsVhd
		lopts.DmVerityMode = true
	}

	if len(lopts.SecurityPolicy) > 0 {
		// will only be false if explicitly set false by the annotation. We will otherwise default to true when there is a security policy
		lopts.EnableScratchEncryption = ParseAnnotationsBool(ctx, a, annotations.LCOWEncryptedScratchDisk, true)
	}
}

// handleWCOWSecurityPolicy handles parsing confidential pods related options and setting
// implied options from the results for WCOW.
func handleWCOWSecurityPolicy(ctx context.Context, a map[string]string, wopts *uvm.OptionsWCOW) error {
	wopts.SecurityPolicy = ParseAnnotationsString(a, annotations.WCOWSecurityPolicy, wopts.SecurityPolicy)
	if len(wopts.SecurityPolicy) == 0 {
		return nil
	}
	wopts.SecurityPolicyEnabled = true

	// overcommit isn't allowed when running in confidential mode and minimum of 2GB memory is required.
	// We can change default values here, but if user provided specific values in annotations we should error out.
	wopts.MemorySizeInMB = ParseAnnotationsUint64(ctx, a, annotations.MemorySizeInMB, 2048)
	if wopts.MemorySizeInMB < 2048 {
		return fmt.Errorf("minimum 2048MB of memory is required for confidential pods, got: %d", wopts.MemorySizeInMB)
	}

	wopts.SecurityPolicyEnforcer = ParseAnnotationsString(a, annotations.WCOWSecurityPolicyEnforcer, wopts.SecurityPolicyEnforcer)
	wopts.DisableSecureBoot = ParseAnnotationsBool(ctx, a, annotations.WCOWDisableSecureBoot, false)
	wopts.GuestStateFilePath = ParseAnnotationsString(a, annotations.WCOWGuestStateFile, uvm.GetDefaultConfidentialVMGSPath())
	wopts.UVMReferenceInfoFile = ParseAnnotationsString(a, annotations.WCOWReferenceInfoFile, uvm.GetDefaultReferenceInfoFilePath())
	wopts.IsolationType = "SecureNestedPaging"
	if noSecurityHardware := ParseAnnotationsBool(ctx, a, annotations.NoSecurityHardware, false); noSecurityHardware {
		wopts.IsolationType = "GuestStateOnly"
	}
	if err := handleWCOWIsolationType(ctx, a, wopts); err != nil {
		return err
	}

	return nil
}

func handleWCOWIsolationType(ctx context.Context, a map[string]string, wopts *uvm.OptionsWCOW) error {
	isolationType := ParseAnnotationsString(a, annotations.WCOWIsolationType, wopts.IsolationType)
	switch isolationType {
	case "SecureNestedPaging", "SNP": // Allow VBS & SNP shorthands
		wopts.IsolationType = "SecureNestedPaging"
		wopts.AllowOvercommit = false
	case "VirtualizationBasedSecurity", "VBS":
		wopts.IsolationType = "VirtualizationBasedSecurity"
		wopts.AllowOvercommit = false
	case "GuestStateOnly":
		wopts.IsolationType = "GuestStateOnly"
		wopts.AllowOvercommit = false
	default:
		return fmt.Errorf("invalid WCOW isolation type %q", isolationType)
	}

	return nil
}

func parseDevices(ctx context.Context, specWindows *specs.Windows) []uvm.VPCIDeviceID {
	if specWindows == nil || specWindows.Devices == nil {
		return nil
	}
	extraDevices := []uvm.VPCIDeviceID{}
	for _, d := range specWindows.Devices {
		pciID, index := devices.GetDeviceInfoFromPath(d.ID)
		if uvm.IsValidDeviceType(d.IDType) {
			key := uvm.NewVPCIDeviceID(pciID, index)
			extraDevices = append(extraDevices, key)
		} else {
			log.G(ctx).WithFields(logrus.Fields{
				"device": d,
			}).Warnf("device type %s invalid, skipping", d.IDType)
		}
	}
	// nil out the devices on the spec so that they aren't re-added to the
	// pause container.
	specWindows.Devices = nil
	return extraDevices
}

// sets options common to both WCOW and LCOW from annotations.
func specToUVMCreateOptionsCommon(ctx context.Context, opts *uvm.Options, s *specs.Spec) error {
	opts.MemorySizeInMB = ParseAnnotationsMemory(ctx, s, annotations.MemorySizeInMB, opts.MemorySizeInMB)
	opts.LowMMIOGapInMB = ParseAnnotationsUint64(ctx, s.Annotations, annotations.MemoryLowMMIOGapInMB, opts.LowMMIOGapInMB)
	opts.HighMMIOBaseInMB = ParseAnnotationsUint64(ctx, s.Annotations, annotations.MemoryHighMMIOBaseInMB, opts.HighMMIOBaseInMB)
	opts.HighMMIOGapInMB = ParseAnnotationsUint64(ctx, s.Annotations, annotations.MemoryHighMMIOGapInMB, opts.HighMMIOGapInMB)
	opts.AllowOvercommit = ParseAnnotationsBool(ctx, s.Annotations, annotations.AllowOvercommit, opts.AllowOvercommit)
	opts.EnableDeferredCommit = ParseAnnotationsBool(ctx, s.Annotations, annotations.EnableDeferredCommit, opts.EnableDeferredCommit)
	opts.ProcessorCount = ParseAnnotationsCPUCount(ctx, s, annotations.ProcessorCount, opts.ProcessorCount)
	opts.ProcessorLimit = ParseAnnotationsCPULimit(ctx, s, annotations.ProcessorLimit, opts.ProcessorLimit)
	opts.ProcessorWeight = ParseAnnotationsCPUWeight(ctx, s, annotations.ProcessorWeight, opts.ProcessorWeight)
	opts.StorageQoSBandwidthMaximum = ParseAnnotationsStorageBps(ctx, s, annotations.StorageQoSBandwidthMaximum, opts.StorageQoSBandwidthMaximum)
	opts.StorageQoSIopsMaximum = ParseAnnotationsStorageIops(ctx, s, annotations.StorageQoSIopsMaximum, opts.StorageQoSIopsMaximum)
	opts.CPUGroupID = ParseAnnotationsString(s.Annotations, annotations.CPUGroupID, opts.CPUGroupID)
	opts.NetworkConfigProxy = ParseAnnotationsString(s.Annotations, annotations.NetworkConfigProxy, opts.NetworkConfigProxy)
	opts.ProcessDumpLocation = ParseAnnotationsString(s.Annotations, annotations.ContainerProcessDumpLocation, opts.ProcessDumpLocation)
	opts.NoWritableFileShares = ParseAnnotationsBool(ctx, s.Annotations, annotations.DisableWritableFileShares, opts.NoWritableFileShares)
	opts.DumpDirectoryPath = ParseAnnotationsString(s.Annotations, annotations.DumpDirectoryPath, opts.DumpDirectoryPath)
	opts.ConsolePipe = ParseAnnotationsString(s.Annotations, iannotations.UVMConsolePipe, opts.ConsolePipe)

	// NUMA settings
	opts.MaxProcessorsPerNumaNode = ParseAnnotationsUint32(ctx, s.Annotations, annotations.NumaMaximumProcessorsPerNode, opts.MaxProcessorsPerNumaNode)
	opts.MaxMemorySizePerNumaNode = ParseAnnotationsUint64(ctx, s.Annotations, annotations.NumaMaximumMemorySizePerNode, opts.MaxMemorySizePerNumaNode)
	opts.PreferredPhysicalNumaNodes = ParseAnnotationCommaSeparatedUint32(ctx, s.Annotations, annotations.NumaPreferredPhysicalNodes,
		opts.PreferredPhysicalNumaNodes)
	opts.NumaMappedPhysicalNodes = ParseAnnotationCommaSeparatedUint32(ctx, s.Annotations, annotations.NumaMappedPhysicalNodes,
		opts.NumaMappedPhysicalNodes)
	opts.NumaProcessorCounts = ParseAnnotationCommaSeparatedUint32(ctx, s.Annotations, annotations.NumaCountOfProcessors,
		opts.NumaProcessorCounts)
	opts.NumaMemoryBlocksCounts = ParseAnnotationCommaSeparatedUint64(ctx, s.Annotations, annotations.NumaCountOfMemoryBlocks,
		opts.NumaMemoryBlocksCounts)

	maps.Copy(opts.AdditionalHyperVConfig, ParseHVSocketServiceTable(ctx, s.Annotations))

	// parse error yielding annotations
	var err error
	opts.ResourcePartitionID, err = ParseAnnotationsGUID(s.Annotations, annotations.ResourcePartitionID, opts.ResourcePartitionID)
	if err != nil {
		return err
	}
	return nil
}

// SpecToUVMCreateOpts parses `s` and returns either `*uvm.OptionsLCOW` or
// `*uvm.OptionsWCOW`.
func SpecToUVMCreateOpts(ctx context.Context, s *specs.Spec, id, owner string) (interface{}, error) {
	if !IsIsolated(s) {
		return nil, errors.New("cannot create UVM opts for non-isolated spec")
	}
	if IsLCOW(s) {
		lopts := uvm.NewDefaultOptionsLCOW(id, owner)
		if err := specToUVMCreateOptionsCommon(ctx, lopts.Options, s); err != nil {
			return nil, err
		}

		/*
			WARNING!!!!!!!!!!

			When adding an option here which must match some security policy by default, make sure that the correct default (ie matches
			a default security policy) is applied in handleSecurityPolicy. Inadvertently adding an "option" which defaults to false but MUST be
			true for a default security	policy to work will force the annotation to have be set by the team that owns the box. That will
			be practically difficult and we	might not find out until a little late in the process.
		*/

		lopts.EnableColdDiscardHint = ParseAnnotationsBool(ctx, s.Annotations, annotations.EnableColdDiscardHint, lopts.EnableColdDiscardHint)
		lopts.VPMemDeviceCount = ParseAnnotationsUint32(ctx, s.Annotations, annotations.VPMemCount, lopts.VPMemDeviceCount)
		lopts.VPMemSizeBytes = ParseAnnotationsUint64(ctx, s.Annotations, annotations.VPMemSize, lopts.VPMemSizeBytes)
		lopts.VPMemNoMultiMapping = ParseAnnotationsBool(ctx, s.Annotations, annotations.VPMemNoMultiMapping, lopts.VPMemNoMultiMapping)
		lopts.VPCIEnabled = ParseAnnotationsBool(ctx, s.Annotations, annotations.VPCIEnabled, lopts.VPCIEnabled)
		lopts.ExtraVSockPorts = ParseAnnotationCommaSeparatedUint32(ctx, s.Annotations, iannotations.ExtraVSockPorts, lopts.ExtraVSockPorts)
		handleAnnotationBootFilesPath(ctx, s.Annotations, lopts)
		lopts.EnableScratchEncryption = ParseAnnotationsBool(ctx, s.Annotations, annotations.LCOWEncryptedScratchDisk, lopts.EnableScratchEncryption)
		lopts.SecurityPolicy = ParseAnnotationsString(s.Annotations, annotations.LCOWSecurityPolicy, lopts.SecurityPolicy)
		lopts.SecurityPolicyEnforcer = ParseAnnotationsString(s.Annotations, annotations.LCOWSecurityPolicyEnforcer, lopts.SecurityPolicyEnforcer)
		lopts.UVMReferenceInfoFile = ParseAnnotationsString(s.Annotations, annotations.LCOWReferenceInfoFile, lopts.UVMReferenceInfoFile)
		lopts.KernelBootOptions = ParseAnnotationsString(s.Annotations, annotations.KernelBootOptions, lopts.KernelBootOptions)
		lopts.DisableTimeSyncService = ParseAnnotationsBool(ctx, s.Annotations, annotations.DisableLCOWTimeSyncService, lopts.DisableTimeSyncService)
		lopts.WritableOverlayDirs = ParseAnnotationsBool(ctx, s.Annotations, iannotations.WritableOverlayDirs, lopts.WritableOverlayDirs)
		handleAnnotationPreferredRootFSType(ctx, s.Annotations, lopts)
		handleAnnotationKernelDirectBoot(ctx, s.Annotations, lopts)
		handleAnnotationFullyPhysicallyBacked(ctx, s.Annotations, lopts)

		// SecurityPolicy is very sensitive to other settings and will silently change those that are incompatible.
		// Eg VMPem device count, overridden kernel option cannot be respected.
		handleLCOWSecurityPolicy(ctx, s.Annotations, lopts)

		// override the default GuestState and DmVerityRootFs filenames if specified
		lopts.GuestStateFilePath = ParseAnnotationsString(s.Annotations, annotations.LCOWGuestStateFile, lopts.GuestStateFilePath)
		lopts.DmVerityRootFsVhd = ParseAnnotationsString(s.Annotations, annotations.DmVerityRootFsVhd, lopts.DmVerityRootFsVhd)
		lopts.DmVerityMode = ParseAnnotationsBool(ctx, s.Annotations, annotations.DmVerityMode, lopts.DmVerityMode)
		lopts.DmVerityCreateArgs = ParseAnnotationsString(s.Annotations, annotations.DmVerityCreateArgs, lopts.DmVerityCreateArgs)
		// Set HclEnabled if specified. Else default to a null pointer, which is omitted from the resulting JSON.
		lopts.HclEnabled = ParseAnnotationsNullableBool(ctx, s.Annotations, annotations.LCOWHclEnabled)

		// Add devices on the spec to the UVM's options
		lopts.AssignedDevices = parseDevices(ctx, s.Windows)
		lopts.PolicyBasedRouting = ParseAnnotationsBool(ctx, s.Annotations, iannotations.NetworkingPolicyBasedRouting, lopts.PolicyBasedRouting)
		return lopts, nil
	} else if IsWCOW(s) {
		wopts := uvm.NewDefaultOptionsWCOW(id, owner)
		if err := specToUVMCreateOptionsCommon(ctx, wopts.Options, s); err != nil {
			return nil, err
		}

		wopts.DisableCompartmentNamespace = ParseAnnotationsBool(ctx, s.Annotations, annotations.DisableCompartmentNamespace, wopts.DisableCompartmentNamespace)
		wopts.NoDirectMap = ParseAnnotationsBool(ctx, s.Annotations, annotations.VSMBNoDirectMap, wopts.NoDirectMap)
		wopts.NoInheritHostTimezone = ParseAnnotationsBool(ctx, s.Annotations, annotations.NoInheritHostTimezone, wopts.NoInheritHostTimezone)
		wopts.AdditionalRegistryKeys = append(wopts.AdditionalRegistryKeys, parseAdditionalRegistryValues(ctx, s.Annotations)...)
		handleAnnotationFullyPhysicallyBacked(ctx, s.Annotations, wopts)

		// Writable EFI is valid for both confidential and regular Hyper-V isolated WCOW.
		wopts.WritableEFI = ParseAnnotationsBool(ctx, s.Annotations, annotations.WCOWWritableEFI, wopts.WritableEFI)

		// Handle WCOW security policy settings
		if err := handleWCOWSecurityPolicy(ctx, s.Annotations, wopts); err != nil {
			return nil, err
		}
		// If security policy is enable, wopts.ForwardLogs default value should be false
		if wopts.SecurityPolicyEnabled {
			wopts.ForwardLogs = false
		}
		wopts.LogSources = ParseAnnotationsString(s.Annotations, annotations.LogSources, wopts.LogSources)
		wopts.ForwardLogs = ParseAnnotationsBool(ctx, s.Annotations, annotations.ForwardLogs, wopts.ForwardLogs)
		return wopts, nil
	}
	return nil, errors.New("cannot create UVM opts spec is not LCOW or WCOW")
}

// UpdateSpecFromOptions sets extra annotations on the OCI spec based on the
// `opts` struct.
func UpdateSpecFromOptions(s specs.Spec, opts *runhcsopts.Options) specs.Spec {
	if opts == nil {
		return s
	}

	if _, ok := s.Annotations[annotations.BootFilesRootPath]; !ok && opts.BootFilesRootPath != "" {
		s.Annotations[annotations.BootFilesRootPath] = opts.BootFilesRootPath
	}

	if _, ok := s.Annotations[annotations.ProcessorCount]; !ok && opts.VmProcessorCount != 0 {
		s.Annotations[annotations.ProcessorCount] = strconv.FormatInt(int64(opts.VmProcessorCount), 10)
	}

	if _, ok := s.Annotations[annotations.MemorySizeInMB]; !ok && opts.VmMemorySizeInMb != 0 {
		s.Annotations[annotations.MemorySizeInMB] = strconv.FormatInt(int64(opts.VmMemorySizeInMb), 10)
	}

	if _, ok := s.Annotations[annotations.NetworkConfigProxy]; !ok && opts.NCProxyAddr != "" {
		s.Annotations[annotations.NetworkConfigProxy] = opts.NCProxyAddr
	}

	for key, value := range opts.DefaultContainerAnnotations {
		// Make sure not to override any annotations which are set explicitly
		if _, ok := s.Annotations[key]; !ok {
			s.Annotations[key] = value
		}
	}

	return s
}
