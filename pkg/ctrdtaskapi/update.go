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
	// MediaType is the media type of the blob carried in Fragment. It allows
	// the same delivery mechanism to carry payloads other than Rego policy
	// fragments (e.g. a Transparency Trust List). An empty value is treated by
	// the guest as the default "application/cose-x509+rego" for backward
	// compatibility with older hosts that do not set this field.
	MediaType string `json:"mediaType,omitempty"`
}

type ContainerMount struct {
	HostPath      string
	ContainerPath string
	ReadOnly      bool
	Type          string
}
