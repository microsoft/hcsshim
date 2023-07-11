//go:build windows

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
	ncproxygrpc "github.com/Microsoft/hcsshim/pkg/ncproxy/ncproxygrpc/v1"
	"go.uber.org/mock/gomock"
)

const (
	startMACAddress = "00-15-5D-52-C0-00"
	endMACAddress   = "00-15-5D-52-CF-FF"
)

func getTestIPv4Subnets() []hcn.Subnet {
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

func getTestIPv6Subnets() []hcn.Subnet {
	testSubnet := hcn.Subnet{
		IpAddressPrefix: "2001:db8:abcd:0012::0/64",
		Routes: []hcn.Route{
			{
				NextHop:           "2001:db8:abcd:0012::1",
				DestinationPrefix: "::/0",
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

func createTestIPv4NATNetwork(name string) (*hcn.HostComputeNetwork, error) {
	ipam := hcn.Ipam{
		Type:    "Static",
		Subnets: getTestIPv4Subnets(),
	}
	ipams := []hcn.Ipam{ipam}
	network := &hcn.HostComputeNetwork{
		Type: hcn.NAT,
		Name: name,
		MacPool: hcn.MacPool{
			Ranges: []hcn.MacRange{
				{
					StartMacAddress: startMACAddress,
					EndMacAddress:   endMACAddress,
				},
			},
		},
		Ipams:         ipams,
		SchemaVersion: hcn.V2SchemaVersion(),
	}
	return network.Create()
}

func createTestDualStackNATNetwork(name string) (*hcn.HostComputeNetwork, error) {
	ipam := hcn.Ipam{
		Type:    "Static",
		Subnets: append(getTestIPv4Subnets(), getTestIPv6Subnets()...),
	}
	ipams := []hcn.Ipam{ipam}
	network := &hcn.HostComputeNetwork{
		Type: hcn.NAT,
		Name: name,
		MacPool: hcn.MacPool{
			Ranges: []hcn.MacRange{
				{
					StartMacAddress: startMACAddress,
					EndMacAddress:   endMACAddress,
				},
			},
		},
		Ipams:         ipams,
		SchemaVersion: hcn.V2SchemaVersion(),
	}
	return network.Create()
}

func TestAddNIC_HCN(t *testing.T) {
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
		name              string
		containerID       string
		iovPolicySettings *ncproxygrpc.IovEndpointPolicySetting
		networkCreateFunc func(string) (*hcn.HostComputeNetwork, error)
	}
	tests := []config{
		{
			name:              "AddNIC returns no error",
			containerID:       containerID,
			networkCreateFunc: createTestIPv4NATNetwork,
		},
		{
			name:              "AddNIC dual stack returns no error",
			containerID:       containerID,
			networkCreateFunc: createTestDualStackNATNetwork,
		},
		{
			name:              "AddNIC returns no error with iov policy set",
			containerID:       containerID,
			networkCreateFunc: createTestIPv4NATNetwork,
			iovPolicySettings: &ncproxygrpc.IovEndpointPolicySetting{
				IovOffloadWeight: 100,
			},
		},
		{
			name:              "AddNIC dual stack returns no error with iov policy set",
			containerID:       containerID,
			networkCreateFunc: createTestDualStackNATNetwork,
			iovPolicySettings: &ncproxygrpc.IovEndpointPolicySetting{
				IovOffloadWeight: 100,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(subtest *testing.T) {
			// test network
			testNetworkName := subtest.Name() + "-network"
			network, err := test.networkCreateFunc(testNetworkName)
			if err != nil {
				subtest.Fatalf("failed to create test network with %v", err)
			}
			defer func() {
				_ = network.Delete()
			}()

			testEndpointName := subtest.Name() + "-endpoint"
			endpoint, err := createTestEndpoint(testEndpointName, network.Id)
			if err != nil {
				subtest.Fatalf("failed to create test endpoint with %v", err)
			}
			// defer cleanup in case of error. ignore error from the delete call here
			// since we may have already successfully deleted the endpoint.
			defer func() {
				_ = endpoint.Delete()
			}()

			endpointSettings := &ncproxygrpc.EndpointSettings{}
			if test.iovPolicySettings != nil {
				if osversion.Build() < osversion.V21H1 {
					subtest.Skip("Requires build +21H1")
				}
				endpointSettings.Settings = &ncproxygrpc.EndpointSettings_HcnEndpoint{
					HcnEndpoint: &ncproxygrpc.HcnEndpointSettings{
						Policies: &ncproxygrpc.HcnEndpointPolicies{
							IovPolicySettings: test.iovPolicySettings,
						},
					},
				}
			}
			testNICID := subtest.Name() + "-nicID"
			req := &ncproxygrpc.AddNICRequest{
				ContainerID:      test.containerID,
				NicID:            testNICID,
				EndpointName:     testEndpointName,
				EndpointSettings: endpointSettings,
			}

			_, err = gService.AddNIC(ctx, req)
			if err != nil {
				subtest.Fatalf("expected AddNIC to return no error, instead got %v", err)
			}
		})
	}
}

func TestAddNIC_HCN_Error_InvalidArgument(t *testing.T) {
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
			endpointSettings := &ncproxygrpc.EndpointSettings{}
			req := &ncproxygrpc.AddNICRequest{
				ContainerID:      test.containerID,
				NicID:            test.nicID,
				EndpointName:     test.endpointName,
				EndpointSettings: endpointSettings,
			}

			_, err := gService.AddNIC(ctx, req)
			if err == nil {
				subtest.Fatalf("expected AddNIC to return an error")
			}
		})
	}
}

func TestDeleteNIC_HCN(t *testing.T) {
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
		name              string
		containerID       string
		networkCreateFunc func(string) (*hcn.HostComputeNetwork, error)
	}
	tests := []config{
		{
			name:              "DeleteNIC returns no error",
			containerID:       containerID,
			networkCreateFunc: createTestIPv4NATNetwork,
		},
		{
			name:              "DeleteNIC dual stack returns no error",
			containerID:       containerID,
			networkCreateFunc: createTestDualStackNATNetwork,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(subtest *testing.T) {
			// test network
			network, err := test.networkCreateFunc(subtest.Name() + "network")
			if err != nil {
				subtest.Fatalf("failed to create test network with %v", err)
			}
			defer func() {
				_ = network.Delete()
			}()

			testEndpointName := subtest.Name() + "-endpoint"
			endpoint, err := createTestEndpoint(testEndpointName, network.Id)
			if err != nil {
				subtest.Fatalf("failed to create test endpoint with %v", err)
			}
			// defer cleanup in case of error. ignore error from the delete call here
			// since we may have already successfully deleted the endpoint.
			defer func() {
				_ = endpoint.Delete()
			}()

			testNICID := subtest.Name() + "-nicID"
			req := &ncproxygrpc.DeleteNICRequest{
				ContainerID:  test.containerID,
				NicID:        testNICID,
				EndpointName: testEndpointName,
			}

			_, err = gService.DeleteNIC(ctx, req)
			if err != nil {
				subtest.Fatalf("expected DeleteNIC to return no error, instead got %v", err)
			}
		})
	}
}

func TestDeleteNIC_HCN_Error_InvalidArgument(t *testing.T) {
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
			req := &ncproxygrpc.DeleteNICRequest{
				ContainerID:  test.containerID,
				NicID:        test.nicID,
				EndpointName: test.endpointName,
			}

			_, err := gService.DeleteNIC(ctx, req)
			if err == nil {
				subtest.Fatalf("expected DeleteNIC to return an error")
			}
		})
	}
}

