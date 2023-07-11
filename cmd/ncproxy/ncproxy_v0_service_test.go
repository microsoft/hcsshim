package main

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/Microsoft/hcsshim/hcn"
	"github.com/Microsoft/hcsshim/internal/computeagent"
	computeagentMock "github.com/Microsoft/hcsshim/internal/computeagent/mock"
	"github.com/Microsoft/hcsshim/osversion"
	ncproxygrpcv0 "github.com/Microsoft/hcsshim/pkg/ncproxy/ncproxygrpc/v0"
	"go.uber.org/mock/gomock"
)

func TestAddNIC_V0_HCN(t *testing.T) {
	ctx := context.Background()
	containerID := t.Name() + "-containerID"
	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)
	v0Service := newV0ServiceWrapper(gService)

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

	// test network
	testNetworkName := t.Name() + "-network"
	network, err := createTestIPv4NATNetwork(testNetworkName)
	if err != nil {
		t.Fatalf("failed to create test network with %v", err)
	}
	defer func() {
		_ = network.Delete()
	}()

	testEndpointName := t.Name() + "-endpoint"
	endpoint, err := createTestEndpoint(testEndpointName, network.Id)
	if err != nil {
		t.Fatalf("failed to create test endpoint with %v", err)
	}
	// defer cleanup in case of error. ignore error from the delete call here
	// since we may have already successfully deleted the endpoint.
	defer func() {
		_ = endpoint.Delete()
	}()

	testNICID := t.Name() + "-nicID"
	req := &ncproxygrpcv0.AddNICRequest{
		ContainerID:  containerID,
		NicID:        testNICID,
		EndpointName: testEndpointName,
	}

	_, err = v0Service.AddNIC(ctx, req)
	if err != nil {
		t.Fatalf("expected AddNIC to return no error, instead got %v", err)
	}
}

func TestAddNIC_V0_HCN_Error_InvalidArgument(t *testing.T) {
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
	v0Service := newV0ServiceWrapper(gService)

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

	type config struct {
		name         string
		containerID  string
		nicID        string
		endpointName string
	}
	tests := []config{
		{
			name:         "AddNIC returns error with blank container ID",
			containerID:  "",
			nicID:        testNICID,
			endpointName: testEndpointName,
		},
		{
			name:         "AddNIC returns error with blank nic ID",
			containerID:  containerID,
			nicID:        "",
			endpointName: testEndpointName,
		},
		{
			name:         "AddNIC returns error with blank endpoint name",
			containerID:  containerID,
			nicID:        testNICID,
			endpointName: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(subtest *testing.T) {
			req := &ncproxygrpcv0.AddNICRequest{
				ContainerID:  test.containerID,
				NicID:        test.nicID,
				EndpointName: test.endpointName,
			}

			_, err := v0Service.AddNIC(ctx, req)
			if err == nil {
				subtest.Fatalf("expected AddNIC to return an error")
			}
		})
	}
}

func TestDeleteNIC_V0_HCN(t *testing.T) {
	ctx := context.Background()
	containerID := t.Name() + "-containerID"
	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)
	v0Service := newV0ServiceWrapper(gService)

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

	// test network
	network, err := createTestIPv4NATNetwork(t.Name() + "network")
	if err != nil {
		t.Fatalf("failed to create test network with %v", err)
	}
	defer func() {
		_ = network.Delete()
	}()

	testEndpointName := t.Name() + "-endpoint"
	endpoint, err := createTestEndpoint(testEndpointName, network.Id)
	if err != nil {
		t.Fatalf("failed to create test endpoint with %v", err)
	}
	// defer cleanup in case of error. ignore error from the delete call here
	// since we may have already successfully deleted the endpoint.
	defer func() {
		_ = endpoint.Delete()
	}()

	testNICID := t.Name() + "-nicID"
	req := &ncproxygrpcv0.DeleteNICRequest{
		ContainerID:  containerID,
		NicID:        testNICID,
		EndpointName: testEndpointName,
	}

	_, err = v0Service.DeleteNIC(ctx, req)
	if err != nil {
		t.Fatalf("expected DeleteNIC to return no error, instead got %v", err)
	}
}

