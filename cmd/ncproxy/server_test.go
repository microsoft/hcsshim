package main

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	ncproxyMock "github.com/Microsoft/hcsshim/cmd/ncproxy/ncproxy_mock"
	"github.com/Microsoft/hcsshim/cmd/ncproxy/ncproxygrpc"
	"github.com/Microsoft/hcsshim/cmd/ncproxy/nodenetsvc"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/Microsoft/hcsshim/internal/computeagent"
	"github.com/Microsoft/hcsshim/internal/ncproxyttrpc"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/containerd/ttrpc"
	"github.com/golang/mock/gomock"
)

func exists(target string, list []string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

func networkExists(targetName string, networks []*ncproxygrpc.GetNetworkResponse) bool {
	for _, n := range networks {
		if n.Name == targetName {
			return true
		}
	}
	return false
}

func endpointExists(targetName string, endpoints []*ncproxygrpc.GetEndpointResponse) bool {
	for _, ep := range endpoints {
		if ep.Name == targetName {
			return true
		}
	}
	return false
}

func getTestSubnets() []hcn.Subnet {
	testSubnet := hcn.Subnet{
		IpAddressPrefix: "192.168.100.0/24",
		Routes: []hcn.Route{
			{
				NextHop:           "192.168.100.1",
				DestinationPrefix: "0.0.0.0/0",
			},
		},
	}
	return []hcn.Subnet{testSubnet}
}

func createTestEndpoint(name, networkID string) (*hcn.HostComputeEndpoint, error) {
	endpoint := &hcn.HostComputeEndpoint{
		Name:               name,
		HostComputeNetwork: networkID,
		SchemaVersion:      hcn.V2SchemaVersion(),
	}
	return endpoint.Create()
}

func createTestNATNetwork() (*hcn.HostComputeNetwork, error) {
	ipam := hcn.Ipam{
		Type:    "Static",
		Subnets: getTestSubnets(),
	}
	ipams := []hcn.Ipam{ipam}
	network := &hcn.HostComputeNetwork{
		Type: hcn.NAT,
		Name: "test-nat-network-name",
		MacPool: hcn.MacPool{
			Ranges: []hcn.MacRange{
				{
					StartMacAddress: "00-15-5D-52-C0-00",
					EndMacAddress:   "00-15-5D-52-CF-FF",
				},
			},
		},
		Ipams:         ipams,
		SchemaVersion: hcn.V2SchemaVersion(),
	}
	return network.Create()
}

func TestAddNIC(t *testing.T) {
	ctx := context.Background()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache)

	var (
		containerID      = "test-container"
		testNICID        = "test-nic-id"
		testEndpointName = "test-endpoint-name"
	)

	// create mocked compute agent service
	computeAgentCtrl := gomock.NewController(t)
	defer computeAgentCtrl.Finish()
	mockedService := ncproxyMock.NewMockComputeAgentService(computeAgentCtrl)

	// put mocked compute agent in agent cache for test
	agentCache.put(containerID, mockedService)

	// setup expected mocked calls
	mockedService.EXPECT().AddNIC(gomock.Any(), gomock.Any()).Return(&computeagent.AddNICInternalResponse{}, nil).AnyTimes()

	type config struct {
		name          string
		containerID   string
		nicID         string
		endpointName  string
		errorExpected bool
	}
	tests := []config{
		{
			name:          "AddNIC returns no error",
			containerID:   containerID,
			nicID:         testNICID,
			endpointName:  testEndpointName,
			errorExpected: false,
		},
		{
			name:          "AddNIC returns error with blank container ID",
			containerID:   "",
			nicID:         testNICID,
			endpointName:  testEndpointName,
			errorExpected: true,
		},
		{
			name:          "AddNIC returns error with blank nic ID",
			containerID:   containerID,
			nicID:         "",
			endpointName:  testEndpointName,
			errorExpected: true,
		},
		{
			name:          "AddNIC returns error with blank endpoint name",
			containerID:   containerID,
			nicID:         testNICID,
			endpointName:  "",
			errorExpected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(_ *testing.T) {

			req := &ncproxygrpc.AddNICRequest{
				ContainerID:  test.containerID,
				NicID:        test.nicID,
				EndpointName: test.endpointName,
			}

			_, err := gService.AddNIC(ctx, req)
			if test.errorExpected && err == nil {
				t.Fatalf("expected AddNIC to return an error")
			}
			if !test.errorExpected && err != nil {
				t.Fatalf("expected AddNIC to return no error, instead got %v", err)
			}
		})
	}
}

