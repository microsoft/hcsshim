package main

import (
	"github.com/Microsoft/hcsshim/internal/appargs"
	"github.com/Microsoft/hcsshim/internal/lcow"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"
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
		cli.UintFlag{
			Name:  "sizeGB",
			Value: 0,
			Usage: "optional: The size in GB of the scratch file to create",
		},
		cli.StringFlag{
			Name:  "cache-path",
			Usage: "optional: The path to an existing scratch.vhdx to copy instead of create.",
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

		sizeGB := uint32(context.Uint("sizeGB"))
		if sizeGB == 0 {
			sizeGB = lcow.DefaultScratchSizeGB
		}

		convertUVM, err := uvm.CreateLCOW(opts)
		if err != nil {
			return errors.Wrapf(err, "failed to create '%s'", opts.ID)
		}
		defer convertUVM.Close()
		if err := convertUVM.Start(); err != nil {
			return errors.Wrapf(err, "failed to start '%s'", opts.ID)
		}

		if err := lcow.CreateScratch(convertUVM, dest, sizeGB, context.String("cache-path")); err != nil {
			return errors.Wrapf(err, "failed to create ext4vhdx for '%s'", opts.ID)
		}

		return nil
	},
}