func TestDeleteNIC_V0_HCN_Error_InvalidArgument(t *testing.T) {
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
	v0Service := newV0ServiceWrapper(gService)

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

	type config struct {
		name         string
		containerID  string
		nicID        string
		endpointName string
	}
	tests := []config{
		{
			name:         "DeleteNIC returns error with blank container ID",
			containerID:  "",
			nicID:        testNICID,
			endpointName: testEndpointName,
		},
		{
			name:         "DeleteNIC returns error with blank nic ID",
			containerID:  containerID,
			nicID:        "",
			endpointName: testEndpointName,
		},
		{
			name:         "DeleteNIC returns error with blank endpoint name",
			containerID:  containerID,
			nicID:        testNICID,
			endpointName: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(subtest *testing.T) {
			req := &ncproxygrpcv0.DeleteNICRequest{
				ContainerID:  test.containerID,
				NicID:        test.nicID,
				EndpointName: test.endpointName,
			}

			_, err := v0Service.DeleteNIC(ctx, req)
			if err == nil {
				subtest.Fatalf("expected DeleteNIC to return an error")
			}
		})
	}
}

func TestModifyNIC_V0_HCN(t *testing.T) {
	// support for setting IOV policy was added in 21H1
	if osversion.Build() < osversion.V21H1 {
		t.Skip("Requires build +21H1")
	}
	ctx := context.Background()
	containerID := t.Name() + "-containerID"

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)
	v0Service := newV0ServiceWrapper(gService)

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

	iovOffloadOn := &ncproxygrpcv0.IovEndpointPolicySetting{
		IovOffloadWeight: 100,
	}

	type config struct {
		name              string
		containerID       string
		iovPolicySettings *ncproxygrpcv0.IovEndpointPolicySetting
		networkCreateFunc func(string) (*hcn.HostComputeNetwork, error)
	}
	tests := []config{
		{
			name:              "ModifyNIC returns no error",
			containerID:       containerID,
			networkCreateFunc: createTestIPv4NATNetwork,
			iovPolicySettings: iovOffloadOn,
		},
		{
			name:              "ModifyNIC dual stack returns no error",
			containerID:       containerID,
			networkCreateFunc: createTestDualStackNATNetwork,
			iovPolicySettings: iovOffloadOn,
		},
		{
			name:              "ModifyNIC returns no error when turning off iov policy",
			containerID:       containerID,
			networkCreateFunc: createTestIPv4NATNetwork,
			iovPolicySettings: &ncproxygrpcv0.IovEndpointPolicySetting{
				IovOffloadWeight: 0,
			},
		},
		{
			name:              "ModifyNIC dual stack returns no error when turning off iov policy",
			containerID:       containerID,
			networkCreateFunc: createTestDualStackNATNetwork,
			iovPolicySettings: &ncproxygrpcv0.IovEndpointPolicySetting{
				IovOffloadWeight: 0,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(subtest *testing.T) {
			// create test network
			networkName := subtest.Name() + "-network"
			network, err := test.networkCreateFunc(networkName)
			if err != nil {
				subtest.Fatalf("failed to create test network with %v", err)
			}
			defer func() {
				_ = network.Delete()
			}()

			// create test endpoint
			endpointName := subtest.Name() + "-endpoint"
			endpoint, err := createTestEndpoint(endpointName, network.Id)
			if err != nil {
				subtest.Fatalf("failed to create test endpoint with %v", err)
			}
			defer func() {
				_ = endpoint.Delete()
			}()
			testNICID := subtest.Name() + "-nicID"
			req := &ncproxygrpcv0.ModifyNICRequest{
				ContainerID:       test.containerID,
				NicID:             testNICID,
				EndpointName:      endpointName,
				IovPolicySettings: test.iovPolicySettings,
			}

			_, err = v0Service.ModifyNIC(ctx, req)
			if err != nil {
				subtest.Fatalf("expected ModifyNIC to return no error, instead got %v", err)
			}
		})
	}
}

