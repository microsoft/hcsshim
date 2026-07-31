//go:build linux
// +build linux

package hcsv2

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	oci "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/sys/unix"
)

func Test_isStrictSubPath(t *testing.T) {
	for _, tc := range []struct {
		base   string
		target string
		want   bool
	}{
		{"/mnt/data", "/mnt/data/subdir", true},
		{"/mnt/data", "/mnt/data", false},
		{"/mnt/data", "/mnt/database", false},
		{"/mnt/data", "/mnt", false},
		{"/mnt/data/", "/mnt/data/subdir/", true},
		{"/", "/etc", true},
		{"/mnt/data", "/other", false},
	} {
		if got := isStrictSubPath(tc.base, tc.target); got != tc.want {
			t.Errorf("isStrictSubPath(%q, %q) = %v, want %v", tc.base, tc.target, got, tc.want)
		}
	}
}

func Test_mountIsReadonly(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options []string
		want    bool
	}{
		{"none", []string{"bind"}, false},
		{"ro", []string{"bind", "ro"}, true},
		{"rw", []string{"bind", "rw"}, false},
		{"ro_then_rw", []string{"ro", "rw"}, false},
		{"rw_then_ro", []string{"rw", "ro"}, true},
		{"empty", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := mountIsReadonly(oci.Mount{Options: tc.options}); got != tc.want {
				t.Errorf("mountIsReadonly(%v) = %v, want %v", tc.options, got, tc.want)
			}
		})
	}
}

func Test_mountIsBind(t *testing.T) {
	for _, tc := range []struct {
		name string
		m    oci.Mount
		want bool
	}{
		{"type_bind", oci.Mount{Type: "bind"}, true},
		{"opt_bind", oci.Mount{Options: []string{"bind"}}, true},
		{"opt_rbind", oci.Mount{Options: []string{"rbind"}}, true},
		{"tmpfs", oci.Mount{Type: "tmpfs", Source: "none"}, false},
		{"empty", oci.Mount{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := mountIsBind(tc.m); got != tc.want {
				t.Errorf("mountIsBind(%+v) = %v, want %v", tc.m, got, tc.want)
			}
		})
	}
}

func Test_deepestParentMount(t *testing.T) {
	mounts := []oci.Mount{
		{Destination: "/mnt/data"},
		{Destination: "/mnt/data/a"},
		{Destination: "/other"},
	}
	for _, tc := range []struct {
		name     string
		dest     string
		wantDest string
		wantOK   bool
	}{
		{"direct_child", "/mnt/data/x", "/mnt/data", true},
		{"deepest_wins", "/mnt/data/a/b", "/mnt/data/a", true},
		{"self_excluded", "/mnt/data", "", false},
		{"no_parent", "/nope", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := deepestParentMount(tc.dest, mounts)
			if ok != tc.wantOK {
				t.Fatalf("deepestParentMount(%q) ok = %v, want %v", tc.dest, ok, tc.wantOK)
			}
			if ok && got.Destination != tc.wantDest {
				t.Errorf("deepestParentMount(%q) = %q, want %q", tc.dest, got.Destination, tc.wantDest)
			}
		})
	}
}

// Test_ensureNestedMountTargets_ReadonlyParent verifies the reported bug is
// fixed: a volume mounted into a subdirectory of a read-only volume gets its
// mountpoint created inside the read-only parent's (writable) source.
func Test_ensureNestedMountTargets_ReadonlyParent(t *testing.T) {
	parentSrc := t.TempDir()
	childSrc := t.TempDir()

	spec := &oci.Spec{
		Mounts: []oci.Mount{
			{
				Destination: "/mnt/data",
				Source:      parentSrc,
				Type:        "bind",
				Options:     []string{"bind", "ro"},
			},
			{
				Destination: "/mnt/data/subdir",
				Source:      childSrc,
				Type:        "bind",
				Options:     []string{"bind"},
			},
		},
	}

	ensureNestedMountTargets(context.Background(), spec)

	created := filepath.Join(parentSrc, "subdir")
	info, err := os.Stat(created)
	if err != nil {
		t.Fatalf("expected mountpoint %q to be created: %v", created, err)
	}
	if !info.IsDir() {
		t.Errorf("expected %q to be a directory", created)
	}
}

