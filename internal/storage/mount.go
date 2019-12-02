// +build linux

package storage

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/Microsoft/opengcs/internal/oc"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"golang.org/x/sys/unix"
)

const procMountFile = "/proc/mounts"
const numProcMountFields = 6

// Test dependencies
var (
	osStat      = os.Stat
	unixUnmount = unix.Unmount
	unixMount   = unix.Mount
	osRemoveAll = os.RemoveAll
	listMounts  = listMountPointsUnderPath
)

// MountRShared creates a bind mountpoint and marks it as rshared
// Expected that the filepath exists before calling this function
func MountRShared(path string) error {
	if path == "" {
		return errors.New("Path must not be empty to mount as rshared")
	}
	if err := unixMount(path, path, "", syscall.MS_BIND, ""); err != nil {
		return fmt.Errorf("Failed to create bind mount for %v: %v", path, err)
	}
	if err := unixMount(path, path, "", syscall.MS_SHARED|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("Failed to make %v rshared: %v", path, err)
	}
	return nil
}

// UnmountPath unmounts the target path if it exists and is a mount path. If
// removeTarget this will remove the previously mounted folder.
func UnmountPath(ctx context.Context, target string, removeTarget bool) (err error) {
	_, span := trace.StartSpan(ctx, "storage::UnmountPath")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("target", target),
		trace.BoolAttribute("remove", removeTarget))

	if _, err := osStat(target); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to determine if path '%s' exists", target)
	}

	if err := unixUnmount(target, 0); err != nil {
		// If `Unmount` returns `EINVAL` it's not mounted. Just delete the
		// folder.
		if err != unix.EINVAL {
			return errors.Wrapf(err, "failed to unmount path '%s'", target)
		}
	}
	if removeTarget {
		return osRemoveAll(target)
	}
	return nil
}

func UnmountAllInPath(ctx context.Context, path string, removeTarget bool) (err error) {
	childMounts, err := listMounts(path)
	if err != nil {
		return err
	}

	for i := len(childMounts) - 1; i >= 0; i-- {
		childPath := childMounts[i]
		if err := UnmountPath(ctx, childPath, removeTarget); err != nil {
			return err
		}
	}
	return nil
}

func listMountPointsUnderPath(path string) ([]string, error) {
	var mountPoints []string
	f, err := os.Open(procMountFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Split(line, " ")
		if len(fields) < numProcMountFields {
			continue
		}
		destPath := fields[1]
		if strings.HasPrefix(destPath, path) {
			mountPoints = append(mountPoints, destPath)
		}
	}

	return mountPoints, nil
}
