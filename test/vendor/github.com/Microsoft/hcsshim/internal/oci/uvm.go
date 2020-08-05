package oci

import (
	"context"
	"errors"
	"strconv"
	"strings"

	runhcsopts "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

const (
	// AnnotationContainerMemorySizeInMB overrides the container memory size set
	// via the OCI spec.
	//
	// Note: This annotation is in MB. OCI is in Bytes. When using this override
	// the caller MUST use MB or sizing will be wrong.
	//
	// Note: This is only present because CRI does not (currently) have a
	// `WindowsPodSandboxConfig` for setting this correctly. It should not be
	// used via OCI runtimes and rather use
	// `spec.Windows.Resources.Memory.Limit`.
	AnnotationContainerMemorySizeInMB = "io.microsoft.container.memory.sizeinmb"
	// AnnotationContainerProcessorCount overrides the container processor count
	// set via the OCI spec.
	//
	// Note: For Windows Process Containers CPU Count/Limit/Weight are mutually
	// exclusive and the caller MUST only set one of the values.
	//
	// Note: This is only present because CRI does not (currently) have a
	// `WindowsPodSandboxConfig` for setting this correctly. It should not be
	// used via OCI runtimes and rather use `spec.Windows.Resources.CPU.Count`.
	AnnotationContainerProcessorCount = "io.microsoft.container.processor.count"
	// AnnotationContainerProcessorLimit overrides the container processor limit
	// set via the OCI spec.
	//
	// Limit allows values 1 - 10,000 where 10,000 means 100% CPU. (And is the
	// default if omitted)
	//
	// Note: For Windows Process Containers CPU Count/Limit/Weight are mutually
	// exclusive and the caller MUST only set one of the values.
	//
	// Note: This is only present because CRI does not (currently) have a
	// `WindowsPodSandboxConfig` for setting this correctly. It should not be
	// used via OCI runtimes and rather use
	// `spec.Windows.Resources.CPU.Maximum`.
	AnnotationContainerProcessorLimit = "io.microsoft.container.processor.limit"
	// AnnotationContainerProcessorWeight overrides the container processor
	// weight set via the OCI spec.
	//
	// Weight allows values 0 - 10,000. (100 is the default)
	//
	// Note: For Windows Process Containers CPU Count/Limit/Weight are mutually
	// exclusive and the caller MUST only set one of the values.
	//
	// Note: This is only present because CRI does not (currently) have a
	// `WindowsPodSandboxConfig` for setting this correctly. It should not be
	// used via OCI runtimes and rather use `spec.Windows.Resources.CPU.Shares`.
	AnnotationContainerProcessorWeight = "io.microsoft.container.processor.weight"
	// AnnotationContainerStorageQoSBandwidthMaximum overrides the container
	// storage bandwidth per second set via the OCI spec.
	//
	// Note: This is only present because CRI does not (currently) have a
	// `WindowsPodSandboxConfig` for setting this correctly. It should not be
	// used via OCI runtimes and rather use
	// `spec.Windows.Resources.Storage.Bps`.
	AnnotationContainerStorageQoSBandwidthMaximum = "io.microsoft.container.storage.qos.bandwidthmaximum"
	// AnnotationContainerStorageQoSIopsMaximum overrides the container storage
	// maximum iops set via the OCI spec.
	//
	// Note: This is only present because CRI does not (currently) have a
	// `WindowsPodSandboxConfig` for setting this correctly. It should not be
	// used via OCI runtimes and rather use
	// `spec.Windows.Resources.Storage.Iops`.
	AnnotationContainerStorageQoSIopsMaximum = "io.microsoft.container.storage.qos.iopsmaximum"
	// AnnotationGPUVHDPath overrides the default path to search for the gpu vhd
	AnnotationGPUVHDPath            = "io.microsoft.lcow.gpuvhdpath"
	annotationAllowOvercommit       = "io.microsoft.virtualmachine.computetopology.memory.allowovercommit"
	annotationEnableDeferredCommit  = "io.microsoft.virtualmachine.computetopology.memory.enabledeferredcommit"
	annotationEnableColdDiscardHint = "io.microsoft.virtualmachine.computetopology.memory.enablecolddiscardhint"
	// annotationMemorySizeInMB overrides the container memory size set via the
	// OCI spec.
	//
	// Note: This annotation is in MB. OCI is in Bytes. When using this override
	// the caller MUST use MB or sizing will be wrong.
	annotationMemorySizeInMB         = "io.microsoft.virtualmachine.computetopology.memory.sizeinmb"
	annotationMemoryLowMMIOGapInMB   = "io.microsoft.virtualmachine.computetopology.memory.lowmmiogapinmb"
	annotationMemoryHighMMIOBaseInMB = "io.microsoft.virtualmachine.computetopology.memory.highmmiobaseinmb"
	annotationMemoryHighMMIOGapInMB  = "io.microsoft.virtualmachine.computetopology.memory.highmmiogapinmb"
	// annotationProcessorCount overrides the hypervisor isolated vCPU count set
	// via the OCI spec.
	//
	// Note: Unlike Windows process isolated container QoS Count/Limt/Weight on
	// the UVM are not mutually exclusive and can be set together.
	annotationProcessorCount = "io.microsoft.virtualmachine.computetopology.processor.count"
	// annotationProcessorLimit overrides the hypervisor isolated vCPU limit set
	// via the OCI spec.
	//
	// Limit allows values 1 - 100,000 where 100,000 means 100% CPU. (And is the
	// default if omitted)
	//
	// Note: Unlike Windows process isolated container QoS Count/Limt/Weight on
	// the UVM are not mutually exclusive and can be set together.
	annotationProcessorLimit = "io.microsoft.virtualmachine.computetopology.processor.limit"
	// annotationProcessorWeight overrides the hypervisor isolated vCPU weight set
	// via the OCI spec.
	//
	// Weight allows values 0 - 10,000. (100 is the default if omitted)
	//
	// Note: Unlike Windows process isolated container QoS Count/Limt/Weight on
	// the UVM are not mutually exclusive and can be set together.
	annotationProcessorWeight            = "io.microsoft.virtualmachine.computetopology.processor.weight"
	annotationVPMemCount                 = "io.microsoft.virtualmachine.devices.virtualpmem.maximumcount"
	annotationVPMemSize                  = "io.microsoft.virtualmachine.devices.virtualpmem.maximumsizebytes"
	annotationPreferredRootFSType        = "io.microsoft.virtualmachine.lcow.preferredrootfstype"
	annotationBootFilesRootPath          = "io.microsoft.virtualmachine.lcow.bootfilesrootpath"
	annotationKernelDirectBoot           = "io.microsoft.virtualmachine.lcow.kerneldirectboot"
	annotationVPCIEnabled                = "io.microsoft.virtualmachine.lcow.vpcienabled"
	annotationStorageQoSBandwidthMaximum = "io.microsoft.virtualmachine.storageqos.bandwidthmaximum"
	annotationStorageQoSIopsMaximum      = "io.microsoft.virtualmachine.storageqos.iopsmaximum"
	annotationFullyPhysicallyBacked      = "io.microsoft.virtualmachine.fullyphysicallybacked"
	// A boolean annotation to control whether to use an external bridge or the
	// HCS-GCS bridge. Default value is true which means external bridge will be used
	// by default.
	annotationUseExternalGCSBridge = "io.microsoft.virtualmachine.useexternalgcsbridge"
)

// parseAnnotationsBool searches `a` for `key` and if found verifies that the
// value is `true` or `false` in any case. If `key` is not found returns `def`.
func parseAnnotationsBool(ctx context.Context, a map[string]string, key string, def bool) bool {
	if v, ok := a[key]; ok {
		switch strings.ToLower(v) {
		case "true":
			return true
		case "false":
			return false
		default:
			log.G(ctx).WithFields(logrus.Fields{
				logfields.OCIAnnotation: key,
				logfields.Value:         v,
				logfields.ExpectedType:  logfields.Bool,
			}).Warning("annotation could not be parsed")
		}
	}
	return def
}

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
func ParseAnnotationsMemory(ctx context.Context, s *specs.Spec, annotation string, def int32) int32 {
	if m := parseAnnotationsUint64(ctx, s.Annotations, annotation, 0); m != 0 {
		return int32(m)
	}
	if s.Windows != nil &&
		s.Windows.Resources != nil &&
		s.Windows.Resources.Memory != nil &&
		s.Windows.Resources.Memory.Limit != nil &&
		*s.Windows.Resources.Memory.Limit > 0 {
		return int32(*s.Windows.Resources.Memory.Limit / 1024 / 1024)
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

// parseAnnotationsUint32 searches `a` for `key` and if found verifies that the
// value is a 32 bit unsigned integer. If `key` is not found returns `def`.
func parseAnnotationsUint32(ctx context.Context, a map[string]string, key string, def uint32) uint32 {
	if v, ok := a[key]; ok {
		countu, err := strconv.ParseUint(v, 10, 32)
		if err == nil {
			v := uint32(countu)
			return v
		}
		log.G(ctx).WithFields(logrus.Fields{
			logfields.OCIAnnotation: key,
			logfields.Value:         v,
			logfields.ExpectedType:  logfields.Uint32,
			logrus.ErrorKey:         err,
		}).Warning("annotation could not be parsed")
	}
	return def
}

// parseAnnotationsUint64 searches `a` for `key` and if found verifies that the
// value is a 64 bit unsigned integer. If `key` is not found returns `def`.
func parseAnnotationsUint64(ctx context.Context, a map[string]string, key string, def uint64) uint64 {
	if v, ok := a[key]; ok {
		countu, err := strconv.ParseUint(v, 10, 64)
		if err == nil {
			return countu
		}
		log.G(ctx).WithFields(logrus.Fields{
			logfields.OCIAnnotation: key,
			logfields.Value:         v,
			logfields.ExpectedType:  logfields.Uint64,
			logrus.ErrorKey:         err,
		}).Warning("annotation could not be parsed")
	}
	return def
}

// parseAnnotationsString searches `a` for `key`. If `key` is not found returns `def`.
func parseAnnotationsString(a map[string]string, key string, def string) string {
	if v, ok := a[key]; ok {
		return v
	}
	return def
}

// handleAnnotationKernelDirectBoot handles parsing annotationKernelDirectBoot and setting
// implied annotations from the result.
func handleAnnotationKernelDirectBoot(ctx context.Context, a map[string]string, lopts *uvm.OptionsLCOW) {
	lopts.KernelDirect = parseAnnotationsBool(ctx, a, annotationKernelDirectBoot, lopts.KernelDirect)
	if !lopts.KernelDirect {
		lopts.KernelFile = uvm.KernelFile
	}
}

// handleAnnotationPreferredRootFSType handles parsing annotationPreferredRootFSType and setting
// implied annotations from the result
func handleAnnotationPreferredRootFSType(ctx context.Context, a map[string]string, lopts *uvm.OptionsLCOW) {
	lopts.PreferredRootFSType = parseAnnotationsPreferredRootFSType(ctx, a, annotationPreferredRootFSType, lopts.PreferredRootFSType)
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
		options.FullyPhysicallyBacked = parseAnnotationsBool(ctx, a, annotationFullyPhysicallyBacked, options.FullyPhysicallyBacked)
		if options.FullyPhysicallyBacked {
			options.AllowOvercommit = false
			options.PreferredRootFSType = uvm.PreferredRootFSTypeInitRd
			options.RootFSFile = uvm.InitrdFile
			options.VPMemDeviceCount = 0
		}
	case *uvm.OptionsWCOW:
		options.FullyPhysicallyBacked = parseAnnotationsBool(ctx, a, annotationFullyPhysicallyBacked, options.FullyPhysicallyBacked)
		if options.FullyPhysicallyBacked {
			options.AllowOvercommit = false
		}
	}
}

// SpecToUVMCreateOpts parses `s` and returns either `*uvm.OptionsLCOW` or
// `*uvm.OptionsWCOW`.
func SpecToUVMCreateOpts(ctx context.Context, s *specs.Spec, id, owner string) (interface{}, error) {
	if !IsIsolated(s) {
		return nil, errors.New("cannot create UVM opts for non-isolated spec")
	}
	if IsLCOW(s) {
		lopts := uvm.NewDefaultOptionsLCOW(id, owner)
		lopts.MemorySizeInMB = ParseAnnotationsMemory(ctx, s, annotationMemorySizeInMB, lopts.MemorySizeInMB)
		lopts.LowMMIOGapInMB = parseAnnotationsUint64(ctx, s.Annotations, annotationMemoryLowMMIOGapInMB, lopts.LowMMIOGapInMB)
		lopts.HighMMIOBaseInMB = parseAnnotationsUint64(ctx, s.Annotations, annotationMemoryHighMMIOBaseInMB, lopts.HighMMIOBaseInMB)
		lopts.HighMMIOGapInMB = parseAnnotationsUint64(ctx, s.Annotations, annotationMemoryHighMMIOGapInMB, lopts.HighMMIOGapInMB)
		lopts.AllowOvercommit = parseAnnotationsBool(ctx, s.Annotations, annotationAllowOvercommit, lopts.AllowOvercommit)
		lopts.EnableDeferredCommit = parseAnnotationsBool(ctx, s.Annotations, annotationEnableDeferredCommit, lopts.EnableDeferredCommit)
		lopts.EnableColdDiscardHint = parseAnnotationsBool(ctx, s.Annotations, annotationEnableColdDiscardHint, lopts.EnableColdDiscardHint)
		lopts.ProcessorCount = ParseAnnotationsCPUCount(ctx, s, annotationProcessorCount, lopts.ProcessorCount)
		lopts.ProcessorLimit = ParseAnnotationsCPULimit(ctx, s, annotationProcessorLimit, lopts.ProcessorLimit)
		lopts.ProcessorWeight = ParseAnnotationsCPUWeight(ctx, s, annotationProcessorWeight, lopts.ProcessorWeight)
		lopts.VPMemDeviceCount = parseAnnotationsUint32(ctx, s.Annotations, annotationVPMemCount, lopts.VPMemDeviceCount)
		lopts.VPMemSizeBytes = parseAnnotationsUint64(ctx, s.Annotations, annotationVPMemSize, lopts.VPMemSizeBytes)
		lopts.StorageQoSBandwidthMaximum = ParseAnnotationsStorageBps(ctx, s, annotationStorageQoSBandwidthMaximum, lopts.StorageQoSBandwidthMaximum)
		lopts.StorageQoSIopsMaximum = ParseAnnotationsStorageIops(ctx, s, annotationStorageQoSIopsMaximum, lopts.StorageQoSIopsMaximum)
		lopts.VPCIEnabled = parseAnnotationsBool(ctx, s.Annotations, annotationVPCIEnabled, lopts.VPCIEnabled)
		lopts.BootFilesPath = parseAnnotationsString(s.Annotations, annotationBootFilesRootPath, lopts.BootFilesPath)
		lopts.ExternalGuestConnection = parseAnnotationsBool(ctx, s.Annotations, annotationUseExternalGCSBridge, lopts.ExternalGuestConnection)
		handleAnnotationPreferredRootFSType(ctx, s.Annotations, lopts)
		handleAnnotationKernelDirectBoot(ctx, s.Annotations, lopts)

		// parsing of FullyPhysicallyBacked needs to go after handling kernel direct boot and
		// preferred rootfs type since it may overwrite settings created by those
		handleAnnotationFullyPhysicallyBacked(ctx, s.Annotations, lopts)
		return lopts, nil
	} else if IsWCOW(s) {
		wopts := uvm.NewDefaultOptionsWCOW(id, owner)
		wopts.MemorySizeInMB = ParseAnnotationsMemory(ctx, s, annotationMemorySizeInMB, wopts.MemorySizeInMB)
		wopts.LowMMIOGapInMB = parseAnnotationsUint64(ctx, s.Annotations, annotationMemoryLowMMIOGapInMB, wopts.LowMMIOGapInMB)
		wopts.HighMMIOBaseInMB = parseAnnotationsUint64(ctx, s.Annotations, annotationMemoryHighMMIOBaseInMB, wopts.HighMMIOBaseInMB)
		wopts.HighMMIOGapInMB = parseAnnotationsUint64(ctx, s.Annotations, annotationMemoryHighMMIOGapInMB, wopts.HighMMIOGapInMB)
		wopts.AllowOvercommit = parseAnnotationsBool(ctx, s.Annotations, annotationAllowOvercommit, wopts.AllowOvercommit)
		wopts.EnableDeferredCommit = parseAnnotationsBool(ctx, s.Annotations, annotationEnableDeferredCommit, wopts.EnableDeferredCommit)
		wopts.ProcessorCount = ParseAnnotationsCPUCount(ctx, s, annotationProcessorCount, wopts.ProcessorCount)
		wopts.ProcessorLimit = ParseAnnotationsCPULimit(ctx, s, annotationProcessorLimit, wopts.ProcessorLimit)
		wopts.ProcessorWeight = ParseAnnotationsCPUWeight(ctx, s, annotationProcessorWeight, wopts.ProcessorWeight)
		wopts.StorageQoSBandwidthMaximum = ParseAnnotationsStorageBps(ctx, s, annotationStorageQoSBandwidthMaximum, wopts.StorageQoSBandwidthMaximum)
		wopts.StorageQoSIopsMaximum = ParseAnnotationsStorageIops(ctx, s, annotationStorageQoSIopsMaximum, wopts.StorageQoSIopsMaximum)
		wopts.ExternalGuestConnection = parseAnnotationsBool(ctx, s.Annotations, annotationUseExternalGCSBridge, wopts.ExternalGuestConnection)
		handleAnnotationFullyPhysicallyBacked(ctx, s.Annotations, wopts)
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

	if _, ok := s.Annotations[annotationBootFilesRootPath]; !ok && opts.BootFilesRootPath != "" {
		s.Annotations[annotationBootFilesRootPath] = opts.BootFilesRootPath
	}

	if _, ok := s.Annotations[annotationProcessorCount]; !ok && opts.VmProcessorCount != 0 {
		s.Annotations[annotationProcessorCount] = strconv.FormatInt(int64(opts.VmProcessorCount), 10)
	}

	if _, ok := s.Annotations[annotationMemorySizeInMB]; !ok && opts.VmMemorySizeInMb != 0 {
		s.Annotations[annotationMemorySizeInMB] = strconv.FormatInt(int64(opts.VmMemorySizeInMb), 10)
	}

	if _, ok := s.Annotations[AnnotationGPUVHDPath]; !ok && opts.GPUVHDPath != "" {
		s.Annotations[AnnotationGPUVHDPath] = opts.GPUVHDPath
	}

	return s
}
