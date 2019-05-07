package lcow

import "github.com/microsoft/hcsshim/internal/schema2"

// Additional fields to hcsschema.ProcessParameters used by LCOW
type ProcessParameters struct {
	hcsschema.ProcessParameters

	CreateInUtilityVm bool        `json:",omitempty"`
	OCIProcess        interface{} `json:"OciProcess,omitempty"`
}
