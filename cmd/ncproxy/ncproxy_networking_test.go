package main

import (
	"context"
	"testing"

	"github.com/Microsoft/hcsshim/internal/computeagent"
	computeagentMock "github.com/Microsoft/hcsshim/internal/computeagent/mock"
	ncproxynetworking "github.com/Microsoft/hcsshim/internal/ncproxy/networking"
	ncproxygrpc "github.com/Microsoft/hcsshim/pkg/ncproxy/ncproxygrpc/v1"
	"github.com/golang/mock/gomock"
)

func TestAddNIC_NCProxy(t *testing.T) {
	ctx := context.Background()

	var (
		containerID      = t.Name() + "-containerID"
		testNICID        = t.Name() + "-nicID"
		testEndpointName = t.Name() + "-endpoint"
	)

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	// create mocked compute agent service
	computeAgentCtrl := gomock.NewController(t)
	defer computeAgentCtrl.Finish()
	mockedService := computeagentMock.NewMockComputeAgentService(computeAgentCtrl)
	mockedAgentClient := &computeAgentClient{nil, mockedService}

	// put mocked compute agent in agent cache for test
	if err := agentCache.put(containerID, mockedAgentClient); err != nil {
		t.Fatal(err)
	}

	// setup expected mocked calls
	mockedService.EXPECT().AddNIC(gomock.Any(), gomock.Any()).Return(&computeagent.AddNICInternalResponse{}, nil).AnyTimes()
	mockedService.EXPECT().AssignPCI(gomock.Any(), gomock.Any()).Return(&computeagent.AssignPCIInternalResponse{}, nil).AnyTimes()

	// add entry to database of ncproxy networking endpoints
	endpoint := &ncproxynetworking.Endpoint{
		EndpointName: testEndpointName,
		Settings: &ncproxynetworking.EndpointSettings{
			DeviceDetails: &ncproxynetworking.DeviceDetails{
				PCIDeviceDetails: &ncproxynetworking.PCIDeviceDetails{},
			},
		},
	}
	if err := gService.ncpNetworkingStore.CreatEndpoint(ctx, endpoint); err != nil {
		t.Fatal(err)
	}

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
		t.Run(test.name, func(subtest *testing.T) {
			req := &ncproxygrpc.AddNICRequest{
				ContainerID:  test.containerID,
				NicID:        test.nicID,
				EndpointName: test.endpointName,
			}

			_, err := gService.AddNIC(ctx, req)
			if test.errorExpected && err == nil {
				subtest.Fatalf("expected AddNIC to return an error")
			}
			if !test.errorExpected && err != nil {
				subtest.Fatalf("expected AddNIC to return no error, instead got %v", err)
			}
		})
	}
}

func TestDeleteNIC_NCProxy(t *testing.T) {
	ctx := context.Background()

	var (
		containerID      = t.Name() + "-containerID"
		testNICID        = t.Name() + "-nicID"
		testEndpointName = t.Name() + "-endpoint"
	)

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	// create mocked compute agent service
	computeAgentCtrl := gomock.NewController(t)
	defer computeAgentCtrl.Finish()
	mockedService := computeagentMock.NewMockComputeAgentService(computeAgentCtrl)
	mockedAgentClient := &computeAgentClient{nil, mockedService}

	// put mocked compute agent in agent cache for test
	if err := agentCache.put(containerID, mockedAgentClient); err != nil {
		t.Fatal(err)
	}

	// setup expected mocked calls
	mockedService.EXPECT().DeleteNIC(gomock.Any(), gomock.Any()).Return(&computeagent.DeleteNICInternalResponse{}, nil).AnyTimes()

	// add entry to ncproxy networking database to use during the test
	endpoint := &ncproxynetworking.Endpoint{
		EndpointName: testEndpointName,
		Settings: &ncproxynetworking.EndpointSettings{
			DeviceDetails: &ncproxynetworking.DeviceDetails{
				PCIDeviceDetails: &ncproxynetworking.PCIDeviceDetails{},
			},
		},
	}
	_ = gService.ncpNetworkingStore.CreatEndpoint(ctx, endpoint)

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
		t.Run(test.name, func(subtest *testing.T) {
			req := &ncproxygrpc.DeleteNICRequest{
				ContainerID:  test.containerID,
				NicID:        test.nicID,
				EndpointName: test.endpointName,
			}

			_, err := gService.DeleteNIC(ctx, req)
			if test.errorExpected && err == nil {
				subtest.Fatalf("expected DeleteNIC to return an error")
			}
			if !test.errorExpected && err != nil {
				subtest.Fatalf("expected DeleteNIC to return no error, instead got %v", err)
			}
		})
	}
}

