package store

import (
	"context"
	"path/filepath"
	"testing"

	ncproxynetworking "github.com/Microsoft/hcsshim/internal/ncproxy/networking"
	bolt "go.etcd.io/bbolt"
)

func TestComputeAgentStore(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	db, err := bolt.Open(filepath.Join(tempDir, "networkproxy.db.test"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := NewComputeAgentStore(db)
	containerID := "fake-container-id"
	address := "123412341234"

	if err := store.UpdateComputeAgent(ctx, containerID, address); err != nil {
		t.Fatal(err)
	}

	actual, err := store.GetComputeAgent(ctx, containerID)
	if err != nil {
		t.Fatal(err)
	}

	if address != actual {
		t.Fatalf("compute agent addresses are not equal, expected %v but got %v", address, actual)
	}

	if err := store.DeleteComputeAgent(ctx, containerID); err != nil {
		t.Fatal(err)
	}

	value, err := store.GetComputeAgent(ctx, containerID)
	if err == nil {
		t.Fatalf("expected an error, instead found value %s", value)
	}
}

func TestComputeAgentStore_GetComputeAgents(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	db, err := bolt.Open(filepath.Join(tempDir, "networkproxy.db.test"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := NewComputeAgentStore(db)

	containerIDs := []string{"fake-container-id", "fake-container-id-2"}
	addresses := []string{"123412341234", "234523452345"}

	target := make(map[string]string)
	for i := 0; i < len(containerIDs); i++ {
		target[containerIDs[i]] = addresses[i]
		if err := store.UpdateComputeAgent(ctx, containerIDs[i], addresses[i]); err != nil {
			t.Fatal(err)
		}
	}

	actual, err := store.GetComputeAgents(ctx)
	if err != nil {
		t.Fatal(err)
	}

	for k, v := range actual {
		if target[k] != v {
			t.Fatalf("expected to get %s for key %s, instead got %s", target[k], k, v)
		}
	}
}

func TestEndpointStore(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	db, err := bolt.Open(filepath.Join(tempDir, "networkproxy.db.test"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := NewNetworkingStore(db)
	endpointName := "test-endpoint-name"
	namespaceID := "test-namespace-id"

	endpoint := &ncproxynetworking.Endpoint{
		EndpointName: endpointName,
		NamespaceID:  namespaceID,
	}

	if err := store.CreatEndpoint(ctx, endpoint); err != nil {
		t.Fatal(err)
	}

	actual, err := store.GetEndpointByName(ctx, endpointName)
	if err != nil {
		t.Fatal(err)
	}

	if actual.EndpointName != endpointName {
		t.Fatalf("endpoint name is not equal, expected %v but got %v", endpointName, actual.EndpointName)
	}

	if actual.NamespaceID != namespaceID {
		t.Fatalf("endpoint namespace id is not equal, expected %v but got %v", namespaceID, actual.NamespaceID)
	}

	if err := store.DeleteEndpoint(ctx, endpointName); err != nil {
		t.Fatal(err)
	}

	actual, err = store.GetEndpointByName(ctx, endpointName)
	if err == nil {
		t.Fatalf("expected an error, instead found endpoint %v", actual)
	}
}

func TestEndpointStore_GetAll(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	db, err := bolt.Open(filepath.Join(tempDir, "networkproxy.db.test"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := NewNetworkingStore(db)

	endpoints := []*ncproxynetworking.Endpoint{
		{
			EndpointName: "endpoint-name-1",
		},
		{
			EndpointName: "endpoint-name-2",
		},
	}

	target := make(map[string]*ncproxynetworking.Endpoint)
	for i := 0; i < len(endpoints); i++ {
		target[endpoints[i].EndpointName] = endpoints[i]
		if err := store.CreatEndpoint(ctx, endpoints[i]); err != nil {
			t.Fatal(err)
		}
	}

	actual, err := store.ListEndpoints(ctx)
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range actual {
		endpt, ok := target[e.EndpointName]
		if !ok {
			t.Fatalf("unexpected endpoint with name %v found", e.EndpointName)
		}
		if endpt.EndpointName != e.EndpointName {
			t.Fatalf("expected found endpoint to have name %v, instead found %v", endpt.EndpointName, e.EndpointName)
		}
	}
}

func TestNetworkStore(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	db, err := bolt.Open(filepath.Join(tempDir, "networkproxy.db.test"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := NewNetworkingStore(db)
	networkName := "test-network-name"

	network := &ncproxynetworking.Network{
		NetworkName: networkName,
	}

	if err := store.CreateNetwork(ctx, network); err != nil {
		t.Fatal(err)
	}

	actual, err := store.GetNetworkByName(ctx, networkName)
	if err != nil {
		t.Fatal(err)
	}

	if actual.NetworkName != networkName {
		t.Fatalf("network name is not equal, expected %v but got %v", networkName, actual.NetworkName)
	}

	if err := store.DeleteNetwork(ctx, networkName); err != nil {
		t.Fatal(err)
	}

	actual, err = store.GetNetworkByName(ctx, networkName)
	if err == nil {
		t.Fatalf("expected an error, instead found network %v", actual)
	}
}

func TestNetworkStore_GetAll(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	db, err := bolt.Open(filepath.Join(tempDir, "networkproxy.db.test"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := NewNetworkingStore(db)

	networks := []*ncproxynetworking.Network{
		{
			NetworkName: "network-name-1",
		},
		{
			NetworkName: "network-name-2",
		},
	}

	target := make(map[string]*ncproxynetworking.Network)
	for i := 0; i < len(networks); i++ {
		target[networks[i].NetworkName] = networks[i]
		if err := store.CreateNetwork(ctx, networks[i]); err != nil {
			t.Fatal(err)
		}
	}

	actual, err := store.ListNetworks(ctx)
	if err != nil {
		t.Fatal(err)
	}

	for _, n := range actual {
		network, ok := target[n.NetworkName]
		if !ok {
			t.Fatalf("unexpected network with name %v found", n.NetworkName)
		}
		if network.NetworkName != n.NetworkName {
			t.Fatalf("expected found network to have name %v, instead found %v", network.NetworkName, n.NetworkName)
		}
	}
}
