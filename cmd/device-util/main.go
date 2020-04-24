package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Microsoft/hcsshim/internal/cfgmgr"
	"github.com/Microsoft/hcsshim/internal/ntobjectdir"
	"github.com/urfave/cli"
)

const usage = `device-util is a command line tool for querying devices present in a Windows UVM`

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

var (
	parentIDFlag = "parentID"
	propertyFlag = "property"

	locationProperty = "location"
	idProperty       = "id"
)

var readObjDirCommand = cli.Command{
	Name:  "read-global",
	Usage: "outputs contents of global object directory",
	Flags: []cli.Flag{},
	Action: func(context *cli.Context) error {
		entries, err := ntobjectdir.EnumerateNTGlobalObjectDirectory()
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

		children, err := cfgmgr.GetChildrenFromInstanceIDs(parents)
		if err != nil {
			return err
		}

		property := ""
		if context.IsSet(propertyFlag) {
			property = context.String(propertyFlag)
		}
		if property == locationProperty {
			children, err = cfgmgr.GetDeviceLocationPathsFromIDs(children)
			if err != nil {
				return err
			}
		}
		formattedChildren := strings.Join(children, ",")
		fmt.Fprintln(os.Stdout, formattedChildren)
		return nil
	},
}