func TestModifyNIC_V0_HCN_Error_InvalidArgument(t *testing.T) {
	// support for setting IOV policy was added in 21H1
	if osversion.Build() < osversion.V21H1 {
		t.Skip("Requires build +21H1")
	}
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)
	v0Service := newV0ServiceWrapper(gService)

	var (
		containerID  = t.Name() + "-containerID"
		testNICID    = t.Name() + "-nicID"
		endpointName = t.Name() + "-endpoint"
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

	iovOffloadOn := &ncproxygrpcv0.IovEndpointPolicySetting{
		IovOffloadWeight: 100,
	}

	type config struct {
		name              string
		containerID       string
		nicID             string
		endpointName      string
		iovPolicySettings *ncproxygrpcv0.IovEndpointPolicySetting
	}
	tests := []config{
		{
			name:              "ModifyNIC returns error with blank container ID",
			containerID:       "",
			nicID:             testNICID,
			endpointName:      endpointName,
			iovPolicySettings: iovOffloadOn,
		},
		{
			name:              "ModifyNIC returns error with blank nic ID",
			containerID:       containerID,
			nicID:             "",
			endpointName:      endpointName,
			iovPolicySettings: iovOffloadOn,
		},
		{
			name:              "ModifyNIC returns error with blank endpoint name",
			containerID:       containerID,
			nicID:             testNICID,
			endpointName:      "",
			iovPolicySettings: iovOffloadOn,
		},
		{
			name:              "ModifyNIC returns error with blank iov policy settings",
			containerID:       containerID,
			nicID:             testNICID,
			endpointName:      endpointName,
			iovPolicySettings: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(subtest *testing.T) {
			req := &ncproxygrpcv0.ModifyNICRequest{
				ContainerID:       test.containerID,
				NicID:             test.nicID,
				EndpointName:      test.endpointName,
				IovPolicySettings: test.iovPolicySettings,
			}

			_, err := v0Service.ModifyNIC(ctx, req)
			if err == nil {
				subtest.Fatalf("expected ModifyNIC to return an error")
			}
		})
	}
}

func TestCreateNetwork_V0_HCN(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)
	v0Service := newV0ServiceWrapper(gService)

	networkName := t.Name() + "-network"
	ipv4Subnets := getTestIPv4Subnets()
	req := &ncproxygrpcv0.CreateNetworkRequest{
		Name:                  networkName,
		Mode:                  ncproxygrpcv0.CreateNetworkRequest_NAT,
		SubnetIpaddressPrefix: []string{ipv4Subnets[0].IpAddressPrefix},
		DefaultGateway:        ipv4Subnets[0].Routes[0].NextHop,
	}
	_, err = v0Service.CreateNetwork(ctx, req)
	if err != nil {
		t.Fatalf("expected CreateNetwork to return no error, instead got %v", err)
	}
	// validate that the network exists
	network, err := hcn.GetNetworkByName(networkName)
	if err != nil {
		t.Fatalf("failed to find created network with %v", err)
	}
	// cleanup the created network
	if err = network.Delete(); err != nil {
		t.Fatalf("failed to cleanup network %v created by test with %v", networkName, err)
	}
}

func TestCreateNetwork_V0_HCN_Error_EmptyNetworkName(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)
	v0Service := newV0ServiceWrapper(gService)

	req := &ncproxygrpcv0.CreateNetworkRequest{
		Name: "",
		Mode: ncproxygrpcv0.CreateNetworkRequest_Transparent,
	}
	_, err = v0Service.CreateNetwork(ctx, req)
	if err == nil {
		t.Fatalf("expected CreateNetwork to return an error")
	}
}

