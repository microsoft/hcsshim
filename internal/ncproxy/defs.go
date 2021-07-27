package ncproxy

import (
	"encoding/json"
)

type HcnNetworkMode int32

const (
	HcnNetworkModeTransparent HcnNetworkMode = iota
)

type HcnNetworkIpamType int32

const (
	HcnIpamTypeStatic HcnNetworkIpamType = iota
	HcnIpamTypeDHCP
)

type NetworkSettings interface{}

var _ = (NetworkSettings)(&HcnSettings{})
var _ = (NetworkSettings)(&CustomNetworkSettings{})

type HcnSettings struct {
	Mode                  HcnNetworkMode
	SwitchName            string
	IpamType              HcnNetworkIpamType
	SubnetIpaddressPrefix []string
	DefaultGateway        string
}

type CustomNetworkSettings struct{}

type NetworkType int32

const (
	HcnNetworkType    NetworkType = 0
	CustomNetworkType NetworkType = 1
)

type RawNetwork struct {
	Type     NetworkType
	Settings json.RawMessage
}