func TestDeleteNIC(t *testing.T) {
	ctx := context.Background()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache)

	var (
		containerID      = "test-container"
		testNICID        = "test-nic-id"
		testEndpointName = "test-endpoint-name"
	)

	// create mocked compute agent service
	computeAgentCtrl := gomock.NewController(t)
	defer computeAgentCtrl.Finish()
	mockedService := ncproxyMock.NewMockComputeAgentService(computeAgentCtrl)

	// put mocked compute agent in agent cache for test
	agentCache.put(containerID, mockedService)

	// setup expected mocked calls
	mockedService.EXPECT().DeleteNIC(gomock.Any(), gomock.Any()).Return(&computeagent.DeleteNICInternalResponse{}, nil).AnyTimes()

	type config struct {
		name          string
		containerID   string
		nicID         string
		endpointName  string
		errorExpected bool
	}
	tests := []config{
		{
			name:          "DeleteNIC returns no error",
			containerID:   containerID,
			nicID:         testNICID,
			endpointName:  testEndpointName,
			errorExpected: false,
		},
		{
			name:          "DeleteNIC returns error with blank container ID",
			containerID:   "",
			nicID:         testNICID,
			endpointName:  testEndpointName,
			errorExpected: true,
		},
		{
			name:          "DeleteNIC returns error with blank nic ID",
			containerID:   containerID,
			nicID:         "",
			endpointName:  testEndpointName,
			errorExpected: true,
		},
		{
			name:          "DeleteNIC returns error with blank endpoint name",
			containerID:   containerID,
			nicID:         testNICID,
			endpointName:  "",
			errorExpected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(_ *testing.T) {
			req := &ncproxygrpc.DeleteNICRequest{
				ContainerID:  test.containerID,
				NicID:        test.nicID,
				EndpointName: test.endpointName,
			}

			_, err := gService.DeleteNIC(ctx, req)
			if test.errorExpected && err == nil {
				t.Fatalf("expected DeleteNIC to return an error")
			}
			if !test.errorExpected && err != nil {
				t.Fatalf("expected DeleteNIC to return no error, instead got %v", err)
			}
		})
	}
}

