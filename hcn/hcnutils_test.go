// +build integration

package hcn

import (
	"encoding/json"
)

func CreateSubnet(AddressPrefix string, NextHop string, DestPrefix string) *Subnet {
	return &Subnet{
		IpAddressPrefix: AddressPrefix,
		Routes: []Route{
			{
				NextHop:           NextHop,
				DestinationPrefix: DestPrefix,
			},
		},
	}
}

func GetDefaultSubnet() *Subnet {
	return CreateSubnet("192.168.100.0/24", "192.168.100.1", "0.0.0.0/0")
}

func cleanup(networkName string) {
	// Delete test network (if exists)
	testNetwork, err := GetNetworkByName(networkName)
	if err != nil {
		return
	}
	if testNetwork != nil {
		err := testNetwork.Delete()
		if err != nil {
			return
		}
	}
}

func HcnGenerateNATNetwork(subnet *Subnet) *HostComputeNetwork {
	ipams := []Ipam{}
	if subnet != nil {
		ipam := Ipam{
			Type: "Static",
			Subnets: []Subnet{
				*subnet,
			},
		}
		ipams = append(ipams, ipam)
	}
	network := &HostComputeNetwork{
		Type: "NAT",
		Name: NatTestNetworkName,
		MacPool: MacPool{
			Ranges: []MacRange{
				{
					StartMacAddress: "00-15-5D-52-C0-00",
					EndMacAddress:   "00-15-5D-52-CF-FF",
				},
			},
		},
		Ipams: ipams,
		SchemaVersion: SchemaVersion{
			Major: 2,
			Minor: 0,
		},
	}
	return network
}

func HcnCreateTestNATNetworkWithSubnet(subnet *Subnet) (*HostComputeNetwork, error) {
	cleanup(NatTestNetworkName)
	network := HcnGenerateNATNetwork(subnet)
	return network.Create()
}

func HcnCreateTestNATNetwork() (*HostComputeNetwork, error) {
	return HcnCreateTestNATNetworkWithSubnet(GetDefaultSubnet())
}

func CreateTestOverlayNetwork() (*HostComputeNetwork, error) {
	cleanup(OverlayTestNetworkName)
	subnet := GetDefaultSubnet()
	network := &HostComputeNetwork{
		Type: "Overlay",
		Name: OverlayTestNetworkName,
		MacPool: MacPool{
			Ranges: []MacRange{
				{
					StartMacAddress: "00-15-5D-52-C0-00",
					EndMacAddress:   "00-15-5D-52-CF-FF",
				},
			},
		},
		Ipams: []Ipam{
			{
				Type: "Static",
				Subnets: []Subnet{
					*subnet,
				},
			},
		},
		Flags: EnableNonPersistent,
		SchemaVersion: SchemaVersion{
			Major: 2,
			Minor: 0,
		},
	}

	vsid := &VsidPolicySetting{
		IsolationId: 5000,
	}
	vsidJson, err := json.Marshal(vsid)
	if err != nil {
		return nil, err
	}

	sp := &SubnetPolicy{
		Type: VSID,
	}
	sp.Settings = vsidJson

	spJson, err := json.Marshal(sp)
	if err != nil {
		return nil, err
	}

	network.Ipams[0].Subnets[0].Policies = append(network.Ipams[0].Subnets[0].Policies, spJson)

	return network.Create()
}

func HcnCreateTestEndpoint(network *HostComputeNetwork) (*HostComputeEndpoint, error) {
	if network == nil {

	}
	Endpoint := &HostComputeEndpoint{
		Name: NatTestEndpointName,
		SchemaVersion: SchemaVersion{
			Major: 2,
			Minor: 0,
		},
	}

	return network.CreateEndpoint(Endpoint)
}

func HcnCreateTestEndpointWithPolicies(network *HostComputeNetwork, policies []EndpointPolicy) (*HostComputeEndpoint, error) {
	Endpoint := &HostComputeEndpoint{
		Name:     NatTestEndpointName,
		Policies: policies,
		SchemaVersion: SchemaVersion{
			Major: 2,
			Minor: 0,
		},
	}

	return network.CreateEndpoint(Endpoint)
}

