package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Microsoft/hcsshim/internal/shimdiag"
	"github.com/urfave/cli"
)

func main() {
	app := cli.NewApp()
	app.Name = "shimdiag"
	app.Usage = "runhcs shim diagnostic tool"
	app.Commands = []cli.Command{
		listCommand,
		execCommand,
		stacksCommand,
		shareCommand,
	}
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func getPid(shimName string) (int32, error) {
	shim, err := shimdiag.GetShim(shimName)
	if err != nil {
		return 0, err
	}
	defer shim.Close()

	svc := shimdiag.NewShimDiagClient(shim)
	resp, err := svc.DiagPid(context.Background(), &shimdiag.PidRequest{})
	if err != nil {
		return 0, err
	}
	return resp.Pid, nil
}