func TestCreateEndpoint_V0_HCN(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)
	v0Service := newV0ServiceWrapper(gService)

	// test network
	networkName := t.Name() + "-network"
	network, err := createTestIPv4NATNetwork(networkName)
	if err != nil {
		t.Fatalf("failed to create test network with %v", err)
	}
	defer func() {
		_ = network.Delete()
	}()

	endpointName := t.Name() + "-endpoint"
	req := &ncproxygrpcv0.CreateEndpointRequest{
		Name:                  endpointName,
		Macaddress:            "00-15-5D-52-C0-00",
		Ipaddress:             "192.168.100.4",
		IpaddressPrefixlength: "24",
		NetworkName:           networkName,
	}

	_, err = v0Service.CreateEndpoint(ctx, req)
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

func TestCreateEndpoint_V0_HCN_Error_InvalidArgument(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)
	v0Service := newV0ServiceWrapper(gService)

	type config struct {
		name        string
		networkName string
		ipaddress   string
		macaddress  string
	}
	tests := []config{
		{
			name:        "CreateEndpoint returns error when network name is empty",
			networkName: "",
			ipaddress:   "192.168.100.4",
			macaddress:  "00-15-5D-52-C0-00",
		},
		{
			name:        "CreateEndpoint returns error when ip address is empty",
			networkName: "testName",
			ipaddress:   "",
			macaddress:  "00-15-5D-52-C0-00",
		},
		{
			name:        "CreateEndpoint returns error when mac address is empty",
			networkName: "testName",
			ipaddress:   "192.168.100.4",
			macaddress:  "",
		},
	}

	for i, test := range tests {
		t.Run(test.name, func(subtest *testing.T) {
			endpointName := t.Name() + "-endpoint-" + strconv.Itoa(i)

			req := &ncproxygrpcv0.CreateEndpointRequest{
				Name:        endpointName,
				Macaddress:  test.macaddress,
				Ipaddress:   test.ipaddress,
				NetworkName: test.networkName,
			}

			_, err = v0Service.CreateEndpoint(ctx, req)
			if err == nil {
				subtest.Fatalf("expected CreateEndpoint to return an error")
			}
		})
	}
}

func TestAddEndpoint_V0_NoError(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)
	v0Service := newV0ServiceWrapper(gService)

	// create test network namespace
	// we need to create a (host) namespace other than the HostDefault to differentiate between
	// the nominal AddEndpoint functionality, and when specifying attach to host
	// the DefaultHost namespace is retrieved below.
	namespace := hcn.NewNamespace(hcn.NamespaceTypeHost)
	namespace, err = namespace.Create()
	if err != nil {
		t.Fatalf("failed to create test namespace with %v", err)
	}
	defer func() {
		_ = namespace.Delete()
	}()

	// test network
	networkName := t.Name() + "-network"
	network, err := createTestIPv4NATNetwork(networkName)
	if err != nil {
		t.Fatalf("failed to create test network with %v", err)
	}
	defer func() {
		_ = network.Delete()
	}()

	endpointName := t.Name() + "-endpoint"
	endpoint, err := createTestEndpoint(endpointName, network.Id)
	if err != nil {
		t.Fatalf("failed to create test endpoint with %v", err)
	}
	defer func() {
		_ = endpoint.Delete()
	}()

	req := &ncproxygrpcv0.AddEndpointRequest{
		Name:        endpointName,
		NamespaceID: namespace.Id,
	}

	_, err = v0Service.AddEndpoint(ctx, req)
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

