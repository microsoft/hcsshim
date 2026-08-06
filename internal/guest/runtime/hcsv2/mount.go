//go:build linux
// +build linux

package hcsv2

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	oci "github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/hcsshim/internal/log"
)

// ensureNestedMountTargets pre-creates the mount point for any mount whose
// destination is nested inside a read-only bind mount.
//
// runc applies the mounts in spec order and remounts a bind mount read-only as
// soon as it processes it. If a later mount targets a path inside that mount and
// the mount point does not already exist, runc tries to create it under the now
// read-only parent and fails with EROFS ("read-only file system"). This breaks
// valid CRI configs where a read-only volume has another volume mounted into a
// subdirectory of it (e.g. a read-only /etc/coredns configMap with a custom
// config volume at /etc/coredns/custom).
//
// On a regular Kubernetes node the kubelet creates these subdirectories on the
// host first. Inside an LCOW UVM the guest owns the mount setup, so we do the
// equivalent: create the mount point inside the parent's (writable) source, so
// it already exists once runc makes the parent read-only.
//
// Best effort: a failure is logged and skipped so runc's own error still
// surfaces and unaffected containers are unchanged.
func ensureNestedMountTargets(ctx context.Context, spec *oci.Spec) {
	// runc applies mounts in spec order, so when it creates a mount point only
	// the mounts before this one are active. The parent the mount point is
	// created under is therefore the deepest ancestor among the preceding mounts.
	for i, child := range spec.Mounts {
		parent, ok := deepestParentMount(child.Destination, spec.Mounts[:i])
		if !ok || !mountIsReadonly(parent) || !mountIsBind(parent) {
			// runc only fails when it must create the mount point under a
			// read-only bind mount; otherwise it creates the target itself.
			continue
		}
		if info, err := os.Stat(parent.Source); err != nil || !info.IsDir() {
			continue
		}
		rel, err := filepath.Rel(parent.Destination, child.Destination)
		if err != nil {
			continue
		}
		target := filepath.Join(parent.Source, rel)
		if err := createMountTarget(target, child.Source); err != nil {
			log.G(ctx).WithError(err).WithField("target", target).
				Warn("failed to pre-create mount point under read-only mount")
		}
	}
}

// deepestParentMount returns the mount in mounts whose destination is the
// closest strict path ancestor of dest. When several are ancestors (stacked
// mounts) the one with the longest destination wins, which is where dest
// resolves to once those mounts are applied. Callers pass the mounts preceding
// dest in spec order, since those are the ones runc has already applied.
func deepestParentMount(dest string, mounts []oci.Mount) (oci.Mount, bool) {
	cleanDest := filepath.Clean(dest)
	var best oci.Mount
	found := false
	for _, m := range mounts {
		p := filepath.Clean(m.Destination)
		if !isStrictSubPath(p, cleanDest) {
			continue
		}
		if !found || len(p) > len(filepath.Clean(best.Destination)) {
			best = m
			found = true
		}
	}
	return best, found
}

// isStrictSubPath reports whether target is strictly nested underneath base.
func isStrictSubPath(base, target string) bool {
	base = filepath.Clean(base)
	target = filepath.Clean(target)
	if base == target {
		return false
	}
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

// mountIsReadonly reports whether the mount will be mounted read-only, honoring
// the last of any ro/rw options (which is how they resolve when both appear).
func mountIsReadonly(m oci.Mount) bool {
	ro := false
	for _, o := range m.Options {
		switch o {
		case "ro":
			ro = true
		case "rw":
			ro = false
		}
	}
	return ro
}

// mountIsBind reports whether the mount is a bind mount.
func mountIsBind(m oci.Mount) bool {
	if m.Type == "bind" {
		return true
	}
	for _, o := range m.Options {
		if o == "bind" || o == "rbind" {
			return true
		}
	}
	return false
}

// createMountTarget creates the mountpoint at target. It mirrors runc's own
// behavior: if source is a non-directory the target is created as an empty file,
// otherwise it is created as a directory. Intermediate directories are created
// as needed, and an already-existing target is left untouched.
func createMountTarget(target, source string) error {
	if _, err := os.Lstat(target); err == nil {
		// Something already exists at the mountpoint (e.g. shipped in the image
		// or the volume). Leave it as-is and let runc validate compatibility.
		return nil
	}

	sourceIsDir := true
	if info, err := os.Stat(source); err == nil {
		sourceIsDir = info.IsDir()
	}
	if sourceIsDir {
		return mkdirAllModePerm(target)
	}
	if err := mkdirAllModePerm(filepath.Dir(target)); err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	return f.Close()
}