func HcnCreateTestEndpointWithNamespace(network *HostComputeNetwork, namespace *HostComputeNamespace) (*HostComputeEndpoint, error) {
	Endpoint := &HostComputeEndpoint{
		Name:                 NatTestEndpointName,
		HostComputeNamespace: namespace.Id,
		SchemaVersion: SchemaVersion{
			Major: 2,
			Minor: 0,
		},
	}

	return network.CreateEndpoint(Endpoint)
}

func HcnCreateTestNamespace() (*HostComputeNamespace, error) {
	namespace := &HostComputeNamespace{
		Type:        NamespaceTypeHostDefault,
		NamespaceId: 5,
		SchemaVersion: SchemaVersion{
			Major: 2,
			Minor: 0,
		},
	}

	return namespace.Create()
}

func HcnCreateAcls() (*PolicyEndpointRequest, error) {
	in := AclPolicySetting{
		Protocols:       "6",
		Action:          ActionTypeAllow,
		Direction:       DirectionTypeIn,
		LocalAddresses:  "192.168.100.0/24,10.0.0.21",
		RemoteAddresses: "192.168.100.0/24,10.0.0.21",
		LocalPorts:      "80,8080",
		RemotePorts:     "80,8080",
		RuleType:        RuleTypeSwitch,
		Priority:        200,
	}

	rawJSON, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	inPolicy := EndpointPolicy{
		Type:     ACL,
		Settings: rawJSON,
	}

	out := AclPolicySetting{
		Protocols:       "6",
		Action:          ActionTypeAllow,
		Direction:       DirectionTypeOut,
		LocalAddresses:  "192.168.100.0/24,10.0.0.21",
		RemoteAddresses: "192.168.100.0/24,10.0.0.21",
		LocalPorts:      "80,8080",
		RemotePorts:     "80,8080",
		RuleType:        RuleTypeSwitch,
		Priority:        200,
	}

	rawJSON, err = json.Marshal(out)
	if err != nil {
		return nil, err
	}
	outPolicy := EndpointPolicy{
		Type:     ACL,
		Settings: rawJSON,
	}

	endpointRequest := PolicyEndpointRequest{
		Policies: []EndpointPolicy{inPolicy, outPolicy},
	}

	return &endpointRequest, nil
}

func HcnCreateWfpProxyPolicyRequest() (*PolicyEndpointRequest, error) {
	policySetting := L4WfpProxyPolicySetting{
		InboundProxyPort:  "80",
		OutboundProxyPort: "81",
		FilterTuple: FiveTuple{
			Protocols:       "6",
			RemoteAddresses: "10.0.0.4",
			Priority:        8,
		},
		OutboundExceptions: ProxyExceptions{
			IpAddressExceptions: []string{"10.0.1.12"},
			PortExceptions:      []string{"81"},
		},
		InboundExceptions: ProxyExceptions{
			IpAddressExceptions: []string{"12.0.1.12"},
			PortExceptions:      []string{"8181"},
		},
	}

	policyJSON, err := json.Marshal(policySetting)
	if err != nil {
		return nil, err
	}

	endpointPolicy := EndpointPolicy{
		Type:     L4WFPPROXY,
		Settings: policyJSON,
	}

	endpointRequest := PolicyEndpointRequest{
		Policies: []EndpointPolicy{endpointPolicy},
	}

	return &endpointRequest, nil
}

func HcnCreateTestLoadBalancer(endpoint *HostComputeEndpoint) (*HostComputeLoadBalancer, error) {
	loadBalancer := &HostComputeLoadBalancer{
		HostComputeEndpoints: []string{endpoint.Id},
		SourceVIP:            "10.0.0.1",
		PortMappings: []LoadBalancerPortMapping{
			{
				Protocol:     6, // TCP
				InternalPort: 8080,
				ExternalPort: 8090,
			},
		},
		FrontendVIPs: []string{"1.1.1.2", "1.1.1.3"},
		SchemaVersion: SchemaVersion{
			Major: 2,
			Minor: 0,
		},
	}

	return loadBalancer.Create()
}

func HcnCreateTestRemoteSubnetRoute() (*PolicyNetworkRequest, error) {
	rsr := RemoteSubnetRoutePolicySetting{
		DestinationPrefix:           "192.168.2.0/24",
		IsolationId:                 5000,
		ProviderAddress:             "1.1.1.1",
		DistributedRouterMacAddress: "00-12-34-56-78-9a",
	}

	rawJSON, err := json.Marshal(rsr)
	if err != nil {
		return nil, err
	}
	rsrPolicy := NetworkPolicy{
		Type:     RemoteSubnetRoute,
		Settings: rawJSON,
	}

	networkRequest := PolicyNetworkRequest{
		Policies: []NetworkPolicy{rsrPolicy},
	}

	return &networkRequest, nil
}