func TestModifyNIC(t *testing.T) {
	// support for setting IOV policy was added in 21H1
	if osversion.Build() < osversion.V21H1 {
		t.Skip("Requires build +21H1")
	}
	ctx := context.Background()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache)

	var (
		containerID = "test-container"
		testNICID   = "test-nic-id"
	)

	// create mock compute agent service
	computeAgentCtrl := gomock.NewController(t)
	defer computeAgentCtrl.Finish()
	mockedService := ncproxyMock.NewMockComputeAgentService(computeAgentCtrl)

	// populate agent cache with mocked service for test
	agentCache.put(containerID, mockedService)

	// setup expected mocked calls
	mockedService.EXPECT().ModifyNIC(gomock.Any(), gomock.Any()).Return(&computeagent.ModifyNICInternalResponse{}, nil).AnyTimes()

	// create test network
	network, err := createTestNATNetwork()
	if err != nil {
		t.Fatalf("failed to create test network with %v", err)
	}
	defer func() {
		_ = network.Delete()
	}()

	// create test endpoint
	endpointName := "test-endpoint-name"
	endpoint, err := createTestEndpoint(endpointName, network.Id)
	if err != nil {
		t.Fatalf("failed to create test endpoint with %v", err)
	}
	defer func() {
		_ = endpoint.Delete()
	}()

	iovOffloadOn := &ncproxygrpc.IovEndpointPolicySetting{
		IovOffloadWeight: 100,
	}

	type config struct {
		name              string
		containerID       string
		nicID             string
		endpointName      string
		iovPolicySettings *ncproxygrpc.IovEndpointPolicySetting
		errorExpected     bool
	}
	tests := []config{
		{
			name:              "ModifyNIC returns no error",
			containerID:       containerID,
			nicID:             testNICID,
			endpointName:      endpointName,
			iovPolicySettings: iovOffloadOn,
			errorExpected:     false,
		},
		{
			name:         "ModifyNIC returns no error when turning off iov policy",
			containerID:  containerID,
			nicID:        testNICID,
			endpointName: endpointName,
			iovPolicySettings: &ncproxygrpc.IovEndpointPolicySetting{
				IovOffloadWeight: 0,
			},
			errorExpected: false,
		},
		{
			name:              "ModifyNIC returns error with blank container ID",
			containerID:       "",
			nicID:             testNICID,
			endpointName:      endpointName,
			iovPolicySettings: iovOffloadOn,
			errorExpected:     true,
		},
		{
			name:              "ModifyNIC returns error with blank nic ID",
			containerID:       containerID,
			nicID:             "",
			endpointName:      endpointName,
			iovPolicySettings: iovOffloadOn,
			errorExpected:     true,
		},
		{
			name:              "ModifyNIC returns error with blank endpoint name",
			containerID:       containerID,
			nicID:             testNICID,
			endpointName:      "",
			iovPolicySettings: iovOffloadOn,
			errorExpected:     true,
		},
		{
			name:              "ModifyNIC returns error with blank iov policy settings",
			containerID:       containerID,
			nicID:             testNICID,
			endpointName:      endpointName,
			iovPolicySettings: nil,
			errorExpected:     true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(_ *testing.T) {
			req := &ncproxygrpc.ModifyNICRequest{
				ContainerID:       test.containerID,
				NicID:             test.nicID,
				EndpointName:      test.endpointName,
				IovPolicySettings: test.iovPolicySettings,
			}

			_, err := gService.ModifyNIC(ctx, req)
			if test.errorExpected && err == nil {
				t.Fatalf("expected ModifyNIC to return an error")
			}
			if !test.errorExpected && err != nil {
				t.Fatalf("expected ModifyNIC to return no error, instead got %v", err)
			}
		})
	}
}

func TestCreateNetwork(t *testing.T) {
	ctx := context.Background()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache)

	type config struct {
		name          string
		networkName   string
		errorExpected bool
	}
	tests := []config{
		{
			name:          "CreateNetwork returns no error",
			networkName:   "test-network-name",
			errorExpected: false,
		},
		{
			name:          "CreateNetwork returns error with blank network name",
			networkName:   "",
			errorExpected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(_ *testing.T) {
			req := &ncproxygrpc.CreateNetworkRequest{
				Name: test.networkName,
				Mode: ncproxygrpc.CreateNetworkRequest_NAT,
			}
			_, err := gService.CreateNetwork(ctx, req)
			if test.errorExpected && err == nil {
				t.Fatalf("expected CreateNetwork to return an error")
			}

			if !test.errorExpected {
				if err != nil {
					t.Fatalf("expected CreateNetwork to return no error, instead got %v", err)
				}
				// validate that the network exists
				network, err := hcn.GetNetworkByName(test.networkName)
				if err != nil {
					t.Fatalf("failed to find created network with %v", err)
				}
				// cleanup the created network
				if err = network.Delete(); err != nil {
					t.Fatalf("failed to cleanup network %v created by test with %v", network.Name, err)
				}
			}
		})
	}
}

