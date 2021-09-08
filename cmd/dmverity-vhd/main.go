package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	"github.com/Microsoft/hcsshim/ext4/dmverity"
	"github.com/Microsoft/hcsshim/ext4/tar2ext4"
)

const usage = `dmverity-vhd is a command line tool for creating LCOW layer VHDs with dm-verity hashes.`

const (
	usernameFlag  = "username"
	passwordFlag  = "password"
	imageFlag     = "image"
	verboseFlag   = "verbose"
	outputDirFlag = "out-dir"
	maxVHDSize    = 128 * 1024 * 1024 * 1024
)

func init() {
	log.SetFormatter(&log.TextFormatter{
		DisableTimestamp: true,
	})

	log.SetOutput(os.Stdout)

	log.SetLevel(log.WarnLevel)
}

func main() {
	cli.VersionFlag = cli.BoolFlag{
		Name: "version",
	}

	app := cli.NewApp()
	app.Name = "dmverity-vhd"
	app.Commands = []cli.Command{
		createVHDCommand,
		rootHashVHDCommand,
	}
	app.Usage = usage
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  verboseFlag + ",v",
			Usage: "Optional: verbose output",
		},
	}

	if err := app.Run(os.Args); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func fetchImageLayers(ctx *cli.Context) (layers []v1.Layer, err error) {
	image := ctx.String(imageFlag)
	ref, err := name.ParseReference(image)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse image reference: %s", image)
	}

	var remoteOpts []remote.Option
	if ctx.IsSet(usernameFlag) && ctx.IsSet(passwordFlag) {
		auth := authn.Basic{
			Username: ctx.String(usernameFlag),
			Password: ctx.String(passwordFlag),
		}
		authConf, err := auth.Authorization()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to set remote")
		}
		log.Debug("using basic auth")
		authOpt := remote.WithAuth(authn.FromConfig(*authConf))
		remoteOpts = append(remoteOpts, authOpt)
	}

	img, err := remote.Image(ref, remoteOpts...)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to fetch image %q, make sure it exists", image)
	}
	conf, _ := img.ConfigName()
	log.Debugf("Image id: %s", conf.String())
	return img.Layers()
}

var createVHDCommand = cli.Command{
	Name:  "create",
	Usage: "creates LCOW layer VHDs inside the output directory with dm-verity super block and merkle tree appended at the end",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:     imageFlag + ",i",
			Usage:    "Required: container image reference",
			Required: true,
		},
		cli.StringFlag{
			Name:     outputDirFlag + ",o",
			Usage:    "Required: output directory path",
			Required: true,
		},
		cli.StringFlag{
			Name:  usernameFlag + ",u",
			Usage: "Optional: custom registry username",
		},
		cli.StringFlag{
			Name:  passwordFlag + ",p",
			Usage: "Optional: custom registry password",
		},
	},
	Action: func(ctx *cli.Context) error {
		verbose := ctx.GlobalBool(verboseFlag)
		if verbose {
			log.SetLevel(log.DebugLevel)
		}

		layers, err := fetchImageLayers(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to fetch image layers")
		}

		outDir := ctx.String(outputDirFlag)
		if _, err := os.Stat(outDir); os.IsNotExist(err) {
			log.Debugf("creating output directory %q", outDir)
			if err := os.MkdirAll(outDir, 0755); err != nil {
				return errors.Wrapf(err, "failed to create output directory %s", outDir)
			}
		}

		log.Debug("creating layer VHDs with dm-verity:")
		for layerNumber, layer := range layers {
			diffID, err := layer.DiffID()
			if err != nil {
				return errors.Wrap(err, "failed to read layer diff")
			}
			log.Debugf("Layer #%d, layer hash: %s", layerNumber, diffID.String())

			rc, err := layer.Uncompressed()
			if err != nil {
				return errors.Wrapf(err, "failed to uncompress layer %s", diffID.String())
			}

			vhdPath := filepath.Join(ctx.String(outputDirFlag), diffID.Hex+".vhd")
			out, err := os.Create(vhdPath)
			if err != nil {
				return errors.Wrapf(err, "failed to create layer vhd %s", vhdPath)
			}

			log.Debug("converting tar to layer VHD")
			opts := []tar2ext4.Option{
				tar2ext4.ConvertWhiteout,
				tar2ext4.MaximumDiskSize(maxVHDSize),
				tar2ext4.AppendVhdFooter,
				tar2ext4.AppendDMVerity,
			}
			if err := tar2ext4.Convert(rc, out, opts...); err != nil {
				return errors.Wrap(err, "failed to convert tar to ext4")
			}
			fmt.Fprintf(os.Stdout, "Layer %d: %s\n", layerNumber, vhdPath)
		}
		return nil
	},
}

var rootHashVHDCommand = cli.Command{
	Name:  "roothash",
	Usage: "compute root hashes for each LCOW layer VHD",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:     imageFlag + ",i",
			Usage:    "Required: container image reference",
			Required: true,
		},
		cli.StringFlag{
			Name:  usernameFlag + ",u",
			Usage: "Optional: custom registry username",
		},
		cli.StringFlag{
			Name:  passwordFlag + ",p",
			Usage: "Optional: custom registry password",
		},
	},
	Action: func(ctx *cli.Context) error {
		verbose := ctx.GlobalBool(verboseFlag)
		if verbose {
			log.SetLevel(log.DebugLevel)
		}

		layers, err := fetchImageLayers(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to fetch image layers")
		}
		log.Debugf("%d layers found", len(layers))

		tmpFile, err := ioutil.TempFile("", "")
		if err != nil {
			return errors.Wrap(err, "failed to create temporary file")
		}
		defer os.Remove(tmpFile.Name())

		for layerNumber, layer := range layers {
			diffID, err := layer.DiffID()
			if err != nil {
				return errors.Wrap(err, "failed to read layer diff")
			}
			log.Debugf("Layer %d. Uncompressed layer hash: %s", layerNumber, diffID.String())

			rc, err := layer.Uncompressed()
			if err != nil {
				return errors.Wrapf(err, "failed to uncompress layer %s", diffID.String())
			}

			opts := []tar2ext4.Option{
				tar2ext4.ConvertWhiteout,
				tar2ext4.MaximumDiskSize(maxVHDSize),
			}

			if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
				return errors.Wrapf(err, "failed seek start on temp file when processing layer %d", layerNumber)
			}
			if err := tmpFile.Truncate(0); err != nil {
				return errors.Wrapf(err, "failed truncate temp file when processing layer %d", layerNumber)
			}

			if err := tar2ext4.Convert(rc, tmpFile, opts...); err != nil {
				return errors.Wrap(err, "failed to convert tar to ext4")
			}

			data, err := ioutil.ReadFile(tmpFile.Name())
			if err != nil {
				return errors.Wrap(err, "failed to read temporary VHD file")
			}

			tree, err := dmverity.MerkleTree(data)
			if err != nil {
				return errors.Wrap(err, "failed to create merkle tree")
			}
			hash := dmverity.RootHash(tree)
			fmt.Fprintf(os.Stdout, "Layer %d\nroot hash: %x\n", layerNumber, hash)
		}
		return nil
	},
}
