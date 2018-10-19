// +build integration

package hcn

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestCreateDeleteNetwork(t *testing.T) {
	network, err := HcnCreateTestNetwork()
	if err != nil {
		t.Fatal(err)
	}
	jsonString, err := json.Marshal(network)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("Network JSON:\n%s \n", jsonString)
	_, err = network.Delete()
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetNetworkByName(t *testing.T) {
	network, err := HcnCreateTestNetwork()
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
	_, err = network.Delete()
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetNetworkById(t *testing.T) {
	network, err := HcnCreateTestNetwork()
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
	_, err = network.Delete()
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
