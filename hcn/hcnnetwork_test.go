// +build integration

package hcn

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

type HcnNetworkMakerFunc func() (*HostComputeNetwork, error)

func TestCreateDeleteNetworks(t *testing.T) {
	var netMaker HcnNetworkMakerFunc
	netMaker = HcnCreateTestNATNetwork
	err := CreateDeleteNetworksHelper(t, netMaker)
	if err != nil {
		t.Fatal(err)
	}
	err = CreateDeleteNetworksHelper(t, func() (*HostComputeNetwork, error) { return HcnCreateTestNATNetworkWithSubnet(nil) })
	if err != nil {
		t.Fatal(err)
	}

	snet1 := CreateSubnet("192.168.100.0/24", "192.168.100.1", "1.1.1.1/0")
	err = CreateDeleteNetworksHelper(t, func() (*HostComputeNetwork, error) { return HcnCreateTestNATNetworkWithSubnet(snet1) })
	if err == nil {
		t.Fatal(errors.New("expected failure for subnet with no default gateway provided"))
	}
	snet2 := CreateSubnet("192.168.100.0/24", "", "")
	err = CreateDeleteNetworksHelper(t, func() (*HostComputeNetwork, error) { return HcnCreateTestNATNetworkWithSubnet(snet2) })
	if err == nil {
		t.Fatal(errors.New("expected failure for subnet with no nexthop provided but a gateway provided"))
	}
}

func CreateDeleteNetworksHelper(t *testing.T, networkFunction HcnNetworkMakerFunc) error {
	network, err := networkFunction()
	if err != nil {
		return err
	}
	jsonString, err := json.Marshal(network)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("Network JSON:\n%s \n", jsonString)
	err = network.Delete()
	if err != nil {
		return err
	}
	return nil
}

func TestGetNetworkByName(t *testing.T) {
	network, err := HcnCreateTestNATNetwork()
	if err != nil {
		t.Fatal(err)
	}
	network, err = GetNetworkByName(network.Name)
	if err != nil {
		t.Fatal(err)
	}
	if network == nil {
		t.Fatal("No Network found")
	}
	err = network.Delete()
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetNetworkById(t *testing.T) {
	network, err := HcnCreateTestNATNetwork()
	if err != nil {
		t.Fatal(err)
	}
	network, err = GetNetworkByID(network.Id)
	if err != nil {
		t.Fatal(err)
	}
	if network == nil {
		t.Fatal("No Network found")
	}
	err = network.Delete()
	if err != nil {
		t.Fatal(err)
	}
}

func TestListNetwork(t *testing.T) {
	_, err := ListNetworks()
	if err != nil {
		t.Fatal(err)
	}
}

func testNetworkPolicy(t *testing.T, policiesToTest *PolicyNetworkRequest) {
	network, err := CreateTestOverlayNetwork()
	if err != nil {
		t.Fatal(err)
	}

	network.AddPolicy(*policiesToTest)

	//Reload the network object from HNS.
	network, err = GetNetworkByID(network.Id)
	if err != nil {
		t.Fatal(err)
	}

	for _, policyToTest := range policiesToTest.Policies {
		foundPolicy := false
		for _, policy := range network.Policies {
			if policy.Type == policyToTest.Type {
				foundPolicy = true
				break
			}
		}
		if !foundPolicy {
			t.Fatalf("Could not find %s policy on network.", policyToTest.Type)
		}
	}

	network.RemovePolicy(*policiesToTest)

	//Reload the network object from HNS.
	network, err = GetNetworkByID(network.Id)
	if err != nil {
		t.Fatal(err)
	}

	for _, policyToTest := range policiesToTest.Policies {
		foundPolicy := false
		for _, policy := range network.Policies {
			if policy.Type == policyToTest.Type {
				foundPolicy = true
				break
			}
		}
		if foundPolicy {
			t.Fatalf("Found %s policy on network when it should have been deleted.", policyToTest.Type)
		}
	}

	err = network.Delete()
	if err != nil {
		t.Fatal(err)
	}
}

func TestAddRemoveRemoteSubnetRoutePolicy(t *testing.T) {

	remoteSubnetRoutePolicy, err := HcnCreateTestRemoteSubnetRoute()
	if err != nil {
		t.Fatal(err)
	}

	testNetworkPolicy(t, remoteSubnetRoutePolicy)
}

func TestAddRemoveHostRoutePolicy(t *testing.T) {

	hostRoutePolicy, err := HcnCreateTestHostRoute()
	if err != nil {
		t.Fatal(err)
	}

	testNetworkPolicy(t, hostRoutePolicy)
}

func TestAddRemoveNetworACLPolicy(t *testing.T){

	networkACLPolicy, err := HcnCreateNetworkACLs()
	if err != nil {
		t.Fatal(err)
	}

	testNetworkPolicy(t, networkACLPolicy)

}

func TestNetworkFlags(t *testing.T) {

	network, err := CreateTestOverlayNetwork()
	if err != nil {
		t.Fatal(err)
	}

	//Reload the network object from HNS.
	network, err = GetNetworkByID(network.Id)
	if err != nil {
		t.Fatal(err)
	}

	if network.Flags != EnableNonPersistent {
		t.Errorf("EnableNonPersistent flag (%d) is not set on network", EnableNonPersistent)
	}

	err = network.Delete()
	if err != nil {
		t.Fatal(err)
	}
}
