package main

import (
	"errors"

	"github.com/urfave/cli"
)

var serveCommand = cli.Command{
	Name:   "serve",
	Hidden: true,
	Action: func(context *cli.Context) error {
		return errors.New("not implemented")
	},
}
