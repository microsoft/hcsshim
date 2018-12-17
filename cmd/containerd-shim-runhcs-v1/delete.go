package shim

import (
	"errors"

	"github.com/urfave/cli"
)

var deleteCommand = cli.Command{
	Name: "delete",
	Usage: `This command allows containerd to delete any container resources created, mounted, and/or run by a shim when containerd can no longer communicate over rpc. This happens if a shim is SIGKILL'd with a running container. These resources will need to be cleaned up when containerd looses the connection to a shim. This is also used when containerd boots and reconnects to shims. If a bundle is still on disk but containerd cannot connect to a shim, the delete command is invoked.
	
The delete command will be executed in the container's bundle as its cwd.`,
	Action: func(context *cli.Context) error {
		return errors.New("not implemented")
	},
}
