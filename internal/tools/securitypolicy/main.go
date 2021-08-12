package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/Microsoft/hcsshim/ext4/dmverity"
	"github.com/Microsoft/hcsshim/ext4/tar2ext4"
	sp "github.com/Microsoft/hcsshim/pkg/securitypolicy"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

var (
	configFile = flag.String("c", "", "config")
	outputJson = flag.Bool("j", false, "json")
	username    = flag.String("u", "", "username")
	password    = flag.String("p", "", "password")
)

func main() {
	flag.Parse()
	if flag.NArg() != 0 || len(*configFile) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	err := func() (err error) {
		configData, err := ioutil.ReadFile(*configFile)
		if err != nil {
			return err
		}

		config := &Config{
			AllowAll: false,
			Images:   []Image{},
		}

		err = toml.Unmarshal(configData, config)
		if err != nil {
			return err
		}

		policy, err := func() (sp.SecurityPolicy, error) {
			if config.AllowAll {
				return createOpenDoorPolicy(), nil
			} else {
				return createPolicyFromConfig(*config)
			}
		}()

		if err != nil {
			return err
		}

		j, err := json.Marshal(policy)
		if err != nil {
			return err
		}
		if *outputJson {
			fmt.Printf("%s\n", j)
		}
		b := base64.StdEncoding.EncodeToString(j)
		fmt.Printf("%s\n", b)

		return nil
	}()

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type Image struct {
	Name    string `toml:"name"`
	Command string `toml:"command"`
}

type Config struct {
	AllowAll bool    `toml:"allow_all"`
	Images   []Image `toml:"image"`
}

func createOpenDoorPolicy() sp.SecurityPolicy {
	return sp.SecurityPolicy{
		AllowAll: true,
	}
}

func createPolicyFromConfig(config Config) (sp.SecurityPolicy, error) {
	p := sp.SecurityPolicy{}

	// for now, we hardcode the pause container version 3.1 here
	// in a final end user tool, we would not do it this way.
	// as this is a tool for use by developers currently working
	// on security policy implementation code
	pausec := sp.SecurityPolicyContainer{
		Command: "/pause",
		Layers:  []string{"16b514057a06ad665f92c02863aca074fd5976c755d26bff16365299169e8415"},
	}
	p.Containers = append(p.Containers, pausec)

	var imageOptions []remote.Option
	if len(*username) != 0 && len(*password) != 0 {
		auth := authn.Basic{
			Username: *username,
			Password: *password}
		c, _ := auth.Authorization()
		authOption := remote.WithAuth(authn.FromConfig(*c))
		imageOptions = append(imageOptions, authOption)
	}

	for _, image := range config.Images {
		container := sp.SecurityPolicyContainer{
			Command: image.Command,
			Layers:  []string{},
		}
		ref, err := name.ParseReference(image.Name)
		if err != nil {
			return p, fmt.Errorf("'%s' isn't a valid image name\n", image.Name)
		}
		img, err := remote.Image(ref, imageOptions...)
		if err != nil {
			return p, fmt.Errorf("unable to fetch image '%s': %s", image.Name, err.Error())
		}

		layers, err := img.Layers()
		if err != nil {
			return p, err
		}

		for _, layer := range layers {
			r, err := layer.Uncompressed()
			if err != nil {
				return p, err
			}

			out, err := ioutil.TempFile("", "")
			if err != nil {
				return p, err
			}
			defer os.Remove(out.Name())

			opts := []tar2ext4.Option{
				tar2ext4.ConvertWhiteout,
				tar2ext4.MaximumDiskSize(128 * 1024 * 1024 * 1024),
			}

			err = tar2ext4.Convert(r, out, opts...)
			if err != nil {
				return p, err
			}

			data, err := ioutil.ReadFile(out.Name())
			if err != nil {
				return p, err
			}

			tree, err := dmverity.MerkleTree(data)
			if err != nil {
				return p, err
			}
			hash := dmverity.RootHash(tree)
			hash_string := fmt.Sprintf("%x", hash)
			container.Layers = append(container.Layers, hash_string)
		}

		p.Containers = append(p.Containers, container)
	}

	return p, nil
}
