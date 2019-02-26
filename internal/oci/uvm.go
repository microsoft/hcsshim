package oci

import (
	"errors"
	"strconv"
	"strings"

	runhcsopts "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

const (
	annotationAllowOvercommit      = "io.microsoft.virtualmachine.computetopology.memory.allowovercommit"
	annotationEnableDeferredCommit = "io.microsoft.virtualmachine.computetopology.memory.enabledeferredcommit"
	annotationMemorySizeInMB       = "io.microsoft.virtualmachine.computetopology.memory.sizeinmb"
	annotationProcessorCount       = "io.microsoft.virtualmachine.computetopology.processor.count"
	annotationVPMemCount           = "io.microsoft.virtualmachine.devices.virtualpmem.maximumcount"
	annotationVPMemSize            = "io.microsoft.virtualmachine.devices.virtualpmem.maximumsizebytes"
	annotationPreferredRootFSType  = "io.microsoft.virtualmachine.lcow.preferredrootfstype"
	annotationBootFilesRootPath    = "io.microsoft.virtualmachine.lcow.bootfilesrootpath"
)

// parseAnnotationsBool searches `a` for `key` and if found verifies that the
// value is `true` or `false` in any case. If `key` is not found returns `def`.
func parseAnnotationsBool(a map[string]string, key string, def bool) bool {
	if v, ok := a[key]; ok {
		switch strings.ToLower(v) {
		case "true":
			return true
		case "false":
			return false
		default:
			logrus.WithFields(logrus.Fields{
				logfields.OCIAnnotation: key,
				logfields.Value:         v,
				logfields.ExpectedType:  logfields.Bool,
			}).Warning("annotation could not be parsed")
		}
	}
	return def
}

// parseAnnotationsCPU searches `s.Annotations` for the CPU annotation. If
// not found searches `s` for the Windows CPU section. If neither are found
// returns `def`.
func parseAnnotationsCPU(s *specs.Spec, annotation string, def int32) int32 {
	if m := parseAnnotationsUint64(s.Annotations, annotation, 0); m != 0 {
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

// parseAnnotationsMemory searches `s.Annotations` for the memory annotation. If
// not found searches `s` for the Windows memory section. If neither are found
// returns `def`.
func parseAnnotationsMemory(s *specs.Spec, annotation string, def int32) int32 {
	if m := parseAnnotationsUint64(s.Annotations, annotation, 0); m != 0 {
		return int32(m)
	}
	if s.Windows != nil &&
		s.Windows.Resources != nil &&
		s.Windows.Resources.Memory != nil &&
		s.Windows.Resources.Memory.Limit != nil &&
		*s.Windows.Resources.Memory.Limit > 0 {
		return int32(*s.Windows.Resources.Memory.Limit)
	}
	return def
}

// parseAnnotationsPreferredRootFSType searches `a` for `key` and verifies that the
// value is in the set of allowed values. If `key` is not found returns `def`.
func parseAnnotationsPreferredRootFSType(a map[string]string, key string, def uvm.PreferredRootFSType) uvm.PreferredRootFSType {
	if v, ok := a[key]; ok {
		switch v {
		case "initrd":
			return uvm.PreferredRootFSTypeInitRd
		case "vhd":
			return uvm.PreferredRootFSTypeVHD
		default:
			logrus.Warningf("annotation: '%s', with value: '%s' must be 'initrd' or 'vhd'", key, v)
		}
	}
	return def
}

// parseAnnotationsUint32 searches `a` for `key` and if found verifies that the
// value is a 32 bit unsigned integer. If `key` is not found returns `def`.
func parseAnnotationsUint32(a map[string]string, key string, def uint32) uint32 {
	if v, ok := a[key]; ok {
		countu, err := strconv.ParseUint(v, 10, 32)
		if err == nil {
			v := uint32(countu)
			return v
		}
		logrus.WithFields(logrus.Fields{
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
func parseAnnotationsUint64(a map[string]string, key string, def uint64) uint64 {
	if v, ok := a[key]; ok {
		countu, err := strconv.ParseUint(v, 10, 64)
		if err == nil {
			return countu
		}
		logrus.WithFields(logrus.Fields{
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

// SpecToUVMCreateOpts parses `s` and returns either `*uvm.OptionsLCOW` or
// `*uvm.OptionsWCOW`.
func SpecToUVMCreateOpts(s *specs.Spec, id, owner string) (interface{}, error) {
	if !IsIsolated(s) {
		return nil, errors.New("cannot create UVM opts for non-isolated spec")
	}
	if IsLCOW(s) {
		lopts := uvm.NewDefaultOptionsLCOW(id, owner)
		lopts.MemorySizeInMB = parseAnnotationsMemory(s, annotationMemorySizeInMB, lopts.MemorySizeInMB)
		lopts.AllowOvercommit = parseAnnotationsBool(s.Annotations, annotationAllowOvercommit, lopts.AllowOvercommit)
		lopts.EnableDeferredCommit = parseAnnotationsBool(s.Annotations, annotationEnableDeferredCommit, lopts.EnableDeferredCommit)
		lopts.ProcessorCount = parseAnnotationsCPU(s, annotationProcessorCount, lopts.ProcessorCount)
		lopts.VPMemDeviceCount = parseAnnotationsUint32(s.Annotations, annotationVPMemCount, lopts.VPMemDeviceCount)
		lopts.VPMemSizeBytes = parseAnnotationsUint64(s.Annotations, annotationVPMemSize, lopts.VPMemSizeBytes)
		lopts.PreferredRootFSType = parseAnnotationsPreferredRootFSType(s.Annotations, annotationPreferredRootFSType, lopts.PreferredRootFSType)
		switch lopts.PreferredRootFSType {
		case uvm.PreferredRootFSTypeInitRd:
			lopts.RootFSFile = uvm.InitrdFile
		case uvm.PreferredRootFSTypeVHD:
			lopts.RootFSFile = uvm.VhdFile
		}
		lopts.BootFilesPath = parseAnnotationsString(s.Annotations, annotationBootFilesRootPath, lopts.BootFilesPath)
		return lopts, nil
	} else if IsWCOW(s) {
		wopts := uvm.NewDefaultOptionsWCOW(id, owner)
		wopts.MemorySizeInMB = parseAnnotationsMemory(s, annotationMemorySizeInMB, wopts.MemorySizeInMB)
		wopts.AllowOvercommit = parseAnnotationsBool(s.Annotations, annotationAllowOvercommit, wopts.AllowOvercommit)
		wopts.EnableDeferredCommit = parseAnnotationsBool(s.Annotations, annotationEnableDeferredCommit, wopts.EnableDeferredCommit)
		wopts.ProcessorCount = parseAnnotationsCPU(s, annotationProcessorCount, wopts.ProcessorCount)
		return wopts, nil
	}
	return nil, errors.New("cannot create UVM opts spec is not LCOW or WCOW")
}

// UpdateSpecFromOptions sets extra annotations on the OCI spec based on the
// `opts` struct.
func UpdateSpecFromOptions(s specs.Spec, opts *runhcsopts.Options) specs.Spec {
	if opts.BootFilesRootPath != "" {
		s.Annotations[annotationBootFilesRootPath] = opts.BootFilesRootPath
	}

	return s
}
