package main

import (
	"errors"
	"io/ioutil"
	"os"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/containerd/containerd/runtime/v2/task"
	"github.com/gogo/protobuf/proto"
	"github.com/urfave/cli"
)

var deleteCommand = cli.Command{
	Name: "delete",
	Usage: `
This command allows containerd to delete any container resources created, mounted, and/or run by a shim when containerd can no longer communicate over rpc. This happens if a shim is SIGKILL'd with a running container. These resources will need to be cleaned up when containerd looses the connection to a shim. This is also used when containerd boots and reconnects to shims. If a bundle is still on disk but containerd cannot connect to a shim, the delete command is invoked.
	
The delete command will be executed in the container's bundle as its cwd.
`,
	SkipArgReorder: true,
	Action: func(context *cli.Context) error {
		// We cant write anything to stdout/stderr for this cmd.
		logrus.SetOutput(ioutil.Discard)

		bundleFlag := context.GlobalString("bundle")
		if bundleFlag == "" {
			return errors.New("bundle is required")
		}

		// Attempt to find the hcssystem for this bundle and terminate it.
		if sys, _ := hcs.OpenComputeSystem(idFlag); sys != nil {
			sys.Terminate()
			sys.Wait()
			sys.Close()
		}

		// Determine if the config file was a POD and if so kill the whole POD.
		if s, err := getSpecAnnotations(bundleFlag); err != nil {
			if !os.IsNotExist(err) {
				return err
			}
		} else {
			if containerType := s["io.kubernetes.cri.container-type"]; containerType == "container" {
				if sandboxID := s["io.kubernetes.cri.sandbox-id"]; sandboxID != "" {
					if sys, _ := hcs.OpenComputeSystem(sandboxID); sys != nil {
						sys.Terminate()
						sys.Wait()
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
