package gcscompat

import "testing"

func TestCompatible(t *testing.T) {
	cases := []struct {
		name                   string
		lMin, lMax, rMin, rMax uint32
		want                   bool
	}{
		{"identical point", 1, 1, 1, 1, true},
		{"identical range", 4, 6, 4, 6, true},
		{"overlap", 4, 6, 2, 5, true},
		{"touching at low edge", 4, 6, 1, 4, true},
		{"touching at high edge", 4, 6, 6, 9, true},
		{"remote strictly below", 4, 6, 1, 3, false},
		{"remote strictly above", 4, 6, 7, 9, false},
		{"remote is a superset", 4, 6, 1, 9, true},
		{"remote is a subset", 1, 9, 4, 6, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Compatible(tc.lMin, tc.lMax, tc.rMin, tc.rMax); got != tc.want {
				t.Fatalf("Compatible(%d,%d,%d,%d) = %v, want %v",
					tc.lMin, tc.lMax, tc.rMin, tc.rMax, got, tc.want)
			}
		})
	}
}

// TestCompatibleSymmetric verifies that swapping the local and remote ranges
// does not change the verdict, so either the host or the guest can evaluate it.
func TestCompatibleSymmetric(t *testing.T) {
	ranges := [][4]uint32{
		{4, 6, 2, 5},
		{4, 6, 1, 3},
		{1, 1, 2, 2},
		{1, 3, 3, 5},
	}
	for _, r := range ranges {
		forward := Compatible(r[0], r[1], r[2], r[3])
		reverse := Compatible(r[2], r[3], r[0], r[1])
		if forward != reverse {
			t.Fatalf("asymmetric verdict for %v: forward=%v reverse=%v", r, forward, reverse)
		}
	}
}

// TestSelfCompatible verifies that a binary is always compatible with another
// build of itself (identical range), which must hold for the common case.
func TestSelfCompatible(t *testing.T) {
	if !Compatible(MinCompatibleContractVersion, GuestHostContractVersion,
		MinCompatibleContractVersion, GuestHostContractVersion) {
		t.Fatal("a binary must be compatible with its own contract range")
	}
}
