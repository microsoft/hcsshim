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

	hm.Lock()
	defer hm.Unlock()

	if err := hm.AddRWDevice(mountPath, sourcePath, false); err != nil {
		t.Fatalf("unexpected error adding RW device: %s", err)
	}
	if err := hm.RemoveRWDevice(mountPath, sourcePath, false); err != nil {
		t.Fatalf("unexpected error removing RW device: %s", err)
	}
}

func Test_Cannot_AddRWDevice_Twice(t *testing.T) {
	hm := newHostMounts()
	mountPath := "/run/gcs/c/abc"
	sourcePath := "/dev/sda"

	hm.Lock()
	if err := hm.AddRWDevice(mountPath, sourcePath, false); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	hm.Unlock()

	hm.Lock()
	if err := hm.AddRWDevice(mountPath, sourcePath, false); err == nil {
		t.Fatalf("expected error adding %q for the second time", mountPath)
	}
	hm.Unlock()
}

func Test_Cannot_RemoveRWDevice_Wrong_Source(t *testing.T) {
	hm := newHostMounts()
	hm.Lock()
	defer hm.Unlock()

	mountPath := "/run/gcs/c/abcd"
	sourcePath := "/dev/sda"
	wrongSource := "/dev/sdb"
	if err := hm.AddRWDevice(mountPath, sourcePath, false); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := hm.RemoveRWDevice(mountPath, wrongSource, false); err == nil {
		t.Fatalf("expected error removing wrong source %s", wrongSource)
	}
}

func Test_Cannot_RemoveRWDevice_Wrong_Encrypted(t *testing.T) {
	hm := newHostMounts()
	hm.Lock()
	defer hm.Unlock()

	mountPath := "/run/gcs/c/abcd"
	sourcePath := "/dev/sda"
	if err := hm.AddRWDevice(mountPath, sourcePath, false); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := hm.RemoveRWDevice(mountPath, sourcePath, true); err == nil {
		t.Fatalf("expected error removing RW device with wrong encrypted flag")
	}
}

