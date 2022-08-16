//go:build windows

package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	cli "github.com/urfave/cli/v2"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
)

const pollTick = 5 * time.Millisecond

// flag names

const (
	flagDriverName     = "name"
	flagDriverDispName = "display-name"
	flagDriverPath     = "path"
	flagWait           = "wait"
	flagTimeout        = "timeout"
)

var driverCommand = &cli.Command{
	Name:    "kernel-driver",
	Aliases: []string{"kdriver", "kd"},
	Usage:   "manage legacy-type (kernel) drivers",
	Before:  verifyElevated,
	Subcommands: []*cli.Command{
		{
			Name:    "install",
			Aliases: []string{"i"},
			Action:  installDriver,
			Usage:   "install a legacy-mode kernel driver",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    flagDriverName,
					Aliases: []string{"n"},
					Usage:   "`name` of the driver to install. Defaults to executable name.",
				},
				&cli.StringFlag{
					Name:    flagDriverDispName,
					Aliases: []string{"dn", "disp-name"},
					Usage:   "driver display `name`.",
				},
				&cli.StringFlag{
					Name:    flagDriverPath,
					Aliases: []string{"p"},
					Usage:   "kernel driver `path`.",
				},
				&cli.BoolFlag{
					Name:  flagWait,
					Usage: "wait for the kernel driver to start running after install.",
					Value: false,
				},
				&cli.DurationFlag{
					Name:  flagTimeout,
					Usage: "`duration` to wait for kernel driver service to start after installation.",
					Value: 100 * time.Millisecond,
				},
			},
		},
		{
			Name:    "list",
			Aliases: []string{"ls"},
			Usage:   "list active legacy kernel drivers",
			Action: func(ctx *cli.Context) error {
				return printServices(ctx, windows.SERVICE_KERNEL_DRIVER, windows.SERVICE_ACTIVE)
			},
		},
	},
}

func installDriver(ctx *cli.Context) (err error) {
	path := ctx.String(flagDriverPath)
	if path == "" {
		return fmt.Errorf("driver path is required and cannot be empty")
	}
	if path, err = filepath.Abs(path); err != nil {
		return fmt.Errorf("could not get absolute path for %q: %w", path, err)
	}
	if _, err = os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("no driver found at %q: %w", path, err)
	}

	// create driver name, if necessary
	name := ctx.String(flagDriverName)
	if name == "" {
		n := filepath.Base(path)
		ext := filepath.Ext(n)
		name = n[:(len(n) - len(ext))]
	}
	dispName := ctx.String(flagDriverDispName)
	if dispName == "" {
		dispName = name
	}
	log.Printf("installing driver %q (%q) from  %q", name, dispName, path)

	scm, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("could not create service manager: %w", err)
	}

	svc, err := scm.CreateService(name, path, mgr.Config{
		DisplayName:  dispName,
		ServiceType:  windows.SERVICE_KERNEL_DRIVER,
		StartType:    windows.SERVICE_AUTO_START,
		ErrorControl: windows.SERVICE_ERROR_NORMAL,
	})
	if err != nil {
		return fmt.Errorf("could not create service %q: %w", name, err)
	}
	log.Printf("created service %q", name)

	if err = svc.Start(); err != nil {
		return fmt.Errorf("could not start service %q: %w", svc.Name, err)
	}
	log.Printf("started service %q", svc.Name)

	if !ctx.Bool(flagWait) {
		return nil
	}

	timeout := ctx.Duration(flagTimeout)
	log.Printf("waiting for service %q to start", svc.Name)
	tick := pollTick
	if pollTick > timeout {
		tick = timeout
	}
	poll := time.NewTicker(tick)
	defer poll.Stop()

	t := time.NewTimer(timeout)
	defer t.Stop()

	for {
		// check before waiting on ticker and timeout
		st, err := svc.Query()
		if err != nil {
			return fmt.Errorf("could not get service %q status: %w", svc.Name, err)
		}
		if st.State == windows.SERVICE_RUNNING {
			break
		}

		select {
		case <-poll.C:
		case <-t.C:
			return fmt.Errorf("timed out while waiting for service to start: last seen state was %d", st)
		}
	}
	return nil
}
