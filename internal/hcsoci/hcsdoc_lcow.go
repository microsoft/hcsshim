//go:build windows
// +build windows

package hcsoci

import (
	"context"
	"encoding/json"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

func createLCOWSpec(ctx context.Context, coi *createOptionsInternal) (*specs.Spec, error) {
	// Remarshal the spec to perform a deep copy.
	j, err := json.Marshal(coi.Spec)
	if err != nil {
		return nil, err
	}
	spec := &specs.Spec{}
	err = json.Unmarshal(j, spec)
	if err != nil {
		return nil, err
	}

	// Linux containers don't care about Windows aspects of the spec except the
	// network namespace and windows devices
	spec.Windows = nil
	if coi.Spec.Windows != nil {
		setWindowsNetworkNamespace(coi, spec)
		setWindowsDevices(coi, spec)
	}

	// Hooks are not supported (they should be run in the host)
	spec.Hooks = nil

	// Clear unsupported features
	spec.Linux.CgroupsPath = "" // GCS controls its cgroups hierarchy on its own.
	if spec.Linux.Resources != nil {
		spec.Linux.Resources.Devices = nil
		spec.Linux.Resources.Pids = nil
		spec.Linux.Resources.BlockIO = nil
		spec.Linux.Resources.HugepageLimits = nil
		spec.Linux.Resources.Network = nil
	}

	if oci.ParseAnnotationsBool(ctx, spec.Annotations, annotations.LCOWPrivileged, false) {
		spec.Linux.Seccomp = nil
	}

	return spec, nil
}

func setWindowsNetworkNamespace(coi *createOptionsInternal, spec *specs.Spec) {
	if coi.Spec.Windows.Network != nil &&
		coi.Spec.Windows.Network.NetworkNamespace != "" {
		if spec.Windows == nil {
			spec.Windows = &specs.Windows{}
		}
		spec.Windows.Network = &specs.WindowsNetwork{
			NetworkNamespace: coi.Spec.Windows.Network.NetworkNamespace,
		}
	}
}

func setWindowsDevices(coi *createOptionsInternal, spec *specs.Spec) {
	if coi.Spec.Windows.Devices != nil {
		if spec.Windows == nil {
			spec.Windows = &specs.Windows{}
		}
		spec.Windows.Devices = coi.Spec.Windows.Devices
	}
}

type linuxHostedSystem struct {
	SchemaVersion    *hcsschema.Version
	OciBundlePath    string
	OciSpecification *specs.Spec

	// ScratchDirPath represents the path inside the UVM at which the container scratch
	// directory is present.  Usually, this is the path at which the container scratch
	// VHD is mounted inside the UVM. But in case of scratch sharing this is a
	// directory under the UVM scratch directory.
	ScratchDirPath string
}

func createLinuxContainerDocument(ctx context.Context, coi *createOptionsInternal, guestRoot, scratchPath string) (*linuxHostedSystem, error) {
	spec, err := createLCOWSpec(ctx, coi)
	if err != nil {
		return nil, err
	}

	log.G(ctx).WithField("guestRoot", guestRoot).Debug("hcsshim::createLinuxContainerDoc")
	return &linuxHostedSystem{
		SchemaVersion:    schemaversion.SchemaV21(),
		OciBundlePath:    guestRoot,
		OciSpecification: spec,
		ScratchDirPath:   scratchPath,
	}, nil
}
