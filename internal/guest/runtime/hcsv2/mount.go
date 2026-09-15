//go:build linux
// +build linux

package hcsv2

import (
	"context"
	"os"
	"path/filepath"

	securejoin "github.com/cyphar/filepath-securejoin"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/sys/unix"

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
	// the mounts before this one are active. seen holds those preceding mounts
	// keyed by cleaned destination, so a child's deepest ancestor is found in
	// O(path depth) instead of rescanning every prior mount.
	seen := make(map[string]oci.Mount, len(spec.Mounts))
	for _, child := range spec.Mounts {
		cleanDest := filepath.Clean(child.Destination)
		parent, ok := deepestParentMount(cleanDest, seen)
		// Register this child before any skip below, since a skipped mount can
		// still be the parent of a later one.
		if _, exists := seen[cleanDest]; !exists {
			seen[cleanDest] = child
		}
		if !ok || !mountIsReadonly(parent) || !mountIsBind(parent) {
			// runc only fails when it must create the mount point under a
			// read-only bind mount; otherwise it creates the target itself.
			continue
		}
		if info, err := os.Stat(parent.Source); err != nil || !info.IsDir() {
			continue
		}
		// Only pre-create when the parent's source is writable. A read-only
		// source (e.g. a SCSI VHD attached read-only, or a dm-verity layer) has
		// nowhere to create the mount point, and runc would hit the same
		// read-only filesystem, so there is nothing we can do here.
		if unix.Access(parent.Source, unix.W_OK) != nil {
			continue
		}
		rel, err := filepath.Rel(parent.Destination, child.Destination)
		if err != nil {
			continue
		}
		// Resolve the mount point within the parent's source, clamping any
		// symlink so one planted in the (writable, tenant-controlled) source
		// cannot redirect creation outside it. In a shared UVM the guest runs as
		// root across pods, so blindly following such a symlink would let a pod
		// create paths in another pod's or the UVM's filesystem. This mirrors how
		// runc resolves mount destinations.
		target, err := securejoin.SecureJoin(parent.Source, rel)
		if err != nil {
			log.G(ctx).WithError(err).WithField("source", parent.Source).
				Warn("failed to resolve nested mount point under read-only mount")
			continue
		}
		if err := createMountTarget(target, child); err != nil {
			log.G(ctx).WithError(err).WithField("target", target).
				Warn("failed to pre-create mount point under read-only mount")
		}
	}
}

// deepestParentMount returns the mount in seen whose destination is the closest
// ancestor of dest. It walks up the path one component at a time, so the first
// match is the deepest (closest) ancestor. seen must hold only the mounts that
// precede dest in spec order (the ones runc has already applied), keyed by
// cleaned destination.
func deepestParentMount(dest string, seen map[string]oci.Mount) (oci.Mount, bool) {
	dest = filepath.Clean(dest)
	for {
		parent := filepath.Dir(dest)
		if parent == dest {
			// Reached the root without finding an ancestor mount.
			return oci.Mount{}, false
		}
		if m, ok := seen[parent]; ok {
			return m, true
		}
		dest = parent
	}
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

// createMountTarget creates the mountpoint at target. Only a bind mount uses its
// source's type to decide the target type: a non-directory source needs a file
// mountpoint, anything else a directory. Every other mount type (e.g. tmpfs)
// needs a directory regardless of any source label. Intermediate directories are
// created as needed, and an already-existing target is left untouched.
func createMountTarget(target string, child oci.Mount) error {
	if _, err := os.Lstat(target); err == nil {
		// Something already exists at the mountpoint (e.g. shipped in the image
		// or the volume). Leave it as-is and let runc validate compatibility.
		return nil
	}

	if mountIsBind(child) {
		info, err := os.Stat(child.Source)
		if err != nil {
			// Surface the failure (e.g. a missing bind source) to the caller,
			// which logs it, instead of silently assuming a directory.
			return err
		}
		if !info.IsDir() {
			if err := mkdirAllModePerm(filepath.Dir(target)); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return err
			}
			return f.Close()
		}
	}
	return mkdirAllModePerm(target)
}
