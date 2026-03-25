//go:build windows && !wcow

package plan9

import (
	"context"
	"sync"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

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

// ==============================================================================
// INTERNAL DATA STRUCTURES
// Types below this line are unexported and used for state tracking.
// ==============================================================================

// shareEntry records one Plan9 share's full lifecycle state and reference count.
type shareEntry struct {
	// mu serializes state transitions.
	mu sync.Mutex

	// opts is the immutable share parameters used to match duplicate add requests.
	opts *AddOptions

	// name is the HCS-level identifier for this share, generated at allocation time.
	name string

	// refCount is the number of active callers sharing this entry.
	// Access must be guarded by [Manager.mu].
	refCount uint

	// state tracks the forward-only lifecycle position of this share.
	// Access must be guarded by mu.
	state shareState

	// stateErr records the error that caused a transition to [shareInvalid].
	// Waiters that find the entry in the invalid state return this error so
	// that every caller sees the original failure reason.
	stateErr error
}

// optionsMatch reports whether two [AddOptions] values describe the same share.
// AllowedNames is compared in order.
func optionsMatch(a, b *AddOptions) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.HostPath != b.HostPath || a.ReadOnly != b.ReadOnly || a.Restrict != b.Restrict {
		return false
	}
	if len(a.AllowedNames) != len(b.AllowedNames) {
		return false
	}
	for i := range a.AllowedNames {
		if a.AllowedNames[i] != b.AllowedNames[i] {
			return false
		}
	}
	return true
}