func TestAddEndpoint_V0_Error_EmptyEndpointName(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)
	v0Service := newV0ServiceWrapper(gService)

	// create test network namespace
	namespace := hcn.NewNamespace(hcn.NamespaceTypeHostDefault)
	namespace, err = namespace.Create()
	if err != nil {
		t.Fatalf("failed to create test namespace with %v", err)
	}
	defer func() {
		_ = namespace.Delete()
	}()

	req := &ncproxygrpcv0.AddEndpointRequest{
		Name:        "",
		NamespaceID: namespace.Id,
	}

	_, err = v0Service.AddEndpoint(ctx, req)
	if err == nil {
		t.Fatal("expected AddEndpoint to return error when endpoint name is empty")
	}
}

func TestAddEndpoint_V0_Error_NoEndpoint(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)
	v0Service := newV0ServiceWrapper(gService)

	// create test network namespace
	namespace := hcn.NewNamespace(hcn.NamespaceTypeHostDefault)
	namespace, err = namespace.Create()
	if err != nil {
		t.Fatalf("failed to create test namespace with %v", err)
	}
	defer func() {
		_ = namespace.Delete()
	}()

	req := &ncproxygrpcv0.AddEndpointRequest{
		Name:        t.Name() + "-endpoint",
		NamespaceID: namespace.Id,
	}

	_, err = v0Service.AddEndpoint(ctx, req)
	if err == nil {
		t.Fatal("expected AddEndpoint to return error when endpoint name is empty")
	}
}

func TestAddEndpoint_V0_Error_EmptyNamespaceID(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)
	v0Service := newV0ServiceWrapper(gService)

	// test network
	networkName := t.Name() + "-network"
	network, err := createTestIPv4NATNetwork(networkName)
	if err != nil {
		t.Fatalf("failed to create test network with %v", err)
	}
	defer func() {
		_ = network.Delete()
	}()

	endpointName := t.Name() + "-endpoint"
	endpoint, err := createTestEndpoint(endpointName, network.Id)
	if err != nil {
		t.Fatalf("failed to create test endpoint with %v", err)
	}
	defer func() {
		_ = endpoint.Delete()
	}()

	req := &ncproxygrpcv0.AddEndpointRequest{
		Name:        endpointName,
		NamespaceID: "",
	}

	_, err = v0Service.AddEndpoint(ctx, req)
	if err == nil {
		t.Fatal("expected AddEndpoint to return error when namespace ID is empty")
	}
}

func TestDeleteEndpoint_V0_NoError(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)
	v0Service := newV0ServiceWrapper(gService)

	// test network
	networkName := t.Name() + "-network"
	network, err := createTestIPv4NATNetwork(networkName)
	if err != nil {
		t.Fatalf("failed to create test network with %v", err)
	}
	// defer cleanup in case of error. ignore error from the delete call here
	// since we may have already successfully deleted the network.
	defer func() {
		_ = network.Delete()
	}()

	endpointName := t.Name() + "-endpoint"
	endpoint, err := createTestEndpoint(endpointName, network.Id)
	if err != nil {
		t.Fatalf("failed to create test endpoint with %v", err)
	}
	// defer cleanup in case of error. ignore error from the delete call here
	// since we may have already successfully deleted the endpoint.
	defer func() {
		_ = endpoint.Delete()
	}()

	req := &ncproxygrpcv0.DeleteEndpointRequest{
		Name: endpointName,
	}
	_, err = v0Service.DeleteEndpoint(ctx, req)
	if err != nil {
		t.Fatalf("expected DeleteEndpoint to return no error, instead got %v", err)
	}
	// validate that the endpoint was created
	ep, err := hcn.GetEndpointByName(endpointName)
	if err == nil {
		t.Fatalf("expected endpoint to be deleted, instead found %v", ep)
	}
}

func TestDeleteEndpoint_V0_Error_NoEndpoint(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)
	v0Service := newV0ServiceWrapper(gService)

	// test network
	networkName := t.Name() + "-network"
	network, err := createTestIPv4NATNetwork(networkName)
	if err != nil {
		t.Fatalf("failed to create test network with %v", err)
	}
	defer func() {
		_ = network.Delete()
	}()

	endpointName := t.Name() + "-endpoint"
	req := &ncproxygrpcv0.DeleteEndpointRequest{
		Name: endpointName,
	}

	_, err = v0Service.DeleteEndpoint(ctx, req)
	if err == nil {
		t.Fatalf("expected to return an error on deleting nonexistent endpoint")
	}
}