func HcnCreateTestHostRoute() (*PolicyNetworkRequest, error) {
	hostRoutePolicy := NetworkPolicy{
		Type:     HostRoute,
		Settings: []byte("{}"),
	}

	networkRequest := PolicyNetworkRequest{
		Policies: []NetworkPolicy{hostRoutePolicy},
	}

	return &networkRequest, nil
}

func HcnCreateTestSdnRoute(endpoint *HostComputeEndpoint) (*HostComputeRoute, error) {
	route := &HostComputeRoute{
		SchemaVersion: V2SchemaVersion(),
		Setting: []SDNRoutePolicySetting{
			{
				DestinationPrefix: "169.254.169.254/24",
				NextHop:           "127.10.0.34",
				NeedEncap:         false,
			},
		},
	}

	route.HostComputeEndpoints = append(route.HostComputeEndpoints, endpoint.Id)

	return route.Create()
}

func HcnCreateTestL2BridgeNetwork() (*HostComputeNetwork, error) {
	cleanup(BridgeTestNetworkName)
	subnet := GetDefaultSubnet()
	network := &HostComputeNetwork{
		Type: "L2Bridge",
		Name: BridgeTestNetworkName,
		MacPool: MacPool{
			Ranges: []MacRange{
				{
					StartMacAddress: "00-15-5D-52-C0-00",
					EndMacAddress:   "00-15-5D-52-CF-FF",
				},
			},
		},
		Ipams: []Ipam{
			{
				Type: "Static",
				Subnets: []Subnet{
					*subnet,
				},
			},
		},
		Flags: EnableNonPersistent,
		SchemaVersion: SchemaVersion{
			Major: 2,
			Minor: 0,
		},
	}

	return network.Create()

}