func TestModifyNIC_NCProxy_Returns_Error(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	var (
		containerID      = t.Name() + "-containerID"
		testNICID        = t.Name() + "-nicID"
		testEndpointName = t.Name() + "-endpoint"
	)

	// create mock compute agent service
	computeAgentCtrl := gomock.NewController(t)
	defer computeAgentCtrl.Finish()
	mockedService := computeagentMock.NewMockComputeAgentService(computeAgentCtrl)
	mockedAgentClient := &computeAgentClient{nil, mockedService}

	// populate agent cache with mocked service for test
	if err := agentCache.put(containerID, mockedAgentClient); err != nil {
		t.Fatal(err)
	}

	// setup expected mocked calls
	mockedService.EXPECT().ModifyNIC(gomock.Any(), gomock.Any()).Return(&computeagent.ModifyNICInternalResponse{}, nil).AnyTimes()

	// add entry to cache of ncproxy networking endpoints
	endpoint := &ncproxynetworking.Endpoint{
		EndpointName: testEndpointName,
		Settings: &ncproxynetworking.EndpointSettings{
			DeviceDetails: &ncproxynetworking.DeviceDetails{
				PCIDeviceDetails: &ncproxynetworking.PCIDeviceDetails{},
			},
		},
	}
	_ = gService.ncpNetworkingStore.CreatEndpoint(ctx, endpoint)

	// create request
	settings := &ncproxygrpc.NCProxyEndpointSettings{}
	req := &ncproxygrpc.ModifyNICRequest{
		ContainerID:  containerID,
		NicID:        testNICID,
		EndpointName: testEndpointName,
		EndpointSettings: &ncproxygrpc.EndpointSettings{
			Settings: &ncproxygrpc.EndpointSettings_NcproxyEndpoint{
				NcproxyEndpoint: settings,
			},
		},
	}

	_, err = gService.ModifyNIC(ctx, req)
	if err == nil {
		t.Fatal("expected ModifyNIC to return an error for a ncproxy networking endpoint")
	}
}

func TestCreateNetwork_NCProxy(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	containerID := t.Name() + "-test-networkf"

	// create mock compute agent service
	computeAgentCtrl := gomock.NewController(t)
	defer computeAgentCtrl.Finish()
	mockedService := computeagentMock.NewMockComputeAgentService(computeAgentCtrl)
	mockedAgentClient := &computeAgentClient{nil, mockedService}

	// populate agent cache with mocked service for test
	if err := agentCache.put(containerID, mockedAgentClient); err != nil {
		t.Fatal(err)
	}

	// setup expected mocked calls
	mockedService.EXPECT().AssignPCI(gomock.Any(), gomock.Any()).Return(&computeagent.AssignPCIInternalResponse{}, nil).AnyTimes()

	type config struct {
		name          string
		networkName   string
		errorExpected bool
	}
	tests := []config{
		{
			name:          "CreateNetwork returns no error",
			networkName:   t.Name() + "-network",
			errorExpected: false,
		},
		{
			name:          "CreateNetwork returns error with blank network name",
			networkName:   "",
			errorExpected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(subtest *testing.T) {
			network := &ncproxygrpc.NCProxyNetworkSettings{
				Name: test.networkName,
			}
			req := &ncproxygrpc.CreateNetworkRequest{
				Network: &ncproxygrpc.Network{
					Settings: &ncproxygrpc.Network_NcproxyNetwork{
						NcproxyNetwork: network,
					},
				},
			}
			_, err := gService.CreateNetwork(ctx, req)
			if test.errorExpected && err == nil {
				subtest.Fatalf("expected CreateNetwork to return an error")
			}

			if !test.errorExpected {
				_, err := gService.ncpNetworkingStore.GetNetworkByName(ctx, test.networkName)
				if err != nil {
					subtest.Fatalf("failed to find created network with %v", err)
				}
			}
		})
	}
}