func TestModifyNIC_HCN(t *testing.T) {
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

	iovOffloadOn := &ncproxygrpc.IovEndpointPolicySetting{
		IovOffloadWeight: 100,
	}

	type config struct {
		name              string
		containerID       string
		iovPolicySettings *ncproxygrpc.IovEndpointPolicySetting
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
			iovPolicySettings: &ncproxygrpc.IovEndpointPolicySetting{
				IovOffloadWeight: 0,
			},
		},
		{
			name:              "ModifyNIC dual stack returns no error when turning off iov policy",
			containerID:       containerID,
			networkCreateFunc: createTestDualStackNATNetwork,
			iovPolicySettings: &ncproxygrpc.IovEndpointPolicySetting{
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
			endpointSettings := &ncproxygrpc.HcnEndpointSettings{
				Policies: &ncproxygrpc.HcnEndpointPolicies{
					IovPolicySettings: test.iovPolicySettings,
				},
			}
			testNICID := subtest.Name() + "-nicID"
			req := &ncproxygrpc.ModifyNICRequest{
				ContainerID:  test.containerID,
				NicID:        testNICID,
				EndpointName: endpointName,
				EndpointSettings: &ncproxygrpc.EndpointSettings{
					Settings: &ncproxygrpc.EndpointSettings_HcnEndpoint{
						HcnEndpoint: endpointSettings,
					},
				},
			}

			_, err = gService.ModifyNIC(ctx, req)
			if err != nil {
				subtest.Fatalf("expected ModifyNIC to return no error, instead got %v", err)
			}
		})
	}
}