func Test_ensureNestedMountTargets_MultiLevelRelPath(t *testing.T) {
	parentSrc := t.TempDir()
	childSrc := t.TempDir()

	spec := &oci.Spec{
		Mounts: []oci.Mount{
			{
				Destination: "/etc/coredns",
				Source:      parentSrc,
				Type:        "bind",
				Options:     []string{"bind", "ro"},
			},
			{
				Destination: "/etc/coredns/a/b/custom",
				Source:      childSrc,
				Type:        "bind",
				Options:     []string{"bind"},
			},
		},
	}

	ensureNestedMountTargets(context.Background(), spec)

	created := filepath.Join(parentSrc, "a", "b", "custom")
	if info, err := os.Stat(created); err != nil {
		t.Fatalf("expected nested mountpoint %q to be created: %v", created, err)
	} else if !info.IsDir() {
		t.Errorf("expected %q to be a directory", created)
	}
}

func Test_ensureNestedMountTargets_FileSourceCreatesFile(t *testing.T) {
	parentSrc := t.TempDir()
	childFile := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(childFile, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	spec := &oci.Spec{
		Mounts: []oci.Mount{
			{
				Destination: "/etc/coredns",
				Source:      parentSrc,
				Type:        "bind",
				Options:     []string{"bind", "ro"},
			},
			{
				Destination: "/etc/coredns/Corefile",
				Source:      childFile,
				Type:        "bind",
				Options:     []string{"bind"},
			},
		},
	}

	ensureNestedMountTargets(context.Background(), spec)

	created := filepath.Join(parentSrc, "Corefile")
	info, err := os.Stat(created)
	if err != nil {
		t.Fatalf("expected file mountpoint %q to be created: %v", created, err)
	}
	if info.IsDir() {
		t.Errorf("expected %q to be a regular file, got directory", created)
	}
}

func Test_ensureNestedMountTargets_WritableParentNoOp(t *testing.T) {
	parentSrc := t.TempDir()
	childSrc := t.TempDir()

	spec := &oci.Spec{
		Mounts: []oci.Mount{
			{
				Destination: "/mnt/data",
				Source:      parentSrc,
				Type:        "bind",
				Options:     []string{"bind"}, // read-write parent
			},
			{
				Destination: "/mnt/data/subdir",
				Source:      childSrc,
				Type:        "bind",
				Options:     []string{"bind"},
			},
		},
	}

	ensureNestedMountTargets(context.Background(), spec)

	created := filepath.Join(parentSrc, "subdir")
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Errorf("expected no mountpoint to be created under writable parent, stat err = %v", err)
	}
}

func Test_ensureNestedMountTargets_UsesPrecedingMountsOnly(t *testing.T) {
	mntSrc := t.TempDir() // source of the read-only /mnt
	abSrc := t.TempDir()  // source of /mnt/a/b
	aSrc := t.TempDir()   // source of /mnt/a, listed after its own child

	spec := &oci.Spec{
		Mounts: []oci.Mount{
			{Destination: "/mnt", Source: mntSrc, Type: "bind", Options: []string{"bind", "ro"}},
			{Destination: "/mnt/a/b", Source: abSrc, Type: "bind", Options: []string{"bind"}},
			{Destination: "/mnt/a", Source: aSrc, Type: "bind", Options: []string{"bind"}},
		},
	}

	ensureNestedMountTargets(context.Background(), spec)

	// runc processes /mnt/a/b while only the read-only /mnt is mounted, so its
	// mount point must be created under /mnt's source, not /mnt/a's.
	if _, err := os.Stat(filepath.Join(mntSrc, "a", "b")); err != nil {
		t.Fatalf("expected mount point under /mnt source: %v", err)
	}
	if entries, _ := os.ReadDir(aSrc); len(entries) != 0 {
		t.Errorf("nothing should be created under /mnt/a source, found %d entries", len(entries))
	}
}

// Test_ensureNestedMountTargets_GrandparentReadonlyParentWritable verifies that
// only the deepest preceding parent matters. A grandchild whose immediate parent
// is writable is left alone even when a shallower ancestor is read-only, because
// runc creates that mount point under the writable parent, not the read-only
// grandparent.
func Test_ensureNestedMountTargets_GrandparentReadonlyParentWritable(t *testing.T) {
	mntSrc := t.TempDir() // /mnt, read-only
	aSrc := t.TempDir()   // /mnt/a, writable
	bSrc := t.TempDir()   // child /mnt/a/b

	spec := &oci.Spec{
		Mounts: []oci.Mount{
			{Destination: "/mnt", Source: mntSrc, Type: "bind", Options: []string{"bind", "ro"}},
			{Destination: "/mnt/a", Source: aSrc, Type: "bind", Options: []string{"bind"}},
			{Destination: "/mnt/a/b", Source: bSrc, Type: "bind", Options: []string{"bind"}},
		},
	}

	ensureNestedMountTargets(context.Background(), spec)

	// /mnt/a is nested directly under the read-only /mnt, so its mount point is
	// pre-created in /mnt's source.
	if _, err := os.Stat(filepath.Join(mntSrc, "a")); err != nil {
		t.Fatalf("expected /mnt/a mount point pre-created under /mnt source: %v", err)
	}
	// /mnt/a/b's deepest preceding parent is the writable /mnt/a, so runc creates
	// that mount point itself and nothing should be pre-created under /mnt/a.
	if entries, _ := os.ReadDir(aSrc); len(entries) != 0 {
		t.Errorf("expected nothing pre-created under writable parent /mnt/a, found %d entries", len(entries))
	}
}

