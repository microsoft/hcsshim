package hcsshimtest

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Microsoft/hcsshim"
)

func TestCreateDeleteNetwork(t *testing.T) {
	network, err := HcnCreateTestNetwork()
	if err != nil {
		t.Error(err)
	}
	jsonString, err := json.Marshal(network)
	if err != nil {
		t.Error(err)
	}
	fmt.Printf("Network JSON:\n%s \n", jsonString)
	_, err = network.Delete()
	if err != nil {
		t.Error(err)
	}
}

func TestGetNetworkByName(t *testing.T) {
	network, err := HcnCreateTestNetwork()
	if err != nil {
		t.Error(err)
	}
	network, err = hcsshim.GetNetworkByName(network.Name)
	if err != nil {
		t.Error(err)
	}
	if network == nil {
		t.Errorf("No Network found")
	}
	_, err = network.Delete()
	if err != nil {
		t.Error(err)
	}
}

func TestGetNetworkById(t *testing.T) {
	network, err := HcnCreateTestNetwork()
	if err != nil {
		t.Error(err)
	}
	network, err = hcsshim.GetNetworkByID(network.Id)
	if err != nil {
		t.Error(err)
	}
	if network == nil {
		t.Errorf("No Network found")
	}
	_, err = network.Delete()
	if err != nil {
		t.Error(err)
	}
}

func TestListNetwork(t *testing.T) {
	_, err := hcsshim.ListNetworks()
	if err != nil {
		t.Error(err)
	}
}
