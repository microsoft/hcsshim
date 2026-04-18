//go:build windows && lcow

package share

import (
	"context"
	"slices"
	"strings"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

// Config describes the host-side Plan9 share to add to the VM.
type Config struct {
	// HostPath is the path on the host to share into the VM.
	HostPath string
	// ReadOnly specifies whether the share should be read-only.
	ReadOnly bool
	// Restrict enables single-file mapping mode for the share.
	Restrict bool
	// AllowedNames is the list of file names allowed when Restrict is true.
	AllowedNames []string
}

// Equals reports whether two share Config values describe the same share parameters.
func (c Config) Equals(other Config) bool {
	cmpFoldCase := func(a, b string) int {
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	}

	return c.HostPath == other.HostPath &&
		c.ReadOnly == other.ReadOnly &&
		c.Restrict == other.Restrict &&
		slices.EqualFunc(
			slices.SortedFunc(slices.Values(c.AllowedNames), cmpFoldCase),
			slices.SortedFunc(slices.Values(other.AllowedNames), cmpFoldCase),
			strings.EqualFold,
		)
}

// VMPlan9Adder adds a Plan9 share to a Utility VM.
type VMPlan9Adder interface {
	// AddPlan9 adds a Plan9 share to a running Utility VM.
	AddPlan9(ctx context.Context, settings hcsschema.Plan9Share) error
}

// VMPlan9Remover removes a Plan9 share from a Utility VM.
type VMPlan9Remover interface {
	// RemovePlan9 removes a Plan9 share from a running Utility VM.
	RemovePlan9(ctx context.Context, settings hcsschema.Plan9Share) error
}
