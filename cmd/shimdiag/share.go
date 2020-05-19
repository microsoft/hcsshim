package main

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/appargs"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	"github.com/urfave/cli"
)

// LCOW root FS VHD is mounted read only so the only place you can mount a directory
// is over an existing mount point like /home etc. Single file mapping will not work and
// will return 'mkdir /path: read-only file system'. You can set the preferred root fs type
// to initrd to circumvent this.

var shareCommand = cli.Command{
	Name:      "share",
	Usage:     "Share a file/directory in a shim's hosting utility VM",
	ArgsUsage: "<shim name> <host_path> <uvm_path>",
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "readonly,ro",
			Usage: "Make the directory/file being shared read only",
		},
	},
	Before: appargs.Validate(appargs.String, appargs.String, appargs.String),
	Action: func(c *cli.Context) error {
		args := c.Args()
		var (
			readOnly = c.Bool("readonly")
			shimName = args[0]
			hostPath = args[1]
			uvmPath  = args[2]
		)
		shim, err := getShim(shimName)
		if err != nil {
			return err
		}

		req := &shimdiag.ShareRequest{
			HostPath: hostPath,
			UvmPath:  uvmPath,
			ReadOnly: readOnly,
		}

		svc := shimdiag.NewShimDiagClient(shim)
		_, err = svc.DiagShare(context.Background(), req)
		if err != nil {
			return fmt.Errorf("failed to share directory %s into UVM: %s", hostPath, err)
		}

		fmt.Printf("Shared %s into %s at %s\n", hostPath, shimName, uvmPath)
		return nil
	},
}
