//go:build windows

package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	cli "github.com/urfave/cli/v2"

	"github.com/Microsoft/hcsshim/internal/winapi"
)

const desc = `A stand-alone tool that replicates a limited subset of sc.exe and other tools.
It can be used within WCOW uVMs and containers without requiring additional DLLs.`

func main() {
	app := &cli.App{
		Name:        "device-util",
		Usage:       "tool for managing devices, drivers, and services on Windows",
		Description: desc,
		Commands: []*cli.Command{
			driverCommand,
			serviceCommand,
			queryChildrenCommand,
			readObjDirCommand,
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.New(os.Stderr, "", 0).Fatal(err)
	}
}

func verifyElevated(ctx *cli.Context) error {
	if !winapi.IsElevated() {
		n := strings.TrimSpace(ctx.App.Name + " " + ctx.Command.FullName())
		return fmt.Errorf("%q must be run in an elevated context", n)
	}
	return nil
}
