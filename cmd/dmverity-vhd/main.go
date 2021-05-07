package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Microsoft/hcsshim/ext4/tar2ext4"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

var (
	image    = flag.String("i", "", "image")
	output   = flag.String("o", "", "output")
	verbose  = flag.Bool("v", false, "verbose")
	username = flag.String("u", "", "username")
	password = flag.String("p", "", "password")
)

func main() {
	flag.Parse()
	if flag.NArg() != 0 || len(*image) == 0 || len(*output) == 0 {
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

		if _, err := os.Stat(*output); os.IsNotExist(err) {
			err = os.Mkdir(*output, 0755)
			if err != nil {
				fmt.Printf("Unable to create directory '%s'\n", *output)
				os.Exit(1)
			}
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
			did, _ := layer.DiffID()

			if *verbose {
				fmt.Printf("Layer %d ====>\n", layer_number)
				fmt.Printf("Uncompressed layer hash: %s\n", did.String())
			}
			r, _ := layer.Uncompressed()

			path := fmt.Sprintf("%s/%s.vhd", *output, did.Hex)

			out, err := os.Create(path)
			if err != nil {
				return err
			}

			var opts []tar2ext4.Option
			opts = append(opts, tar2ext4.ConvertWhiteout)
			opts = append(opts, tar2ext4.MaximumDiskSize(128*1024*1024*1024))
			opts = append(opts, tar2ext4.AppendVhdFooter)
			opts = append(opts, tar2ext4.AppendDMVerity)

			err = tar2ext4.Convert(r, out, opts...)
			if err != nil {
				return err
			}

			fmt.Printf("%s\n", out.Name())
		}

		return nil
	}()

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
