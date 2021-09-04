package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

var configCommand = cli.Command{
	Name:  "config",
	Usage: "Information on the ncproxy config",
	Subcommands: []cli.Command{
		{
			Name:  "default",
			Usage: "Print the output of the default config to stdout or to a file",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "file",
					Usage: "Output config to a file",
				},
			},
			Action: func(context *cli.Context) error {
				file := context.String("file")

				configData, err := json.MarshalIndent(defaultConfig(), "", "  ")
				if err != nil {
					return errors.Wrap(err, "failed to marshal ncproxy config to json")
				}

				if file != "" {
					// Make the directory if it doesn't exist.
					if _, err := os.Stat(filepath.Dir(file)); err != nil {
						if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
							return errors.Wrap(err, "failed to make path to config file")
						}
					}
					if err := ioutil.WriteFile(
						file,
						[]byte(configData),
						0700,
					); err != nil {
						return err
					}
				} else {
					fmt.Fprint(os.Stdout, string(configData))
				}

				return nil
			},
		},
	},
}

// defaultConfig generates a default ncproxy configuration file with every config option filled in.
func defaultConfig() *config {
	return &config{
		TTRPCAddr:      "\\\\.\\pipe\\ncproxy-ttrpc",
		GRPCAddr:       "127.0.0.1:6669",
		NodeNetSvcAddr: "127.0.0.1:6668",
		Timeout:        10,
	}
}

type config struct {
	TTRPCAddr      string `json:"ttrpc,omitempty"`
	GRPCAddr       string `json:"grpc,omitempty"`
	NodeNetSvcAddr string `json:"node_net_svc_addr,omitempty"`
	// Timeout in seconds to wait to connect to a NodeNetworkService.
	// 0 represents no timeout and ncproxy will continuously try and connect in the
	// background.
	Timeout uint32 `json:"timeout,omitempty"`
}

// Returns config. If path is "" will check the default location of the config
// which is /path/of/executable/ncproxy.json
func loadConfig(path string) (*config, error) {
	// Check if config path was passed in.
	if path != "" {
		return readConfig(path)
	}
	// Otherwise check if config is in default location (same dir as executable)
	if path, exists := configPresent(); exists {
		return readConfig(path)
	}
	return nil, errors.New("no config specified and no config found in current directory. Run `ncproxy config default` to generate a default config")
}

// Reads config from path and returns config struct if path is valid and marshaling
// succeeds
func readConfig(path string) (*config, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read config file")
	}
	conf := &config{}
	if err := json.Unmarshal(data, conf); err != nil {
		return nil, errors.New("failed to unmarshal config data")
	}
	return conf, nil
}

// Checks to see if there is an ncproxy.json in the directory of the executable.
func configPresent() (string, bool) {
	path := filepath.Join(filepath.Dir(os.Args[0]), "ncproxy.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", false
	}
	return path, true
}
