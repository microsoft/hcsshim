package hcsshimtest

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Microsoft/hcsshim"
)

func TestCreateDeleteNamespace(t *testing.T) {
	namespace, err := HcnCreateTestNamespace()
	if err != nil {
		t.Error(err)
	}

	jsonString, err := json.Marshal(namespace)
	if err != nil {
		t.Error(err)
	}
	fmt.Printf("Namespace JSON:\n%s \n", jsonString)

	_, err = namespace.Delete()
	if err != nil {
		t.Error(err)
	}
}

func TestGetNamespaceById(t *testing.T) {
	namespace, err := HcnCreateTestNamespace()
	if err != nil {
		t.Error(err)
	}

	foundNamespace, err := hcsshim.GetNamespaceByID(namespace.Id)
	if err != nil {
		t.Error(err)
	}
	if foundNamespace == nil {
		t.Errorf("No namespace found")
	}

	_, err = namespace.Delete()
	if err != nil {
		t.Error(err)
	}
}

func TestListNamespaces(t *testing.T) {
	namespace, err := HcnCreateTestNamespace()
	if err != nil {
		t.Error(err)
	}

	foundNamespaces, err := hcsshim.ListNamespaces()
	if err != nil {
		t.Error(err)
	}
	if len(foundNamespaces) == 0 {
		t.Errorf("No Namespaces found")
	}

	_, err = namespace.Delete()
	if err != nil {
		t.Error(err)
	}
}

func TestGetNamespaceEndpointIds(t *testing.T) {
	network, err := HcnCreateTestNetwork()
	if err != nil {
		t.Error(err)
	}
	endpoint, err := HcnCreateTestEndpoint(network)
	if err != nil {
		t.Error(err)
	}
	namespace, err := HcnCreateTestNamespace()
	if err != nil {
		t.Error(err)
	}

	err = endpoint.NamespaceAttach(namespace.Id)
	if err != nil {
		t.Error(err)
	}
	foundEndpoints, err := hcsshim.GetNamespaceEndpointIds(namespace.Id)
	if err != nil {
		t.Error(err)
	}
	if len(foundEndpoints) == 0 {
		t.Errorf("No Endpoint found")
	}
	err = endpoint.NamespaceDetach(namespace.Id)
	if err != nil {
		t.Error(err)
	}

	_, err = namespace.Delete()
	if err != nil {
		t.Error(err)
	}
	_, err = endpoint.Delete()
	if err != nil {
		t.Error(err)
	}
	_, err = network.Delete()
	if err != nil {
		t.Error(err)
	}
}

func TestGetNamespaceContainers(t *testing.T) {
	namespace, err := HcnCreateTestNamespace()
	if err != nil {
		t.Error(err)
	}

	foundEndpoints, err := hcsshim.GetNamespaceContainerIds(namespace.Id)
	if err != nil {
		t.Error(err)
	}
	if len(foundEndpoints) != 0 {
		t.Errorf("Found containers when none should exist")
	}

	_, err = namespace.Delete()
	if err != nil {
		t.Error(err)
	}
}

func TestAddRemoveNamespaceEndpoint(t *testing.T) {
	network, err := HcnCreateTestNetwork()
	if err != nil {
		t.Error(err)
	}
	endpoint, err := HcnCreateTestEndpoint(network)
	if err != nil {
		t.Error(err)
	}
	namespace, err := HcnCreateTestNamespace()
	if err != nil {
		t.Error(err)
	}

	err = hcsshim.AddNamespaceEndpoint(namespace.Id, endpoint.Id)
	if err != nil {
		t.Error(err)
	}
	foundEndpoints, err := hcsshim.GetNamespaceEndpointIds(namespace.Id)
	if err != nil {
		t.Error(err)
	}
	if len(foundEndpoints) == 0 {
		t.Errorf("No Endpoint found")
	}
	err = hcsshim.RemoveNamespaceEndpoint(namespace.Id, endpoint.Id)
	if err != nil {
		t.Error(err)
	}

	_, err = namespace.Delete()
	if err != nil {
		t.Error(err)
	}
	_, err = endpoint.Delete()
	if err != nil {
		t.Error(err)
	}
	_, err = network.Delete()
	if err != nil {
		t.Error(err)
	}
}

func TestModifyNamespaceSettings(t *testing.T) {
	network, err := HcnCreateTestNetwork()
	if err != nil {
		t.Error(err)
	}
	endpoint, err := HcnCreateTestEndpoint(network)
	if err != nil {
		t.Error(err)
	}
	namespace, err := HcnCreateTestNamespace()
	if err != nil {
		t.Error(err)
	}

	mapA := map[string]string{"EndpointId": endpoint.Id}
	settingsJson, err := json.Marshal(mapA)
	if err != nil {
		t.Error(err)
	}
	requestMessage := &hcsshim.ModifyNamespaceSettingRequest{
		ResourceType: "Endpoint",
		RequestType:  "Add",
		Settings:     settingsJson,
	}

	err = hcsshim.ModifyNamespaceSettings(namespace.Id, requestMessage)
	if err != nil {
		t.Error(err)
	}
	foundEndpoints, err := hcsshim.GetNamespaceEndpointIds(namespace.Id)
	if err != nil {
		t.Error(err)
	}
	if len(foundEndpoints) == 0 {
		t.Errorf("No Endpoint found")
	}
	err = hcsshim.RemoveNamespaceEndpoint(namespace.Id, endpoint.Id)
	if err != nil {
		t.Error(err)
	}

	_, err = namespace.Delete()
	if err != nil {
		t.Error(err)
	}
	_, err = endpoint.Delete()
	if err != nil {
		t.Error(err)
	}
	_, err = network.Delete()
	if err != nil {
		t.Error(err)
	}
}
