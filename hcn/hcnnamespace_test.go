// +build integration

package hcn

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Microsoft/hcsshim/internal/cni"
	"github.com/Microsoft/hcsshim/internal/guid"
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

func TestCreateDeleteNamespaceGuest(t *testing.T) {
	namespace := &HostComputeNamespace{
		Type:        NamespaceTypeGuestDefault,
		SchemaVersion: SchemaVersion{
			Major: 2,
			Minor: 0,
		},
	}

	hnsNamespace, err := namespace.Create()
	if err != nil {
		t.Error(err)
	}

	_, err = hnsNamespace.Delete()
	if err != nil {
		t.Error(err)
	}
}

func TestGetNamespaceById(t *testing.T) {
	namespace, err := HcnCreateTestNamespace()
	if err != nil {
		t.Error(err)
	}

	foundNamespace, err := GetNamespaceByID(namespace.Id)
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

	foundNamespaces, err := ListNamespaces()
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
	foundEndpoints, err := GetNamespaceEndpointIds(namespace.Id)
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

	foundEndpoints, err := GetNamespaceContainerIds(namespace.Id)
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

	err = AddNamespaceEndpoint(namespace.Id, endpoint.Id)
	if err != nil {
		t.Error(err)
	}
	foundEndpoints, err := GetNamespaceEndpointIds(namespace.Id)
	if err != nil {
		t.Error(err)
	}
	if len(foundEndpoints) == 0 {
		t.Errorf("No Endpoint found")
	}
	err = RemoveNamespaceEndpoint(namespace.Id, endpoint.Id)
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
	requestMessage := &ModifyNamespaceSettingRequest{
		ResourceType: NamespaceResourceTypeEndpoint,
		RequestType:  RequestTypeAdd,
		Settings:     settingsJson,
	}

	err = ModifyNamespaceSettings(namespace.Id, requestMessage)
	if err != nil {
		t.Error(err)
	}
	foundEndpoints, err := GetNamespaceEndpointIds(namespace.Id)
	if err != nil {
		t.Error(err)
	}
	if len(foundEndpoints) == 0 {
		t.Errorf("No Endpoint found")
	}
	err = RemoveNamespaceEndpoint(namespace.Id, endpoint.Id)
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

// Sync Tests

func TestSyncNamespaceHostDefault(t *testing.T) {
	namespace := &HostComputeNamespace{
		Type:        NamespaceTypeHostDefault,
		NamespaceId: 5,
		SchemaVersion: SchemaVersion{
			Major: 2,
			Minor: 0,
		},
	}

	hnsNamespace, err := namespace.Create()
	if err != nil {
		t.Error(err)
	}

	// Host namespace types should be no-op success
	err = hnsNamespace.Sync()
	if err != nil {
		t.Error(err)
	}

	_, err = hnsNamespace.Delete()
	if err != nil {
		t.Error(err)
	}
}

func TestSyncNamespaceHost(t *testing.T) {
	namespace := &HostComputeNamespace{
		Type:        NamespaceTypeHost,
		NamespaceId: 5,
		SchemaVersion: SchemaVersion{
			Major: 2,
			Minor: 0,
		},
	}

	hnsNamespace, err := namespace.Create()
	if err != nil {
		t.Error(err)
	}

	// Host namespace types should be no-op success
	err = hnsNamespace.Sync()
	if err != nil {
		t.Error(err)
	}

	_, err = hnsNamespace.Delete()
	if err != nil {
		t.Error(err)
	}
}

func TestSyncNamespaceGuestNoReg(t *testing.T) {
	namespace := &HostComputeNamespace{
		Type:        NamespaceTypeGuest,
		NamespaceId: 5,
		SchemaVersion: SchemaVersion{
			Major: 2,
			Minor: 0,
		},
	}

	hnsNamespace, err := namespace.Create()
	if err != nil {
		t.Error(err)
	}

	// Guest namespace type with out reg state should be no-op success
	err = hnsNamespace.Sync()
	if err != nil {
		t.Error(err)
	}

	_, err = hnsNamespace.Delete()
	if err != nil {
		t.Error(err)
	}
}

func TestSyncNamespaceGuestDefaultNoReg(t *testing.T) {
	namespace := &HostComputeNamespace{
		Type:        NamespaceTypeGuestDefault,
		NamespaceId: 5,
		SchemaVersion: SchemaVersion{
			Major: 2,
			Minor: 0,
		},
	}

	hnsNamespace, err := namespace.Create()
	if err != nil {
		t.Error(err)
	}

	// Guest namespace type with out reg state should be no-op success
	err = hnsNamespace.Sync()
	if err != nil {
		t.Error(err)
	}

	_, err = hnsNamespace.Delete()
	if err != nil {
		t.Error(err)
	}
}

func TestSyncNamespaceGuest(t *testing.T) {
	namespace := &HostComputeNamespace{
		Type:        NamespaceTypeGuest,
		NamespaceId: 5,
		SchemaVersion: SchemaVersion{
			Major: 2,
			Minor: 0,
		},
	}

	hnsNamespace, err := namespace.Create()

	if err != nil {
		t.Error(err)
	}

	// Create registry state
	pnc := cni.NewPersistedNamespaceConfig(t.Name(), "test-container", guid.New())
	err = pnc.Store()
	if err != nil {
		pnc.Remove()
		t.Error(err)
	}

	// Guest namespace type with reg state but not Vm shim should pass...
	// after trying to connect to VM shim that it doesn't find and remove the Key so it doesn't look again.
	err = hnsNamespace.Sync()
	if err != nil {
		t.Error(err)
	}

	err = pnc.Remove()
	if err != nil {
		t.Error(err)
	}
	_, err = hnsNamespace.Delete()
	if err != nil {
		t.Error(err)
	}
}

func TestSyncNamespaceGuestDefault(t *testing.T) {
	namespace := &HostComputeNamespace{
		Type:        NamespaceTypeGuestDefault,
		NamespaceId: 5,
		SchemaVersion: SchemaVersion{
			Major: 2,
			Minor: 0,
		},
	}

	hnsNamespace, err := namespace.Create()
	if err != nil {
		t.Error(err)
	}

	// Create registry state
	pnc := cni.NewPersistedNamespaceConfig(t.Name(), "test-container", guid.New())
	err = pnc.Store()
	if err != nil {
		pnc.Remove()
		t.Error(err)
	}

	// Guest namespace type with reg state but not Vm shim should pass...
	// after trying to connect to VM shim that it doesn't find and remove the Key so it doesn't look again.
	err = hnsNamespace.Sync()
	if err != nil {
		t.Error(err)
	}

	err = pnc.Remove()
	if err != nil {
		t.Error(err)
	}
	_, err = hnsNamespace.Delete()
	if err != nil {
		t.Error(err)
	}
}
