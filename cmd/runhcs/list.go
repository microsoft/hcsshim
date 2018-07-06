package main

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"encoding/json"

	"github.com/Microsoft/hcsshim/internal/appargs"
	"github.com/urfave/cli"
)

const formatOptions = `table or json`

// containerState represents the platform agnostic pieces relating to a
// running container's status and state
type containerState struct {
	// Version is the OCI version for the container
	Version string `json:"ociVersion"`
	// ID is the container ID
	ID string `json:"id"`
	// InitProcessPid is the init process id in the parent namespace
	InitProcessPid int `json:"pid"`
	// Status is the current status of the container, running, paused, ...
	Status string `json:"status"`
	// Bundle is the path on the filesystem to the bundle
	Bundle string `json:"bundle"`
	// Rootfs is a path to a directory containing the container's root filesystem.
	Rootfs string `json:"rootfs"`
	// Created is the unix timestamp for the creation time of the container in UTC
	Created time.Time `json:"created"`
	// Annotations is the user defined annotations added to the config.
	Annotations map[string]string `json:"annotations,omitempty"`
	// The owner of the state directory (the owner of the container).
	Owner string `json:"owner"`
}

var listCommand = cli.Command{
	Name:  "list",
	Usage: "lists containers started by runhcs with the given root",
	ArgsUsage: `

Where the given root is specified via the global option "--root"
(default: "/run/runhcs").

EXAMPLE 1:
To list containers created via the default "--root":
       # runhcs list

EXAMPLE 2:
To list containers created using a non-default value for "--root":
       # runhcs --root value list`,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "format, f",
			Value: "table",
			Usage: `select one of: ` + formatOptions,
		},
		cli.BoolFlag{
			Name:  "quiet, q",
			Usage: "display only container IDs",
		},
	},
	Before: appargs.Validate(),
	Action: func(context *cli.Context) error {
		s, err := getContainers(context)
		if err != nil {
			return err
		}

		if context.Bool("quiet") {
			for _, item := range s {
				fmt.Println(item.ID)
			}
			return nil
		}

		switch context.String("format") {
		case "table":
			w := tabwriter.NewWriter(os.Stdout, 12, 1, 3, ' ', 0)
			fmt.Fprint(w, "ID\tPID\tSTATUS\tBUNDLE\tCREATED\tOWNER\n")
			for _, item := range s {
				fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\n",
					item.ID,
					item.InitProcessPid,
					item.Status,
					item.Bundle,
					item.Created.Format(time.RFC3339Nano),
					item.Owner)
			}
			if err := w.Flush(); err != nil {
				return err
			}
		case "json":
			if err := json.NewEncoder(os.Stdout).Encode(s); err != nil {
				return err
			}
		default:
			return fmt.Errorf("invalid format option")
		}
		return nil
	},
}

func getContainers(context *cli.Context) ([]containerState, error) {
	ids, err := stateKey.Enumerate()
	if err != nil {
		return nil, err
	}

	var s []containerState
	for _, id := range ids {
		c, err := getContainer(id, false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "reading state for %s: %v\n", id, err)
			continue
		}
		status, err := c.Status()
		if err != nil {
			fmt.Fprintf(os.Stderr, "reading status for %s: %v\n", id, err)
		}

		s = append(s, containerState{
			ID:             id,
			Version:        c.Spec.Version,
			InitProcessPid: c.ShimPid,
			Status:         string(status),
			Bundle:         c.Bundle,
			Rootfs:         c.Rootfs,
			Created:        c.Created,
			Annotations:    c.Spec.Annotations,
		})
	}
	return s, nil
}