func Test_HostMounts_IsEncrypted(t *testing.T) {
	hm := newHostMounts()
	hm.Lock()
	defer hm.Unlock()

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

func Test_HostMounts_AddRemoveRODevice(t *testing.T) {
	hm := newHostMounts()
	hm.Lock()
	defer hm.Unlock()

	mountPath := "/run/gcs/c/abcd"
	sourcePath := "/dev/sda"

	if err := hm.AddRODevice(mountPath, sourcePath); err != nil {
		t.Fatalf("unexpected error adding RO device: %s", err)
	}

	if err := hm.RemoveRODevice(mountPath, sourcePath); err != nil {
		t.Fatalf("unexpected error removing RO device: %s", err)
	}
}

func Test_HostMounts_Cannot_AddRODevice_Twice(t *testing.T) {
	hm := newHostMounts()
	hm.Lock()
	defer hm.Unlock()

	mountPath := "/run/gcs/c/abc"
	sourcePath := "/dev/sda"

	if err := hm.AddRODevice(mountPath, sourcePath); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := hm.AddRODevice(mountPath, sourcePath); err == nil {
		t.Fatalf("expected error adding %q for the second time", mountPath)
	}
}

func Test_HostMounts_AddRemoveOverlay(t *testing.T) {
	hm := newHostMounts()
	hm.Lock()
	defer hm.Unlock()

	mountPath := "/run/gcs/c/aaaa/rootfs"
	layers := []string{
		"/run/mounts/scsi/m1",
		"/run/mounts/scsi/m2",
		"/run/mounts/scsi/m3",
	}
	for _, layer := range layers {
		if err := hm.AddRODevice(layer, layer); err != nil {
			t.Fatalf("unexpected error adding RO device: %s", err)
		}
	}
	scratchDir := "/run/gcs/c/aaaa/scratch"
	if err := hm.AddRWDevice(scratchDir, scratchDir, true); err != nil {
		t.Fatalf("unexpected error adding RW device: %s", err)
	}
	if err := hm.AddOverlay(mountPath, layers, scratchDir); err != nil {
		t.Fatalf("unexpected error adding overlay: %s", err)
	}
	undo, err := hm.RemoveOverlay(mountPath)
	if err != nil {
		t.Fatalf("unexpected error removing overlay: %s", err)
	}
	if undo == nil {
		t.Fatalf("expected undo function to be non-nil")
	}
	undo()
	if _, err = hm.RemoveOverlay(mountPath); err != nil {
		t.Fatalf("unexpected error removing overlay again: %s", err)
	}
}

func Test_HostMounts_Cannot_RemoveInUseDeviceByOverlay(t *testing.T) {
	hm := newHostMounts()
	hm.Lock()
	defer hm.Unlock()

	mountPath := "/run/gcs/c/aaaa/rootfs"
	layers := []string{
		"/run/mounts/scsi/m1",
		"/run/mounts/scsi/m2",
		"/run/mounts/scsi/m3",
	}
	for _, layer := range layers {
		if err := hm.AddRODevice(layer, layer); err != nil {
			t.Fatalf("unexpected error adding RO device: %s", err)
		}
	}
	scratchDir := "/run/gcs/c/aaaa/scratch"
	if err := hm.AddRWDevice(scratchDir, scratchDir, true); err != nil {
		t.Fatalf("unexpected error adding RW device: %s", err)
	}
	if err := hm.AddOverlay(mountPath, layers, scratchDir); err != nil {
		t.Fatalf("unexpected error adding overlay: %s", err)
	}

	for _, layer := range layers {
		if err := hm.RemoveRODevice(layer, layer); err == nil {
			t.Fatalf("expected error removing RO device %s while in use by overlay", layer)
		}
	}
	if err := hm.RemoveRWDevice(scratchDir, scratchDir, true); err == nil {
		t.Fatalf("expected error removing RW device %s while in use by overlay", scratchDir)
	}

	if _, err := hm.RemoveOverlay(mountPath); err != nil {
		t.Fatalf("unexpected error removing overlay: %s", err)
	}

	// now we can remove
	for _, layer := range layers {
		if err := hm.RemoveRODevice(layer, layer); err != nil {
			t.Fatalf("unexpected error removing RO device %s: %s", layer, err)
		}
	}
	if err := hm.RemoveRWDevice(scratchDir, scratchDir, true); err != nil {
		t.Fatalf("unexpected error removing RW device %s: %s", scratchDir, err)
	}
}

func Test_HostMounts_Cannot_RemoveInUseDeviceByOverlay_MultipleUsers(t *testing.T) {
	hm := newHostMounts()
	hm.Lock()
	defer hm.Unlock()

	overlay1 := "/run/gcs/c/aaaa/rootfs"
	overlay2 := "/run/gcs/c/bbbb/rootfs"
	layers := []string{
		"/run/mounts/scsi/m1",
		"/run/mounts/scsi/m2",
		"/run/mounts/scsi/m3",
	}
	for _, layer := range layers {
		if err := hm.AddRODevice(layer, layer); err != nil {
			t.Fatalf("unexpected error adding RO device: %s", err)
		}
	}
	sharedScratchMount := "/run/gcs/c/sandbox"
	scratch1 := sharedScratchMount + "/scratch/aaaa"
	scratch2 := sharedScratchMount + "/scratch/bbbb"
	if err := hm.AddRWDevice(sharedScratchMount, sharedScratchMount, true); err != nil {
		t.Fatalf("unexpected error adding RW device: %s", err)
	}
	if err := hm.AddOverlay(overlay1, layers, scratch1); err != nil {
		t.Fatalf("unexpected error adding overlay1: %s", err)
	}

	if err := hm.AddOverlay(overlay2, layers[0:2], scratch2); err != nil {
		t.Fatalf("unexpected error adding overlay2: %s", err)
	}

	for _, layer := range layers {
		if err := hm.RemoveRODevice(layer, layer); err == nil {
			t.Fatalf("expected error removing RO device %s while in use by overlay", layer)
		}
	}
	if err := hm.RemoveRWDevice(sharedScratchMount, sharedScratchMount, true); err == nil {
		t.Fatalf("expected error removing RW device %s while in use by overlay", sharedScratchMount)
	}

	if _, err := hm.RemoveOverlay(overlay1); err != nil {
		t.Fatalf("unexpected error removing overlay 1: %s", err)
	}

	for _, layer := range layers[0:2] {
		if err := hm.RemoveRODevice(layer, layer); err == nil {
			t.Fatalf("expected error removing RO device %s (still in use by overlay 2)", layer)
		}
	}
	if err := hm.RemoveRODevice(layers[2], layers[2]); err != nil {
		t.Fatalf("unexpected error removing layers[2] which is not being used by overlay 2: %s", err)
	}
	if err := hm.RemoveRWDevice(sharedScratchMount, sharedScratchMount, true); err == nil {
		t.Fatalf("expected error removing RW device %s while in use by overlay 2", scratch2)
	}

	if _, err := hm.RemoveOverlay(overlay2); err != nil {
		t.Fatalf("unexpected error removing overlay 2: %s", err)
	}
	for _, layer := range layers[0:2] {
		if err := hm.RemoveRODevice(layer, layer); err != nil {
			t.Fatalf("unexpected error removing RO device %s: %s", layer, err)
		}
	}
	if err := hm.RemoveRWDevice(sharedScratchMount, sharedScratchMount, true); err != nil {
		t.Fatalf("unexpected error removing RW device %s: %s", sharedScratchMount, err)
	}
}