func TestCreateEndpoint_NCProxy(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	networkName := t.Name() + "-network"
	network := &ncproxynetworking.Network{
		NetworkName: networkName,
		Settings:    &ncproxynetworking.NetworkSettings{},
	}
	_ = gService.ncpNetworkingStore.CreateNetwork(ctx, network)

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
			networkName:   networkName,
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
			networkName:   networkName,
			ipaddress:     "",
			macaddress:    "00-15-5D-52-C0-00",
			errorExpected: true,
		},
		{
			name:          "CreateEndpoint returns error when mac address is empty",
			networkName:   networkName,
			ipaddress:     "192.168.100.4",
			macaddress:    "",
			errorExpected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(subtest *testing.T) {
			endpointName := subtest.Name() + "-endpoint-"
			endpoint := &ncproxygrpc.NCProxyEndpointSettings{
				Name:                  endpointName,
				Macaddress:            test.macaddress,
				Ipaddress:             test.ipaddress,
				IpaddressPrefixlength: 24,
				NetworkName:           test.networkName,
				DeviceDetails: &ncproxygrpc.NCProxyEndpointSettings_PciDeviceDetails{
					PciDeviceDetails: &ncproxygrpc.PCIDeviceDetails{},
				},
			}
			req := &ncproxygrpc.CreateEndpointRequest{
				EndpointSettings: &ncproxygrpc.EndpointSettings{
					Settings: &ncproxygrpc.EndpointSettings_NcproxyEndpoint{
						NcproxyEndpoint: endpoint,
					},
				},
			}

			_, err := gService.CreateEndpoint(ctx, req)
			if test.errorExpected && err == nil {
				subtest.Fatalf("expected CreateEndpoint to return an error")
			}
			if !test.errorExpected {
				if err != nil {
					subtest.Fatalf("expected to get no error, instead got %v", err)
				}
				_, err := gService.ncpNetworkingStore.GetEndpointByName(ctx, endpointName)
				if err != nil {
					subtest.Fatalf("failed to find created endpoint with %v", err)
				}
			}
		})
	}
}

func TestAddEndpoint_NoError_NCProxy(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	namespace := t.Name() + "-namespace"
	endpointName := t.Name() + "-endpoint"
	endpoint := &ncproxynetworking.Endpoint{
		EndpointName: endpointName,
		Settings: &ncproxynetworking.EndpointSettings{
			DeviceDetails: &ncproxynetworking.DeviceDetails{
				PCIDeviceDetails: &ncproxynetworking.PCIDeviceDetails{},
			},
		},
	}

	if err := networkingStore.CreatEndpoint(ctx, endpoint); err != nil {
		t.Fatal(err)
	}

	req := &ncproxygrpc.AddEndpointRequest{
		Name:        endpointName,
		NamespaceID: namespace,
	}

	_, err = gService.AddEndpoint(ctx, req)
	if err != nil {
		t.Fatalf("expected AddEndpoint to return no error, instead got %v", err)
	}
	updatedEndpt, err := networkingStore.GetEndpointByName(ctx, endpointName)
	if err != nil {
		t.Fatalf("expected to find endpoint in the networking store, instead got %v", err)
	}
	if updatedEndpt.NamespaceID != namespace {
		t.Fatalf("expected endpoint have namespace %s, instead got %s", namespace, updatedEndpt.NamespaceID)
	}
}

func TestAddEndpoint_Error_EmptyEndpointName_NCProxy(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	req := &ncproxygrpc.AddEndpointRequest{
		Name:        "",
		NamespaceID: t.Name() + "-namespace",
	}

	_, err = gService.AddEndpoint(ctx, req)
	if err == nil {
		t.Fatal("expected AddEndpoint to return error when endpoint name is empty")
	}
}

func TestAddEndpoint_Error_NoEndpoint_NCProxy(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	req := &ncproxygrpc.AddEndpointRequest{
		Name:        t.Name() + "-endpoint",
		NamespaceID: t.Name() + "-namespace",
	}

	_, err = gService.AddEndpoint(ctx, req)
	if err == nil {
		t.Fatal("expected AddEndpoint to return error when endpoint name is empty")
	}
}