func TestModifyNIC_HCN_Error_InvalidArgument(t *testing.T) {
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

	iovOffloadOn := &ncproxygrpc.IovEndpointPolicySetting{
		IovOffloadWeight: 100,
	}

	type config struct {
		name              string
		containerID       string
		nicID             string
		endpointName      string
		iovPolicySettings *ncproxygrpc.IovEndpointPolicySetting
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
			endpoint := &ncproxygrpc.HcnEndpointSettings{
				Policies: &ncproxygrpc.HcnEndpointPolicies{
					IovPolicySettings: test.iovPolicySettings,
				},
			}
			req := &ncproxygrpc.ModifyNICRequest{
				ContainerID:  test.containerID,
				NicID:        test.nicID,
				EndpointName: test.endpointName,
				EndpointSettings: &ncproxygrpc.EndpointSettings{
					Settings: &ncproxygrpc.EndpointSettings_HcnEndpoint{
						HcnEndpoint: endpoint,
					},
				},
			}

			_, err := gService.ModifyNIC(ctx, req)
			if err == nil {
				subtest.Fatalf("expected ModifyNIC to return an error")
			}
		})
	}
}

func TestCreateNetwork_HCN(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	type config struct {
		name      string
		dualStack bool
	}
	tests := []config{
		{
			name:      "CreateNetwork returns no error",
			dualStack: false,
		},
		{
			name:      "CreateNetwork dual stack returns no error",
			dualStack: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(subtest *testing.T) {
			networkName := subtest.Name() + "-network"
			ipv4Subnets := getTestIPv4Subnets()
			networkSettings := &ncproxygrpc.HostComputeNetworkSettings{
				Name:                  networkName,
				Mode:                  ncproxygrpc.HostComputeNetworkSettings_NAT,
				SubnetIpaddressPrefix: []string{ipv4Subnets[0].IpAddressPrefix},
				DefaultGateway:        ipv4Subnets[0].Routes[0].NextHop,
			}

			if test.dualStack {
				if err := hcn.IPv6DualStackSupported(); err != nil {
					subtest.Skip("dual stack not supported, skipping test")
				}
				ipv6Subnets := getTestIPv6Subnets()
				networkSettings.SubnetIpaddressPrefixIpv6 = []string{ipv6Subnets[0].IpAddressPrefix}
				networkSettings.DefaultGatewayIpv6 = ipv6Subnets[0].Routes[0].NextHop
			}

			req := &ncproxygrpc.CreateNetworkRequest{
				Network: &ncproxygrpc.Network{
					Settings: &ncproxygrpc.Network_HcnNetwork{
						HcnNetwork: networkSettings,
					},
				},
			}
			_, err := gService.CreateNetwork(ctx, req)
			if err != nil {
				subtest.Fatalf("expected CreateNetwork to return no error, instead got %v", err)
			}
			// validate that the network exists
			network, err := hcn.GetNetworkByName(networkName)
			if err != nil {
				subtest.Fatalf("failed to find created network with %v", err)
			}
			// cleanup the created network
			if err = network.Delete(); err != nil {
				subtest.Fatalf("failed to cleanup network %v created by test with %v", networkName, err)
			}
		})
	}
}

