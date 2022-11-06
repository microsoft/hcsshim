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

func Test_HostMounts_IsEncrypted(t *testing.T) {
	hm := newHostMounts()
	encryptedPath := "/run/gcs/c/encrypted"
	encryptedSource := "/dev/sda"
	if err := hm.AddRWDevice(encryptedPath, encryptedSource, true); err != nil {
		t.Fatalf("unexpected error adding RW device: %s", err)
	}
	nestedUnencrypted := "/run/gcs/c/encrypted/unencrypted"
	unencryptedSource := "/dev/sdb"
	if err := hm.AddRWDevice(nestedUnencrypted, unencryptedSource, false); err != nil {
		t.Fatalf("unexpected error adding RW device: %s", err)
	}

	for _, tc := range []struct {
		name     string
		testPath string
		expected bool
	}{
		{
			name:     "ValidSubPath1",
			testPath: "/run/gcs/c/encrypted/nested",
			expected: true,
		},
		{
			name:     "ValidSubPath2",
			testPath: "/run/gcs/c/encrypted/../encrypted/nested",
			expected: true,
		},
		{
			name:     "NotSubPath1",
			testPath: "/run/gcs/c/abcdef",
			expected: false,
		},
		{
			name:     "NotSubPath2",
			testPath: "/run/gcs/c",
			expected: false,
		},
		{
			name:     "NotSubPath3",
			testPath: "/run/gcs/c/../abcd",
			expected: false,
		},
		{
			name:     "NestedUnencrypted",
			testPath: "/run/gcs/c/encrypted/unencrypted/foo",
			expected: false,
		},
		{
			name:     "SamePathEncrypted",
			testPath: "/run/gcs/c/encrypted",
			expected: true,
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
