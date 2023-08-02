package ctrdtaskapi

import (
	typeurl "github.com/containerd/typeurl/v2"
)

func init() {
	typeurl.Register(&PolicyFragment{}, "github.com/Microsoft/hcsshim/pkg/ctrdtaskapi", "PolicyFragment")
	typeurl.Register(&MappedPipe{}, "github.com/Microsoft/hcsshim/pkg/ctrdtaskapi", "MappedPipe")
}

type PolicyFragment struct {
	// Fragment is used by containerd to pass additional security policy
	// constraint fragments as part of shim task Update request.
	// The value is a base64 encoded COSE_Sign1 document that contains the
	// fragment and any additional information required for validation.
	Fragment string `json:"fragment,omitempty"`
}

type MappedPipe struct {
	// IsRemove, default false, determines if the operation update is a remove or an add.
	IsRemove bool `json:"remove,omitempty"`
	// HostPath is the host named pipe path to map to the container. It is required for `add` operations.
	HostPath string `json:"hp,omitempty"`
	// ContainerPath is the path name inside the container namespace to map the HostPath to. If it is a remove
	// operation this path is used to determine which container path to remove from the container namespace.
	ContainerPath string `json:"cp,omitempty"`
}
