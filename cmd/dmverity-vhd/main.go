package main

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	"github.com/Microsoft/hcsshim/ext4/dmverity"
	"github.com/Microsoft/hcsshim/ext4/tar2ext4"
)

const usage = `dmverity-vhd is a command line tool for creating LCOW layer VHDs with dm-verity hashes.`

const (
	usernameFlag       = "username"
	passwordFlag       = "password"
	imageFlag          = "image"
	verboseFlag        = "verbose"
	outputDirFlag      = "out-dir"
	dockerFlag         = "docker"
	bufferedReaderFlag = "buffered-reader"
	tarballFlag        = "tarball"
	hashDeviceVhdFlag  = "hash-dev-vhd"
	maxVHDSize         = dmverity.RecommendedVHDSizeGB
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
		cli.BoolFlag{
			Name:  bufferedReaderFlag + ",b",
			Usage: "Optional: use buffered opener for image",
		},
	}

	if err := app.Run(os.Args); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type LayerProcessor func(string, io.Reader) error

func fetchImageTarball(tarballPath string) (imageReader io.ReadCloser, err error) {
	if imageReader, err = os.Open(tarballPath); err != nil {
		return nil, err
	}

	return imageReader, err
}

func fetchImageDocker(imageName string) (imageReader io.ReadCloser, err error) {

	dockerCtx := context.Background()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	imageReader, err = cli.ImageSave(dockerCtx, []string{imageName})
	if err != nil {
		return nil, err
	}

	return imageReader, err
}

func getlayerIds(manifestData []byte) (map[int]string, error) {
	type Manifest []struct {
		Layers []string `json:"Layers"`
	}

	var manifest Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, err
	}

	layerIds := make(map[int]string)
	for layerNumber, layerId := range manifest[0].Layers {
		layerIdSplit := strings.Split(layerId, ":")
		layerIds[layerNumber] = layerIdSplit[len(layerIdSplit)-1]
	}
	return layerIds, nil
}

func getLayerDigestsV24(manifestData []byte) (map[int]string, error) {
	type RootFs struct {
		DiffIDs []string `json:"diff_ids"`
	}
	type manifestLayerV24 struct {
		RootFS RootFs `json:"rootfs"`
	}

	var manifest manifestLayerV24
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, err
	}

	layerDigests := make(map[int]string)
	for layerNumber, layerId := range manifest.RootFS.DiffIDs {
		layerIdSplit := strings.Split(layerId, ":")
		layerDigests[layerNumber] = layerIdSplit[len(layerIdSplit)-1]
	}
	return layerDigests, nil
}

func getLayerDigestsV25(manifestData []byte) (map[int]string, error) {
	type manifestLayerV25 []struct {
		Layers []string `json:"Layers"`
	}

	var manifest manifestLayerV25
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, err
	}

	layerDigests := make(map[int]string)
	for layerNumber, layerId := range manifest[0].Layers {

		if !strings.HasPrefix(layerId, "blobs") {
			return nil, errors.New("layer path isn't v25")
		}

		layerIdSplit := strings.Split(layerId, "/")
		layerDigests[layerNumber] = layerIdSplit[len(layerIdSplit)-1]
	}
	return layerDigests, nil
}

func processLocalImageLayers(imageReader io.ReadCloser, onLayer LayerProcessor) (layerDigests map[int]string, layerIds map[int]string, err error) {

	imageFileReader := tar.NewReader(imageReader)
	layerDigests = make(map[int]string)
	for {
		hdr, err := imageFileReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, err
		}

		// If the file is a layer, call the callback
		if (strings.HasPrefix(hdr.Name, "blobs/sha256/") && hdr.Name != "blobs/sha256/") || strings.HasSuffix(hdr.Name, ".tar") {
			onLayer(hdr.Name, imageFileReader)
		}

		// If the file is the manifest, link layer numbers to layer paths
		if strings.HasSuffix(hdr.Name, ".json") {

			manifestData, err := io.ReadAll(imageFileReader)
			if err != nil {
				return nil, nil, err
			}

			layerIds, _ = getlayerIds(manifestData)

			layerDigestsV25, err := getLayerDigestsV25(manifestData)
			if err == nil {
				layerDigests = layerDigestsV25
				continue
			}

			layerDigestsV24, err := getLayerDigestsV24(manifestData)
			if err == nil {
				layerDigests = layerDigestsV24
				continue
			}

		}
	}

	return layerDigests, layerIds, nil
}

func processRemoteImageLayers(imageName string, username string, password string, onLayer LayerProcessor) (layerDigests map[int]string, layerIds map[int]string, err error) {

	layerDigests = make(map[int]string)

	ref, err := name.ParseReference(imageName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse image reference %s: %w", imageName, err)
	}

	var remoteOpts []remote.Option
	if username != "" && password != "" {

		auth := authn.Basic{
			Username: username,
			Password: password,
		}

		authConf, err := auth.Authorization()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to set remote: %w", err)
		}

		log.Debug("using basic auth")
		authOpt := remote.WithAuth(authn.FromConfig(*authConf))
		remoteOpts = append(remoteOpts, authOpt)
	}

	image, err := remote.Image(ref, remoteOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to fetch image %q, make sure it exists: %w", imageName, err)
	}

	layers, err := image.Layers()
	if err != nil {
		return nil, nil, fmt.Errorf("unable to fetch image layers: %w", err)
	}

	for layerNumber, layer := range layers {
		diffID, err := layer.DiffID()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read layer diff: %w", err)
		}

		layerDigests[layerNumber] = diffID.String()
		layerReader, err := layer.Uncompressed()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to uncompress layer %s: %w", diffID.String(), err)
		}
		defer layerReader.Close()

		onLayer(diffID.String(), layerReader)
	}

	// For the remote case, use digests for both layer ID and layer digest
	return layerDigests, layerDigests, nil
}

