package main

import (
	"github.com/microsoft/hcsshim/internal/appargs"
	"github.com/microsoft/hcsshim/internal/lcow"
	"github.com/microsoft/hcsshim/internal/uvm"
	"github.com/microsoft/hcsshim/osversion"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

var createScratchCommand = cli.Command{
	Name:        "create-scratch",
	Usage:       "creates a scratch vhdx at 'destpath' that is ext4 formatted",
	Description: "Creates a scratch vhdx at 'destpath' that is ext4 formatted",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "destpath",
			Usage: "Required: describes the destination vhd path",
		},
	},
	Before: appargs.Validate(),
	Action: func(context *cli.Context) error {
		dest := context.String("destpath")
		if dest == "" {
			return errors.New("'destpath' is required")
		}

		if osversion.Get().Build < osversion.RS5 {
			return errors.New("LCOW is not supported pre-RS5")
		}

		opts := uvm.NewDefaultOptionsLCOW("createscratch-uvm", context.GlobalString("owner"))

		// 256MB with boot from vhd supported.
		opts.MemorySizeInMB = 256
		opts.VPMemDeviceCount = 1

		convertUVM, err := uvm.CreateLCOW(opts)
		if err != nil {
			return errors.Wrapf(err, "failed to create '%s'", opts.ID)
		}
		defer convertUVM.Close()
		if err := convertUVM.Start(); err != nil {
			return errors.Wrapf(err, "failed to start '%s'", opts.ID)
		}

		if err := lcow.CreateScratch(convertUVM, dest, lcow.DefaultScratchSizeGB, "", ""); err != nil {
			return errors.Wrapf(err, "failed to create ext4vhdx for '%s'", opts.ID)
		}

		return nil
	},
}
