//go:build linux
// +build linux

package hcsv2

import "testing"

// TestBundleNeedsScratchBind covers the path-based rule that decides whether a
// container bundle must be bind-mounted onto its scratch disk. The cases mirror
// the layouts observed for both shims:
//   - V1 non-shared: the shim mounts the scratch disk at the bundle path, so the
//     scratch dir is nested under the bundle and no bind is needed.
//   - V1 shared-scratch: the scratch lives under the sandbox's bundle, so a
//     workload's own bundle is not disk-backed and needs a bind.
//   - V2: the scratch is mounted at a global path unrelated to the bundle, so
//     the bundle needs a bind.
func TestBundleNeedsScratchBind(t *testing.T) {
	for _, tc := range []struct {
		name       string
		bundleDir  string
		scratchDir string
		want       bool
	}{
		{
			name:       "no scratch",
			bundleDir:  "/run/gcs/c/abc",
			scratchDir: "",
			want:       false,
		},
		{
			name:       "v1 non-shared scratch under bundle",
			bundleDir:  "/run/gcs/c/abc",
			scratchDir: "/run/gcs/c/abc/scratch/abc",
			want:       false,
		},
		{
			name:       "v1 shared scratch under sandbox bundle",
			bundleDir:  "/run/gcs/c/workload",
			scratchDir: "/run/gcs/c/sandbox/scratch/workload",
			want:       true,
		},
		{
			name:       "v2 scratch on global scsi mount",
			bundleDir:  "/run/gcs/pods/pod/ctr",
			scratchDir: "/run/mounts/scsi/0_2_0/scratch/pod/ctr",
			want:       true,
		},
		{
			name:       "scratch equals bundle",
			bundleDir:  "/run/gcs/c/abc",
			scratchDir: "/run/gcs/c/abc",
			want:       false,
		},
		{
			name:       "sibling dir with shared prefix is not under bundle",
			bundleDir:  "/run/gcs/c/abc",
			scratchDir: "/run/gcs/c/abcdef/scratch/abcdef",
			want:       true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := bundleNeedsScratchBind(tc.bundleDir, tc.scratchDir); got != tc.want {
				t.Errorf("bundleNeedsScratchBind(%q, %q) = %v, want %v", tc.bundleDir, tc.scratchDir, got, tc.want)
			}
		})
	}
}
