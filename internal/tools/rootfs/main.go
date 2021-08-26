package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/Microsoft/hcsshim/ext4/dmverity"
	"github.com/Microsoft/hcsshim/ext4/tar2ext4"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/daemon"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

func main() {
	image := flag.String("i", "", "image")
	tag := flag.String("t", "latest", "tag")
	destination := flag.String("d", "local", "destination")

	flag.Parse()
	if flag.NArg() != 0 || len(*image) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	layers, err := getRootfsLayerHashes(*image, *tag, *destination)

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	for i, layer := range layers {
		fmt.Printf("[%d]: %s\n", i, layer)
	}
}

func getRootfsLayerHashes(image_name string, image_tag string, destination string) ([]string, error) {
	hashes := []string{}
	image_string := image_name + ":" + image_tag

	if len(image_name) == 0 || len(image_tag) == 0 {
		return hashes, fmt.Errorf("'%s' is not a valid image name", image_string)
	}

	validDestinations := map[string]bool{
		"local":  true,
		"remote": true,
	}
	if !validDestinations[destination] {
		fmt.Print("Destination value should fall in following values:[")
		for k := range validDestinations {
			fmt.Print(k + ",")
		}
		fmt.Println("]")
		return hashes, errors.New("invalid destination")
	}

	ref, err := name.ParseReference(image_string)
	if err != nil {
		return hashes, fmt.Errorf("'%s' is not a valid image name", image_string)
	}

	// by default, using local as destination
	var localImageOption []daemon.Option
	image, err := daemon.Image(ref, localImageOption...)
	if strings.ToLower(destination) == "remote" {
		var imageOptions []remote.Option
		image, err = remote.Image(ref, imageOptions...)
	}
	if err != nil {
		return hashes, fmt.Errorf("unable to fetch image '%s': %s", image_string, err.Error())
	}

	layers, err := image.Layers()
	if err != nil {
		return hashes, err
	}

	for _, layer := range layers {
		raw_hash, err := getRoothash(layer)
		if err != nil {
			return hashes, err
		}

		hash_string := fmt.Sprintf("%x", raw_hash)
		hashes = append(hashes, hash_string)
	}

	return hashes, nil
}

func getRoothash(layer v1.Layer) ([]byte, error) {
	var hash []byte
	r, err := layer.Uncompressed()
	if err != nil {
		return hash, err
	}

	out, err := ioutil.TempFile("", "")
	if err != nil {
		return hash, err
	}
	defer os.Remove(out.Name())

	opts := []tar2ext4.Option{
		tar2ext4.ConvertWhiteout,
		tar2ext4.MaximumDiskSize(128 * 1024 * 1024 * 1024),
	}

	err = tar2ext4.Convert(r, out, opts...)
	if err != nil {
		return hash, err
	}

	data, err := ioutil.ReadFile(out.Name())
	if err != nil {
		return hash, err
	}

	tree, err := dmverity.MerkleTree(data)
	if err != nil {
		return hash, err
	}
	hash = dmverity.RootHash(tree)

	return hash, nil
}