func TestCreateEndpoint(t *testing.T) {
	ctx := context.Background()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache)

	// test network
	network, err := createTestNATNetwork()
	if err != nil {
		t.Fatalf("failed to create test network with %v", err)
	}
	defer func() {
		_ = network.Delete()
	}()

	type config struct {
		name          string
		networkName   string
		ipaddress     string
		macaddress    string
		errorExpected bool
	}

	tests := []config{
		{
			name:          "CreateEndpoint returns no error",
			networkName:   network.Name,
			ipaddress:     "192.168.100.4",
			macaddress:    "00-15-5D-52-C0-00",
			errorExpected: false,
		},
		{
			name:          "CreateEndpoint returns error when network name is empty",
			networkName:   "",
			ipaddress:     "192.168.100.4",
			macaddress:    "00-15-5D-52-C0-00",
			errorExpected: true,
		},
		{
			name:          "CreateEndpoint returns error when ip address is empty",
			networkName:   network.Name,
			ipaddress:     "",
			macaddress:    "00-15-5D-52-C0-00",
			errorExpected: true,
		},
		{
			name:          "CreateEndpoint returns error when mac address is empty",
			networkName:   network.Name,
			ipaddress:     "192.168.100.4",
			macaddress:    "",
			errorExpected: true,
		},
	}

	for i, test := range tests {
		t.Run(test.name, func(_ *testing.T) {
			endpointName := "test-endpoint-name-" + strconv.Itoa(i)
			req := &ncproxygrpc.CreateEndpointRequest{
				Name:                  endpointName,
				Macaddress:            test.macaddress,
				Ipaddress:             test.ipaddress,
				IpaddressPrefixlength: "24",
				NetworkName:           test.networkName,
			}

			_, err = gService.CreateEndpoint(ctx, req)
			if test.errorExpected && err == nil {
				t.Fatalf("expected CreateEndpoint to return an error")
			}
			if !test.errorExpected {
				if err != nil {
					t.Fatalf("expected CreateEndpoint to return no error, instead got %v", err)
				}
				// validate that the endpoint was created
				ep, err := hcn.GetEndpointByName(endpointName)
				if err != nil {
					t.Fatalf("endpoint was not found: %v", err)
				}
				// cleanup endpoint
				if err := ep.Delete(); err != nil {
					t.Fatalf("failed to delete endpoint created for test %v", err)
				}
			}
		})
	}
}

func TestAddEndpoint_NoError(t *testing.T) {
	ctx := context.Background()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache)

	// create test network namespace
	namespace := hcn.NewNamespace(hcn.NamespaceTypeHostDefault)
	namespace, err := namespace.Create()
	if err != nil {
		t.Fatalf("failed to create test namespace with %v", err)
	}
	defer func() {
		_ = namespace.Delete()
	}()

	// test network
	network, err := createTestNATNetwork()
	if err != nil {
		t.Fatalf("failed to create test network with %v", err)
	}
	defer func() {
		_ = network.Delete()
	}()

	endpointName := "test-endpoint-name"
	endpoint, err := createTestEndpoint(endpointName, network.Id)
	if err != nil {
		t.Fatalf("failed to create test endpoint with %v", err)
	}
	defer func() {
		_ = endpoint.Delete()
	}()

	req := &ncproxygrpc.AddEndpointRequest{
		Name:        endpointName,
		NamespaceID: namespace.Id,
	}

	_, err = gService.AddEndpoint(ctx, req)
	if err != nil {
		t.Fatalf("expected AddEndpoint to return no error, instead got %v", err)
	}
	// validate endpoint was added to namespace
	endpoints, err := hcn.GetNamespaceEndpointIds(namespace.Id)
	if err != nil {
		t.Fatalf("failed to get the namespace's endpoints with %v", err)
	}
	if !exists(strings.ToUpper(endpoint.Id), endpoints) {
		t.Fatalf("endpoint %v was not added to namespace %v", endpoint.Id, namespace.Id)
	}
}

func TestAddEndpoint_Error_EmptyEndpointName(t *testing.T) {
	ctx := context.Background()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache)

	// create test network namespace
	namespace := hcn.NewNamespace(hcn.NamespaceTypeHostDefault)
	namespace, err := namespace.Create()
	if err != nil {
		t.Fatalf("failed to create test namespace with %v", err)
	}
	defer func() {
		_ = namespace.Delete()
	}()

	req := &ncproxygrpc.AddEndpointRequest{
		Name:        "",
		NamespaceID: namespace.Id,
	}

	_, err = gService.AddEndpoint(ctx, req)
	if err == nil {
		t.Fatal("expected AddEndpoint to return error when endpoint name is empty")
	}
}

func TestAddEndpoint_Error_NoEndpoint(t *testing.T) {
	ctx := context.Background()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache)

	// create test network namespace
	namespace := hcn.NewNamespace(hcn.NamespaceTypeHostDefault)
	namespace, err := namespace.Create()
	if err != nil {
		t.Fatalf("failed to create test namespace with %v", err)
	}
	defer func() {
		_ = namespace.Delete()
	}()

	req := &ncproxygrpc.AddEndpointRequest{
		Name:        "test-endpoint-name",
		NamespaceID: namespace.Id,
	}

	_, err = gService.AddEndpoint(ctx, req)
	if err == nil {
		t.Fatal("expected AddEndpoint to return error when endpoint name is empty")
	}
}

