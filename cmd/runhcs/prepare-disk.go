//go:build windows

package main

import (
	gcontext "context"

	"github.com/Microsoft/hcsshim/internal/appargs"
	"github.com/Microsoft/hcsshim/internal/lcow"
	"github.com/Microsoft/hcsshim/internal/otelutil"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	// prepareDiskStr string used to name the command and identity in the logs
	prepareDiskStr = "prepare-disk"
)

var prepareDiskCommand = cli.Command{
	Name:        prepareDiskStr,
	Usage:       "format a disk with ext4",
	Description: "Format a disk with ext4. To be used prior to exposing a pass-through disk. Prerequisite is that disk should be offline ('Get-Disk -Number <disk num> | Set-Disk -IsOffline $true').",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "destpath",
			Usage: "Required: describes the destination disk path",
		},
	},
	Before: appargs.Validate(),
	Action: func(context *cli.Context) (err error) {
		ctx, span := otelutil.StartSpan(gcontext.Background(), prepareDiskStr)
		defer span.End()
		defer func() { otelutil.SetSpanStatus(span, err) }()

		dest := context.String("destpath")
		if dest == "" {
			return errors.New("'destpath' is required")
		}

		if osversion.Build() < osversion.RS5 {
			return errors.New("LCOW is not supported pre-RS5")
		}

		opts := uvm.NewDefaultOptionsLCOW("preparedisk-uvm", context.GlobalString("owner"))
		// Default SCSI controller count is 4, we don't need that for this UVM,
		// bring it back to 1 to avoid any confusion with SCSI controller numbers.
		opts.SCSIControllerCount = 1

		preparediskUVM, err := uvm.CreateLCOW(ctx, opts)
		if err != nil {
			return errors.Wrapf(err, "failed to create '%s'", opts.ID)
		}
		defer preparediskUVM.Close()
		if err := preparediskUVM.Start(ctx); err != nil {
			return errors.Wrapf(err, "failed to start '%s'", opts.ID)
		}
		if err := lcow.FormatDisk(ctx, preparediskUVM, dest); err != nil {
			return errors.Wrapf(err, "failed to format disk '%s' with ext4", opts.ID)
		}

		return nil
	},
}