func TestDeleteEndpoint_V0_Error_EmptyEndpoint_Name(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)
	v0Service := newV0ServiceWrapper(gService)

	endpointName := ""
	req := &ncproxygrpcv0.DeleteEndpointRequest{
		Name: endpointName,
	}

	_, err = v0Service.DeleteEndpoint(ctx, req)
	if err == nil {
		t.Fatalf("expected to return an error when endpoint name is empty")
	}
}

func TestDeleteNetwork_V0_NoError(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)
	v0Service := newV0ServiceWrapper(gService)

	// test network
	networkName := t.Name() + "-network"
	network, err := createTestIPv4NATNetwork(networkName)
	if err != nil {
		t.Fatalf("failed to create test network with %v", err)
	}
	// defer cleanup in case of error. ignore error from the delete call here
	// since we may have already successfully deleted the network.
	defer func() {
		_ = network.Delete()
	}()

	req := &ncproxygrpcv0.DeleteNetworkRequest{
		Name: networkName,
	}
	_, err = v0Service.DeleteNetwork(ctx, req)
	if err != nil {
		t.Fatalf("expected no error, instead got %v", err)
	}
}

func TestDeleteNetwork_V0_Error_NoNetwork(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)
	v0Service := newV0ServiceWrapper(gService)

	fakeNetworkName := t.Name() + "-network"

	req := &ncproxygrpcv0.DeleteNetworkRequest{
		Name: fakeNetworkName,
	}
	_, err = v0Service.DeleteNetwork(ctx, req)
	if err == nil {
		t.Fatal("expected to get an error when attempting to delete nonexistent network")
	}
}

func TestDeleteNetwork_V0_Error_EmptyNetworkName(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)
	v0Service := newV0ServiceWrapper(gService)

	req := &ncproxygrpcv0.DeleteNetworkRequest{
		Name: "",
	}
	_, err = v0Service.DeleteNetwork(ctx, req)
	if err == nil {
		t.Fatal("expected to get an error when attempting to delete nonexistent network")
	}
}

func TestGetEndpoint_V0_NoError(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)
	v0Service := newV0ServiceWrapper(gService)

	// test network
	networkName := t.Name() + "-network"
	network, err := createTestIPv4NATNetwork(networkName)
	if err != nil {
		t.Fatalf("failed to create test network with %v", err)
	}
	defer func() {
		_ = network.Delete()
	}()

	// test endpoint
	endpointName := t.Name() + "-endpoint"
	endpoint, err := createTestEndpoint(endpointName, network.Id)
	if err != nil {
		t.Fatalf("failed to create test endpoint with %v", err)
	}
	defer func() {
		_ = endpoint.Delete()
	}()

	req := &ncproxygrpcv0.GetEndpointRequest{
		Name: endpointName,
	}

	if _, err := v0Service.GetEndpoint(ctx, req); err != nil {
		t.Fatalf("expected to get no error, instead got %v", err)
	}
}

func TestGetEndpoint_V0_Error_NoEndpoint(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)
	v0Service := newV0ServiceWrapper(gService)

	endpointName := t.Name() + "-endpoint"
	req := &ncproxygrpcv0.GetEndpointRequest{
		Name: endpointName,
	}

	if _, err := v0Service.GetEndpoint(ctx, req); err == nil {
		t.Fatal("expected to get an error trying to get a nonexistent endpoint")
	}
}

func TestGetEndpoint_V0_Error_EmptyEndpointName(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)
	v0Service := newV0ServiceWrapper(gService)

	req := &ncproxygrpcv0.GetEndpointRequest{
		Name: "",
	}

	if _, err := v0Service.GetEndpoint(ctx, req); err == nil {
		t.Fatal("expected to get an error with empty endpoint name")
	}
}