func TestAddEndpoint_Error_EmptyNamespaceID(t *testing.T) {
	ctx := context.Background()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache)

	// test network
	network, err := createTestNATNetwork()
	if err != nil {
		t.Fatalf("failed to create test network with %v", err)
	}
	defer func() {
		_ = network.Delete()
	}()

	endpointName := "test-endpoint-name"
	endpoint, err := createTestEndpoint(endpointName, network.Id)
	if err != nil {
		t.Fatalf("failed to create test endpoint with %v", err)
	}
	defer func() {
		_ = endpoint.Delete()
	}()

	req := &ncproxygrpc.AddEndpointRequest{
		Name:        endpointName,
		NamespaceID: "",
	}

	_, err = gService.AddEndpoint(ctx, req)
	if err == nil {
		t.Fatal("expected AddEndpoint to return error when namespace ID is empty")
	}
}

func TestDeleteEndpoint_NoError(t *testing.T) {
	ctx := context.Background()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache)

	// test network
	network, err := createTestNATNetwork()
	if err != nil {
		t.Fatalf("failed to create test network with %v", err)
	}
	defer func() {
		_ = network.Delete()
	}()

	endpointName := "test-endpoint-name"
	endpoint, err := createTestEndpoint(endpointName, network.Id)
	if err != nil {
		t.Fatalf("failed to create test endpoint with %v", err)
	}
	// defer cleanup in case of error. ignore error from the delete call here
	// since we may have already successfully deleted the endpoint.
	defer func() {
		_ = endpoint.Delete()
	}()

	req := &ncproxygrpc.DeleteEndpointRequest{
		Name: endpointName,
	}
	_, err = gService.DeleteEndpoint(ctx, req)
	if err != nil {
		t.Fatalf("expected DeleteEndpoint to return no error, instead got %v", err)
	}
	// validate that the endpoint was created
	ep, err := hcn.GetEndpointByName(endpointName)
	if err == nil {
		t.Fatalf("expected endpoint to be deleted, instead found %v", ep)
	}
}

func TestDeleteEndpoint_Error_NoEndpoint(t *testing.T) {
	ctx := context.Background()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache)

	// test network
	network, err := createTestNATNetwork()
	if err != nil {
		t.Fatalf("failed to create test network with %v", err)
	}
	defer func() {
		_ = network.Delete()
	}()

	endpointName := "test-endpoint-name"
	req := &ncproxygrpc.DeleteEndpointRequest{
		Name: endpointName,
	}

	_, err = gService.DeleteEndpoint(ctx, req)
	if err == nil {
		t.Fatalf("expected to return an error on deleting nonexistent endpoint")
	}
}

func TestDeleteEndpoint_Error_EmptyEndpoint_Name(t *testing.T) {
	ctx := context.Background()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache)

	endpointName := ""
	req := &ncproxygrpc.DeleteEndpointRequest{
		Name: endpointName,
	}

	_, err := gService.DeleteEndpoint(ctx, req)
	if err == nil {
		t.Fatalf("expected to return an error when endpoint name is empty")
	}
}

func TestDeleteNetwork_NoError(t *testing.T) {
	ctx := context.Background()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache)

	// create the test network
	network, err := createTestNATNetwork()
	if err != nil {
		t.Fatalf("failed to create test network with %v", err)
	}
	// defer cleanup in case of error. ignore error from the delete call here
	// since we may have already successfully deleted the network.
	defer func() {
		_ = network.Delete()
	}()

	req := &ncproxygrpc.DeleteNetworkRequest{
		Name: network.Name,
	}
	_, err = gService.DeleteNetwork(ctx, req)
	if err != nil {
		t.Fatalf("expected no error, instead got %v", err)
	}
}

func TestDeleteNetwork_Error_NoNetwork(t *testing.T) {
	ctx := context.Background()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache)

	fakeNetworkName := "test-network-name"

	req := &ncproxygrpc.DeleteNetworkRequest{
		Name: fakeNetworkName,
	}
	_, err := gService.DeleteNetwork(ctx, req)
	if err == nil {
		t.Fatal("expected to get an error when attempting to delete nonexistent network")
	}
}

