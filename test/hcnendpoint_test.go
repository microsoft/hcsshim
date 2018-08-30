package hcsshimtest

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Microsoft/hcsshim"
)

func TestCreateDeleteEndpoint(t *testing.T) {
	network, err := HcnCreateTestNetwork()
	if err != nil {
		t.Error(err)
	}
	Endpoint, err := HcnCreateTestEndpoint(network)
	if err != nil {
		t.Error(err)
	}
	jsonString, err := json.Marshal(Endpoint)
	if err != nil {
		t.Error(err)
	}
	fmt.Printf("Endpoint JSON:\n%s \n", jsonString)

	_, err = Endpoint.Delete()
	if err != nil {
		t.Error(err)
	}
	_, err = network.Delete()
	if err != nil {
		t.Error(err)
	}
}

func TestGetEndpointById(t *testing.T) {
	network, err := HcnCreateTestNetwork()
	if err != nil {
		t.Error(err)
	}
	Endpoint, err := HcnCreateTestEndpoint(network)
	if err != nil {
		t.Error(err)
	}

	foundEndpoint, err := hcsshim.GetEndpointByID(Endpoint.Id)
	if err != nil {
		t.Error(err)
	}
	if foundEndpoint == nil {
		t.Errorf("No Endpoint found")
	}

	_, err = foundEndpoint.Delete()
	if err != nil {
		t.Error(err)
	}
	_, err = network.Delete()
	if err != nil {
		t.Error(err)
	}
}

func TestGetEndpointByName(t *testing.T) {
	network, err := HcnCreateTestNetwork()
	if err != nil {
		t.Error(err)
	}
	Endpoint, err := HcnCreateTestEndpoint(network)
	if err != nil {
		t.Error(err)
	}

	foundEndpoint, err := hcsshim.GetEndpointByName(Endpoint.Name)
	if err != nil {
		t.Error(err)
	}
	if foundEndpoint == nil {
		t.Errorf("No Endpoint found")
	}

	_, err = foundEndpoint.Delete()
	if err != nil {
		t.Error(err)
	}
	_, err = network.Delete()
	if err != nil {
		t.Error(err)
	}
}

func TestListEndpoints(t *testing.T) {
	network, err := HcnCreateTestNetwork()
	if err != nil {
		t.Error(err)
	}
	Endpoint, err := HcnCreateTestEndpoint(network)
	if err != nil {
		t.Error(err)
	}

	foundEndpoints, err := hcsshim.ListEndpoints()
	if err != nil {
		t.Error(err)
	}
	if len(foundEndpoints) == 0 {
		t.Errorf("No Endpoint found")
	}

	_, err = Endpoint.Delete()
	if err != nil {
		t.Error(err)
	}
	_, err = network.Delete()
	if err != nil {
		t.Error(err)
	}
}

func TestListEndpointsOfNetwork(t *testing.T) {
	network, err := HcnCreateTestNetwork()
	if err != nil {
		t.Error(err)
	}
	Endpoint, err := HcnCreateTestEndpoint(network)
	if err != nil {
		t.Error(err)
	}

	foundEndpoints, err := hcsshim.ListEndpointsOfNetwork(network.Id)
	if err != nil {
		t.Error(err)
	}
	if len(foundEndpoints) == 0 {
		t.Errorf("No Endpoint found")
	}

	_, err = Endpoint.Delete()
	if err != nil {
		t.Error(err)
	}
	_, err = network.Delete()
	if err != nil {
		t.Error(err)
	}
}

func TestEndpointNamespaceAttachDetach(t *testing.T) {
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

func TestCreateEndpointWithNamespace(t *testing.T) {
	network, err := HcnCreateTestNetwork()
	if err != nil {
		t.Error(err)
	}
	namespace, err := HcnCreateTestNamespace()
	if err != nil {
		t.Error(err)
	}
	Endpoint, err := HcnCreateTestEndpointWithNamespace(network, namespace)
	if err != nil {
		t.Error(err)
	}
	if Endpoint.HostComputeNamespace == "" {
		t.Errorf("No Namespace detected.")
	}

	_, err = Endpoint.Delete()
	if err != nil {
		t.Error(err)
	}
	_, err = network.Delete()
	if err != nil {
		t.Error(err)
	}
}

func TestApplyPolicyOnEndpoint(t *testing.T) {
	network, err := HcnCreateTestNetwork()
	if err != nil {
		t.Error(err)
	}
	Endpoint, err := HcnCreateTestEndpoint(network)
	if err != nil {
		t.Error(err)
	}
	acls, err := HcnCreateAclsAllowIn()
	if err != nil {
		t.Error(err)
	}
	jsonString, err := json.Marshal(acls)
	if err != nil {
		t.Error(err)
	}
	fmt.Printf("ACLS JSON:\n%s \n", jsonString)
	err = Endpoint.ApplyPolicy(acls)
	if err != nil {
		t.Error(err)
	}

	foundEndpoint, err := hcsshim.GetEndpointByName(Endpoint.Name)
	if err != nil {
		t.Error(err)
	}
	if len(foundEndpoint.Policies) == 0 {
		t.Errorf("No Endpoint Policies found")
	}

	_, err = Endpoint.Delete()
	if err != nil {
		t.Error(err)
	}
	_, err = network.Delete()
	if err != nil {
		t.Error(err)
	}
}

func TestModifyEndpointSettings(t *testing.T) {
	network, err := HcnCreateTestNetwork()
	if err != nil {
		t.Error(err)
	}
	endpoint, err := HcnCreateTestEndpoint(network)
	if err != nil {
		t.Error(err)
	}
	endpointPolicy, err := HcnCreateAclsAllowIn()
	if err != nil {
		t.Error(err)
	}

	requestMessage := &hcsshim.ModifyEndpointSettingRequest{
		ResourceType: "Policy",
		RequestType:  "Update",
		Settings:     *endpointPolicy,
	}

	err = hcsshim.ModifyEndpointSettings(endpoint.Id, requestMessage)
	if err != nil {
		t.Error(err)
	}

	foundEndpoint, err := hcsshim.GetEndpointByName(endpoint.Name)
	if err != nil {
		t.Error(err)
	}
	if len(foundEndpoint.Policies) == 0 {
		t.Errorf("No Endpoint Policies found")
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
