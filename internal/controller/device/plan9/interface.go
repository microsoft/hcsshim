//go:build windows && !wcow

package plan9

import (
	"context"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

// Controller manages the lifecycle of Plan9 shares attached to a UVM.
type Controller interface {
	// AddToVM adds a Plan9 share to the host VM and returns the generated share name.
	// Guest-side mount is handled separately by the mount controller.
	AddToVM(ctx context.Context, opts *AddOptions) (string, error)

	// RemoveFromVM removes a Plan9 share identified by shareName from the host VM.
	RemoveFromVM(ctx context.Context, shareName string) error
}

// AddOptions holds the configuration required to add a Plan9 share to the VM.
type AddOptions struct {
	// HostPath is the path on the host to share into the VM.
	HostPath string

	// ReadOnly indicates whether the share should be mounted read-only.
	ReadOnly bool

	// Restrict enables single-file mapping mode for the share.
	Restrict bool

	// AllowedNames is the list of file names allowed when Restrict is true.
	AllowedNames []string
}

// vmPlan9Manager manages adding and removing Plan9 shares on the host VM.
// Implemented by [vmmanager.UtilityVM].
type vmPlan9Manager interface {
	// AddPlan9 adds a plan 9 share to a running Utility VM.
	AddPlan9(ctx context.Context, settings hcsschema.Plan9Share) error

	// RemovePlan9 removes a plan 9 share from a running Utility VM.
	RemovePlan9(ctx context.Context, settings hcsschema.Plan9Share) error
}