func TestCreateNetwork_HCN_Error_EmptyNetworkName(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	network := &ncproxygrpc.HostComputeNetworkSettings{
		Name: "",
		Mode: ncproxygrpc.HostComputeNetworkSettings_NAT,
	}
	req := &ncproxygrpc.CreateNetworkRequest{
		Network: &ncproxygrpc.Network{
			Settings: &ncproxygrpc.Network_HcnNetwork{
				HcnNetwork: network,
			},
		},
	}
	_, err = gService.CreateNetwork(ctx, req)
	if err == nil {
		t.Fatalf("expected CreateNetwork to return an error")
	}
}

func TestCreateEndpoint_HCN(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	type config struct {
		name              string
		ipaddress         string
		ipv6Address       string
		macaddress        string
		networkCreateFunc func(string) (*hcn.HostComputeNetwork, error)
	}

	tests := []config{
		{
			name:              "CreateEndpoint returns no error",
			ipaddress:         "192.168.100.4",
			macaddress:        "00-15-5D-52-C0-00",
			networkCreateFunc: createTestIPv4NATNetwork,
		},
		{
			name:              "CreateEndpoint dual stack returns no error",
			ipaddress:         "192.168.100.5",
			ipv6Address:       "2001:db8:abcd:0012::3",
			macaddress:        "00-15-5D-52-C0-10",
			networkCreateFunc: createTestDualStackNATNetwork,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(subtest *testing.T) {
			// test network
			networkName := subtest.Name() + "-network"
			network, err := test.networkCreateFunc(networkName)
			if err != nil {
				subtest.Fatalf("failed to create test network with %v", err)
			}
			defer func() {
				_ = network.Delete()
			}()

			endpointName := subtest.Name() + "-endpoint"
			endpoint := &ncproxygrpc.HcnEndpointSettings{
				Name:                  endpointName,
				Macaddress:            test.macaddress,
				Ipaddress:             test.ipaddress,
				IpaddressPrefixlength: 24,
				NetworkName:           networkName,
			}
			if test.ipv6Address != "" {
				if err := hcn.IPv6DualStackSupported(); err != nil {
					subtest.Skip("dual stack not supported, skipping test")
				}
				endpoint.Ipv6Address = test.ipv6Address
				endpoint.Ipv6AddressPrefixlength = 64
			}
			req := &ncproxygrpc.CreateEndpointRequest{
				EndpointSettings: &ncproxygrpc.EndpointSettings{
					Settings: &ncproxygrpc.EndpointSettings_HcnEndpoint{
						HcnEndpoint: endpoint,
					},
				},
			}

			_, err = gService.CreateEndpoint(ctx, req)
			if err != nil {
				subtest.Fatalf("expected CreateEndpoint to return no error, instead got %v", err)
			}
			// validate that the endpoint was created
			ep, err := hcn.GetEndpointByName(endpointName)
			if err != nil {
				subtest.Fatalf("endpoint was not found: %v", err)
			}
			// cleanup endpoint
			if err := ep.Delete(); err != nil {
				subtest.Fatalf("failed to delete endpoint created for test %v", err)
			}
		})
	}
}

func TestCreateEndpoint_HCN_Error_InvalidArgument(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

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
			endpoint := &ncproxygrpc.HcnEndpointSettings{
				Name:        endpointName,
				Macaddress:  test.macaddress,
				Ipaddress:   test.ipaddress,
				NetworkName: test.networkName,
			}
			req := &ncproxygrpc.CreateEndpointRequest{
				EndpointSettings: &ncproxygrpc.EndpointSettings{
					Settings: &ncproxygrpc.EndpointSettings_HcnEndpoint{
						HcnEndpoint: endpoint,
					},
				},
			}

			_, err = gService.CreateEndpoint(ctx, req)
			if err == nil {
				subtest.Fatalf("expected CreateEndpoint to return an error")
			}
		})
	}
}

