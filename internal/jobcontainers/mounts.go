//go:build windows

package jobcontainers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// namedPipePath returns true if the given path is to a named pipe.
func isnamedPipePath(p string) bool {
	return strings.HasPrefix(p, `\\.\pipe\`)
}

// Strip the drive letter (if there is one) so we don't end up with "%CONTAINER_SANDBOX_MOUNT_POINT%"\C:\path\to\mount
func stripDriveLetter(name string) string {
	// Remove drive letter
	if len(name) == 2 && name[1] == ':' {
		name = "."
	} else if len(name) > 2 && name[1] == ':' {
		name = name[2:]
	}
	return name
}

// fallbackMountSetup adds the mounts requested in the OCI runtime spec. This is
// the fallback behavior if the Bind Filter dll is not available on the host, so
// typical bind mount like functionality can't be used. Instead, symlink the
// path requested to a relative path under where the container image volume is
// located.
func fallbackMountSetup(spec *specs.Spec, sandboxVolumePath string) error {
	for _, mount := range spec.Mounts {
		if mount.Destination == "" || mount.Source == "" {
			return fmt.Errorf("invalid OCI spec - a mount must have both source and a destination: %+v", mount)
		}

		if isnamedPipePath(mount.Source) {
			return errors.New("named pipe mounts not supported for job containers - interact with the pipe directly")
		}

		fullCtrPath := filepath.Join(sandboxVolumePath, stripDriveLetter(mount.Destination))
		// Make sure all of the dirs leading up to the full path exist.
		strippedCtrPath := filepath.Dir(fullCtrPath)
		if err := os.MkdirAll(strippedCtrPath, 0777); err != nil {
			return errors.Wrap(err, "failed to make directory for job container mount")
		}

		if err := os.Symlink(mount.Source, fullCtrPath); err != nil {
			return errors.Wrap(err, "failed to setup mount for job container")
		}
	}
	return nil
}

// setupMounts sets up all requested mounts present in the OCI runtime spec. They will be mounted from
// mount.Source to mount.Destination as well as mounted from mount.Source to under the rootfs location
// for backwards compat with systems that don't have the Bind Filter functionality available.
func (c *JobContainer) setupMounts(ctx context.Context, spec *specs.Spec) error {
	for _, mount := range spec.Mounts {
		if mount.Destination == "" || mount.Source == "" {
			return fmt.Errorf("invalid OCI spec - a mount must have both source and a destination: %+v", mount)
		}

		if isnamedPipePath(mount.Source) {
			return errors.New("named pipe mounts not supported for job containers - interact with the pipe directly")
		}

		// If the destination exists, log a warning. The default behavior for bindflt is to shadow the directory,
		// but on the host this may lead to more wonky situations than in a normal container. Mounts should not
		// be relied on too heavily so this shouldn't be an error case.
		if _, err := os.Stat(mount.Destination); err == nil {
			log.G(ctx).WithFields(logrus.Fields{
				logfields.ContainerID: c.id,
				"mountSource":         mount.Source,
				"mountDestination":    mount.Destination,
			}).Warn("job container mount destination exists and will be shadowed")
		}

		if err := c.job.ApplyFileBinding(mount.Destination, mount.Source, false); err != nil {
			return err
		}
	}

	return nil
}
