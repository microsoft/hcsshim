//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	task "github.com/containerd/containerd/api/runtime/task/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/memory"
	"github.com/Microsoft/hcsshim/internal/otelutil"
	"github.com/Microsoft/hcsshim/internal/winapi"
)

// LimitedRead reads at max `readLimitBytes` bytes from the file at path `filePath`. If the file has
// more than `readLimitBytes` bytes of data then first `readLimitBytes` will be returned.
func limitedRead(filePath string, readLimitBytes int64) ([]byte, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, errors.Wrapf(err, "limited read failed to open file: %s", filePath)
	}
	defer f.Close()
	if fi, err := f.Stat(); err == nil {
		if fi.Size() < readLimitBytes {
			readLimitBytes = fi.Size()
		}
		buf := make([]byte, readLimitBytes)
		_, err := f.Read(buf)
		if err != nil {
			return []byte{}, errors.Wrapf(err, "limited read failed during file read: %s", filePath)
		}
		return buf, nil
	}
	return []byte{}, errors.Wrapf(err, "limited read failed during file stat: %s", filePath)
}

var deleteCommand = cli.Command{
	Name: "delete",
	Usage: `
This command allows containerd to delete any container resources created, mounted, and/or run by a shim when containerd can no longer communicate over rpc. This happens if a shim is SIGKILL'd with a running container. These resources will need to be cleaned up when containerd loses the connection to a shim. This is also used when containerd boots and reconnects to shims. If a bundle is still on disk but containerd cannot connect to a shim, the delete command is invoked.

The delete command will be executed in the container's bundle as its cwd.
`,
	SkipArgReorder: true,
	Action: func(cCtx *cli.Context) (err error) {
		// We cant write anything to stdout for this cmd other than the
		// task.DeleteResponse by protocol. We can write to stderr which will be
		// logged as a warning in containerd.

		ctx, span := otelutil.StartSpan(context.Background(), "delete")
		defer span.End()
		defer func() { otelutil.SetSpanStatus(span, err) }()

		bundleFlag := cCtx.GlobalString("bundle")
		if bundleFlag == "" {
			return errors.New("bundle is required")
		}

		// hcsshim shim writes panic logs in the bundle directory in a file named "panic.log"
		// log those messages (if any) on stderr so that it shows up in containerd's log.
		// This should be done as the first thing so that we don't miss any panic logs even if
		// something goes wrong during delete op.
		// The file can be very large so read only first 1MB of data.
		readLimit := int64(memory.MiB) // 1MB
		logBytes, err := limitedRead(filepath.Join(bundleFlag, "panic.log"), readLimit)
		if err == nil && len(logBytes) > 0 {
			if int64(len(logBytes)) == readLimit {
				logrus.Warnf("shim panic log file %s is larger than 1MB, logging only first 1MB", filepath.Join(bundleFlag, "panic.log"))
			}
			logrus.WithField("log", string(logBytes)).Warn("found shim panic logs during delete")
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			logrus.WithError(err).Warn("failed to open shim panic log")
		}

		// Attempt to find the hcssystem for this bundle and terminate it.
		if sys, _ := hcs.OpenComputeSystem(ctx, idFlag); sys != nil {
			defer sys.Close()
			if err := sys.Terminate(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "failed to terminate '%s': %v", idFlag, err)
			} else {
				ch := make(chan error, 1)
				go func() { ch <- sys.Wait() }()
				t := time.NewTimer(time.Second * 30)
				select {
				case <-t.C:
					sys.Close()
					return fmt.Errorf("timed out waiting for '%s' to terminate", idFlag)
				case err := <-ch:
					t.Stop()
					if err != nil {
						fmt.Fprintf(os.Stderr, "failed to wait for '%s' to terminate: %v", idFlag, err)
					}
				}
			}
		}

		// For Host Process containers if a group name is passed as the user for the container the shim will create a
		// temporary user for the container to run as and add it to the specified group. On container exit the account will
		// be deleted, but if the shim crashed unexpectedly (panic, terminated etc.) then the account may still be around.
		// The username will be the container ID so try and delete it here. The username character limit is 20, so we need to
		// slice down the container ID a bit.
		username := idFlag
		if len(username) > winapi.UserNameCharLimit {
			username = username[:winapi.UserNameCharLimit]
		}

		// Always try and delete the user, if it doesn't exist we'll get a specific error code that we can use to
		// not log any warnings.
		if err := winapi.NetUserDel(
			"",
			username,
		); err != nil && !errors.Is(err, winapi.NERR_UserNotFound) {
			fmt.Fprintf(os.Stderr, "failed to delete user %q: %v", username, err)
		}

		// TODO(ambarve):
		// correctly handle cleanup of cimfs layers in case of shim process crash here.

		if data, err := proto.Marshal(&task.DeleteResponse{
			ExitedAt:   timestamppb.New(time.Now()),
			ExitStatus: 255,
		}); err != nil {
			return err
		} else {
			if _, err := os.Stdout.Write(data); err != nil {
				return err
			}
		}
		return nil
	},
}
