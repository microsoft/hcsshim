package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

const (
	pathsFlag   = "paths"
	timeoutFlag = "timeout"
)

var errEmptyPaths = errors.New("paths cannot be empty")

// This is a hook that waits for a specific path to appear.
// The hook has required list of comma-separated paths and a default timeout in seconds.

func main() {
	app := newCliApp()
	if err := app.Run(os.Args); err != nil {
		logrus.Fatalf("%s\n", err)
	}
	os.Exit(0)
}

func newCliApp() *cli.App {
	app := cli.NewApp()
	app.Name = "wait-paths"
	app.Usage = "Provide a list paths and an optional timeout"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:     pathsFlag + ",p",
			Usage:    "Comma-separated list of paths that should become available",
			Required: true,
		},
		cli.IntFlag{
			Name:  timeoutFlag + ",t",
			Usage: "Timeout in seconds",
			Value: 30,
		},
	}
	app.Action = run
	return app
}

func run(cCtx *cli.Context) error {
	timeout := cCtx.GlobalInt(timeoutFlag)

	pathsVal := cCtx.GlobalString(pathsFlag)
	if pathsVal == "" {
		return errEmptyPaths
	}
	paths := strings.Split(cCtx.GlobalString(pathsFlag), ",")

	waitCtx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	for _, path := range paths {
		for {
			if _, err := os.Stat(path); err != nil {
				if !os.IsNotExist(err) {
					return err
				}
				select {
				case <-waitCtx.Done():
					return fmt.Errorf("timeout while waiting for path %q to appear: %w", path, context.DeadlineExceeded)
				default:
					time.Sleep(time.Millisecond * 10)
					continue
				}
			}
			break
		}
	}
	return nil
}
