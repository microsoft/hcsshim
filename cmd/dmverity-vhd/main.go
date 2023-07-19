package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/daemon"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	"github.com/Microsoft/hcsshim/ext4/dmverity"
	"github.com/Microsoft/hcsshim/ext4/tar2ext4"
)

const usage = `dmverity-vhd is a command line tool for creating LCOW layer VHDs with dm-verity hashes.`

const (
	usernameFlag      = "username"
	passwordFlag      = "password"
	imageFlag         = "image"
	verboseFlag       = "verbose"
	outputDirFlag     = "out-dir"
	dockerFlag        = "docker"
	tarballFlag       = "tarball"
	hashDeviceVhdFlag = "hash-dev-vhd"
	maxVHDSize        = dmverity.RecommendedVHDSizeGB
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
		cli.BoolFlag{
			Name:  dockerFlag + ",d",
			Usage: "Optional: use local docker daemon",
		},
		cli.StringFlag{
			Name:  tarballFlag + ",t",
			Usage: "Optional: path to tarball containing image info",
		},
	}

	if err := app.Run(os.Args); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func fetchImageLayers(ctx *cli.Context) (layers []v1.Layer, err error) {
	image := ctx.String(imageFlag)
	tarballPath := ctx.GlobalString(tarballFlag)
	ref, err := name.ParseReference(image)
	if err != nil {
		return nil, fmt.Errorf("failed to parse image reference %s: %w", image, err)
	}

	dockerDaemon := ctx.GlobalBool(dockerFlag)

	// error check to make sure docker and tarball are not both defined
	if dockerDaemon && tarballPath != "" {
		return nil, errors.New("cannot use both docker and tarball for image source")
	}

	// by default, using remote as source
	var img v1.Image
	if tarballPath != "" {
		// create a tag and search the tarball for the image specified
		var imageNameAndTag name.Tag
		imageNameAndTag, err = name.NewTag(image)
		if err != nil {
			return nil, fmt.Errorf("failed to failed to create a tag to search tarball for %s: %w", image, err)
		}
		// if only an image name is provided and not a tag, the default is "latest"
		img, err = tarball.ImageFromPath(tarballPath, &imageNameAndTag)
	} else if dockerDaemon {
		img, err = daemon.Image(ref)
	} else {
		var remoteOpts []remote.Option
		if ctx.IsSet(usernameFlag) && ctx.IsSet(passwordFlag) {
			auth := authn.Basic{
				Username: ctx.String(usernameFlag),
				Password: ctx.String(passwordFlag),
			}
			authConf, err := auth.Authorization()
			if err != nil {
				return nil, fmt.Errorf("failed to set remote: %w", err)
			}
			log.Debug("using basic auth")
			authOpt := remote.WithAuth(authn.FromConfig(*authConf))
			remoteOpts = append(remoteOpts, authOpt)
		}

		img, err = remote.Image(ref, remoteOpts...)
	}
	if err != nil {
		return nil, fmt.Errorf("unable to fetch image %q, make sure it exists: %w", image, err)
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
		cli.BoolFlag{
			Name:  hashDeviceVhdFlag + ",hdv",
			Usage: "Optional: save hash-device as a VHD",
		},
	},
	Action: func(ctx *cli.Context) error {
		verbose := ctx.GlobalBool(verboseFlag)
		if verbose {
			log.SetLevel(log.DebugLevel)
		}

		layers, err := fetchImageLayers(ctx)
		if err != nil {
			return fmt.Errorf("failed to fetch image layers: %w", err)
		}

		outDir := ctx.String(outputDirFlag)
		if _, err := os.Stat(outDir); os.IsNotExist(err) {
			log.Debugf("creating output directory %q", outDir)
			if err := os.MkdirAll(outDir, 0755); err != nil {
				return fmt.Errorf("failed to create output directory %s: %w", outDir, err)
			}
		}

		log.Debug("creating layer VHDs with dm-verity")
		for layerNumber, layer := range layers {
			diff, err := layer.DiffID()
			if err != nil {
				return fmt.Errorf("failed to read layer diff: %w", err)
			}
			log.WithFields(log.Fields{
				"layerNumber": layerNumber,
				"layerDiff":   diff.String(),
			}).Debug("converting tar to layer VHD")
			if err := createVHD(layer, ctx.String(outputDirFlag), ctx.Bool(hashDeviceVhdFlag)); err != nil {
				return err
			}
		}
		return nil
	},
}

func createVHD(layer v1.Layer, outDir string, verityHashDev bool) error {
	diffID, err := layer.DiffID()
	if err != nil {
		return fmt.Errorf("failed to read layer diff: %w", err)
	}

	rc, err := layer.Uncompressed()
	if err != nil {
		return fmt.Errorf("failed to uncompress layer %s: %w", diffID.String(), err)
	}
	defer rc.Close()

	vhdPath := filepath.Join(outDir, diffID.Hex+".vhd")
	out, err := os.Create(vhdPath)
	if err != nil {
		return fmt.Errorf("failed to create layer vhd file %s: %w", vhdPath, err)
	}
	defer out.Close()

	opts := []tar2ext4.Option{
		tar2ext4.ConvertWhiteout,
		tar2ext4.MaximumDiskSize(maxVHDSize),
	}
	if !verityHashDev {
		opts = append(opts, tar2ext4.AppendDMVerity)
	}
	if err := tar2ext4.Convert(rc, out, opts...); err != nil {
		return fmt.Errorf("failed to convert tar to ext4: %w", err)
	}
	if verityHashDev {
		hashDevPath := filepath.Join(outDir, diffID.Hex+".hash-dev.vhd")
		hashDev, err := os.Create(hashDevPath)
		if err != nil {
			return fmt.Errorf("failed to create hash device VHD file: %w", err)
		}
		defer hashDev.Close()

		if err := dmverity.ComputeAndWriteHashDevice(out, hashDev); err != nil {
			return err
		}
		if err := tar2ext4.ConvertToVhd(hashDev); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "hash device created at %s\n", hashDevPath)
	}
	if err := tar2ext4.ConvertToVhd(out); err != nil {
		return fmt.Errorf("failed to append VHD footer: %w", err)
	}
	fmt.Fprintf(os.Stdout, "Layer VHD created at %s\n", vhdPath)
	return nil
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
			return fmt.Errorf("failed to fetch image layers: %w", err)
		}
		log.Debugf("%d layers found", len(layers))

		convertFunc := func(layer v1.Layer) (string, error) {
			rc, err := layer.Uncompressed()
			if err != nil {
				return "", err
			}
			defer rc.Close()

			hash, err := tar2ext4.ConvertAndComputeRootDigest(rc)
			if err != nil {
				return "", err
			}
			return hash, err
		}

		for layerNumber, layer := range layers {
			diffID, err := layer.DiffID()
			if err != nil {
				return fmt.Errorf("failed to read layer diff: %w", err)
			}
			log.WithFields(log.Fields{
				"layerNumber": layerNumber,
				"layerDiff":   diffID.String(),
			}).Debug("uncompressed layer")

			hash, err := convertFunc(layer)
			if err != nil {
				return fmt.Errorf("failed to compute root digest: %w", err)
			}
			fmt.Fprintf(os.Stdout, "Layer %d root hash: %s\n", layerNumber, hash)
		}
		return nil
	},
}
