// +build windows

package hcsoci

import (
	"encoding/json"

	"github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
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

	// TODO
	// Translate the mounts. The root has already been translated in
	// allocateLinuxResources.
	/*
		for i := range spec.Mounts {
			spec.Mounts[i].Source = "???"
			spec.Mounts[i].Destination = "???"
		}
	*/

	// Linux containers don't care about Windows aspects of the spec
	spec.Windows = nil

	// Hooks are not supported (they should be run in the host)
	spec.Hooks = nil

	// Clear unsupported features
	if spec.Linux.Resources != nil {
		spec.Linux.Resources.Devices = nil
		spec.Linux.Resources.Memory = nil
		spec.Linux.Resources.Pids = nil
		spec.Linux.Resources.BlockIO = nil
		spec.Linux.Resources.HugepageLimits = nil
		spec.Linux.Resources.Network = nil
	}
	spec.Linux.Seccomp = nil

	// Clear any specified namespaces
	var namespaces []specs.LinuxNamespace
	for _, ns := range spec.Linux.Namespaces {
		switch ns.Type {
		case specs.NetworkNamespace:
		default:
			ns.Path = ""
			namespaces = append(namespaces, ns)
		}
	}
	spec.Linux.Namespaces = namespaces

	return spec, nil
}

type linuxHostedSystem struct {
	SchemaVersion    *schemaversion.SchemaVersion
	OciBundlePath    string
	OciSpecification *specs.Spec
}

func createLinuxContainerDocument(coi *createOptionsInternal, guestRoot string) (interface{}, error) {
	spec, err := createLCOWSpec(coi)
	if err != nil {
		return nil, err
	}

	logrus.Debugf("hcsshim::createLinuxContainerDoc: guestRoot:%s", guestRoot)
	v2 := &schema2.ComputeSystemV2{
		Owner:                             coi.actualOwner,
		SchemaVersion:                     schemaversion.SchemaV20(),
		ShouldTerminateOnLastHandleClosed: true,
		HostingSystemId:                   coi.HostingSystem.ID(),
		HostedSystem: &linuxHostedSystem{
			SchemaVersion:    schemaversion.SchemaV20(),
			OciBundlePath:    guestRoot,
			OciSpecification: spec,
		},
	}

	return v2, nil
}