func TestAddEndpoint_NoError(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

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

	hostDefaultNSID, err := getHostDefaultNamespace()
	if err != nil {
		ns := hcn.NewNamespace(hcn.NamespaceTypeHostDefault)
		ns, err = ns.Create()
		if err != nil {
			t.Fatalf("failed to create host-default test namespace: %v", err)
		}
		defer func() {
			_ = ns.Delete()
		}()
		hostDefaultNSID = ns.Id
	}

	type config struct {
		name              string
		networkCreateFunc func(string) (*hcn.HostComputeNetwork, error)
		attachToHost      bool
	}
	tests := []config{
		{
			name:              "AddEndpoint returns no error",
			networkCreateFunc: createTestIPv4NATNetwork,
		},
		{
			name:              "AddEndpoint dual stack returns no error",
			networkCreateFunc: createTestDualStackNATNetwork,
		},
		{
			name:              "AddEndpoint with host attach returns no error",
			networkCreateFunc: createTestIPv4NATNetwork,
			attachToHost:      true,
		},
		{
			name:              "AddEndpoint dual stack with host attach returns no error",
			networkCreateFunc: createTestDualStackNATNetwork,
			attachToHost:      true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(subtest *testing.T) {
			// test network
			networkName := subtest.Name() + "-network"
			network, err := test.networkCreateFunc(networkName)
			if err != nil {
				subtest.Fatalf("failed to create test network with %v", err)
			}
			defer func() {
				_ = network.Delete()
			}()

			endpointName := subtest.Name() + "-endpoint"
			endpoint, err := createTestEndpoint(endpointName, network.Id)
			if err != nil {
				subtest.Fatalf("failed to create test endpoint with %v", err)
			}
			defer func() {
				_ = endpoint.Delete()
			}()

			req := &ncproxygrpc.AddEndpointRequest{
				Name: endpointName,
			}
			if test.attachToHost {
				req.AttachToHost = true
			} else {
				req.NamespaceID = namespace.Id
			}

			_, err = gService.AddEndpoint(ctx, req)
			if err != nil {
				subtest.Fatalf("expected AddEndpoint to return no error, instead got %v", err)
			}
			// validate endpoint was added to namespace
			nsID := namespace.Id
			if test.attachToHost {
				nsID = hostDefaultNSID
			}
			endpoints, err := hcn.GetNamespaceEndpointIds(nsID)
			if err != nil {
				subtest.Fatalf("failed to get the namespace's endpoints with %v", err)
			}
			if !exists(strings.ToUpper(endpoint.Id), endpoints) {
				subtest.Fatalf("endpoint %v was not added to namespace %v", endpoint.Id, namespace.Id)
			}
		})
	}
}

func TestAddEndpoint_Error_EmptyEndpointName(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	// create test network namespace
	namespace := hcn.NewNamespace(hcn.NamespaceTypeHostDefault)
	namespace, err = namespace.Create()
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

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	// create test network namespace
	namespace := hcn.NewNamespace(hcn.NamespaceTypeHostDefault)
	namespace, err = namespace.Create()
	if err != nil {
		t.Fatalf("failed to create test namespace with %v", err)
	}
	defer func() {
		_ = namespace.Delete()
	}()

	req := &ncproxygrpc.AddEndpointRequest{
		Name:        t.Name() + "-endpoint",
		NamespaceID: namespace.Id,
	}

	_, err = gService.AddEndpoint(ctx, req)
	if err == nil {
		t.Fatal("expected AddEndpoint to return error when endpoint name is empty")
	}
}

func TestAddEndpoint_Error_EmptyNamespaceID(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

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

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	type config struct {
		name              string
		networkCreateFunc func(string) (*hcn.HostComputeNetwork, error)
	}
	tests := []config{
		{
			name:              "DeleteEndpoint returns no error",
			networkCreateFunc: createTestIPv4NATNetwork,
		},
		{
			name:              "DeleteEndpoint dual stack returns no error",
			networkCreateFunc: createTestDualStackNATNetwork,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(subtest *testing.T) {
			// test network
			networkName := subtest.Name() + "-network"
			network, err := test.networkCreateFunc(networkName)
			if err != nil {
				subtest.Fatalf("failed to create test network with %v", err)
			}
			// defer cleanup in case of error. ignore error from the delete call here
			// since we may have already successfully deleted the network.
			defer func() {
				_ = network.Delete()
			}()

			endpointName := subtest.Name() + "-endpoint"
			endpoint, err := createTestEndpoint(endpointName, network.Id)
			if err != nil {
				subtest.Fatalf("failed to create test endpoint with %v", err)
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
				subtest.Fatalf("expected DeleteEndpoint to return no error, instead got %v", err)
			}
			// validate that the endpoint was created
			ep, err := hcn.GetEndpointByName(endpointName)
			if err == nil {
				subtest.Fatalf("expected endpoint to be deleted, instead found %v", ep)
			}
		})
	}
}

