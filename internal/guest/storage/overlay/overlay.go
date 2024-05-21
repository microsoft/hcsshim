//go:build linux
// +build linux

package overlay

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/memory"
	"github.com/Microsoft/hcsshim/internal/otelutil"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sys/unix"
)

// Test dependencies.
var (
	osMkdirAll  = os.MkdirAll
	osRemoveAll = os.RemoveAll
	unixMount   = unix.Mount
)

// processErrNoSpace logs disk space and inode information for `path` that we encountered the ENOSPC error on.
// This can be used to get a better view of whats going on on the disk at the time of the error.
func processErrNoSpace(ctx context.Context, path string, err error) {
	st := &unix.Statfs_t{}
	// Pass in filepath.Dir() of the path as if we got an error while creating the directory it definitely doesn't exist.
	// Take its parent, which should be on the same drive.
	if statErr := unix.Statfs(filepath.Dir(path), st); statErr != nil {
		log.G(ctx).WithError(statErr).WithField("path", filepath.Dir(path)).Warn("failed to get disk information for ENOSPC error")
		return
	}

	all := st.Blocks * uint64(st.Bsize)
	available := st.Bavail * uint64(st.Bsize)
	free := st.Bfree * uint64(st.Bsize)
	used := all - free

	toGigabyteStr := func(val uint64) string {
		return fmt.Sprintf("%.1f", float64(val)/float64(memory.GiB))
	}

	log.G(ctx).WithFields(logrus.Fields{
		"available-disk-space-GiB": toGigabyteStr(available),
		"free-disk-space-GiB":      toGigabyteStr(free),
		"used-disk-space-GiB":      toGigabyteStr(used),
		"total-inodes":             st.Files,
		"free-inodes":              st.Ffree,
		"path":                     path,
	}).WithError(err).Warn("got ENOSPC, gathering diagnostics")
}

// MountLayer first enforces the security policy for the container's layer paths
// and then calls Mount to mount the layer paths as an overlayfs.
func MountLayer(
	ctx context.Context,
	layerPaths []string,
	upperdirPath, workdirPath, rootfsPath string,
	readonly bool,
) (err error) {
	_, span := otelutil.StartSpan(ctx, "overlay::MountLayer")
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	return Mount(ctx, layerPaths, upperdirPath, workdirPath, rootfsPath, readonly)
}

// Mount creates an overlay mount with `basePaths` at `target`.
//
// If `upperdirPath != ""` the path will be created. On mount failure the
// created `upperdirPath` will be automatically cleaned up.
//
// If `workdirPath != ""` the path will be created. On mount failure the created
// `workdirPath` will be automatically cleaned up.
//
// Always creates `target`. On mount failure the created `target` will
// be automatically cleaned up.
func Mount(ctx context.Context, basePaths []string, upperdirPath, workdirPath, target string, readonly bool) (err error) {
	lowerdir := strings.Join(basePaths, ":")

	_, span := otelutil.StartSpan(ctx, "overlay::Mount", trace.WithAttributes(
		attribute.String("lowerdir", lowerdir),
		attribute.String("upperdirPath", upperdirPath),
		attribute.String("workdirPath", workdirPath),
		attribute.String("target", target),
		attribute.Bool("readonly", readonly)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	// If we got an ENOSPC error on creating any directories, log disk space and inode info for
	//  the mount that the directory belongs to get a better view of the where the problem lies.
	defer func() {
		var perr *os.PathError
		if errors.As(err, &perr) && errors.Is(perr.Err, unix.ENOSPC) {
			processErrNoSpace(ctx, perr.Path, err)
		}
	}()

	if target == "" {
		return errors.New("cannot have empty target")
	}

	if readonly && (upperdirPath != "" || workdirPath != "") {
		return errors.Errorf("upperdirPath: %q, and workdirPath: %q must be empty when readonly==true", upperdirPath, workdirPath)
	}

	options := []string{"lowerdir=" + lowerdir}
	if upperdirPath != "" {
		if err := osMkdirAll(upperdirPath, 0755); err != nil {
			return errors.Wrap(err, "failed to create upper directory in scratch space")
		}
		defer func() {
			if err != nil {
				_ = osRemoveAll(upperdirPath)
			}
		}()
		options = append(options, "upperdir="+upperdirPath)
	}
	if workdirPath != "" {
		if err := osMkdirAll(workdirPath, 0755); err != nil {
			return errors.Wrap(err, "failed to create workdir in scratch space")
		}
		defer func() {
			if err != nil {
				_ = osRemoveAll(workdirPath)
			}
		}()
		options = append(options, "workdir="+workdirPath)
	}
	if err := osMkdirAll(target, 0755); err != nil {
		return errors.Wrapf(err, "failed to create directory for container root filesystem %s", target)
	}
	defer func() {
		if err != nil {
			_ = osRemoveAll(target)
		}
	}()
	var flags uintptr
	if readonly {
		flags |= unix.MS_RDONLY
	}
	if err := unixMount("overlay", target, "overlay", flags, strings.Join(options, ",")); err != nil {
		return errors.Wrapf(err, "failed to mount overlayfs at %s", target)
	}
	return nil
}
