//go:build windows

package main

import (
	"fmt"
	"os"
	"strings"

	cli "github.com/urfave/cli/v2"

	"github.com/Microsoft/hcsshim/internal/windevice"
)

const (
	parentIDFlag = "parentID"
	propertyFlag = "property"

	locationProperty = "location"
	idProperty       = "id"
)

var queryChildrenCommand = &cli.Command{
	Name:    "children",
	Aliases: []string{"ch"},
	Usage:   "Queries for given devices' children on the system",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    parentIDFlag,
			Aliases: []string{"id", "p"},
			Usage:   "Required: Parent device's instance IDs. Comma separated string.",
		},
		&cli.StringFlag{
			Name:  propertyFlag,
			Usage: "The `property` to query devices for; either 'location' or 'id'.",
			Value: idProperty,
		},
	},
	Action: func(ctx *cli.Context) error {
		if !ctx.IsSet(parentIDFlag) {
			return fmt.Errorf("%q command must specify at least one parent instance ID", ctx.Command.Name)
		}
		csParents := ctx.String(parentIDFlag)
		parents := strings.Split(csParents, ",")

		children, err := windevice.GetChildrenFromInstanceIDs(parents)
		if err != nil {
			return fmt.Errorf("could not find children for parents %q: %w", parents, err)
		}

		// should be defined, since there is a default value
		property := ctx.String(propertyFlag)
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
