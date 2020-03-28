// +build windows

package hcsoci

import (
	"context"
	"encoding/json"

	"github.com/Microsoft/hcsshim/internal/log"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

func createLCOWSpec(coi *createOptionsInternal) (*specs.Spec, error) {
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
		spec.Windows.Devices = coi.Spec.Windows.Devices
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
	spec.Linux.Seccomp = nil

	return spec, nil
}

func setWindowsNetworkNamespace(coi *createOptionsInternal, spec *specs.Spec) {
	if coi.Spec.Windows.Network != nil &&
		coi.Spec.Windows.Network.NetworkNamespace != "" {
		spec.Windows = &specs.Windows{
			Network: &specs.WindowsNetwork{
				NetworkNamespace: coi.Spec.Windows.Network.NetworkNamespace,
			},
		}
	}
}

type linuxHostedSystem struct {
	SchemaVersion    *hcsschema.Version
	OciBundlePath    string
	OciSpecification *specs.Spec
}

func createLinuxContainerDocument(ctx context.Context, coi *createOptionsInternal, guestRoot string) (*linuxHostedSystem, error) {
	spec, err := createLCOWSpec(coi)
	if err != nil {
		return nil, err
	}

	log.G(ctx).WithField("guestRoot", guestRoot).Debug("hcsshim::createLinuxContainerDoc")
	return &linuxHostedSystem{
		SchemaVersion:    schemaversion.SchemaV21(),
		OciBundlePath:    guestRoot,
		OciSpecification: spec,
	}, nil
}