func processImageLayers(ctx *cli.Context, onLayer LayerProcessor) (layerDigests map[int]string, layerIds map[int]string, err error) {
	imageName := ctx.String(imageFlag)
	tarballPath := ctx.GlobalString(tarballFlag)
	useDocker := ctx.GlobalBool(dockerFlag)

	if useDocker && tarballPath != "" {
		return nil, nil, errors.New("cannot use both docker and tarball for image source")
	}

	processLocal := func(fetcher func(string) (io.ReadCloser, error), image string) (map[int]string, map[int]string, error) {
		imageReader, err := fetcher(image)
		if err != nil {
			return nil, nil, err
		}
		defer imageReader.Close()
		return processLocalImageLayers(imageReader, onLayer)
	}

	if tarballPath != "" {
		return processLocal(fetchImageTarball, tarballPath)
	} else if useDocker {
		return processLocal(fetchImageDocker, imageName)
	} else {
		return processRemoteImageLayers(
			imageName,
			ctx.String(usernameFlag),
			ctx.String(passwordFlag),
			onLayer,
		)
	}
}

func moveFile(src string, dst string) error {
	err := os.Rename(src, dst)

	// If a simple rename didn't work, for example moving to or from a mount,
	// then copy and delete the file
	if err != nil {
		sourceFile, err := os.Open(src)
		if err != nil {
			return err
		}
		defer sourceFile.Close()

		destFile, err := os.Create(dst)
		if err != nil {
			return err
		}
		defer destFile.Close()

		if _, err = io.Copy(destFile, sourceFile); err != nil {
			return err
		}
		sourceFile.Close()

		if err = os.Remove(src); err != nil {
			return err
		}
	}

	return nil
}

func sanitiseVHDFilename(vhdFilename string) string {
	return strings.TrimSuffix(
		strings.ReplaceAll(vhdFilename, "/", "_"),
		".tar",
	)
}

func createVHD(layerId string, layerReader io.Reader, verityHashDev bool, outDir string) error {
	sanitisedFileName := sanitiseVHDFilename(layerId)

	// Create this file in a temp directory because at this point we don't have
	// the layer digest to properly name the file, it will be moved later
	vhdPath := filepath.Join(os.TempDir(), sanitisedFileName+".vhd")

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

	if err := tar2ext4.Convert(layerReader, out, opts...); err != nil {
		return fmt.Errorf("failed to convert tar to ext4: %w", err)
	}

	if verityHashDev {

		hashDevPath := filepath.Join(outDir, sanitisedFileName+".hash-dev.vhd")

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
		verityHashDev := ctx.Bool(hashDeviceVhdFlag)

		outDir := ctx.String(outputDirFlag)
		if _, err := os.Stat(outDir); os.IsNotExist(err) {
			log.Debugf("creating output directory %q", outDir)
			if err := os.MkdirAll(outDir, 0755); err != nil {
				return fmt.Errorf("failed to create output directory %s: %w", outDir, err)
			}
		}

		createVHDLayer := func(layerId string, layerReader io.Reader) error {
			return createVHD(layerId, layerReader, verityHashDev, outDir)
		}

		log.Debug("creating layer VHDs with dm-verity")
		layerDigests, layerIds, err := processImageLayers(ctx, createVHDLayer)
		if err != nil {
			return err
		}

		// Move the VHDs to the output directory
		// They can't immediately be in the output directory because they have
		// temporary file names based on the layer id which isn't necessarily
		// the layer digest
		for layerNumber := 0; layerNumber < len(layerDigests); layerNumber++ {
			layerDigest := layerDigests[layerNumber]
			layerId := layerIds[layerNumber]
			sanitisedFileName := sanitiseVHDFilename(layerId)

			suffixes := []string{".vhd"}
			if verityHashDev {
				suffixes = append(suffixes, ".hash-dev.vhd")
			}

			for _, src_suffix := range suffixes {
				src := filepath.Join(os.TempDir(), sanitisedFileName+src_suffix)
				if _, err := os.Stat(src); os.IsNotExist(err) {
					return fmt.Errorf("layer VHD %s does not exist", src)
				}

				dst := filepath.Join(outDir, layerDigest+src_suffix)
				if err := moveFile(src, dst); err != nil {
					return err
				}

				fmt.Fprintf(os.Stdout, "Layer VHD moved from %s to %s\n", src, dst)
			}

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

		layerHashes := make(map[string]string)
		getLayerHash := func(layerDigest string, layerReader io.Reader) error {
			hash, err := tar2ext4.ConvertAndComputeRootDigest(layerReader)
			if err != nil {
				return err
			}
			layerHashes[layerDigest] = hash
			return nil
		}

		_, layerIds, err := processImageLayers(ctx, getLayerHash)
		if err != nil {
			return err
		}

		// Print the layer number to layer hash
		for layerNumber := 0; layerNumber < len(layerIds); layerNumber++ {
			fmt.Fprintf(os.Stdout, "Layer %d root hash: %s\n", layerNumber, layerHashes[layerIds[layerNumber]])
		}

		return nil
	},
}
