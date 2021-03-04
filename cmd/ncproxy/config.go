package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

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
	return nil, errors.New("no config specified and no default config found")
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
