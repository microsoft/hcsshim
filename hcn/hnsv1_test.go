//go:build windows && integration
// +build windows,integration

package hcn

import (
	"os"
	"testing"

	"github.com/Microsoft/hcsshim"
)

const (
	NatTestNetworkName     string = "GoTestNat"
	NatTestEndpointName    string = "GoTestNatEndpoint"
	OverlayTestNetworkName string = "GoTestOverlay"
	BridgeTestNetworkName  string = "GoTestL2Bridge"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func CreateTestNetwork() (*hcsshim.HNSNetwork, error) {
	network := &hcsshim.HNSNetwork{
		Type: "NAT",
		Name: NatTestNetworkName,
		Subnets: []hcsshim.Subnet{
			{
				AddressPrefix:  "192.168.100.0/24",
				GatewayAddress: "192.168.100.1",
			},
		},
	}

	return network.Create()
}

func TestEndpoint(t *testing.T) {
	network, err := CreateTestNetwork()
	if err != nil {
		t.Fatal(err)
	}

	Endpoint := &hcsshim.HNSEndpoint{
		Name: NatTestEndpointName,
	}

	Endpoint, err = network.CreateEndpoint(Endpoint)
	if err != nil {
		t.Fatal(err)
	}

	err = Endpoint.HostAttach(1)
	if err != nil {
		t.Fatal(err)
	}

	err = Endpoint.HostDetach()
	if err != nil {
		t.Fatal(err)
	}

	_, err = Endpoint.Delete()
	if err != nil {
		t.Fatal(err)
	}

	_, err = network.Delete()
	if err != nil {
		t.Fatal(err)
	}
}

func TestEndpointGetAll(t *testing.T) {
	_, err := hcsshim.HNSListEndpointRequest()
	if err != nil {
		t.Fatal(err)
	}
}

func TestEndpointStatsAll(t *testing.T) {
	network, err := CreateTestNetwork()
	if err != nil {
		t.Fatal(err)
	}

	Endpoint := &hcsshim.HNSEndpoint{
		Name: NatTestEndpointName,
	}

	_, err = network.CreateEndpoint(Endpoint)
	if err != nil {
		t.Fatal(err)
	}

	epList, err := hcsshim.HNSListEndpointRequest()
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range epList {
		_, err := hcsshim.GetHNSEndpointStats(e.Id)
		if err != nil {
			t.Fatal(err)
		}
	}

	_, err = network.Delete()
	if err != nil {
		t.Fatal(err)
	}
}

func TestNetworkGetAll(t *testing.T) {
	_, err := hcsshim.HNSListNetworkRequest("GET", "", "")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNetwork(t *testing.T) {
	network, err := CreateTestNetwork()
	if err != nil {
		t.Fatal(err)
	}
	_, err = network.Delete()
	if err != nil {
		t.Fatal(err)
	}
}

func TestAccelnetNnvManagementMacAddresses(t *testing.T) {
	network, err := CreateTestNetwork()
	if err != nil {
		t.Fatal(err)
	}

	macList := []string{"00-15-5D-0A-B7-C6", "00-15-5D-38-01-00"}
	newMacList, err := hcsshim.SetNnvManagementMacAddresses(macList)

	if err != nil {
		t.Fatal(err)
	}

	if len(newMacList.MacAddressList) != 2 {
		t.Errorf("After Create: Expected macaddress count %d, got %d", 2, len(newMacList.MacAddressList))
	}

	newMacList, err = hcsshim.GetNnvManagementMacAddresses()
	if err != nil {
		t.Fatal(err)
	}

	if len(newMacList.MacAddressList) != 2 {
		t.Errorf("Get After Create: Expected macaddress count %d, got %d", 2, len(newMacList.MacAddressList))
	}

	newMacList, err = hcsshim.DeleteNnvManagementMacAddresses()
	if err != nil {
		t.Fatal(err)
	}

	if len(newMacList.MacAddressList) != 0 {
		t.Errorf("After Delete: Expected macaddress count %d, got %d", 0, len(newMacList.MacAddressList))
	}

	newMacList, err = hcsshim.GetNnvManagementMacAddresses()
	if err != nil {
		t.Fatal(err)
	}

	if len(newMacList.MacAddressList) != 0 {
		t.Errorf("Get After Delete: Expected macaddress count %d, got %d", 0, len(newMacList.MacAddressList))
	}

	_, err = network.Delete()
	if err != nil {
		t.Fatal(err)
	}
}
