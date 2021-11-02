// +build linux

package overlay

import (
	"context"
	"os"
	"strings"

	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"golang.org/x/sys/unix"
)

// Test dependencies
var (
	osMkdirAll  = os.MkdirAll
	osRemoveAll = os.RemoveAll
	unixMount   = unix.Mount
)

// MountLayer first enforces the security policy for the container's layer paths
// and then calls Mount to mount the layer paths as an overlayfs
func MountLayer(ctx context.Context, layerPaths []string, upperdirPath, workdirPath, rootfsPath string, readonly bool, containerId string, securityPolicy securitypolicy.SecurityPolicyEnforcer) (err error) {
	_, span := trace.StartSpan(ctx, "overlay::MountLayer")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	if err := securityPolicy.EnforceOverlayMountPolicy(containerId, layerPaths); err != nil {
		return err
	}
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
	_, span := trace.StartSpan(ctx, "overlay::Mount")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	lowerdir := strings.Join(basePaths, ":")
	span.AddAttributes(
		trace.StringAttribute("lowerdir", lowerdir),
		trace.StringAttribute("upperdirPath", upperdirPath),
		trace.StringAttribute("workdirPath", workdirPath),
		trace.StringAttribute("target", target),
		trace.BoolAttribute("readonly", readonly))

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
				osRemoveAll(upperdirPath)
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
				osRemoveAll(workdirPath)
			}
		}()
		options = append(options, "workdir="+workdirPath)
	}
	if err := osMkdirAll(target, 0755); err != nil {
		return errors.Wrapf(err, "failed to create directory for container root filesystem %s", target)
	}
	defer func() {
		if err != nil {
			osRemoveAll(target)
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