func TestDeleteNetwork_Error_EmptyNetworkName(t *testing.T) {
	ctx := context.Background()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache)

	req := &ncproxygrpc.DeleteNetworkRequest{
		Name: "",
	}
	_, err := gService.DeleteNetwork(ctx, req)
	if err == nil {
		t.Fatal("expected to get an error when attempting to delete nonexistent network")
	}
}

func TestGetEndpoint_NoError(t *testing.T) {
	ctx := context.Background()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache)

	// test network
	network, err := createTestNATNetwork()
	if err != nil {
		t.Fatalf("failed to create test network with %v", err)
	}
	defer func() {
		_ = network.Delete()
	}()

	endpointName := "test-endpoint-name"
	endpoint, err := createTestEndpoint(endpointName, network.Id)
	if err != nil {
		t.Fatalf("failed to create test endpoint with %v", err)
	}
	defer func() {
		_ = endpoint.Delete()
	}()

	req := &ncproxygrpc.GetEndpointRequest{
		Name: endpointName,
	}

	if _, err := gService.GetEndpoint(ctx, req); err != nil {
		t.Fatalf("expected to get no error, instead got %v", err)
	}
}

func TestGetEndpoint_Error_NoEndpoint(t *testing.T) {
	ctx := context.Background()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache)

	endpointName := "test-endpoint-name"
	req := &ncproxygrpc.GetEndpointRequest{
		Name: endpointName,
	}

	if _, err := gService.GetEndpoint(ctx, req); err == nil {
		t.Fatal("expected to get an error trying to get a nonexistent endpoint")
	}
}

func TestGetEndpoint_Error_EmptyEndpointName(t *testing.T) {
	ctx := context.Background()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache)

	req := &ncproxygrpc.GetEndpointRequest{
		Name: "",
	}

	if _, err := gService.GetEndpoint(ctx, req); err == nil {
		t.Fatal("expected to get an error with empty endpoint name")
	}
}

func TestGetEndpoints_NoError(t *testing.T) {
	ctx := context.Background()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache)

	// test network
	network, err := createTestNATNetwork()
	if err != nil {
		t.Fatalf("failed to create test network with %v", err)
	}
	defer func() {
		_ = network.Delete()
	}()

	// test endpoint
	endpointName := "test-endpoint-name"
	endpoint, err := createTestEndpoint(endpointName, network.Id)
	if err != nil {
		t.Fatalf("failed to create test endpoint with %v", err)
	}
	defer func() {
		_ = endpoint.Delete()
	}()

	req := &ncproxygrpc.GetEndpointsRequest{}
	resp, err := gService.GetEndpoints(ctx, req)
	if err != nil {
		t.Fatalf("expected to get no error, instead got %v", err)
	}

	if !endpointExists(endpointName, resp.Endpoints) {
		t.Fatalf("created endpoint was not found")
	}
}

func TestGetNetwork_NoError(t *testing.T) {
	ctx := context.Background()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache)

	// create the test network
	network, err := createTestNATNetwork()
	if err != nil {
		t.Fatalf("failed to create test network with %v", err)
	}
	// defer cleanup in case of error. ignore error from the delete call here
	// since we may have already successfully deleted the network.
	defer func() {
		_ = network.Delete()
	}()

	req := &ncproxygrpc.GetNetworkRequest{
		Name: network.Name,
	}
	_, err = gService.GetNetwork(ctx, req)
	if err != nil {
		t.Fatalf("expected no error, instead got %v", err)
	}
}

func TestGetNetwork_Error_NoNetwork(t *testing.T) {
	ctx := context.Background()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache)

	fakeNetworkName := "test-network-name"

	req := &ncproxygrpc.GetNetworkRequest{
		Name: fakeNetworkName,
	}
	_, err := gService.GetNetwork(ctx, req)
	if err == nil {
		t.Fatal("expected to get an error when attempting to get nonexistent network")
	}
}

func TestGetNetwork_Error_EmptyNetworkName(t *testing.T) {
	ctx := context.Background()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache)

	req := &ncproxygrpc.GetNetworkRequest{
		Name: "",
	}
	_, err := gService.GetNetwork(ctx, req)
	if err == nil {
		t.Fatal("expected to get an error when network name is empty")
	}
}