// Test_ensureNestedMountTargets_ExistingTargetPreserved verifies pre-creation is
// non-destructive: when the mount point already exists (shipped in the image or
// the volume) its contents are left untouched and runc is left to validate it.
func Test_ensureNestedMountTargets_ExistingTargetPreserved(t *testing.T) {
	parentSrc := t.TempDir()
	childSrc := t.TempDir()

	existing := filepath.Join(parentSrc, "subdir")
	if err := os.Mkdir(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(existing, "keep")
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	spec := &oci.Spec{
		Mounts: []oci.Mount{
			{Destination: "/mnt/data", Source: parentSrc, Type: "bind", Options: []string{"bind", "ro"}},
			{Destination: "/mnt/data/subdir", Source: childSrc, Type: "bind", Options: []string{"bind"}},
		},
	}

	ensureNestedMountTargets(context.Background(), spec)

	// The pre-existing mount point and its contents must survive.
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected existing content under the mount point to be preserved: %v", err)
	}
	if info, err := os.Stat(existing); err != nil || !info.IsDir() {
		t.Fatalf("expected existing mount point to remain a directory: info=%v err=%v", info, err)
	}
}

// skipIfCannotMount skips the test unless the process can create bind mounts
// (root with CAP_SYS_ADMIN). This keeps the functional tests from failing in
// non-privileged or restricted CI environments while still running wherever
// mounts are available (e.g. the guest UVM or a privileged dev box).
func skipIfCannotMount(t *testing.T) {
	t.Helper()
	if os.Getuid() != 0 {
		t.Skip("requires root to create bind mounts")
	}
	src, dst := t.TempDir(), t.TempDir()
	if err := unix.Mount(src, dst, "", unix.MS_BIND, ""); err != nil {
		t.Skipf("bind mounts not permitted in this environment: %v", err)
	}
	_ = unix.Unmount(dst, 0)
}

// Test_readonlyBindMount_blocksMkdir is the control that reproduces the failure
// the fix addresses: once a bind mount is read-only, creating a mount point
// under it fails with EROFS. This is exactly what runc hits when it tries to
// create a nested mount's target under an already read-only parent.
func Test_readonlyBindMount_blocksMkdir(t *testing.T) {
	skipIfCannotMount(t)
	src := t.TempDir()
	dst := t.TempDir()
	if err := unix.Mount(src, dst, "", unix.MS_BIND, ""); err != nil {
		t.Fatalf("bind mount: %v", err)
	}
	defer func() { _ = unix.Unmount(dst, 0) }()
	if err := unix.Mount("", dst, "", unix.MS_BIND|unix.MS_REMOUNT|unix.MS_RDONLY, ""); err != nil {
		t.Fatalf("remount read-only: %v", err)
	}

	err := os.Mkdir(filepath.Join(dst, "subdir"), 0o755)
	if !errors.Is(err, unix.EROFS) {
		t.Fatalf("expected EROFS creating a dir under a read-only mount, got %v", err)
	}
}

