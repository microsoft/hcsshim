//go:build windows
// +build windows

package hcsoci

import (
	"errors"
	"testing"

	specs "github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/hcsshim/internal/jobobject"
)

func TestValidateCPUAffinityEntries(t *testing.T) {
	// A zero mask is invalid on every OS version, so this case is host-independent.
	if _, err := ValidateCPUAffinityEntries([]specs.WindowsCPUGroupAffinity{{Group: 0, Mask: 0}}); !errors.Is(err, ErrCPUAffinityMaskZero) {
		t.Fatalf("zero mask: got %v, want %v", err, ErrCPUAffinityMaskZero)
	}

	// Empty input validates to no entries (no affinity requested).
	got, err := ValidateCPUAffinityEntries(nil)
	if err != nil || got != nil {
		t.Fatalf("nil input: got (%v, %v), want (nil, nil)", got, err)
	}

	// A single group-0 entry with a non-zero mask is valid regardless of OS version.
	in := []specs.WindowsCPUGroupAffinity{{Group: 0, Mask: 0x3}}
	got, err = ValidateCPUAffinityEntries(in)
	if err != nil {
		t.Fatalf("group-0 single entry: unexpected error %v", err)
	}
	if len(got) != 1 || got[0] != in[0] {
		t.Fatalf("group-0 single entry: got %+v, want %+v", got, in)
	}
}

func TestToJobObjectAffinities(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []specs.WindowsCPUGroupAffinity
		want []jobobject.GroupAffinity
	}{
		{
			name: "nil",
			in:   nil,
			want: nil,
		},
		{
			name: "empty",
			in:   []specs.WindowsCPUGroupAffinity{},
			want: nil,
		},
		{
			name: "single group",
			in:   []specs.WindowsCPUGroupAffinity{{Group: 0, Mask: 0b1011}},
			want: []jobobject.GroupAffinity{{Group: 0, Mask: 0b1011}},
		},
		{
			name: "multiple groups",
			in: []specs.WindowsCPUGroupAffinity{
				{Group: 0, Mask: 0xff},
				{Group: 1, Mask: 0x1},
			},
			want: []jobobject.GroupAffinity{
				{Group: 0, Mask: 0xff},
				{Group: 1, Mask: 0x1},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ToJobObjectAffinities(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d entries, want %d", len(got), len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("entry %d: got %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}
