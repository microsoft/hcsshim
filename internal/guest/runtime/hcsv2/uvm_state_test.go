//go:build linux
// +build linux

package hcsv2

import (
	"testing"
)

func Test_Add_Remove_RWDevice(t *testing.T) {
	hm := newHostMounts()
	mountPath := "/run/gcs/c/abcd"
	sourcePath := "/dev/sda"

	if err := hm.AddRWDevice(mountPath, sourcePath, false); err != nil {
		t.Fatalf("unexpected error adding RW device: %s", err)
	}
	if err := hm.RemoveRWDevice(mountPath, sourcePath); err != nil {
		t.Fatalf("unexpected error removing RW device: %s", err)
	}
}

func Test_Cannot_AddRWDevice_Twice(t *testing.T) {
	hm := newHostMounts()
	mountPath := "/run/gcs/c/abc"
	sourcePath := "/dev/sda"

	if err := hm.AddRWDevice(mountPath, sourcePath, false); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := hm.AddRWDevice(mountPath, sourcePath, false); err == nil {
		t.Fatalf("expected error adding %q for the second time", mountPath)
	}
}

func Test_Cannot_RemoveRWDevice_Wrong_Source(t *testing.T) {
	hm := newHostMounts()
	mountPath := "/run/gcs/c/abcd"
	sourcePath := "/dev/sda"
	wrongSource := "/dev/sdb"
	if err := hm.AddRWDevice(mountPath, sourcePath, false); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := hm.RemoveRWDevice(mountPath, wrongSource); err == nil {
		t.Fatalf("expected error removing wrong source %s", wrongSource)
	}
}

func Test_Cannot_Remove_With_Active_Overlays(t *testing.T) {
	hm := newHostMounts()
	mountPath := "/run/gcs/c/abc"
	sourcePath := "/dev/sda"
	hm.readWriteMounts[mountPath] = &rwDevice{
		mountPath:  mountPath,
		sourcePath: sourcePath,
		encrypted:  true,
		overlays: map[string]struct{}{
			mountPath + "/nested": {},
		},
	}
	if err := hm.RemoveRWDevice(mountPath, sourcePath); err == nil {
		t.Fatalf("expected error removing RW device with active overlays")
	}
}

func Test_HostMounts_IsEncrypted(t *testing.T) {
	hm := newHostMounts()
	mountPath := "/run/gcs/c/abcd"
	sourcePath := "/dev/sda"
	if err := hm.AddRWDevice(mountPath, sourcePath, true); err != nil {
		t.Fatalf("unexpected error adding RW device: %s", err)
	}

	for _, tc := range []struct {
		name     string
		testPath string
		expected bool
	}{
		{
			name:     "ValidSubPath1",
			testPath: "/run/gcs/c/abcd/nested",
			expected: true,
		},
		{
			name:     "ValidSubPath2",
			testPath: "/run/gcs/c/abcd/../abcd/nested",
			expected: true,
		},
		{
			name:     "NotSubPath2",
			testPath: "/run/gcs/c/abcdef",
			expected: false,
		},
		{
			name:     "NotSubPath2",
			testPath: "/run/gcs/c/../abcd",
			expected: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			encrypted := hm.IsEncrypted(tc.testPath)
			if encrypted != tc.expected {
				t.Fatalf("expected encrypted %t, got %t", tc.expected, encrypted)
			}
		})
	}
}