func TestAddEndpoint_Error_EmptyNamespaceID_NCProxy(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	endpointName := t.Name() + "-test-endpoint"
	endpoint := &ncproxynetworking.Endpoint{
		EndpointName: endpointName,
		Settings: &ncproxynetworking.EndpointSettings{
			DeviceDetails: &ncproxynetworking.DeviceDetails{
				PCIDeviceDetails: &ncproxynetworking.PCIDeviceDetails{},
			},
		},
	}

	if err := networkingStore.CreatEndpoint(ctx, endpoint); err != nil {
		t.Fatal(err)
	}

	req := &ncproxygrpc.AddEndpointRequest{
		Name:        endpointName,
		NamespaceID: "",
	}

	_, err = gService.AddEndpoint(ctx, req)
	if err == nil {
		t.Fatal("expected AddEndpoint to return error when namespace ID is empty")
	}
}

func TestDeleteEndpoint_NoError_NCProxy(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	endpointName := t.Name() + "-test-endpoint"
	endpoint := &ncproxynetworking.Endpoint{
		EndpointName: endpointName,
		Settings: &ncproxynetworking.EndpointSettings{
			DeviceDetails: &ncproxynetworking.DeviceDetails{
				PCIDeviceDetails: &ncproxynetworking.PCIDeviceDetails{},
			},
		},
	}

	if err := networkingStore.CreatEndpoint(ctx, endpoint); err != nil {
		t.Fatal(err)
	}

	req := &ncproxygrpc.DeleteEndpointRequest{
		Name: endpointName,
	}
	_, err = gService.DeleteEndpoint(ctx, req)
	if err != nil {
		t.Fatalf("expected DeleteEndpoint to return no error, instead got %v", err)
	}
	actualEndpoint, err := networkingStore.GetEndpointByName(ctx, endpointName)
	if err == nil {
		t.Fatalf("expected endpoint to be deleted, instead found %v", actualEndpoint)
	}
}

func TestDeleteEndpoint_Error_NoEndpoint_NCProxy(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	req := &ncproxygrpc.DeleteEndpointRequest{
		Name: t.Name() + "-endpoint",
	}
	_, err = gService.DeleteEndpoint(ctx, req)
	if err == nil {
		t.Fatalf("expected to return an error on deleting nonexistent endpoint")
	}
}

func TestDeleteEndpoint_Error_EmptyEndpoint_Name_NCProxy(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	endpointName := ""
	req := &ncproxygrpc.DeleteEndpointRequest{
		Name: endpointName,
	}

	_, err = gService.DeleteEndpoint(ctx, req)
	if err == nil {
		t.Fatalf("expected to return an error when endpoint name is empty")
	}
}

func TestDeleteNetwork_NoError_NCProxy(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	networkName := t.Name() + "-network"
	network := &ncproxynetworking.Network{
		NetworkName: networkName,
		Settings:    &ncproxynetworking.NetworkSettings{},
	}
	if err := networkingStore.CreateNetwork(ctx, network); err != nil {
		t.Fatal(err)
	}

	req := &ncproxygrpc.DeleteNetworkRequest{
		Name: networkName,
	}
	_, err = gService.DeleteNetwork(ctx, req)
	if err != nil {
		t.Fatalf("expected no error, instead got %v", err)
	}
	actualNetwork, err := networkingStore.GetNetworkByName(ctx, networkName)
	if err == nil {
		t.Fatalf("expected network to be deleted, instead found %v", actualNetwork)
	}
}

func TestDeleteNetwork_Error_NoNetwork_NCProxy(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	fakeNetworkName := t.Name() + "-network"

	req := &ncproxygrpc.DeleteNetworkRequest{
		Name: fakeNetworkName,
	}
	_, err = gService.DeleteNetwork(ctx, req)
	if err == nil {
		t.Fatal("expected to get an error when attempting to delete nonexistent network")
	}
}

func TestDeleteNetwork_Error_EmptyNetworkName_NCProxy(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	req := &ncproxygrpc.DeleteNetworkRequest{
		Name: "",
	}
	_, err = gService.DeleteNetwork(ctx, req)
	if err == nil {
		t.Fatal("expected to get an error when attempting to delete nonexistent network")
	}
}

