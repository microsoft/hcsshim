package vm

import (
	"github.com/containerd/typeurl/v2"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

func init() {
	typeurl.Register(&Spec{}, "sandbox-spec-v2/vm/Spec", "Spec")
}

// Spec holds the subset of PodSandboxConfig that the VM sandbox
// implementation needs.
type Spec struct {
	// Unstructured key-value map that may be set by the kubelet to store and
	// retrieve arbitrary metadata.
	Annotations map[string]string `json:"annotations,omitempty"`

	// Devices holds Windows devices that should be assigned to the sandbox VM
	// at boot time.
	Devices []specs.WindowsDevice `json:"devices,omitempty"`
}
