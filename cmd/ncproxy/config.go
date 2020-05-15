package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/Microsoft/hcsshim/cmd/ncproxy/configagent"

	"google.golang.org/grpc"
)

type config struct {
	TTRPCAddr string    `json:"ttrpc,omitempty"`
	GRPCAddr  string    `json:"grpc,omitempty"`
	Networks  []network `json:"networks,omitempty"`
	// Timeout in seconds to wait to connect to a NetworkConfigAgent
	Timeout int `json:"timeout,omitempty"`
}

type network struct {
	// Address of the network configuration service that
	// manages the set of networks.
	Address string `json:"address,omitempty"`

	// Name is the HNS network name
	Name string `json:"name,omitempty"`
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
	return nil, fmt.Errorf("no config specified and no default config found")
}

// Reads config from path and returns config struct if path is valid and marshaling
// succeeds
func readConfig(path string) (*config, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %s", err)
	}
	conf := &config{}
	if err := json.Unmarshal(data, conf); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config data")
	}
	return conf, nil
}

// Checks to see if there is an ncproxy.json in the directory of the executable.
func configPresent() (string, bool) {
	path, err := os.Executable()
	if err != nil {
		return "", false
	}
	path = filepath.Join(filepath.Dir(path), "ncproxy.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", false
	}
	return path, true
}

// Parses networks in config and allocates grpc clients to the global mapping.
func configToClients(ctx context.Context, conf *config) error {
	for _, network := range conf.Networks {
		client, err := grpc.Dial(network.Address, grpc.WithInsecure())
		if err != nil {
			return fmt.Errorf("failed to connect to network configuration agent: %s", err)
		}
		networkToConfigAgent[network.Name] = configagent.NewNetworkConfigAgentClient(client)
	}
	return nil
}
