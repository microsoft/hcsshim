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
		{"/mnt/data", "/mnt/data/a/b/c", true},
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
