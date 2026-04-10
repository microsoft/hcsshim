//go:build windows && lcow

package linuxcontainer

import (
	"context"
	"encoding/json"
	"fmt"

	containerdtypes "github.com/containerd/containerd/api/types"
	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/hcsshim/internal/guestpath"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/ospath"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/Microsoft/hcsshim/pkg/annotations"
)

// vmHostedContainerSettingsV2 defines the portion of the container
// configuration sent via a V2 GCS call for LCOW containers.
type vmHostedContainerSettingsV2 struct {
	SchemaVersion    *hcsschema.Version
	OCIBundlePath    string      `json:"OciBundlePath,omitempty"`
	OCISpecification *specs.Spec `json:"OciSpecification,omitempty"`
	// ScratchDirPath is the path inside the UVM where the container scratch
	// directory is present. Usually the mount path of the scratch VHD, but
	// with scratch sharing it becomes a sub-directory under the UVM scratch.
	ScratchDirPath string
}

// generateContainerDocument allocates all host-side resources (layers, mounts, devices)
// and returns the GCS container configuration document.
func (c *Controller) generateContainerDocument(
	ctx context.Context,
	spec *specs.Spec,
	rootfs []*containerdtypes.Mount,
	isScratchEncryptionEnabled bool,
) (*vmHostedContainerSettingsV2, error) {
	if spec.Linux == nil {
		return nil, fmt.Errorf("linux section must be present for lcow container")
	}

	// If windows section is not present, add an empty section
	// to avoid nil dereference in downstream code.
	if spec.Windows == nil {
		spec.Windows = &specs.Windows{}
	}

	// Allocate host-side resources: layers, mounts, and devices.

	if err := c.allocateLayers(ctx, spec.Windows.LayerFolders, rootfs, isScratchEncryptionEnabled); err != nil {
		return nil, fmt.Errorf("allocate layers: %w", err)
	}

	if err := c.allocateMounts(ctx, spec); err != nil {
		return nil, fmt.Errorf("allocate mounts: %w", err)
	}

	if err := c.allocateDevices(ctx, spec); err != nil {
		return nil, fmt.Errorf("allocate devices: %w", err)
	}

	// Set the rootfs path for the container within guest.
	if spec.Root == nil {
		spec.Root = &specs.Root{}
	}
	spec.Root.Path = c.layers.rootfsPath

	// Build a sanitized deep copy of the spec for the guest.
	linuxSpec, err := sanitizeSpec(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("sanitize spec: %w", err)
	}

	return &vmHostedContainerSettingsV2{
		SchemaVersion:    schemaversion.SchemaV21(),
		OCIBundlePath:    ospath.Join("linux", guestpath.LCOWV2RootPrefixInVM, c.gcsPodID, c.gcsContainerID),
		OCISpecification: linuxSpec,
		ScratchDirPath:   c.layers.scratch.guestPath,
	}, nil
}

// sanitizeSpec deep-copies the OCI spec and strips fields unsupported by the GCS.
func sanitizeSpec(ctx context.Context, origSpec *specs.Spec) (*specs.Spec, error) {
	// Deep copy via JSON round-trip so mutations do not affect the caller.
	raw, err := json.Marshal(origSpec)
	if err != nil {
		return nil, fmt.Errorf("marshal spec: %w", err)
	}
	spec := &specs.Spec{}
	if err = json.Unmarshal(raw, spec); err != nil {
		return nil, fmt.Errorf("unmarshal spec: %w", err)
	}

	// Preserve only the network namespace and assigned devices from the Windows section.
	spec.Windows = extractWindowsFields(origSpec)

	// Hooks are executed on the host, not inside the guest.
	spec.Hooks = nil

	// Apply safe CPU defaults when values are explicitly zeroed.
	if spec.Linux.Resources != nil && spec.Linux.Resources.CPU != nil {
		cpu := spec.Linux.Resources.CPU
		if cpu.Period != nil && *cpu.Period == 0 {
			*cpu.Period = 100000
		}
		if cpu.Quota != nil && *cpu.Quota == 0 {
			*cpu.Quota = -1
		}
	}

	// Clear resource types the GCS manages on its own.
	spec.Linux.CgroupsPath = ""
	if spec.Linux.Resources != nil {
		spec.Linux.Resources.Devices = nil
		spec.Linux.Resources.Pids = nil
		spec.Linux.Resources.BlockIO = nil
		spec.Linux.Resources.HugepageLimits = nil
		spec.Linux.Resources.Network = nil
	}

	// Disable seccomp for privileged containers.
	if oci.ParseAnnotationsBool(ctx, spec.Annotations, annotations.LCOWPrivileged, false) {
		spec.Linux.Seccomp = nil
	}

	return spec, nil
}

// extractWindowsFields keeps only the network namespace and assigned devices.
func extractWindowsFields(origSpec *specs.Spec) *specs.Windows {
	var win *specs.Windows

	if origSpec.Windows.Network != nil && origSpec.Windows.Network.NetworkNamespace != "" {
		win = &specs.Windows{
			Network: &specs.WindowsNetwork{
				NetworkNamespace: origSpec.Windows.Network.NetworkNamespace,
			},
		}
	}

	if len(origSpec.Windows.Devices) > 0 {
		if win == nil {
			win = &specs.Windows{}
		}
		win.Devices = origSpec.Windows.Devices
	}

	return win
}
