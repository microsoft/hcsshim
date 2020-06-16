package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Microsoft/hcsshim/internal/windevice"
	"github.com/Microsoft/hcsshim/internal/winobjdir"
	"github.com/urfave/cli"
)

const usage = `device-util is a command line tool for querying devices present on Windows`

func main() {
	app := cli.NewApp()
	app.Name = "device-util"
	app.Commands = []cli.Command{
		queryChildrenCommand,
		readObjDirCommand,
	}
	app.Usage = usage

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

const (
	parentIDFlag = "parentID"
	propertyFlag = "property"
	objDirFlag   = "dir"

	locationProperty = "location"
	idProperty       = "id"

	globalNTPath = "\\Global??"
)

var readObjDirCommand = cli.Command{
	Name:  "obj-dir",
	Usage: "outputs contents of a NT object directory",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  objDirFlag,
			Usage: "Optional: Object directory to query. Defaults to the global object directory.",
		},
	},
	Action: func(context *cli.Context) error {
		dir := globalNTPath
		if context.IsSet(objDirFlag) {
			dir = context.String(objDirFlag)
		}
		entries, err := winobjdir.EnumerateNTObjectDirectory(dir)
		if err != nil {
			return err
		}
		formatted := strings.Join(entries, ",")
		fmt.Fprintln(os.Stdout, formatted)
		return nil
	},
}

var queryChildrenCommand = cli.Command{
	Name:  "children",
	Usage: "queries for given devices' children on the system",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  parentIDFlag,
			Usage: "Required: Parent device's instance IDs. Comma separated string.",
		},
		cli.StringFlag{
			Name:  propertyFlag,
			Usage: "Either 'location' or 'id', default 'id'. String indicating a property to query devices for.",
		},
	},
	Action: func(context *cli.Context) error {
		if !context.IsSet(parentIDFlag) {
			return errors.New("`children` command must specify at least one parent instance ID")
		}
		csParents := context.String(parentIDFlag)
		parents := strings.Split(csParents, ",")

		children, err := windevice.GetChildrenFromInstanceIDs(parents)
		if err != nil {
			return err
		}

		property := idProperty
		if context.IsSet(propertyFlag) {
			property = context.String(propertyFlag)
		}
		if property == locationProperty {
			children, err = windevice.GetDeviceLocationPathsFromIDs(children)
			if err != nil {
				return err
			}
		}
		formattedChildren := strings.Join(children, ",")
		fmt.Fprintln(os.Stdout, formattedChildren)
		return nil
	},
}