func TestGetEndpoint_NoError_NCProxy(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	endpointName := t.Name() + "-endpoint"
	endpoint := &ncproxynetworking.Endpoint{
		EndpointName: endpointName,
		Settings: &ncproxynetworking.EndpointSettings{
			DeviceDetails: &ncproxynetworking.DeviceDetails{
				PCIDeviceDetails: &ncproxynetworking.PCIDeviceDetails{},
			},
		},
	}
	if err := networkingStore.CreatEndpoint(ctx, endpoint); err != nil {
		t.Fatal(err)
	}
	req := &ncproxygrpc.GetEndpointRequest{
		Name: endpointName,
	}
	if _, err := gService.GetEndpoint(ctx, req); err != nil {
		t.Fatalf("expected to get no error, instead got %v", err)
	}
}

func TestGetEndpoint_Error_NoEndpoint_NCProxy(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	endpointName := t.Name() + "-endpoint"
	req := &ncproxygrpc.GetEndpointRequest{
		Name: endpointName,
	}

	if _, err := gService.GetEndpoint(ctx, req); err == nil {
		t.Fatal("expected to get an error trying to get a nonexistent endpoint")
	}
}

func TestGetEndpoint_Error_EmptyEndpointName_NCProxy(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	req := &ncproxygrpc.GetEndpointRequest{
		Name: "",
	}

	if _, err := gService.GetEndpoint(ctx, req); err == nil {
		t.Fatal("expected to get an error with empty endpoint name")
	}
}

func TestGetEndpoints_NoError_NCProxy(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	endpointName := t.Name() + "-endpoint"
	endpoint := &ncproxynetworking.Endpoint{
		EndpointName: endpointName,
		Settings: &ncproxynetworking.EndpointSettings{
			DeviceDetails: &ncproxynetworking.DeviceDetails{
				PCIDeviceDetails: &ncproxynetworking.PCIDeviceDetails{},
			},
		},
	}

	if err := networkingStore.CreatEndpoint(ctx, endpoint); err != nil {
		t.Fatal(err)
	}

	req := &ncproxygrpc.GetEndpointsRequest{}
	resp, err := gService.GetEndpoints(ctx, req)
	if err != nil {
		t.Fatalf("expected to get no error, instead got %v", err)
	}

	if !endpointExists(endpointName, resp.Endpoints) {
		t.Fatalf("created endpoint was not found")
	}
}

func TestGetNetwork_NoError_NCProxy(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	networkName := t.Name() + "-network"
	network := &ncproxynetworking.Network{
		NetworkName: networkName,
		Settings:    &ncproxynetworking.NetworkSettings{},
	}
	if err := networkingStore.CreateNetwork(ctx, network); err != nil {
		t.Fatal(err)
	}
	req := &ncproxygrpc.GetNetworkRequest{
		Name: networkName,
	}
	_, err = gService.GetNetwork(ctx, req)
	if err != nil {
		t.Fatalf("expected no error, instead got %v", err)
	}
}

func TestGetNetwork_Error_NoNetwork_NCProxy(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	fakeNetworkName := t.Name() + "-network"
	req := &ncproxygrpc.GetNetworkRequest{
		Name: fakeNetworkName,
	}
	_, err = gService.GetNetwork(ctx, req)
	if err == nil {
		t.Fatal("expected to get an error when attempting to get nonexistent network")
	}
}

func TestGetNetwork_Error_EmptyNetworkName_NCProxy(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	// don't actually create a network since we should fail before checking
	req := &ncproxygrpc.GetNetworkRequest{
		Name: "",
	}
	_, err = gService.GetNetwork(ctx, req)
	if err == nil {
		t.Fatal("expected to get an error when network name is empty")
	}
}

func TestGetNetworks_NoError_NCProxy(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	networkName := t.Name() + "-network"
	network := &ncproxynetworking.Network{
		NetworkName: networkName,
		Settings: &ncproxynetworking.NetworkSettings{
			Name: networkName,
		},
	}
	if err := networkingStore.CreateNetwork(ctx, network); err != nil {
		t.Fatal(err)
	}

	req := &ncproxygrpc.GetNetworksRequest{}
	resp, err := gService.GetNetworks(ctx, req)
	if err != nil {
		t.Fatalf("expected no error, instead got %v", err)
	}
	if !networkExists(networkName, resp.Networks) {
		t.Fatalf("failed to find created network, expected to find network with name %s, found network %v", networkName, resp.Networks)
	}
}
