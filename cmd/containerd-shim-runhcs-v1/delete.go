package main

import (
	gcontext "context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/containerd/containerd/runtime/v2/task"
	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"go.opencensus.io/trace"
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
	Action: func(context *cli.Context) (err error) {
		// We cant write anything to stdout for this cmd other than the
		// task.DeleteResponse by protcol. We can write to stderr which will be
		// warning logged in containerd.

		ctx, span := trace.StartSpan(gcontext.Background(), "delete")
		defer span.End()
		defer func() { oc.SetSpanStatus(span, err) }()

		bundleFlag := context.GlobalString("bundle")
		if bundleFlag == "" {
			return errors.New("bundle is required")
		}

		// hcsshim shim writes panic logs in the bundle directory in a file named "panic.log"
		// log those messages (if any) on stderr so that it shows up in containerd's log.
		// This should be done as the first thing so that we don't miss any panic logs even if
		// something goes wrong during delete op.
		// The file can be very large so read only first 1MB of data.
		readLimit := int64(1024 * 1024) // 1MB
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

		if data, err := proto.Marshal(&task.DeleteResponse{
			ExitedAt:   time.Now(),
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
