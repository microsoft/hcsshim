package ctrdtaskapi

import (
	typeurl "github.com/containerd/typeurl/v2"
)

func init() {
	typeurl.Register(&PolicyFragment{}, "github.com/Microsoft/hcsshim/pkg/ctrdtaskapi", "PolicyFragment")
	typeurl.Register(&ContainerMount{}, "github.com/Microsoft/hcsshim/pkg/ctrdtaskapi", "ContainerMount")
}

type PolicyFragment struct {
	// Fragment is used by containerd to pass additional security policy
	// constraint fragments as part of shim task Update request.
	// The value is a base64 encoded COSE_Sign1 document that contains the
	// fragment and any additional information required for validation.
	Fragment string `json:"fragment,omitempty"`
}

type ContainerMount struct {
	HostPath      string
	ContainerPath string
	ReadOnly      bool
	Type          string
}
