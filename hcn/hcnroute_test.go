//go:build integration
// +build integration

package hcn

import (
	"encoding/json"
	"testing"
)

func TestCreateDeleteRoute(t *testing.T) {
	network, err := CreateTestOverlayNetwork()
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := HcnCreateTestEndpoint(network)
	if err != nil {
		t.Fatal(err)
	}
	route, err := HcnCreateTestSdnRoute(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	jsonString, err := json.Marshal(route)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("SDN Route JSON:\n%s \n", jsonString)

	err = route.Delete()
	if err != nil {
		t.Fatal(err)
	}
	err = endpoint.Delete()
	if err != nil {
		t.Fatal(err)
	}
	err = network.Delete()
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetRouteById(t *testing.T) {
	network, err := CreateTestOverlayNetwork()
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := HcnCreateTestEndpoint(network)
	if err != nil {
		t.Fatal(err)
	}
	route, err := HcnCreateTestSdnRoute(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	jsonString, err := json.Marshal(route)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("SDN Route JSON:\n%s \n", jsonString)
	foundRoute, err := GetRouteByID(route.ID)
	if err != nil {
		t.Fatal(err)
	}
	if foundRoute == nil {
		t.Fatalf("No SDN route found")
	}
	err = route.Delete()
	if err != nil {
		t.Fatal(err)
	}
	err = endpoint.Delete()
	if err != nil {
		t.Fatal(err)
	}
	err = network.Delete()
	if err != nil {
		t.Fatal(err)
	}
}

func TestListRoutes(t *testing.T) {
	_, err := ListRoutes()
	if err != nil {
		t.Fatal(err)
	}
}

func TestRouteAddRemoveEndpoint(t *testing.T) {
	network, err := CreateTestOverlayNetwork()
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := HcnCreateTestEndpoint(network)
	if err != nil {
		t.Fatal(err)
	}
	route, err := HcnCreateTestSdnRoute(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	secondEndpoint, err := HcnCreateTestEndpoint(network)
	if err != nil {
		t.Fatal(err)
	}
	newRoute, err := route.AddEndpoint(secondEndpoint)
	if err != nil {
		t.Fatal(err)
	}
	if len(newRoute.HostComputeEndpoints) != 2 {
		t.Fatalf("Endpoint not added to SDN Route")
	}
	newRoute1, err := route.RemoveEndpoint(secondEndpoint)
	if err != nil {
		t.Fatal(err)
	}
	if len(newRoute1.HostComputeEndpoints) != 1 {
		t.Fatalf("Endpoint not removed from SDN Route")
	}
	err = route.Delete()
	if err != nil {
		t.Fatal(err)
	}
	err = endpoint.Delete()
	if err != nil {
		t.Fatal(err)
	}
	err = network.Delete()
	if err != nil {
		t.Fatal(err)
	}
}

func TestAddRoute(t *testing.T) {
	network, err := CreateTestOverlayNetwork()
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := HcnCreateTestEndpoint(network)
	if err != nil {
		t.Fatal(err)
	}
	route, err := AddRoute([]HostComputeEndpoint{*endpoint}, "169.254.169.254/24", "127.10.0.33", false)
	if err != nil {
		t.Fatal(err)
	}
	jsonString, err := json.Marshal(route)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("SDN Route JSON:\n%s \n", jsonString)
	foundRoute, err := GetRouteByID(route.ID)
	if err != nil {
		t.Fatal(err)
	}
	if foundRoute == nil {
		t.Fatalf("No SDN route found")
	}
	err = route.Delete()
	if err != nil {
		t.Fatal(err)
	}
	err = endpoint.Delete()
	if err != nil {
		t.Fatal(err)
	}
	err = network.Delete()
	if err != nil {
		t.Fatal(err)
	}
}
