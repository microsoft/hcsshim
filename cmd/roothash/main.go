package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/Microsoft/hcsshim/ext4/dmverity"
	"github.com/Microsoft/hcsshim/ext4/tar2ext4"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

var (
	image    = flag.String("i", "", "image")
	verbose  = flag.Bool("v", false, "verbose")
	username = flag.String("u", "", "username")
	password = flag.String("p", "", "password")
)

func main() {
	flag.Parse()
	if flag.NArg() != 0 || len(*image) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	err := func() (err error) {
		ref, err := name.ParseReference(*image)
		if err != nil {
			fmt.Printf("'%s' isn't a valid image name\n", *image)
			os.Exit(1)
		}

		var imageOptions []remote.Option
		if len(*username) != 0 && len(*password) != 0 {
			auth := authn.Basic{
				Username: *username,
				Password: *password}
			c, _ := auth.Authorization()
			authOption := remote.WithAuth(authn.FromConfig(*c))
			imageOptions = append(imageOptions, authOption)
		}

		img, err := remote.Image(ref, imageOptions...)
		if err != nil {
			fmt.Printf("Unable to fetch image '%s'\n", *image)
			fmt.Printf("%s\n", err.Error())
			os.Exit(1)
		}

		if *verbose {
			cn, _ := img.ConfigName()
			fmt.Printf("Image id: %s\n", cn.String())
		}

		layers, _ := img.Layers()
		if *verbose {
			fmt.Printf("%d layers found\n", len(layers))
		}
		for layer_number, layer := range layers {
			if *verbose {
				fmt.Printf("Layer %d ====>\n", layer_number)
				did, _ := layer.DiffID()
				fmt.Printf("Uncompressed layer hash: %s\n", did.String())
			}
			r, _ := layer.Uncompressed()

			out, err := ioutil.TempFile("", "")
			if err != nil {
				return err
			}

			var opts []tar2ext4.Option
			opts = append(opts, tar2ext4.ConvertWhiteout)
			opts = append(opts, tar2ext4.MaximumDiskSize(128*1024*1024*1024))

			err = tar2ext4.Convert(r, out, opts...)
			if err != nil {
				return err
			}

			data, err := ioutil.ReadFile(out.Name())
			if err != nil {
				return err
			}

			hash := dmverity.RootHash(dmverity.Tree(data))
			if *verbose {
				fmt.Print("Layer root hash: ")
			}
			fmt.Printf("%x\n", hash)

			os.Remove(out.Name())
		}

		return nil
	}()

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
