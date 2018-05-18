package main

import (
	"os"
	"syscall"

	"github.com/Microsoft/hcsshim/internal/appargs"
	"github.com/urfave/cli"
)

// default action is to start a container
var runCommand = cli.Command{
	Name:  "run",
	Usage: "create and run a container",
	ArgsUsage: `<container-id>

Where "<container-id>" is your name for the instance of the container that you
are starting. The name you provide for the container instance must be unique on
your host.`,
	Description: `The run command creates an instance of a container for a bundle. The bundle
is a directory with a specification file named "` + specConfig + `" and a root
filesystem.

The specification file includes an args parameter. The args parameter is used
to specify command(s) that get run when the container is started. To change the
command(s) that get executed on start, edit the args parameter of the spec. See
"runc spec --help" for more explanation.`,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "bundle, b",
			Value: "",
			Usage: `path to the root of the bundle directory, defaults to the current directory`,
		},
		cli.BoolFlag{
			Name:  "detach, d",
			Usage: "detach from the container's process",
		},
		cli.StringFlag{
			Name:  "pid-file",
			Value: "",
			Usage: "specify the file to write the process id to",
		},
		cli.StringFlag{
			Name:  "shim-log",
			Value: "",
			Usage: "path to the log file for the launched shim process",
		},
		cli.StringFlag{
			Name:  "vm-log",
			Value: "",
			Usage: "path to the log file for the launched VM shim process",
		},
	},
	Before: appargs.Validate(argID),
	Action: func(context *cli.Context) error {
		id := context.Args().First()
		pidFile, err := absPathOrEmpty(context.String("pid-file"))
		if err != nil {
			return err
		}
		shimLog, err := absPathOrEmpty(context.String("shim-log"))
		if err != nil {
			return err
		}
		vmLog, err := absPathOrEmpty(context.String("vm-log"))
		if err != nil {
			return err
		}
		spec, err := setupSpec(context)
		if err != nil {
			return err
		}
		c, err := createContainer(&containerConfig{
			ID:          id,
			PidFile:     pidFile,
			ShimLogFile: shimLog,
			VMLogFile:   vmLog,
			Spec:        spec,
		})
		if err != nil {
			return err
		}
		p, err := os.FindProcess(c.ShimPid)
		if err != nil {
			return err
		}
		err = c.Exec()
		if err != nil {
			return err
		}
		if !context.Bool("detach") {
			state, err := p.Wait()
			if err != nil {
				return err
			}
			c.Remove()
			os.Exit(int(state.Sys().(syscall.WaitStatus).ExitCode))
		}
		return nil
	},
}
