package main

import (
	"encoding/json"
	"io/ioutil"

	ncproxygrpc "github.com/Microsoft/hcsshim/pkg/ncproxy/ncproxygrpc/v1"
	"github.com/pkg/errors"
)

type service struct {
	conf                 *config
	client               ncproxygrpc.NetworkConfigProxyClient
	containerToNamespace map[string]string
	endpointToNicID      map[string]string
	containerToNetwork   map[string][]string
}

type hnsSettings struct {
	SwitchName  string                                `json:"switch_name,omitempty"`
	IOVSettings *ncproxygrpc.IovEndpointPolicySetting `json:"iov_settings,omitempty"`
}

type ncproxynetworkingSettings struct {
	DeviceID             string `json:"device_id,omitempty"`
	VirtualFunctionIndex uint32 `json:"virtual_function_index,omitempty"`
}

type networkingSettings struct {
	HNSSettings               *hnsSettings               `json:"hns_settings,omitempty"`
	NCProxyNetworkingSettings *ncproxynetworkingSettings `json:"ncproxy_networking_settings,omitempty"`
}

type config struct {
	TTRPCAddr      string `json:"ttrpc,omitempty"`
	GRPCAddr       string `json:"grpc,omitempty"`
	NodeNetSvcAddr string `json:"node_net_svc_addr,omitempty"`
	// 0 represents no timeout and networkagent will continuously try and connect in the
	// background.
	Timeout            uint32              `json:"timeout,omitempty"`
	NetworkingSettings *networkingSettings `json:"networking_settings,omitempty"`
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