func TestGetNetworks_NoError(t *testing.T) {
	ctx := context.Background()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache)

	// create the test network
	network, err := createTestNATNetwork()
	if err != nil {
		t.Fatalf("failed to create test network with %v", err)
	}
	// defer cleanup in case of error. ignore error from the delete call here
	// since we may have already successfully deleted the network.
	defer func() {
		_ = network.Delete()
	}()

	req := &ncproxygrpc.GetNetworksRequest{}
	resp, err := gService.GetNetworks(ctx, req)
	if err != nil {
		t.Fatalf("expected no error, instead got %v", err)
	}
	if !networkExists(network.Name, resp.Networks) {
		t.Fatalf("failed to find created network")
	}
}

func TestRegisterComputeAgent(t *testing.T) {
	ctx := context.Background()

	// setup test ncproxy ttrpc service
	agentCache := newComputeAgentCache()
	tService := newTTRPCService(agentCache)

	// setup mocked calls
	winioDialPipe = func(path string, timeout *time.Duration) (net.Conn, error) {
		rPipe, _ := net.Pipe()
		return rPipe, nil
	}
	ttrpcNewClient = func(conn net.Conn, opts ...ttrpc.ClientOpts) *ttrpc.Client {
		return &ttrpc.Client{}
	}

	containerID := "test-container-id"
	req := &ncproxyttrpc.RegisterComputeAgentRequest{
		AgentAddress: "test-agent-address",
		ContainerID:  containerID,
	}
	if _, err := tService.RegisterComputeAgent(ctx, req); err != nil {
		t.Fatalf("expected to get no error, instead got %v", err)
	}

	// validate that the entry was added to the agent cache
	if _, exists := agentCache.get(containerID); !exists {
		t.Fatalf("compute agent client was not put into agent cache")
	}
}

func TestConfigureNetworking(t *testing.T) {
	ctx := context.Background()

	// setup test ncproxy ttrpc service
	agentCache := newComputeAgentCache()
	tService := newTTRPCService(agentCache)

	// setup mocked client and mocked calls for nodenetsvc
	nodeNetCtrl := gomock.NewController(t)
	defer nodeNetCtrl.Finish()
	mockedClient := ncproxyMock.NewMockNodeNetworkServiceClient(nodeNetCtrl)
	nodeNetSvcClient = &nodeNetSvcConn{
		addr:   "",
		client: mockedClient,
	}
	mockedClient.EXPECT().ConfigureNetworking(gomock.Any(), gomock.Any()).Return(&nodenetsvc.ConfigureNetworkingResponse{}, nil).AnyTimes()

	type config struct {
		name          string
		containerID   string
		requestType   ncproxyttrpc.RequestTypeInternal
		errorExpected bool
	}
	containerID := "test-container-id"
	tests := []config{
		{
			name:          "Configure Networking setup returns no error",
			containerID:   containerID,
			requestType:   ncproxyttrpc.RequestTypeInternal_Setup,
			errorExpected: false,
		},
		{
			name:          "Configure Networking teardown returns no error",
			containerID:   containerID,
			requestType:   ncproxyttrpc.RequestTypeInternal_Teardown,
			errorExpected: false,
		},
		{
			name:          "Configure Networking setup returns error when container ID is empty",
			containerID:   "",
			requestType:   ncproxyttrpc.RequestTypeInternal_Setup,
			errorExpected: true,
		},
		{
			name:          "Configure Networking setup returns error when request type is not supported",
			containerID:   containerID,
			requestType:   3, // unsupported request type
			errorExpected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(_ *testing.T) {
			req := &ncproxyttrpc.ConfigureNetworkingInternalRequest{
				ContainerID: test.containerID,
				RequestType: test.requestType,
			}
			_, err := tService.ConfigureNetworking(ctx, req)
			if test.errorExpected && err == nil {
				t.Fatalf("expected ConfigureNetworking to return an error")
			}
			if !test.errorExpected && err != nil {
				t.Fatalf("expected ConfigureNetworking to return no error, instead got %v", err)
			}
		})
	}
}