func HcnCreateTierAcls() (*PolicyEndpointRequest, error) {

	policy := make([]EndpointPolicy, 6)

	tiers := make([]TierAclPolicySetting, 6)

	//inbound rules
	tiers[0] = TierAclPolicySetting{
		Name: "TierIn1",
		Direction: DirectionTypeIn,
		Order: 1001,
	}

	tiers[0].TierAclRules = make([]TierAclRule, 2)

	tiers[0].TierAclRules[0] = TierAclRule{
		Id: "TierIn1Rule1",
		Protocols: "6",
		TierAclRuleAction: ActionTypePass,
		LocalAddresses: "192.168.100.0/24,10.0.0.21",
		RemoteAddresses: "192.168.100.0/24,10.0.0.22",
		LocalPorts: "80",
		RemotePorts: "80",
		Priority: 2001,
	}	

	tiers[0].TierAclRules[1] = TierAclRule{
		Id: "TierIn1Rule2",
		TierAclRuleAction: ActionTypeBlock,
		Priority: 2100,
	}

	policy[0].Type = TierAcl
	rawJSON, err := json.Marshal(tiers[0])
	if err != nil {
		return nil, err
	}

	policy[0].Settings = rawJSON

	tiers[1] = TierAclPolicySetting{
		Name: "TierIn2",
		Direction: DirectionTypeIn,
		Order: 1002,
	}

	tiers[1].TierAclRules = make([]TierAclRule, 3)

	tiers[1].TierAclRules[0] = TierAclRule{
		Id: "TierIn2Rule1",
		TierAclRuleAction: ActionTypePass,
		LocalAddresses: "192.168.100.0/24",
		RemoteAddresses: "192.168.100.0/24",
		Priority: 3000,
	}	

	tiers[1].TierAclRules[1] = TierAclRule{
		Id: "TierIn2Rule2",
		TierAclRuleAction: ActionTypePass,
		LocalAddresses: "10.0.0.21",
		RemoteAddresses: "10.0.0.21",
		Priority: 3010,
	}	

	tiers[1].TierAclRules[2] = TierAclRule{
		Id: "TierIn2Rule3",
		TierAclRuleAction: ActionTypeBlock,
		Priority: 3100,
	}

	policy[1].Type = TierAcl
	rawJSON, err = json.Marshal(tiers[1])
	if err != nil {
		return nil, err
	}

	policy[1].Settings = rawJSON


	tiers[2] = TierAclPolicySetting{
		Name: "TierIn3",
		Direction: DirectionTypeIn,
		Order: 1013,
	}

	tiers[2].TierAclRules = make([]TierAclRule, 2)

	tiers[2].TierAclRules[0] = TierAclRule{
		Id: "TierIn3Rule1",
		Protocols: "17",
		TierAclRuleAction: ActionTypeAllow,
		LocalPorts: "8080",
		RemotePorts: "8080",
		Priority: 3000,
	}	

	tiers[2].TierAclRules[1] = TierAclRule{
		Id: "TierIn3Rule2",
		TierAclRuleAction: ActionTypeBlock,
		Priority: 3010,
	}

	policy[2].Type = TierAcl
	rawJSON, err = json.Marshal(tiers[2])
	if err != nil {
		return nil, err
	}

	policy[2].Settings = rawJSON

	//outbound rules
	tiers[3] = TierAclPolicySetting{
		Name: "TierOut1",
		Direction: DirectionTypeOut,
		Order: 1001,
	}

	tiers[3].TierAclRules = make([]TierAclRule, 2)

	tiers[3].TierAclRules[0] = TierAclRule{
		Id: "TierOut1Rule1",
		Protocols: "6",
		TierAclRuleAction: ActionTypePass,
		LocalAddresses: "192.168.100.0/24,10.0.0.21",
		RemoteAddresses: "192.168.100.0/24,10.0.0.22",
		LocalPorts: "81",
		RemotePorts: "81",
		Priority: 2000,
	}	

	tiers[3].TierAclRules[1] = TierAclRule{
		Id: "TierOut1Rule2",
		TierAclRuleAction: ActionTypeBlock,
		Priority: 2100,
	}

	policy[3].Type = TierAcl
	rawJSON, err = json.Marshal(tiers[3])
	if err != nil {
		return nil, err
	}

	policy[3].Settings = rawJSON


	tiers[4] = TierAclPolicySetting{
		Name: "TierOut2",
		Direction: DirectionTypeOut,
		Order: 1002,
	}

	tiers[4].TierAclRules = make([]TierAclRule, 3)

	tiers[4].TierAclRules[0] = TierAclRule{
		Id: "TierOut2Rule1",
		TierAclRuleAction: ActionTypePass,
		LocalAddresses: "192.168.100.0/24",
		RemoteAddresses: "192.168.100.0/24",
		Priority: 3000,
	}	

	tiers[4].TierAclRules[1] = TierAclRule{
		Id: "TierOut2Rule2",
		Protocols: "6",
		TierAclRuleAction: ActionTypePass,
		LocalAddresses: "10.0.0.21",
		RemoteAddresses: "10.0.0.21",
		LocalPorts: "8082",
		RemotePorts: "8082",
		Priority: 3010,
	}	

	tiers[4].TierAclRules[2] = TierAclRule{
		Id: "TierOut2Rule3",
		TierAclRuleAction: ActionTypeBlock,
		Priority: 3100,
	}

	policy[4].Type = TierAcl
	rawJSON, err = json.Marshal(tiers[4])
	if err != nil {
		return nil, err
	}

	policy[4].Settings = rawJSON


	tiers[5] = TierAclPolicySetting{
		Name: "TierOut3",
		Direction: DirectionTypeOut,
		Order: 1013,
	}

	tiers[5].TierAclRules = make([]TierAclRule, 2)

	tiers[5].TierAclRules[0] = TierAclRule{
		Id: "TierOut3Rule1",
		Protocols: "6",
		TierAclRuleAction: ActionTypeAllow,
		LocalPorts: "90",
		RemotePorts: "90",
		Priority: 3000,
	}	

	tiers[5].TierAclRules[1] = TierAclRule{
		Id: "TierOut3Rule2",
		TierAclRuleAction: ActionTypeBlock,
		Priority: 3010,
	}

	policy[5].Type = TierAcl
	rawJSON, err = json.Marshal(tiers[5])
	if err != nil {
		return nil, err
	}

	policy[5].Settings = rawJSON

	endpointRequest := PolicyEndpointRequest{
		Policies: policy,
	}

	return &endpointRequest, nil
}