// Test_ensureNestedMountTargets_Functional proves the fix end to end: after
// pre-creating the nested mount point in the parent's source, the mount point is
// visible once the parent is bind-mounted read-only, and a child can be mounted
// there without EROFS (which is what runc does). Requires root.
func Test_ensureNestedMountTargets_Functional(t *testing.T) {
	skipIfCannotMount(t)
	parentSrc := t.TempDir()
	childSrc := t.TempDir()
	parentDst := t.TempDir()

	spec := &oci.Spec{
		Mounts: []oci.Mount{
			{Destination: parentDst, Source: parentSrc, Type: "bind", Options: []string{"bind", "ro"}},
			{Destination: filepath.Join(parentDst, "subdir"), Source: childSrc, Type: "bind", Options: []string{"bind"}},
		},
	}
	ensureNestedMountTargets(context.Background(), spec)

	// Bind-mount the parent source read-only, exactly as runc would.
	if err := unix.Mount(parentSrc, parentDst, "", unix.MS_BIND, ""); err != nil {
		t.Fatalf("bind mount parent: %v", err)
	}
	childMountPoint := filepath.Join(parentDst, "subdir")
	defer func() {
		_ = unix.Unmount(childMountPoint, 0)
		_ = unix.Unmount(parentDst, 0)
	}()
	if err := unix.Mount("", parentDst, "", unix.MS_BIND|unix.MS_REMOUNT|unix.MS_RDONLY, ""); err != nil {
		t.Fatalf("remount parent read-only: %v", err)
	}

	// The pre-created mount point must be visible through the read-only parent...
	if _, err := os.Stat(childMountPoint); err != nil {
		t.Fatalf("nested mount point not visible under read-only parent: %v", err)
	}
	// ...and runc must be able to bind-mount the child there without EROFS.
	if err := unix.Mount(childSrc, childMountPoint, "", unix.MS_BIND, ""); err != nil {
		t.Fatalf("failed to bind-mount child under read-only parent: %v", err)
	}
}

// Test_ensureNestedMountTargets_ReadonlySourceSkipped verifies the guard for a
// read-only source filesystem (e.g. a SCSI VHD attached read-only or a dm-verity
// layer): there is nowhere to create the mount point, so the function skips it
// rather than erroring, and runc surfaces its own failure. The parent source is
// bind-mounted onto itself and remounted read-only so its filesystem itself is
// read-only (unix.Access reports EROFS). Requires root.
func Test_ensureNestedMountTargets_ReadonlySourceSkipped(t *testing.T) {
	skipIfCannotMount(t)
	parentSrc := t.TempDir()
	childSrc := t.TempDir()

	if err := unix.Mount(parentSrc, parentSrc, "", unix.MS_BIND, ""); err != nil {
		t.Fatalf("bind mount source onto itself: %v", err)
	}
	defer func() { _ = unix.Unmount(parentSrc, 0) }()
	if err := unix.Mount("", parentSrc, "", unix.MS_BIND|unix.MS_REMOUNT|unix.MS_RDONLY, ""); err != nil {
		t.Fatalf("remount source read-only: %v", err)
	}

	spec := &oci.Spec{
		Mounts: []oci.Mount{
			{Destination: "/mnt/data", Source: parentSrc, Type: "bind", Options: []string{"bind", "ro"}},
			{Destination: "/mnt/data/subdir", Source: childSrc, Type: "bind", Options: []string{"bind"}},
		},
	}

	ensureNestedMountTargets(context.Background(), spec)

	if _, err := os.Stat(filepath.Join(parentSrc, "subdir")); !os.IsNotExist(err) {
		t.Errorf("expected no mount point created under read-only source, stat err = %v", err)
	}
}

// Test_ensureNestedMountTargets_SymlinkInSourceDoesNotEscape verifies that a
// symlink planted in the (writable) parent source cannot redirect pre-creation
// outside that source. This matters in a shared UVM, where the guest creates
// mount points as root across pods: following such a symlink would let one pod
// create paths in another pod's or the UVM's filesystem.
func Test_ensureNestedMountTargets_SymlinkInSourceDoesNotEscape(t *testing.T) {
	parentSrc := t.TempDir()
	childSrc := t.TempDir()
	outside := t.TempDir() // stands in for another pod's or the UVM's filesystem

	// A malicious symlink in the parent source pointing outside it.
	if err := os.Symlink(outside, filepath.Join(parentSrc, "evil")); err != nil {
		t.Fatal(err)
	}

	spec := &oci.Spec{
		Mounts: []oci.Mount{
			{Destination: "/mnt/data", Source: parentSrc, Type: "bind", Options: []string{"bind", "ro"}},
			{Destination: "/mnt/data/evil/pwned", Source: childSrc, Type: "bind", Options: []string{"bind"}},
		},
	}

	ensureNestedMountTargets(context.Background(), spec)

	// Nothing must be created through the symlink, outside the parent source.
	if _, err := os.Stat(filepath.Join(outside, "pwned")); !os.IsNotExist(err) {
		t.Fatalf("symlink escape: %q was created outside the parent source (stat err = %v)",
			filepath.Join(outside, "pwned"), err)
	}
}
