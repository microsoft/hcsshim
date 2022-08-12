package ctrdtaskapi

import (
	"github.com/containerd/typeurl"
)

func init() {
	typeurl.Register(&PolicyFragment{}, "github.com/Microsoft/hcsshim/pkg/ctrdtaskapi", "PolicyFragment")
}

type PolicyFragment struct {
	// Fragment is used by containerd to pass additional security policy
	// constraint fragments as part of shim task Update request.
	Fragment string `json:"fragment,omitempty"`
	// Annotations hold arbitrary additional information that can be used to
	// (e.g.) provide more context about Fragment.
	Annotations map[string]string `json:"annotations,omitempty"`
}
