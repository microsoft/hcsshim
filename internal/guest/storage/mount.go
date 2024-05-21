//go:build linux
// +build linux

package storage

import (
	"bufio"
	"context"
	gerrors "errors"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sys/unix"

	"github.com/Microsoft/hcsshim/internal/otelutil"
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

	flags = map[string]struct {
		clear bool
		flag  uintptr
	}{
		"acl":           {false, unix.MS_POSIXACL},
		"async":         {true, unix.MS_SYNCHRONOUS},
		"atime":         {true, unix.MS_NOATIME},
		"bind":          {false, unix.MS_BIND},
		"defaults":      {false, 0},
		"dev":           {true, unix.MS_NODEV},
		"diratime":      {true, unix.MS_NODIRATIME},
		"dirsync":       {false, unix.MS_DIRSYNC},
		"exec":          {true, unix.MS_NOEXEC},
		"iversion":      {false, unix.MS_I_VERSION},
		"lazytime":      {false, unix.MS_LAZYTIME},
		"loud":          {true, unix.MS_SILENT},
		"mand":          {false, unix.MS_MANDLOCK},
		"noacl":         {true, unix.MS_POSIXACL},
		"noatime":       {false, unix.MS_NOATIME},
		"nodev":         {false, unix.MS_NODEV},
		"nodiratime":    {false, unix.MS_NODIRATIME},
		"noexec":        {false, unix.MS_NOEXEC},
		"noiversion":    {true, unix.MS_I_VERSION},
		"nolazytime":    {true, unix.MS_LAZYTIME},
		"nomand":        {true, unix.MS_MANDLOCK},
		"norelatime":    {true, unix.MS_RELATIME},
		"nostrictatime": {true, unix.MS_STRICTATIME},
		"nosuid":        {false, unix.MS_NOSUID},
		"rbind":         {false, unix.MS_BIND | unix.MS_REC},
		"relatime":      {false, unix.MS_RELATIME},
		"remount":       {false, unix.MS_REMOUNT},
		"ro":            {false, unix.MS_RDONLY},
		"rw":            {true, unix.MS_RDONLY},
		"silent":        {false, unix.MS_SILENT},
		"strictatime":   {false, unix.MS_STRICTATIME},
		"suid":          {true, unix.MS_NOSUID},
		"sync":          {false, unix.MS_SYNCHRONOUS},
	}

	propagationFlags = map[string]uintptr{
		"private":     unix.MS_PRIVATE,
		"shared":      unix.MS_SHARED,
		"slave":       unix.MS_SLAVE,
		"unbindable":  unix.MS_UNBINDABLE,
		"rprivate":    unix.MS_PRIVATE | unix.MS_REC,
		"rshared":     unix.MS_SHARED | unix.MS_REC,
		"rslave":      unix.MS_SLAVE | unix.MS_REC,
		"runbindable": unix.MS_UNBINDABLE | unix.MS_REC,
	}
)

func ParseMountOptions(options []string) (flagOpts uintptr, pgFlags []uintptr, data []string) {
	for _, o := range options {
		if f, exists := flags[o]; exists && f.flag != 0 {
			if f.clear {
				flagOpts &= ^f.flag
			} else {
				flagOpts |= f.flag
			}
		} else if f, exists := propagationFlags[o]; exists && f != 0 {
			pgFlags = append(pgFlags, f)
		} else {
			data = append(data, o)
		}
	}
	return
}

// MountRShared creates a bind mountpoint and marks it as rshared
// Expected that the filepath exists before calling this function
func MountRShared(path string) error {
	if path == "" {
		return errors.New("path must not be empty to mount as rshared")
	}
	if err := unixMount(path, path, "", syscall.MS_BIND, ""); err != nil {
		return fmt.Errorf("failed to create bind mount for %v: %w", path, err)
	}
	if err := unixMount(path, path, "", syscall.MS_SHARED|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("failed to make %v rshared: %w", path, err)
	}
	return nil
}

// UnmountPath unmounts the target path if it exists and is a mount path. If
// removeTarget this will remove the previously mounted folder.
func UnmountPath(ctx context.Context, target string, removeTarget bool) (err error) {
	_, span := otelutil.StartSpan(ctx, "storage::UnmountPath", trace.WithAttributes(
		attribute.String("target", target),
		attribute.Bool("remove", removeTarget)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if _, err := osStat(target); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to determine if path '%s' exists", target)
	}

	if err := unixUnmount(target, 0); err != nil {
		// If `Unmount` returns `EINVAL` it's not mounted. Just delete the
		// folder.
		if !gerrors.Is(err, unix.EINVAL) {
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
