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
)

const pollTick = 5 * time.Millisecond

// flags
var (
	_driverName string
	_driverPath string
	_wait       bool
	_timeout    time.Duration
)

var driverCommand = &cli.Command{
	Name:    "kernel-driver",
	Aliases: []string{"kdriver", "kd"},
	Usage:   "Manage legacy-type (kernel) drivers",
	Before:  verifyElevated,
	Subcommands: []*cli.Command{
		{
			Name:    "install",
			Aliases: []string{"i"},
			Action:  installDriver,
			Usage:   "Install a kernel driver",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "name",
					Aliases:     []string{"n"},
					Destination: &_driverName,
					Usage:       "The `name` of the service to create for the driver",
				},
				&cli.StringFlag{
					Name:        "path",
					Aliases:     []string{"p"},
					Destination: &_driverPath,
					Usage:       "The `path` to kernel driver to install",
				},
				&cli.BoolFlag{
					Name:        "wait",
					Destination: &_wait,
					Usage:       "Wait for the kernel driver service to start running after install",
					Value:       false,
				},
				&cli.DurationFlag{
					Name:        "timeout",
					Destination: &_timeout,
					Usage:       "Timeout `duration` when waiting for kernel driver service to start running after install",
					Value:       100 * time.Millisecond,
				},
			},
		},
		{
			Name:    "list",
			Aliases: []string{"ls"},
			Usage:   "List all active kernel drivers",
			Action: func(ctx *cli.Context) error {
				return printServices(ctx, windows.SERVICE_KERNEL_DRIVER, windows.SERVICE_ACTIVE)
			},
		},
	},
}

func installDriver(_ *cli.Context) (err error) {
	if _driverPath == "" {
		return fmt.Errorf("path is required and cannot be empty")
	}
	if _driverPath, err = filepath.Abs(_driverPath); err != nil {
		return fmt.Errorf("could not get absolute path for %q: %w", _driverPath, err)
	}
	if _, err = os.Stat(_driverPath); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("no driver found at %q: %w", _driverPath, err)
	}
	dpath, err := windows.UTF16PtrFromString(_driverPath)
	if err != nil {
		return fmt.Errorf("could not convert driver path %q to UTF-16: %w", _driverPath, err)
	}

	// create driver name, if necessary
	if _driverName == "" {
		n := filepath.Base(_driverPath)
		ext := filepath.Ext(n)
		_driverName = n[:(len(n) - len(ext))]
	}
	dname, err := windows.UTF16PtrFromString(_driverName)
	if err != nil {
		return fmt.Errorf("could not convert driver name %q to UTF-16: %w", _driverName, err)
	}
	log.Printf("installing driver %q from  %q", _driverName, _driverPath)

	log.Printf("creating service %q", _driverName)
	svc, err := windows.CreateService(
		_scm.handle(),
		dname,
		dname,
		windows.SERVICE_START|windows.SERVICE_QUERY_STATUS,
		windows.SERVICE_KERNEL_DRIVER,
		windows.SERVICE_AUTO_START,
		windows.SERVICE_ERROR_NORMAL,
		dpath,
		nil, // loadOrderGroup
		nil, // tagID
		nil, // dependencies
		nil, // serviceStartName
		nil, // password
	)
	if err != nil {
		return fmt.Errorf("could not create service %q: %w", _driverName, err)
	}

	log.Printf("starting service %q", _driverName)
	if err = windows.StartService(svc, 0, nil); err != nil {
		return fmt.Errorf("could not start service %q: %w", _driverName, err)
	}

	if !_wait {
		return nil
	}

	log.Printf("waiting for service %q to start", _driverName)
	tick := pollTick
	if pollTick > _timeout {
		tick = _timeout
	}
	poll := time.NewTicker(tick)
	defer poll.Stop()

	t := time.NewTimer(_timeout)
	defer t.Stop()

	for {
		// check before waiting on ticker and timeout
		st, err := getServiceState(svc)
		if err != nil {
			return err
		}
		if st == windows.SERVICE_RUNNING {
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

func getServiceState(svc windows.Handle) (uint32, error) {
	var st windows.SERVICE_STATUS
	if err := windows.QueryServiceStatus(svc, &st); err != nil {
		return 0, fmt.Errorf("could not get service %q status: %w", _driverName, err)
	}
	return st.CurrentState, nil
}
