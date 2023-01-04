package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Microsoft/hcsshim/ext4/tar2ext4"
	cli "github.com/urfave/cli/v2"
)

const (
	// tar2ext4 convert flags
	inputFlag   = "input"
	outputFlag  = "output"
	overlayFlag = "overlay"
	vhdFlag     = "vhd"
	inlineFlag  = "inline"
)

func main() {
	app := cli.NewApp()
	app.Name = "tar2ext4"
	app.Usage = "converts tar file(s) into vhd(s)"
	app.Commands = []*cli.Command{}
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:    outputFlag,
			Aliases: []string{"o"},
			Usage:   "output file",
		},
		&cli.BoolFlag{
			Name:  overlayFlag,
			Usage: "produce overlayfs-compatible layer image",
		},
		&cli.BoolFlag{
			Name:  vhdFlag,
			Usage: "add a VHD footer to the end of the image",
		},
		&cli.BoolFlag{
			Name:  inlineFlag,
			Usage: "write small file data into the inode; not compatible with DAX",
		},
		&cli.StringSliceFlag{
			Name:    inputFlag,
			Aliases: []string{"i"},
			Usage:   "input file",
		},
	}
	app.Action = func(cliCtx *cli.Context) (err error) {
		in := os.Stdin
		if cliCtx.String(inputFlag) != "" {
			in, err = os.Open(cliCtx.String(inputFlag))
			if err != nil {
				return err
			}
		}

		outputName := cliCtx.String(outputFlag)
		if outputName == "" {
			return errors.New("output name must not be empty")
		}
		out, err := os.Create(outputName)
		if err != nil {
			return err
		}

		var opts []tar2ext4.Option
		if cliCtx.Bool(overlayFlag) {
			opts = append(opts, tar2ext4.ConvertWhiteout)
		}
		if cliCtx.Bool(vhdFlag) {
			opts = append(opts, tar2ext4.AppendVhdFooter)
		}
		if cliCtx.Bool(inlineFlag) {
			opts = append(opts, tar2ext4.InlineData)
		}

		if err = tar2ext4.Convert(in, out, opts...); err != nil {
			return err
		}
		// Exhaust the tar stream.
		_, _ = io.Copy(io.Discard, in)

		return nil
	}
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
