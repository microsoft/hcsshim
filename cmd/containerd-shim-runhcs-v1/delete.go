package main

import (
	gcontext "context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/containerd/containerd/runtime/v2/task"
	"github.com/gogo/protobuf/proto"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"go.opencensus.io/trace"
)

var deleteCommand = cli.Command{
	Name: "delete",
	Usage: `
This command allows containerd to delete any container resources created, mounted, and/or run by a shim when containerd can no longer communicate over rpc. This happens if a shim is SIGKILL'd with a running container. These resources will need to be cleaned up when containerd loses the connection to a shim. This is also used when containerd boots and reconnects to shims. If a bundle is still on disk but containerd cannot connect to a shim, the delete command is invoked.

The delete command will be executed in the container's bundle as its cwd.
`,
	SkipArgReorder: true,
	Action: func(context *cli.Context) (err error) {
		// We cant write anything to stdout for this cmd other than the
		// task.DeleteResponse by protocol. We can write to stderr which will be
		// warning logged in containerd.
		logrus.SetOutput(ioutil.Discard)

		ctx, span := trace.StartSpan(gcontext.Background(), "delete")
		defer span.End()
		defer func() { oc.SetSpanStatus(span, err) }()

		bundleFlag := context.GlobalString("bundle")
		if bundleFlag == "" {
			return errors.New("bundle is required")
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

		// Determine if the config file was a POD and if so kill the whole POD.
		if s, err := getSpecAnnotations(bundleFlag); err != nil {
			if !os.IsNotExist(err) {
				return err
			}
		} else {
			if containerType := s["io.kubernetes.cri.container-type"]; containerType == "container" {
				if sandboxID := s["io.kubernetes.cri.sandbox-id"]; sandboxID != "" {
					if sys, _ := hcs.OpenComputeSystem(ctx, sandboxID); sys != nil {
						if err := sys.Terminate(ctx); err != nil {
							fmt.Fprintf(os.Stderr, "failed to terminate '%s': %v", idFlag, err)
						} else if err := sys.Wait(); err != nil {
							fmt.Fprintf(os.Stderr, "failed to wait for '%s' to terminate: %v", idFlag, err)
						}
						sys.Close()
					}
				}
			}
		}

		// Remove the bundle on disk
		if err := os.RemoveAll(bundleFlag); err != nil && !os.IsNotExist(err) {
			return err
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
