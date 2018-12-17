package shim

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/containerd/containerd/runtime/v2/task"
	"github.com/gogo/protobuf/proto"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/urfave/cli"
)

var deleteCommand = cli.Command{
	Name: "delete",
	Usage: `This command allows containerd to delete any container resources created, mounted, and/or run by a shim when containerd can no longer communicate over rpc. This happens if a shim is SIGKILL'd with a running container. These resources will need to be cleaned up when containerd looses the connection to a shim. This is also used when containerd boots and reconnects to shims. If a bundle is still on disk but containerd cannot connect to a shim, the delete command is invoked.
	
The delete command will be executed in the container's bundle as its cwd.`,
	Action: func(context *cli.Context) error {
		bundleFlag := context.GlobalString("bundle")
		if bundleFlag == "" {
			return errors.New("bundle required")
		}

		// Attempt to find the hcssystem for this bundle and terminate it.
		if sys, _ := hcs.OpenComputeSystem(idFlag); sys != nil {
			sys.Terminate()
			sys.Wait()
			sys.Close()
		}

		// Determine if the config file was a POD and if so kill the whole POD.
		configPath := filepath.Join(bundleFlag, "config.json")
		if _, err := os.Stat(configPath); err != nil {
			if !os.IsNotExist(err) {
				return err
			}
		} else {
			if s, err := getSpecFromPath(configPath); err != nil {
				return err
			} else {
				if containerType := s.Annotations["io.kubernetes.cri.container-type"]; containerType == "container" {
					if sandboxID := s.Annotations["io.kubernetes.cri.sandbox-id"]; sandboxID != "" {
						if sys, _ := hcs.OpenComputeSystem(sandboxID); sys != nil {
							sys.Terminate()
							sys.Wait()
							sys.Close()
						}
					}
				}
			}
		}

		// Remove the bundle on disk
		if err := os.RemoveAll(bundleFlag); err != nil {
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

func getSpecFromPath(path string) (*oci.Spec, error) {
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var spec oci.Spec
	if err := json.NewDecoder(f).Decode(&spec); err != nil {
		return nil, err
	}
	return &spec, nil
}
