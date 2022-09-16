//go:build windows

package oci

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	runhcsopts "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	"github.com/Microsoft/hcsshim/internal/clone"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

// UVM specific annotation parsing

// ParseAnnotationsCPUCount searches `s.Annotations` for the CPU annotation. If
// not found searches `s` for the Windows CPU section. If neither are found
// returns `def`.
func ParseAnnotationsCPUCount(ctx context.Context, s *specs.Spec, annotation string, def int32) int32 {
	if m := parseAnnotationsUint64(ctx, s.Annotations, annotation, 0); m != 0 {
		return int32(m)
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
	if m := parseAnnotationsUint64(ctx, s.Annotations, annotation, 0); m != 0 {
		return int32(m)
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
	if m := parseAnnotationsUint64(ctx, s.Annotations, annotation, 0); m != 0 {
		return int32(m)
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
	if m := parseAnnotationsUint64(ctx, s.Annotations, annotation, 0); m != 0 {
		return int32(m)
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
	if m := parseAnnotationsUint64(ctx, s.Annotations, annotation, 0); m != 0 {
		return int32(m)
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
	if m := parseAnnotationsUint64(ctx, s.Annotations, annotation, 0); m != 0 {
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

func ParseCloneAnnotations(ctx context.Context, s *specs.Spec) (isTemplate bool, templateID string, err error) {
	templateID = ParseAnnotationsTemplateID(ctx, s)
	isTemplate = ParseAnnotationsSaveAsTemplate(ctx, s)
	if templateID != "" && isTemplate {
		return false, "", fmt.Errorf("templateID and save as template flags can not be passed in the same request")
	}

	if (isTemplate || templateID != "") && !IsWCOW(s) {
		return false, "", fmt.Errorf("save as template and creating clones is only available for WCOW")
	}
	return
}

// handleAnnotationKernelDirectBoot handles parsing annotationKernelDirectBoot and setting
// implied annotations from the result.
func handleAnnotationKernelDirectBoot(ctx context.Context, a map[string]string, lopts *uvm.OptionsLCOW) {
	lopts.KernelDirect = ParseAnnotationsBool(ctx, a, annotations.KernelDirectBoot, lopts.KernelDirect)
	if !lopts.KernelDirect {
		lopts.KernelFile = uvm.KernelFile
	}
}

// handleAnnotationPreferredRootFSType handles parsing annotationPreferredRootFSType and setting
// implied annotations from the result
func handleAnnotationPreferredRootFSType(ctx context.Context, a map[string]string, lopts *uvm.OptionsLCOW) {
	lopts.PreferredRootFSType = parseAnnotationsPreferredRootFSType(ctx, a, annotations.PreferredRootFSType, lopts.PreferredRootFSType)
	switch lopts.PreferredRootFSType {
	case uvm.PreferredRootFSTypeInitRd:
		lopts.RootFSFile = uvm.InitrdFile
	case uvm.PreferredRootFSTypeVHD:
		lopts.RootFSFile = uvm.VhdFile
	}
}

// handleAnnotationFullyPhysicallyBacked handles parsing annotationFullyPhysicallyBacked and setting
// implied annotations from the result. For both LCOW and WCOW options.
func handleAnnotationFullyPhysicallyBacked(ctx context.Context, a map[string]string, opts interface{}) {
	switch options := opts.(type) {
	case *uvm.OptionsLCOW:
		options.FullyPhysicallyBacked = ParseAnnotationsBool(ctx, a, annotations.FullyPhysicallyBacked, options.FullyPhysicallyBacked)
		if options.FullyPhysicallyBacked {
			options.AllowOvercommit = false
			options.PreferredRootFSType = uvm.PreferredRootFSTypeInitRd
			options.RootFSFile = uvm.InitrdFile
			options.VPMemDeviceCount = 0
		}
	case *uvm.OptionsWCOW:
		options.FullyPhysicallyBacked = ParseAnnotationsBool(ctx, a, annotations.FullyPhysicallyBacked, options.FullyPhysicallyBacked)
		if options.FullyPhysicallyBacked {
			options.AllowOvercommit = false
		}
	}
}

// handleCloneAnnotations handles parsing annotations related to template creation and cloning
// Since late cloning is only supported for WCOW this function only deals with WCOW options.
func handleCloneAnnotations(ctx context.Context, a map[string]string, wopts *uvm.OptionsWCOW) (err error) {
	wopts.IsTemplate = ParseAnnotationsBool(ctx, a, annotations.SaveAsTemplate, false)
	templateID := parseAnnotationsString(a, annotations.TemplateID, "")
	if templateID != "" {
		tc, err := clone.FetchTemplateConfig(ctx, templateID)
		if err != nil {
			return err
		}
		wopts.TemplateConfig = &uvm.UVMTemplateConfig{
			UVMID:      tc.TemplateUVMID,
			CreateOpts: tc.TemplateUVMCreateOpts,
			Resources:  tc.TemplateUVMResources,
		}
		wopts.IsClone = true
	}
	return nil
}

// handleSecurityPolicy handles parsing SecurityPolicy and NoSecurityHardware and setting
// implied options from the results. Both LCOW only, not WCOW
func handleSecurityPolicy(ctx context.Context, a map[string]string, lopts *uvm.OptionsLCOW) {
	lopts.SecurityPolicy = parseAnnotationsString(a, annotations.SecurityPolicy, lopts.SecurityPolicy)
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
		lopts.GuestStateFile = uvm.GuestStateFile
		lopts.KernelBootOptions = ""
		lopts.PreferredRootFSType = uvm.PreferredRootFSTypeNA
		lopts.AllowOvercommit = false
		lopts.SecurityPolicyEnabled = true
	}
}

// sets options common to both WCOW and LCOW from annotations
func specToUVMCreateOptionsCommon(ctx context.Context, opts *uvm.Options, s *specs.Spec) {
	opts.MemorySizeInMB = ParseAnnotationsMemory(ctx, s, annotations.MemorySizeInMB, opts.MemorySizeInMB)
	opts.LowMMIOGapInMB = parseAnnotationsUint64(ctx, s.Annotations, annotations.MemoryLowMMIOGapInMB, opts.LowMMIOGapInMB)
	opts.HighMMIOBaseInMB = parseAnnotationsUint64(ctx, s.Annotations, annotations.MemoryHighMMIOBaseInMB, opts.HighMMIOBaseInMB)
	opts.HighMMIOGapInMB = parseAnnotationsUint64(ctx, s.Annotations, annotations.MemoryHighMMIOGapInMB, opts.HighMMIOGapInMB)
	opts.AllowOvercommit = ParseAnnotationsBool(ctx, s.Annotations, annotations.AllowOvercommit, opts.AllowOvercommit)
	opts.EnableDeferredCommit = ParseAnnotationsBool(ctx, s.Annotations, annotations.EnableDeferredCommit, opts.EnableDeferredCommit)
	opts.ProcessorCount = ParseAnnotationsCPUCount(ctx, s, annotations.ProcessorCount, opts.ProcessorCount)
	opts.ProcessorLimit = ParseAnnotationsCPULimit(ctx, s, annotations.ProcessorLimit, opts.ProcessorLimit)
	opts.ProcessorWeight = ParseAnnotationsCPUWeight(ctx, s, annotations.ProcessorWeight, opts.ProcessorWeight)
	opts.StorageQoSBandwidthMaximum = ParseAnnotationsStorageBps(ctx, s, annotations.StorageQoSBandwidthMaximum, opts.StorageQoSBandwidthMaximum)
	opts.StorageQoSIopsMaximum = ParseAnnotationsStorageIops(ctx, s, annotations.StorageQoSIopsMaximum, opts.StorageQoSIopsMaximum)
	opts.CPUGroupID = parseAnnotationsString(s.Annotations, annotations.CPUGroupID, opts.CPUGroupID)
	opts.NetworkConfigProxy = parseAnnotationsString(s.Annotations, annotations.NetworkConfigProxy, opts.NetworkConfigProxy)
	opts.ProcessDumpLocation = parseAnnotationsString(s.Annotations, annotations.ContainerProcessDumpLocation, opts.ProcessDumpLocation)
	opts.NoWritableFileShares = ParseAnnotationsBool(ctx, s.Annotations, annotations.DisableWritableFileShares, opts.NoWritableFileShares)
	opts.DumpDirectoryPath = parseAnnotationsString(s.Annotations, annotations.DumpDirectoryPath, opts.DumpDirectoryPath)
}

// SpecToUVMCreateOpts parses `s` and returns either `*uvm.OptionsLCOW` or
// `*uvm.OptionsWCOW`.
func SpecToUVMCreateOpts(ctx context.Context, s *specs.Spec, id, owner string) (interface{}, error) {
	if !IsIsolated(s) {
		return nil, errors.New("cannot create UVM opts for non-isolated spec")
	}
	if IsLCOW(s) {
		lopts := uvm.NewDefaultOptionsLCOW(id, owner)
		specToUVMCreateOptionsCommon(ctx, lopts.Options, s)

		lopts.EnableColdDiscardHint = ParseAnnotationsBool(ctx, s.Annotations, annotations.EnableColdDiscardHint, lopts.EnableColdDiscardHint)
		lopts.VPMemDeviceCount = parseAnnotationsUint32(ctx, s.Annotations, annotations.VPMemCount, lopts.VPMemDeviceCount)
		lopts.VPMemSizeBytes = parseAnnotationsUint64(ctx, s.Annotations, annotations.VPMemSize, lopts.VPMemSizeBytes)
		lopts.VPMemNoMultiMapping = ParseAnnotationsBool(ctx, s.Annotations, annotations.VPMemNoMultiMapping, lopts.VPMemNoMultiMapping)
		lopts.VPCIEnabled = ParseAnnotationsBool(ctx, s.Annotations, annotations.VPCIEnabled, lopts.VPCIEnabled)
		lopts.BootFilesPath = parseAnnotationsString(s.Annotations, annotations.BootFilesRootPath, lopts.BootFilesPath)
		lopts.EnableScratchEncryption = ParseAnnotationsBool(ctx, s.Annotations, annotations.EncryptedScratchDisk, lopts.EnableScratchEncryption)
		lopts.SecurityPolicy = parseAnnotationsString(s.Annotations, annotations.SecurityPolicy, lopts.SecurityPolicy)
		lopts.SecurityPolicyEnforcer = parseAnnotationsString(s.Annotations, annotations.SecurityPolicyEnforcer, lopts.SecurityPolicyEnforcer)
		lopts.UVMReferenceInfoFile = parseAnnotationsString(s.Annotations, annotations.UVMReferenceInfoFile, lopts.UVMReferenceInfoFile)
		lopts.KernelBootOptions = parseAnnotationsString(s.Annotations, annotations.KernelBootOptions, lopts.KernelBootOptions)
		lopts.DisableTimeSyncService = ParseAnnotationsBool(ctx, s.Annotations, annotations.DisableLCOWTimeSyncService, lopts.DisableTimeSyncService)
		handleAnnotationPreferredRootFSType(ctx, s.Annotations, lopts)
		handleAnnotationKernelDirectBoot(ctx, s.Annotations, lopts)

		// parsing of FullyPhysicallyBacked needs to go after handling kernel direct boot and
		// preferred rootfs type since it may overwrite settings created by those
		handleAnnotationFullyPhysicallyBacked(ctx, s.Annotations, lopts)

		// SecurityPolicy is very sensitive to other settings and will silently change those that are incompatible.
		// Eg VMPem device count, overridden kernel option cannot be respected.
		handleSecurityPolicy(ctx, s.Annotations, lopts)

		// override the default GuestState filename if specified
		lopts.GuestStateFile = parseAnnotationsString(s.Annotations, annotations.GuestStateFile, lopts.GuestStateFile)
		return lopts, nil
	} else if IsWCOW(s) {
		wopts := uvm.NewDefaultOptionsWCOW(id, owner)
		specToUVMCreateOptionsCommon(ctx, wopts.Options, s)

		wopts.DisableCompartmentNamespace = ParseAnnotationsBool(ctx, s.Annotations, annotations.DisableCompartmentNamespace, wopts.DisableCompartmentNamespace)
		wopts.NoDirectMap = ParseAnnotationsBool(ctx, s.Annotations, annotations.VSMBNoDirectMap, wopts.NoDirectMap)
		wopts.NoInheritHostTimezone = ParseAnnotationsBool(ctx, s.Annotations, annotations.NoInheritHostTimezone, wopts.NoInheritHostTimezone)
		handleAnnotationFullyPhysicallyBacked(ctx, s.Annotations, wopts)
		if err := handleCloneAnnotations(ctx, s.Annotations, wopts); err != nil {
			return nil, err
		}
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

	if _, ok := s.Annotations[annotations.GPUVHDPath]; !ok && opts.GPUVHDPath != "" {
		s.Annotations[annotations.GPUVHDPath] = opts.GPUVHDPath
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