func TestDeleteEndpoint_Error_NoEndpoint(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

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

func TestDeleteNetwork_NoError(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	type config struct {
		name              string
		networkCreateFunc func(string) (*hcn.HostComputeNetwork, error)
	}
	tests := []config{
		{
			name:              "DeleteNetwork returns no error",
			networkCreateFunc: createTestIPv4NATNetwork,
		},
		{
			name:              "DeleteNetwork dual stack returns no error",
			networkCreateFunc: createTestDualStackNATNetwork,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(subtest *testing.T) {
			// test network
			networkName := subtest.Name() + "-network"
			network, err := test.networkCreateFunc(networkName)
			if err != nil {
				subtest.Fatalf("failed to create test network with %v", err)
			}
			// defer cleanup in case of error. ignore error from the delete call here
			// since we may have already successfully deleted the network.
			defer func() {
				_ = network.Delete()
			}()

			req := &ncproxygrpc.DeleteNetworkRequest{
				Name: networkName,
			}
			_, err = gService.DeleteNetwork(ctx, req)
			if err != nil {
				subtest.Fatalf("expected no error, instead got %v", err)
			}
		})
	}
}

func TestDeleteNetwork_Error_NoNetwork(t *testing.T) {
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

func TestDeleteNetwork_Error_EmptyNetworkName(t *testing.T) {
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

func TestGetEndpoint_NoError(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	type config struct {
		name              string
		networkCreateFunc func(string) (*hcn.HostComputeNetwork, error)
	}
	tests := []config{
		{
			name:              "GetEndpoint returns no error",
			networkCreateFunc: createTestIPv4NATNetwork,
		},
		{
			name:              "GetEndpoint dual stack returns no error",
			networkCreateFunc: createTestDualStackNATNetwork,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(subtest *testing.T) {
			// test network
			networkName := subtest.Name() + "-network"
			network, err := test.networkCreateFunc(networkName)
			if err != nil {
				subtest.Fatalf("failed to create test network with %v", err)
			}
			defer func() {
				_ = network.Delete()
			}()

			// test endpoint
			endpointName := subtest.Name() + "-endpoint"
			endpoint, err := createTestEndpoint(endpointName, network.Id)
			if err != nil {
				subtest.Fatalf("failed to create test endpoint with %v", err)
			}
			defer func() {
				_ = endpoint.Delete()
			}()

			req := &ncproxygrpc.GetEndpointRequest{
				Name: endpointName,
			}

			if _, err := gService.GetEndpoint(ctx, req); err != nil {
				subtest.Fatalf("expected to get no error, instead got %v", err)
			}
		})
	}
}

func TestGetEndpoint_Error_NoEndpoint(t *testing.T) {
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

func TestGetEndpoint_Error_EmptyEndpointName(t *testing.T) {
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

func TestGetEndpoints_NoError(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	type config struct {
		name              string
		networkCreateFunc func(string) (*hcn.HostComputeNetwork, error)
	}
	tests := []config{
		{
			name:              "GetEndpoints returns no error",
			networkCreateFunc: createTestIPv4NATNetwork,
		},
		{
			name:              "GetEndpoints dual stack returns no error",
			networkCreateFunc: createTestDualStackNATNetwork,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(subtest *testing.T) {
			// test network
			networkName := subtest.Name() + "-network"
			network, err := test.networkCreateFunc(networkName)
			if err != nil {
				subtest.Fatalf("failed to create test network with %v", err)
			}
			defer func() {
				_ = network.Delete()
			}()

			// test endpoint
			endpointName := subtest.Name() + "-endpoint"
			endpoint, err := createTestEndpoint(endpointName, network.Id)
			if err != nil {
				subtest.Fatalf("failed to create test endpoint with %v", err)
			}
			defer func() {
				_ = endpoint.Delete()
			}()

			req := &ncproxygrpc.GetEndpointsRequest{}
			resp, err := gService.GetEndpoints(ctx, req)
			if err != nil {
				subtest.Fatalf("expected to get no error, instead got %v", err)
			}

			if !endpointExists(endpointName, resp.Endpoints) {
				subtest.Fatalf("created endpoint was not found")
			}
		})
	}
}

func TestGetNetwork_NoError(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	type config struct {
		name              string
		networkCreateFunc func(string) (*hcn.HostComputeNetwork, error)
	}
	tests := []config{
		{
			name:              "GetNetwork returns no error",
			networkCreateFunc: createTestIPv4NATNetwork,
		},
		{
			name:              "GetNetwork dual stack returns no error",
			networkCreateFunc: createTestDualStackNATNetwork,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(subtest *testing.T) {
			// create the test network
			networkName := subtest.Name() + "-network"
			network, err := test.networkCreateFunc(networkName)
			if err != nil {
				subtest.Fatalf("failed to create test network with %v", err)
			}
			// defer cleanup in case of error. ignore error from the delete call here
			// since we may have already successfully deleted the network.
			defer func() {
				_ = network.Delete()
			}()

			req := &ncproxygrpc.GetNetworkRequest{
				Name: networkName,
			}
			resp, err := gService.GetNetwork(ctx, req)
			if err != nil {
				subtest.Fatalf("expected no error, instead got %v", err)
			}
			mac := resp.MacRange
			if mac == nil {
				subtest.Fatal("received nil MAC Range")
			}
			if mac.StartMacAddress != startMACAddress {
				subtest.Errorf("got start MAC address %q, wanted %q", mac.StartMacAddress, startMACAddress)
			}
			if mac.EndMacAddress != endMACAddress {
				subtest.Errorf("got end MAC address %q, wanted %q", mac.EndMacAddress, endMACAddress)
			}
		})
	}
}

func TestGetNetwork_Error_NoNetwork(t *testing.T) {
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

func TestGetNetwork_Error_EmptyNetworkName(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	req := &ncproxygrpc.GetNetworkRequest{
		Name: "",
	}
	_, err = gService.GetNetwork(ctx, req)
	if err == nil {
		t.Fatal("expected to get an error when network name is empty")
	}
}

func TestGetNetworks_NoError(t *testing.T) {
	ctx := context.Background()

	networkingStore, closer, err := createTestNetworkingStore()
	if err != nil {
		t.Fatalf("failed to create a test ncproxy networking store with %v", err)
	}
	defer closer()

	// setup test ncproxy grpc service
	agentCache := newComputeAgentCache()
	gService := newGRPCService(agentCache, networkingStore)

	type config struct {
		name              string
		networkCreateFunc func(string) (*hcn.HostComputeNetwork, error)
	}
	tests := []config{
		{
			name:              "GetNetworks returns no error",
			networkCreateFunc: createTestIPv4NATNetwork,
		},
		{
			name:              "GetNetworks dual stack returns no error",
			networkCreateFunc: createTestDualStackNATNetwork,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(subtest *testing.T) {
			// create the test network
			networkName := subtest.Name() + "-network"
			network, err := test.networkCreateFunc(networkName)
			if err != nil {
				subtest.Fatalf("failed to create test network with %v", err)
			}
			// defer cleanup in case of error. ignore error from the delete call here
			// since we may have already successfully deleted the network.
			defer func() {
				_ = network.Delete()
			}()

			req := &ncproxygrpc.GetNetworksRequest{}
			resp, err := gService.GetNetworks(ctx, req)
			if err != nil {
				subtest.Fatalf("expected no error, instead got %v", err)
			}
			if !networkExists(networkName, resp.Networks) {
				subtest.Fatalf("failed to find created network")
			}
		})
	}
}