func TestGetEndpoints_V0_NoError(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)
	v0Service := newV0ServiceWrapper(gService)

	// test network
	networkName := t.Name() + "-network"
	network, err := createTestIPv4NATNetwork(networkName)
	if err != nil {
		t.Fatalf("failed to create test network with %v", err)
	}
	defer func() {
		_ = network.Delete()
	}()

	// test endpoint
	endpointName := t.Name() + "-endpoint"
	endpoint, err := createTestEndpoint(endpointName, network.Id)
	if err != nil {
		t.Fatalf("failed to create test endpoint with %v", err)
	}
	defer func() {
		_ = endpoint.Delete()
	}()

	req := &ncproxygrpcv0.GetEndpointsRequest{}
	resp, err := v0Service.GetEndpoints(ctx, req)
	if err != nil {
		t.Fatalf("expected to get no error, instead got %v", err)
	}

	if !v0EndpointExists(endpointName, resp.Endpoints) {
		t.Fatalf("created endpoint was not found")
	}
}

func TestGetNetwork_V0_NoError(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)
	v0Service := newV0ServiceWrapper(gService)

	// create the test network
	networkName := t.Name() + "-network"
	network, err := createTestIPv4NATNetwork(networkName)
	if err != nil {
		t.Fatalf("failed to create test network with %v", err)
	}
	// defer cleanup in case of error. ignore error from the delete call here
	// since we may have already successfully deleted the network.
	defer func() {
		_ = network.Delete()
	}()

	req := &ncproxygrpcv0.GetNetworkRequest{
		Name: networkName,
	}
	_, err = v0Service.GetNetwork(ctx, req)
	if err != nil {
		t.Fatalf("expected no error, instead got %v", err)
	}
}

func TestGetNetwork_V0_Error_NoNetwork(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)
	v0Service := newV0ServiceWrapper(gService)

	fakeNetworkName := t.Name() + "-network"

	req := &ncproxygrpcv0.GetNetworkRequest{
		Name: fakeNetworkName,
	}
	_, err = v0Service.GetNetwork(ctx, req)
	if err == nil {
		t.Fatal("expected to get an error when attempting to get nonexistent network")
	}
}

func TestGetNetwork_V0_Error_EmptyNetworkName(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)
	v0Service := newV0ServiceWrapper(gService)

	req := &ncproxygrpcv0.GetNetworkRequest{
		Name: "",
	}
	_, err = v0Service.GetNetwork(ctx, req)
	if err == nil {
		t.Fatal("expected to get an error when network name is empty")
	}
}

func TestGetNetworks_V0_NoError(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)
	v0Service := newV0ServiceWrapper(gService)

	// create the test network
	networkName := t.Name() + "-network"
	network, err := createTestIPv4NATNetwork(networkName)
	if err != nil {
		t.Fatalf("failed to create test network with %v", err)
	}
	// defer cleanup in case of error. ignore error from the delete call here
	// since we may have already successfully deleted the network.
	defer func() {
		_ = network.Delete()
	}()

	req := &ncproxygrpcv0.GetNetworksRequest{}
	resp, err := v0Service.GetNetworks(ctx, req)
	if err != nil {
		t.Fatalf("expected no error, instead got %v", err)
	}
	if !v0NetworkExists(networkName, resp.Networks) {
		t.Fatalf("failed to find created network")
	}
}

func v0NetworkExists(targetName string, networks []*ncproxygrpcv0.GetNetworkResponse) bool {
	for _, resp := range networks {
		if resp.Name == targetName {
			return true
		}
	}
	return false
}

func v0EndpointExists(targetName string, endpoints []*ncproxygrpcv0.GetEndpointResponse) bool {
	for _, resp := range endpoints {
		if resp.Name == targetName {
			return true
		}
	}
	return false
}
