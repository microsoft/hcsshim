//go:build windows && integration
// +build windows,integration

package hcn

import (
	"encoding/json"
	"github.com/Microsoft/hcsshim/hns"
	"testing"
)

func TestV1Network(t *testing.T) {
	cleanup(NatTestNetworkName)

	v1network := hns.HNSNetwork{
		Type: "NAT",
		Name: NatTestNetworkName,
		MacPools: []hns.MacPool{
			{
				StartMacAddress: "00-15-5D-52-C0-00",
				EndMacAddress:   "00-15-5D-52-CF-FF",
			},
		},
		Subnets: []hns.Subnet{
			{
				AddressPrefix:  "192.168.100.0/24",
				GatewayAddress: "192.168.100.1",
			},
		},
	}

	jsonString, err := json.Marshal(v1network)
	if err != nil {
		t.Fatal(err)
		t.Fail()
	}

	network, err := createNetwork(string(jsonString))
	if err != nil {
		t.Fatal(err)
		t.Fail()
	}

	err = network.Delete()
	if err != nil {
		t.Fatal(err)
		t.Fail()
	}
}

func TestV1Endpoint(t *testing.T) {
	cleanup(NatTestNetworkName)

	v1network := hns.HNSNetwork{
		Type: "NAT",
		Name: NatTestNetworkName,
		MacPools: []hns.MacPool{
			{
				StartMacAddress: "00-15-5D-52-C0-00",
				EndMacAddress:   "00-15-5D-52-CF-FF",
			},
		},
		Subnets: []hns.Subnet{
			{
				AddressPrefix:  "192.168.100.0/24",
				GatewayAddress: "192.168.100.1",
			},
		},
	}

	jsonString, err := json.Marshal(v1network)
	if err != nil {
		t.Fatal(err)
		t.Fail()
	}

	network, err := createNetwork(string(jsonString))
	if err != nil {
		t.Fatal(err)
		t.Fail()
	}

	v1endpoint := hns.HNSEndpoint{
		Name:           NatTestEndpointName,
		VirtualNetwork: network.Id,
	}

	jsonString, err = json.Marshal(v1endpoint)
	if err != nil {
		t.Fatal(err)
		t.Fail()
	}

	endpoint, err := createEndpoint(network.Id, string(jsonString))
	if err != nil {
		t.Fatal(err)
		t.Fail()
	}

	err = endpoint.Delete()
	if err != nil {
		t.Fatal(err)
		t.Fail()
	}

	err = network.Delete()
	if err != nil {
		t.Fatal(err)
		t.Fail()
	}
}
