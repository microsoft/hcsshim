package shim

import (
	"errors"

	"github.com/urfave/cli"
)

var startCommand = cli.Command{
	Name: "start",
	Usage: `This command will launch new shims.

The start command, as well as all binary calls to the shim, has the bundle for the container set as the cwd.

The start command MUST return an address to a shim for containerd to issue API requests for container operations.

The start command can either start a new shim or return an address to an existing shim based on the shim's logic.`,
	Action: func(context *cli.Context) error {
		return errors.New("not implemented")
	},
}
